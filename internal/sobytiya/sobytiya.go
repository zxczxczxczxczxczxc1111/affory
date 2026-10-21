// Package sobytiya читает журнал событий Windows для выгрузки диагностики
// (1.10.0).
//
// Свои журналы у нас файлы (§13 спеки), но два вида бед в них не попадают
// никогда: служба, которую диспетчер служб не смог запустить или которая
// умерла (события 7000-7034 в журнале System), и процесс, который Windows
// закрыла из-за сбоя (Application Error 1000, Windows Error Reporting 1001,
// Application Hang 1002). Чтобы их увидеть, человеку приходилось открывать
// «Просмотр событий», а просили его об этом раз в жизни.
//
// Чтение через wevtapi напрямую, без wevtutil и PowerShell: чужая программа
// это ещё один отказ, который надо объяснять, и ещё одна кодовая страница.
package sobytiya

import (
	"encoding/xml"
	"errors"
	"fmt"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	wevtapi        = windows.NewLazySystemDLL("wevtapi.dll")
	procEvtQuery   = wevtapi.NewProc("EvtQuery")
	procEvtNext    = wevtapi.NewProc("EvtNext")
	procEvtRender  = wevtapi.NewProc("EvtRender")
	procEvtClose   = wevtapi.NewProc("EvtClose")
	errNetWevtapi  = errors.New("wevtapi.dll недоступна")
	errPustoyOtvet = errors.New("пустой ответ журнала")
)

const (
	evtQueryChannelPath      = 0x1
	evtQueryReverseDirection = 0x200
	evtRenderEventXml        = 1
	beskonechno              = 0xFFFFFFFF
	porciya                  = 16
)

// Zapis это одно событие в том виде, в каком оно уходит в выгрузку.
type Zapis struct {
	Vremya    time.Time
	Istochnik string
	Kod       uint32
	Dannye    []string
}

// Stroka это запись одной строкой: время местное, данные через «|».
func (z Zapis) Stroka() string {
	return fmt.Sprintf("%s %s %d: %s", z.Vremya.Local().Format("2006-01-02 15:04:05"), z.Istochnik, z.Kod,
		strings.Join(z.Dannye, " | "))
}

// Prochitat отдаёт до predel событий журнала zhurnal по запросу XPath,
// новые первыми, прошедшие filtr. filtr nil пропускает всё.
func Prochitat(zhurnal, zapros string, predel int, filtr func(Zapis) bool) ([]Zapis, error) {
	if err := wevtapi.Load(); err != nil {
		return nil, fmt.Errorf("%w: %v", errNetWevtapi, err)
	}
	put, err := windows.UTF16PtrFromString(zhurnal)
	if err != nil {
		return nil, err
	}
	z, err := windows.UTF16PtrFromString(zapros)
	if err != nil {
		return nil, err
	}
	nabor, _, errZ := procEvtQuery.Call(0, uintptr(unsafe.Pointer(put)), uintptr(unsafe.Pointer(z)),
		evtQueryChannelPath|evtQueryReverseDirection)
	if nabor == 0 {
		return nil, fmt.Errorf("журнал %s не открылся: %w", zhurnal, errZ)
	}
	defer zakryt(nabor)

	var itog []Zapis
	ruchki := make([]uintptr, porciya)
	for len(itog) < predel {
		var poluchili uint32
		ok, _, errN := procEvtNext.Call(nabor, porciya, uintptr(unsafe.Pointer(&ruchki[0])), beskonechno, 0,
			uintptr(unsafe.Pointer(&poluchili)))
		if ok == 0 {
			if errors.Is(errN, windows.ERROR_NO_MORE_ITEMS) {
				break
			}
			return itog, fmt.Errorf("журнал %s не дочитан: %w", zhurnal, errN)
		}
		for _, r := range ruchki[:poluchili] {
			zap, err := razobrat(r)
			zakryt(r)
			if err != nil || (filtr != nil && !filtr(zap)) || len(itog) >= predel {
				continue
			}
			itog = append(itog, zap)
		}
	}
	return itog, nil
}

func zakryt(r uintptr) { _, _, _ = procEvtClose.Call(r) }

// razobrat отрисовывает событие в XML и достаёт поля.
func razobrat(r uintptr) (Zapis, error) {
	var nuzhno, svoystv uint32
	_, _, err := procEvtRender.Call(0, r, evtRenderEventXml, 0, 0,
		uintptr(unsafe.Pointer(&nuzhno)), uintptr(unsafe.Pointer(&svoystv)))
	if !errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER) || nuzhno == 0 {
		return Zapis{}, errPustoyOtvet
	}
	buf := make([]uint16, (nuzhno+1)/2)
	ok, _, err := procEvtRender.Call(0, r, evtRenderEventXml, uintptr(nuzhno), uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&nuzhno)), uintptr(unsafe.Pointer(&svoystv)))
	if ok == 0 {
		return Zapis{}, fmt.Errorf("событие не отрисовано: %w", err)
	}
	return razobratXML(windows.UTF16ToString(buf))
}

type sobytieXML struct {
	System struct {
		Provider struct {
			Name string `xml:"Name,attr"`
		} `xml:"Provider"`
		EventID     uint32 `xml:"EventID"`
		TimeCreated struct {
			SystemTime string `xml:"SystemTime,attr"`
		} `xml:"TimeCreated"`
	} `xml:"System"`
	EventData struct {
		Data []struct {
			Value string `xml:",chardata"`
		} `xml:"Data"`
	} `xml:"EventData"`
}

// razobratXML чистая: проверяется без журнала.
func razobratXML(s string) (Zapis, error) {
	var x sobytieXML
	if err := xml.Unmarshal([]byte(s), &x); err != nil {
		return Zapis{}, fmt.Errorf("событие не разобрано: %w", err)
	}
	z := Zapis{Istochnik: x.System.Provider.Name, Kod: x.System.EventID}
	if t, err := time.Parse(time.RFC3339Nano, x.System.TimeCreated.SystemTime); err == nil {
		z.Vremya = t
	}
	for _, d := range x.EventData.Data {
		if v := strings.TrimSpace(d.Value); v != "" {
			z.Dannye = append(z.Dannye, v)
		}
	}
	return z, nil
}

// Upominaet отвечает, называет ли событие одно из имён: так из всех падений
// машины выбираются наши.
func (z Zapis) Upominaet(imena ...string) bool {
	vse := strings.ToLower(strings.Join(z.Dannye, " "))
	for _, imya := range imena {
		if strings.Contains(vse, strings.ToLower(imya)) {
			return true
		}
	}
	return false
}
