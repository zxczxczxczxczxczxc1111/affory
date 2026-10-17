package wfp

import (
	"encoding/binary"
	"fmt"
	"net/netip"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Filtr описывает один фильтр в слое исходящих соединений.
//
// Условия с одним полем складываются через ИЛИ, с разными через И: три
// адреса, два порта и два протокола в одном фильтре значат «любой из адресов
// на любом из портов любым из протоколов».
type Filtr struct {
	// Klyuch постоянный: по нему фильтр заменяется и снимается.
	Klyuch windows.GUID
	Imya   string
	V6     bool
	// Razreshit: разрешение, иначе запрет.
	Razreshit bool
	// Ves внутри подслоя, от 0 до 15: решает совпавший фильтр с наибольшим.
	Ves      uint8
	Usloviya []Uslovie
}

type vidUsloviya uint8

const (
	vidUdalyonnyySet vidUsloviya = iota + 1
	vidLokalnyyAdres
	vidUdalyonnyePorty
	vidLokalnyyPort
	vidProtokol
	vidPrilozhenie
	vidPetlya
)

// Uslovie это одно условие фильтра. Сравнимо через ==, поэтому набор
// фильтров проверяется тестом как данные.
type Uslovie struct {
	vid      vidUsloviya
	set      netip.Prefix
	ot, do   uint16
	protokol uint8
	put      string
}

// UdalyonnyySet: адрес назначения внутри сети.
func UdalyonnyySet(p netip.Prefix) Uslovie {
	return Uslovie{vid: vidUdalyonnyySet, set: p.Masked()}
}

// LokalnyyAdres: адрес, с которого уходит соединение.
func LokalnyyAdres(a netip.Addr) Uslovie {
	return Uslovie{vid: vidLokalnyyAdres, set: netip.PrefixFrom(a, a.BitLen())}
}

// UdalyonnyePorty: порт назначения от ot до do включительно.
func UdalyonnyePorty(ot, do uint16) Uslovie {
	return Uslovie{vid: vidUdalyonnyePorty, ot: ot, do: do}
}

// LokalnyyPort: порт, с которого уходит соединение.
func LokalnyyPort(p uint16) Uslovie { return Uslovie{vid: vidLokalnyyPort, ot: p, do: p} }

// Protokol: номер протокола IP, 6 это TCP, 17 это UDP.
func Protokol(p uint8) Uslovie { return Uslovie{vid: vidProtokol, protokol: p} }

// Prilozhenie: соединение открывает программа с этим путём.
func Prilozhenie(put string) Uslovie { return Uslovie{vid: vidPrilozhenie, put: put} }

// Petlya: соединение внутри машины.
func Petlya() Uslovie { return Uslovie{vid: vidPetlya} }

// UdalyonnayaSet отдаёт сеть назначения, если условие про неё.
func (u Uslovie) UdalyonnayaSet() (netip.Prefix, bool) {
	return u.set, u.vid == vidUdalyonnyySet
}

// DobavitFiltr ставит фильтр постоянным под основой o.
func (s *Seans) DobavitFiltr(o Osnova, f Filtr) error {
	var d derzhatel
	usloviya := make([]fwpmFilterCondition0, 0, len(f.Usloviya))
	for _, u := range f.Usloviya {
		c, err := d.uslovie(u, f.V6)
		if err != nil {
			return fmt.Errorf("фильтр %s: %w", f.Imya, err)
		}
		usloviya = append(usloviya, c)
	}
	imya, err := windows.UTF16PtrFromString(f.Imya)
	if err != nil {
		return err
	}
	provayder := o.Provayder
	sloy, deystvie := sloyConnectV4, uint32(fwpActionBlock)
	if f.V6 {
		sloy = sloyConnectV6
	}
	if f.Razreshit {
		deystvie = fwpActionPermit
	}
	filtr := fwpmFilter0{
		klyuch:    f.Klyuch,
		podpis:    fwpmDisplayData0{imya: imya},
		flagi:     flagPostoyannyy,
		provayder: &provayder,
		sloy:      sloy,
		podsloy:   o.Podsloy,
		ves:       fwpValue0{tip: fwpUint8, znachenie: uintptr(f.Ves)},
		deystvie:  fwpmAction0{tip: deystvie},
	}
	if len(usloviya) > 0 {
		filtr.chisloUsloviy = uint32(len(usloviya))
		filtr.usloviya = &usloviya[0]
	}
	var id uint64
	r, _, _ := procFilterAdd.Call(uintptr(s.h), uintptr(unsafe.Pointer(&filtr)), 0, uintptr(unsafe.Pointer(&id)))
	// Значения условий лежат в памяти Go, а WFP видит их адресами-числами,
	// которые сборщик мусора за ссылки не считает.
	runtime.KeepAlive(&d)
	runtime.KeepAlive(usloviya)
	return oshibka("FwpmFilterAdd0 "+f.Imya, r)
}

// derzhatel держит живыми значения, на которые условия ссылаются числом.
// Память Go не двигается, но без ссылки её соберут до вызова.
type derzhatel struct {
	zhivye []any
}

func (d *derzhatel) uslovie(u Uslovie, v6 bool) (fwpmFilterCondition0, error) {
	switch u.vid {
	case vidUdalyonnyySet, vidLokalnyyAdres:
		pole := poleUdalyonnyyAdres
		if u.vid == vidLokalnyyAdres {
			pole = poleLokalnyyAdres
		}
		z, err := d.set(u.set, v6)
		if err != nil {
			return fwpmFilterCondition0{}, err
		}
		return fwpmFilterCondition0{pole: pole, sravnenie: fwpMatchEqual, znachenie: z}, nil
	case vidUdalyonnyePorty, vidLokalnyyPort:
		pole := poleUdalyonnyyPort
		if u.vid == vidLokalnyyPort {
			pole = poleLokalnyyPort
		}
		if u.ot == u.do {
			return fwpmFilterCondition0{pole: pole, sravnenie: fwpMatchEqual,
				znachenie: fwpValue0{tip: fwpUint16, znachenie: uintptr(u.ot)}}, nil
		}
		if u.ot > u.do {
			return fwpmFilterCondition0{}, fmt.Errorf("диапазон портов %d-%d перевёрнут", u.ot, u.do)
		}
		r := &fwpRange0{
			ot: fwpValue0{tip: fwpUint16, znachenie: uintptr(u.ot)},
			do: fwpValue0{tip: fwpUint16, znachenie: uintptr(u.do)},
		}
		d.zhivye = append(d.zhivye, r)
		return fwpmFilterCondition0{pole: pole, sravnenie: fwpMatchRange,
			znachenie: fwpValue0{tip: fwpRangeTip, znachenie: uintptr(unsafe.Pointer(r))}}, nil
	case vidProtokol:
		return fwpmFilterCondition0{pole: poleProtokol, sravnenie: fwpMatchEqual,
			znachenie: fwpValue0{tip: fwpUint8, znachenie: uintptr(u.protokol)}}, nil
	case vidPrilozhenie:
		id, err := idPrilozheniya(u.put)
		if err != nil {
			return fwpmFilterCondition0{}, err
		}
		blob := &fwpByteBlob{razmer: uint32(len(id)), dannye: &id[0]}
		d.zhivye = append(d.zhivye, blob, id)
		return fwpmFilterCondition0{pole: polePrilozhenie, sravnenie: fwpMatchEqual,
			znachenie: fwpValue0{tip: fwpByteBlobTip, znachenie: uintptr(unsafe.Pointer(blob))}}, nil
	case vidPetlya:
		return fwpmFilterCondition0{pole: poleFlagi, sravnenie: fwpMatchFlagsAllSet,
			znachenie: fwpValue0{tip: fwpUint32, znachenie: flagPetlya}}, nil
	}
	return fwpmFilterCondition0{}, fmt.Errorf("неизвестное условие %d", u.vid)
}

// set раскладывает сеть в значение условия. Семейство обязано совпасть со
// слоем: адрес IPv6 в слое IPv4 WFP не отвергнет, а фильтр не сработает
// никогда.
func (d *derzhatel) set(p netip.Prefix, v6 bool) (fwpValue0, error) {
	if !p.IsValid() {
		return fwpValue0{}, fmt.Errorf("пустая сеть")
	}
	a := p.Addr()
	if a.Is4() == v6 {
		return fwpValue0{}, fmt.Errorf("сеть %s не того семейства для слоя", p)
	}
	if a.Is4() {
		b := a.As4()
		m := &fwpV4AddrAndMask{adres: binary.BigEndian.Uint32(b[:]), maska: maskaV4(p.Bits())}
		d.zhivye = append(d.zhivye, m)
		return fwpValue0{tip: fwpV4AddrMask, znachenie: uintptr(unsafe.Pointer(m))}, nil
	}
	m := &fwpV6AddrAndMask{adres: a.As16(), prefiks: uint8(p.Bits())}
	d.zhivye = append(d.zhivye, m)
	return fwpValue0{tip: fwpV6AddrMask, znachenie: uintptr(unsafe.Pointer(m))}, nil
}

func maskaV4(bity int) uint32 {
	if bity <= 0 {
		return 0
	}
	return ^uint32(0) << (32 - bity)
}

// idPrilozheniya отдаёт опознание программы в том виде, в каком его
// сравнивает WFP: путь через имя устройства, а не через букву диска. Файл
// обязан существовать.
func idPrilozheniya(put string) ([]byte, error) {
	if err := zagruzit(); err != nil {
		return nil, err
	}
	p, err := windows.UTF16PtrFromString(put)
	if err != nil {
		return nil, err
	}
	var blob *fwpByteBlob
	r, _, _ := procGetAppIdFromFile.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&blob)))
	if err := oshibka("FwpmGetAppIdFromFileName0 "+put, r); err != nil {
		return nil, err
	}
	defer procFreeMemory.Call(uintptr(unsafe.Pointer(&blob)))
	if blob == nil || blob.razmer == 0 {
		return nil, fmt.Errorf("пустое опознание программы %s", put)
	}
	return append([]byte(nil), unsafe.Slice(blob.dannye, blob.razmer)...), nil
}
