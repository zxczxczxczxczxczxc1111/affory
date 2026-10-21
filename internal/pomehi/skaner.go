package pomehi

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/set"
)

// Skaner помнит, какие процессы уже осмотрены на WinDivert.dll. Осмотр модулей
// это снимок на процесс, и делать его на каждой проверке раз в 30 с по всем
// процессам машины незачем: новый процесс появляется редко.
//
// Все источники это швы: тест проверяет решения, не имея ни zapret, ни
// чужого VPN, и не читая процессы машины разработчика.
type Skaner struct {
	mu        sync.Mutex
	osmotreny map[klyuchOsmotra]osmotr

	protsessy        func() ([]Protsess, error)
	drayveryWinDiv   func() ([]string, error)
	derzhitWinDivert func(pid uint32) (bool, error)
	adaptery         func() ([]set.Adapter, error)
	polovinki        func() (map[uint32]bool, error)
}

// klyuchOsmotra это pid вместе с именем: номер процесса Windows отдаёт
// повторно, и новый процесс под старым номером без имени унаследовал бы
// чужой ответ.
type klyuchOsmotra struct {
	pid  uint32
	imya string
}

// osmotr это ответ осмотра и сколько раз процесс осмотрен. Процесс без
// WinDivert.dll осматривается дважды: первый осмотр мог прийтись на
// миллисекунды между запуском процесса и загрузкой библиотеки, и одного «нет»
// хватило бы, чтобы не заметить перехватчик до его перезапуска.
type osmotr struct {
	derzhit bool
	raz     int
}

const osmotrovBezDll = 2

func NovyySkaner() *Skaner {
	return &Skaner{
		osmotreny:        map[klyuchOsmotra]osmotr{},
		protsessy:        protsessySistemy,
		drayveryWinDiv:   drayveryWinDivert,
		derzhitWinDivert: derzhitWinDivertSistemno,
		adaptery:         set.Adaptery,
		polovinki:        set.PolovinkiUmolchaniya,
	}
}

// Proverit обходит процессы, драйверы и адаптеры. Отказ одного источника не
// отменяет находки других: ошибка возвращается вместе с ними.
func (s *Skaner) Proverit(svoi Svoi) ([]Nahodka, error) {
	var vse []Nahodka
	var oshibki []error

	protsessy, err := s.protsessy()
	if err != nil {
		oshibki = append(oshibki, err)
	}
	poImeni := poImenam(protsessy, svoi)
	vse = append(vse, poImeni...)

	// Драйвер WinDivert переживает свою программу (опыт в госте 21.10.2026),
	// поэтому сам по себе он не находка, а только повод осмотреть процессы.
	// Нет процесса с WinDivert.dll: драйвер остался от прошлого запуска.
	if drayvery, err := s.drayveryWinDiv(); err != nil {
		oshibki = append(oshibki, err)
	} else if len(drayvery) > 0 {
		vse = append(vse, s.derzhateliWinDivert(protsessy, svoi)...)
	}

	adaptery, err := s.adaptery()
	if err != nil {
		oshibki = append(oshibki, fmt.Errorf("адаптеры не перечислены: %w", err))
	} else {
		polovinki, err := s.polovinki()
		if err != nil {
			// Без половинок остаётся 0.0.0.0/0, и это лучше, чем ничего.
			oshibki = append(oshibki, fmt.Errorf("маршруты-половинки не прочитаны: %w", err))
		}
		vse = append(vse, chuzhieTunneli(adaptery, polovinki, svoi, klientyVPN(poImeni))...)
	}
	return svesti(vse), errors.Join(oshibki...)
}

// derzhateliWinDivert осматривает модули ещё не осмотренных процессов.
// Отказ осмотра одного процесса это не ошибка проверки: защищённые процессы
// и процессы, завершившиеся между снимками, модулей не отдают. Такой процесс
// осматривается снова на следующей проверке.
func (s *Skaner) derzhateliWinDivert(protsessy []Protsess, svoi Svoi) []Nahodka {
	s.mu.Lock()
	defer s.mu.Unlock()
	zhivye := make(map[klyuchOsmotra]bool, len(protsessy))
	var itog []Nahodka
	for _, p := range protsessy {
		// 0 и 4 это Idle и System: модулей пользователя у них нет.
		if p.Pid == 0 || p.Pid == 4 || svoy(p, svoi.KatalogProgrammy) {
			continue
		}
		k := klyuchOsmotra{p.Pid, strings.ToLower(p.Imya)}
		zhivye[k] = true
		o := s.osmotreny[k]
		if !o.derzhit && o.raz < osmotrovBezDll {
			derzhit, err := s.derzhitWinDivert(p.Pid)
			if err != nil {
				continue
			}
			o = osmotr{derzhit: derzhit, raz: o.raz + 1}
			s.osmotreny[k] = o
		}
		if o.derzhit {
			itog = append(itog, derzhatelWinDivert(p))
		}
	}
	// Завершившиеся забываются, иначе память растёт с каждым запуском браузера.
	for k := range s.osmotreny {
		if !zhivye[k] {
			delete(s.osmotreny, k)
		}
	}
	return itog
}

// protsessySistemy снимает процессы всех сеансов. Служба работает от SYSTEM и
// видит и службу zapret в нулевом сеансе, и окно человека.
func protsessySistemy() ([]Protsess, error) {
	snimok, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, fmt.Errorf("снимок процессов не получен: %w", err)
	}
	defer windows.CloseHandle(snimok)

	var zapis windows.ProcessEntry32
	zapis.Size = uint32(unsafe.Sizeof(zapis))
	var itog []Protsess
	// Один буфер на весь обход: 64 КиБ на каждый процесс раз в 30 с давали бы
	// службе десяток мегабайт мусора на каждую проверку.
	bufer := make([]uint16, windows.MAX_LONG_PATH)
	for err = windows.Process32First(snimok, &zapis); err == nil; err = windows.Process32Next(snimok, &zapis) {
		itog = append(itog, Protsess{
			Pid:  zapis.ProcessID,
			Imya: windows.UTF16ToString(zapis.ExeFile[:]),
			Put:  putObraza(zapis.ProcessID, bufer),
		})
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return itog, fmt.Errorf("обход процессов оборвался: %w", err)
	}
	return itog, nil
}

// putObraza отдаёт полный путь образа или пустую строку. Буфер MAX_LONG_PATH:
// программа в папке глубже 260 знаков иначе осталась бы без пути и не
// отличилась бы от нашей по каталогу.
func putObraza(pid uint32, bufer []uint16) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	dlina := uint32(len(bufer))
	if err := windows.QueryFullProcessImageName(h, 0, &bufer[0], &dlina); err != nil {
		return ""
	}
	return windows.UTF16ToString(bufer[:dlina])
}

// drayveryWinDivert отдаёт запущенные драйверы с именем на «windivert»:
// zapret ставит `WinDivert`, GoodbyeDPI 0.2.2 `WinDivert1.4`.
func drayveryWinDivert() ([]string, error) {
	m, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_ENUMERATE_SERVICE)
	if err != nil {
		return nil, fmt.Errorf("диспетчер служб не открылся: %w", err)
	}
	defer windows.CloseServiceHandle(m)

	var nuzhno, chislo uint32
	var buf []byte
	for popytka := 0; popytka < 3; popytka++ {
		var ukaz *byte
		if len(buf) > 0 {
			ukaz = &buf[0]
		}
		err = windows.EnumServicesStatusEx(m, windows.SC_ENUM_PROCESS_INFO, windows.SERVICE_DRIVER,
			windows.SERVICE_ACTIVE, ukaz, uint32(len(buf)), &nuzhno, &chislo, nil, nil)
		if err == nil {
			break
		}
		if !errors.Is(err, windows.ERROR_MORE_DATA) {
			return nil, fmt.Errorf("драйверы не перечислены: %w", err)
		}
		// Список растёт между вызовами, поэтому запас сверх названного.
		buf = make([]byte, nuzhno+4096)
	}
	if err != nil {
		return nil, fmt.Errorf("драйверы не перечислены: %w", err)
	}
	if chislo == 0 {
		return nil, nil
	}
	zapisi := unsafe.Slice((*windows.ENUM_SERVICE_STATUS_PROCESS)(unsafe.Pointer(&buf[0])), chislo)
	var itog []string
	for _, z := range zapisi {
		imya := windows.UTF16PtrToString(z.ServiceName)
		if strings.HasPrefix(strings.ToLower(imya), "windivert") {
			itog = append(itog, imya)
		}
	}
	return itog, nil
}

// derzhitWinDivertSistemno ищет WinDivert.dll среди модулей процесса, и
// 64-битных, и 32-битных: GoodbyeDPI выходит в обеих сборках.
func derzhitWinDivertSistemno(pid uint32) (bool, error) {
	var snimok windows.Handle
	var err error
	// ERROR_BAD_LENGTH: процесс грузит или выгружает модули прямо сейчас, и
	// документация велит просто повторить.
	for popytka := 0; popytka < 3; popytka++ {
		snimok, err = windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPMODULE|windows.TH32CS_SNAPMODULE32, pid)
		if !errors.Is(err, windows.ERROR_BAD_LENGTH) {
			break
		}
	}
	if err != nil {
		return false, fmt.Errorf("модули процесса %d не сняты: %w", pid, err)
	}
	defer windows.CloseHandle(snimok)

	var m windows.ModuleEntry32
	m.Size = uint32(unsafe.Sizeof(m))
	for err = windows.Module32First(snimok, &m); err == nil; err = windows.Module32Next(snimok, &m) {
		if strings.EqualFold(windows.UTF16ToString(m.Module[:]), "windivert.dll") {
			return true, nil
		}
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return false, fmt.Errorf("обход модулей процесса %d оборвался: %w", pid, err)
	}
	return false, nil
}
