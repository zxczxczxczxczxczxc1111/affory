package main

import (
	"bytes"
	"fmt"
	"net/netip"
	"runtime"
	"strings"
	"time"
	"unicode/utf16"

	"golang.org/x/sys/windows"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/kodirovki"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/set"
)

// Шапка выгрузки диагностики (1.10.0). Каждая строка здесь это вопрос,
// который при разборе жалобы приходилось задавать человеку, а он отвечал на
// него через сутки или никогда: какой у него часовой пояс, кодовая страница,
// куда поставлена программа, есть ли рядом чужой VPN, верны ли часы.
//
// Адресов машины здесь нет: ни своих адресов адаптеров, ни шлюзов. DNS
// называется видом («частный», «внешний»), а адресом только когда это
// известный общий резолвер, по которому видно, что его поставили руками.

var izvestnyeDNS = map[netip.Addr]string{
	netip.MustParseAddr("1.1.1.1"):      "Cloudflare",
	netip.MustParseAddr("1.0.0.1"):      "Cloudflare",
	netip.MustParseAddr("8.8.8.8"):      "Google",
	netip.MustParseAddr("8.8.4.4"):      "Google",
	netip.MustParseAddr("9.9.9.9"):      "Quad9",
	netip.MustParseAddr("77.88.8.8"):    "Яндекс",
	netip.MustParseAddr("77.88.8.1"):    "Яндекс",
	netip.MustParseAddr("94.140.14.14"): "AdGuard",
	netip.MustParseAddr("94.140.15.15"): "AdGuard",
}

var cgnat = netip.MustParsePrefix("100.64.0.0/10")

func (s *Sluzhba) shapkaDiagnostiki(b *bytes.Buffer) {
	u := ustroystvoMashiny()
	fmt.Fprintf(b, "система: %s %s, %s\n", u.OS, u.VersiyaOS, runtime.GOARCH)
	seychas := time.Now()
	fmt.Fprintf(b, "время: %s\n", seychas.Format("2006-01-02 15:04:05 -07:00 MST"))
	ansi, konsoli := kodirovki.NomeraStranic()
	fmt.Fprintf(b, "кодовые страницы: ANSI %d, консоли %d\n", ansi, konsoli)
	if yazyki, err := windows.GetSystemPreferredUILanguages(windows.MUI_LANGUAGE_NAME); err == nil {
		fmt.Fprintf(b, "язык системы: %s\n", strings.Join(yazyki, ", "))
	} else {
		fmt.Fprintf(b, "язык системы не прочитан: %v\n", err)
	}
	fmt.Fprintf(b, "папка программы: %s (длина %d)\n", s.dirProgrammy, len(utf16.Encode([]rune(s.dirProgrammy))))

	s.mu.Lock()
	d, kogda := s.chasyRashozhdenie, s.chasyZamereno
	s.mu.Unlock()
	if kogda.IsZero() {
		b.WriteString("часы: не сверены\n")
	} else {
		fmt.Fprintf(b, "часы: %s точного времени, сверено %s\n", opisatRashozhdenie(d), kogda.Format("2006-01-02 15:04:05"))
	}

	if opis := opisatPomehi(s.obnovitPomehi()); opis != "" {
		fmt.Fprintf(b, "помехи: %s\n", opis)
	} else {
		b.WriteString("помехи: не найдено\n")
	}

	s.opisatSet(b)
}

// opisatSet пишет поднятые адаптеры и системный прокси вошедших людей.
func (s *Sluzhba) opisatSet(b *bytes.Buffer) {
	adaptery, err := s.adaptery()
	if err != nil {
		fmt.Fprintf(b, "сеть: адаптеры не перечислены: %v\n", err)
	} else {
		polovinki, errPol := set.PolovinkiUmolchaniya()
		b.WriteString("сеть:\n")
		opushcheno := 0
		for _, a := range adaptery {
			if a.Tip == windows.IF_TYPE_SOFTWARE_LOOPBACK {
				continue
			}
			if a.Sostoyanie != windows.IfOperStatusUp {
				opushcheno++
				continue
			}
			fmt.Fprintf(b, "  «%s» (%s), вид %d", a.Imya, a.Opisanie, a.Tip)
			switch {
			case a.Umolchanie:
				fmt.Fprintf(b, ", маршрут по умолчанию (метрика %d+%d)", a.MetrikaMarshruta, a.Metrika)
			case polovinki[a.Indeks]:
				b.WriteString(", маршрут по умолчанию половинками")
			}
			if len(a.Resolvery) > 0 {
				vidy := make([]string, 0, len(a.Resolvery))
				for _, r := range a.Resolvery {
					vidy = append(vidy, opisatDNS(r))
				}
				fmt.Fprintf(b, ", DNS: %s", strings.Join(vidy, ", "))
			}
			b.WriteByte('\n')
		}
		fmt.Fprintf(b, "  опущенных адаптеров: %d\n", opushcheno)
		if errPol != nil {
			fmt.Fprintf(b, "  маршруты-половинки не прочитаны: %v\n", errPol)
		}
	}

	proksi, err := s.prochitatProksi()
	switch {
	case err != nil:
		fmt.Fprintf(b, "системный прокси не прочитан: %v\n", err)
	case len(proksi) == 0:
		b.WriteString("системный прокси: вошедших людей нет\n")
	default:
		for _, p := range proksi {
			if p.Vklyuchen {
				fmt.Fprintf(b, "системный прокси: включён, %s\n", p.Adres)
			} else {
				b.WriteString("системный прокси: выключен\n")
			}
		}
	}
}

// opisatDNS называет резолвер видом, а не адресом: адрес резолвера
// провайдера это адрес его сети.
func opisatDNS(a netip.Addr) string {
	if imya, est := izvestnyeDNS[a]; est {
		return a.String() + " (" + imya + ")"
	}
	switch {
	case a.IsLoopback():
		return "на этой машине"
	case a.IsPrivate(), a.IsLinkLocalUnicast(), cgnat.Contains(a):
		return "частный"
	}
	return "внешний"
}
