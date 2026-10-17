package wfp

import (
	"errors"
	"fmt"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ErrNedostupen: служба базовой фильтрации (BFE) не отвечает. Без неё фильтры
// не ставятся и не снимаются, а уже стоящие постоянные она поднимет сама, как
// только запустится.
var ErrNedostupen = errors.New("служба базовой фильтрации Windows (BFE) не отвечает")

var (
	fwpuclnt = windows.NewLazySystemDLL("fwpuclnt.dll")

	procEngineOpen          = fwpuclnt.NewProc("FwpmEngineOpen0")
	procEngineClose         = fwpuclnt.NewProc("FwpmEngineClose0")
	procTransactionBegin    = fwpuclnt.NewProc("FwpmTransactionBegin0")
	procTransactionCommit   = fwpuclnt.NewProc("FwpmTransactionCommit0")
	procTransactionAbort    = fwpuclnt.NewProc("FwpmTransactionAbort0")
	procProviderAdd         = fwpuclnt.NewProc("FwpmProviderAdd0")
	procProviderDeleteByKey = fwpuclnt.NewProc("FwpmProviderDeleteByKey0")
	procSubLayerAdd         = fwpuclnt.NewProc("FwpmSubLayerAdd0")
	procSubLayerDeleteByKey = fwpuclnt.NewProc("FwpmSubLayerDeleteByKey0")
	procFilterAdd           = fwpuclnt.NewProc("FwpmFilterAdd0")
	procFilterDeleteByKey   = fwpuclnt.NewProc("FwpmFilterDeleteByKey0")
	procFilterCreateEnum    = fwpuclnt.NewProc("FwpmFilterCreateEnumHandle0")
	procFilterEnum          = fwpuclnt.NewProc("FwpmFilterEnum0")
	procFilterDestroyEnum   = fwpuclnt.NewProc("FwpmFilterDestroyEnumHandle0")
	procFreeMemory          = fwpuclnt.NewProc("FwpmFreeMemory0")
	procGetAppIdFromFile    = fwpuclnt.NewProc("FwpmGetAppIdFromFileName0")
)

// zagruzit находит все функции заранее: LazyProc.Call на ненайденной функции
// паникует, а паника в службе хуже честного отказа.
var zagruzit = sync.OnceValue(func() error {
	for _, p := range []*windows.LazyProc{procEngineOpen, procEngineClose,
		procTransactionBegin, procTransactionCommit, procTransactionAbort,
		procProviderAdd, procProviderDeleteByKey, procSubLayerAdd, procSubLayerDeleteByKey,
		procFilterAdd, procFilterDeleteByKey, procFilterCreateEnum, procFilterEnum,
		procFilterDestroyEnum, procFreeMemory, procGetAppIdFromFile} {
		if err := p.Find(); err != nil {
			return fmt.Errorf("fwpuclnt.dll: %w", err)
		}
	}
	return nil
})

// oshibka переводит код возврата функций WFP в ошибку. Они отвечают кодом
// прямо, а не через GetLastError.
func oshibka(funkciya string, r uintptr) error {
	if r == 0 {
		return nil
	}
	kod := windows.Errno(r)
	if kod == eptSNotRegistered || kod == rpcSServerUnavailable {
		return fmt.Errorf("%s: %w: %w", funkciya, ErrNedostupen, kod)
	}
	return fmt.Errorf("%s: %w", funkciya, kod)
}

// netTakogo узнаёт «такого объекта нет». Снятие несуществующего это не отказ.
func netTakogo(err error) bool {
	var kod windows.Errno
	if !errors.As(err, &kod) {
		return false
	}
	return kod == fwpEFilterNotFound || kod == fwpEProviderNotFound || kod == fwpESublayerNotFound
}

func uzheEst(err error) bool {
	var kod windows.Errno
	return errors.As(err, &kod) && kod == fwpEAlreadyExists
}

// Osnova это провайдер и подслой, под которыми живут фильтры. Подслой свой:
// внутри подслоя решает фильтр с наибольшим весом, а запрет любого подслоя
// бьёт разрешения остальных, поэтому чужие разрешения наш запрет не снимут.
type Osnova struct {
	Provayder windows.GUID
	Podsloy   windows.GUID
	Imya      string
	// VesPodsloya: подслои с большим весом решают раньше.
	VesPodsloya uint16
}

// Seans это открытое соединение с BFE. Сессия НЕ динамическая: всё, что она
// ставит с флагом постоянства, переживает и её закрытие, и перезагрузку.
type Seans struct {
	h windows.Handle
}

// Otkryt открывает сессию. Нужны права администратора.
func Otkryt() (*Seans, error) {
	if err := zagruzit(); err != nil {
		return nil, err
	}
	imya, err := windows.UTF16PtrFromString("Affory")
	if err != nil {
		return nil, err
	}
	sessiya := fwpmSession0{podpis: fwpmDisplayData0{imya: imya}}
	var h windows.Handle
	r, _, _ := procEngineOpen.Call(0, rpcCAuthnDefault, 0,
		uintptr(unsafe.Pointer(&sessiya)), uintptr(unsafe.Pointer(&h)))
	if err := oshibka("FwpmEngineOpen0", r); err != nil {
		return nil, err
	}
	return &Seans{h: h}, nil
}

// Zakryt закрывает сессию. Незавершённая транзакция при этом откатывается.
func (s *Seans) Zakryt() error {
	r, _, _ := procEngineClose.Call(uintptr(s.h))
	return oshibka("FwpmEngineClose0", r)
}

// Tranzakciya выполняет f целиком или никак: отказ посередине откатывает всё,
// что f успела сделать.
func (s *Seans) Tranzakciya(f func() error) error {
	r, _, _ := procTransactionBegin.Call(uintptr(s.h), 0)
	if err := oshibka("FwpmTransactionBegin0", r); err != nil {
		return err
	}
	if err := f(); err != nil {
		r, _, _ := procTransactionAbort.Call(uintptr(s.h))
		return errors.Join(err, oshibka("FwpmTransactionAbort0", r))
	}
	r, _, _ = procTransactionCommit.Call(uintptr(s.h))
	return oshibka("FwpmTransactionCommit0", r)
}

// ZavestiOsnovu ставит провайдера и подслой постоянными. Уже стоящие не
// отказ.
func (s *Seans) ZavestiOsnovu(o Osnova) error {
	imya, err := windows.UTF16PtrFromString(o.Imya)
	if err != nil {
		return err
	}
	provayder := fwpmProvider0{klyuch: o.Provayder, podpis: fwpmDisplayData0{imya: imya}, flagi: flagPostoyannyy}
	r, _, _ := procProviderAdd.Call(uintptr(s.h), uintptr(unsafe.Pointer(&provayder)), 0)
	if err := oshibka("FwpmProviderAdd0", r); err != nil && !uzheEst(err) {
		return err
	}
	klyuchProvaydera := o.Provayder
	podsloy := fwpmSublayer0{klyuch: o.Podsloy, podpis: fwpmDisplayData0{imya: imya},
		flagi: flagPostoyannyy, provayder: &klyuchProvaydera, ves: o.VesPodsloya}
	r, _, _ = procSubLayerAdd.Call(uintptr(s.h), uintptr(unsafe.Pointer(&podsloy)), 0)
	if err := oshibka("FwpmSubLayerAdd0", r); err != nil && !uzheEst(err) {
		return err
	}
	return nil
}

// UdalitOsnovu снимает подслой и провайдера. Фильтры к этому моменту должны
// быть сняты: занятый подслой WFP не отдаёт.
func (s *Seans) UdalitOsnovu(o Osnova) error {
	podsloy := o.Podsloy
	r, _, _ := procSubLayerDeleteByKey.Call(uintptr(s.h), uintptr(unsafe.Pointer(&podsloy)))
	if err := oshibka("FwpmSubLayerDeleteByKey0", r); err != nil && !netTakogo(err) {
		return err
	}
	provayder := o.Provayder
	r, _, _ = procProviderDeleteByKey.Call(uintptr(s.h), uintptr(unsafe.Pointer(&provayder)))
	if err := oshibka("FwpmProviderDeleteByKey0", r); err != nil && !netTakogo(err) {
		return err
	}
	return nil
}

// UdalitFiltr снимает фильтр по ключу. Снятие отсутствующего не отказ.
func (s *Seans) UdalitFiltr(klyuch windows.GUID) error {
	r, _, _ := procFilterDeleteByKey.Call(uintptr(s.h), uintptr(unsafe.Pointer(&klyuch)))
	if err := oshibka("FwpmFilterDeleteByKey0", r); err != nil && !netTakogo(err) {
		return err
	}
	return nil
}

// KlyuchiFiltrov отдаёт ключи всех фильтров провайдера в обоих слоях
// исходящих соединений. Перебор идёт по провайдеру, а не по нашему списку
// ключей: так снимается и то, что поставила другая версия программы.
func (s *Seans) KlyuchiFiltrov(provayder windows.GUID) ([]windows.GUID, error) {
	var itog []windows.GUID
	for _, sloy := range []windows.GUID{sloyConnectV4, sloyConnectV6} {
		klyuchi, err := s.klyuchiVSloe(provayder, sloy)
		if err != nil {
			return nil, err
		}
		itog = append(itog, klyuchi...)
	}
	return itog, nil
}

func (s *Seans) klyuchiVSloe(provayder, sloy windows.GUID) (itog []windows.GUID, err error) {
	shablon := fwpmFilterEnumTemplate0{provayder: &provayder, sloy: sloy,
		tipPerebora: fwpFilterEnumOverlapping, maskaDeystviy: 0xFFFFFFFF}
	var perebor windows.Handle
	r, _, _ := procFilterCreateEnum.Call(uintptr(s.h), uintptr(unsafe.Pointer(&shablon)),
		uintptr(unsafe.Pointer(&perebor)))
	if err := oshibka("FwpmFilterCreateEnumHandle0", r); err != nil {
		if netTakogo(err) {
			return nil, nil
		}
		return nil, err
	}
	defer func() {
		r, _, _ := procFilterDestroyEnum.Call(uintptr(s.h), uintptr(perebor))
		err = errors.Join(err, oshibka("FwpmFilterDestroyEnumHandle0", r))
	}()
	const zaRaz = 256
	for {
		var zapisi **fwpmFilter0
		var n uint32
		r, _, _ := procFilterEnum.Call(uintptr(s.h), uintptr(perebor), zaRaz,
			uintptr(unsafe.Pointer(&zapisi)), uintptr(unsafe.Pointer(&n)))
		if err := oshibka("FwpmFilterEnum0", r); err != nil {
			return nil, err
		}
		if zapisi != nil {
			for _, f := range unsafe.Slice(zapisi, n) {
				itog = append(itog, f.klyuch)
			}
			// Память отдала WFP, она же её и забирает.
			procFreeMemory.Call(uintptr(unsafe.Pointer(&zapisi)))
		}
		if n < zaRaz {
			return itog, nil
		}
	}
}
