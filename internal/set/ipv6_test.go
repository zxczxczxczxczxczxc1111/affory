package set

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestPravilaZavodyatsyaSImenem(t *testing.T) {
	// The rule is created through netsh, where the name IS the identity: every
	// later lookup, and the two commands printed in ESLI-NET-INTERNETA.txt, find
	// it by that exact string. A rule created without one is a rule nobody can
	// remove without opening the firewall UI and squinting.
	k := strings.Join(komandaSozdaniya(), " ")
	if !strings.Contains(k, "name="+ImyaPravilaIPv6) {
		t.Fatal("правило заводится без имени")
	}
}

// Самый дорогой тест этого файла. Он охраняет замер, а не вкус.
func TestPravilaNeRubitVesTrafik(t *testing.T) {
	k := strings.Join(komandaSozdaniya(), " ")
	// Measured in the guest on 01.09.2026: with remoteip=any this exact rule took
	// the machine off the network entirely. netsh has no address-family selector,
	// so "any" means every address, not every IPv6 address, and a blocking rule
	// outranks every allow rule there is.
	if strings.Contains(k, "remoteip=any") {
		t.Fatal("remoteip=any это ВЕСЬ исходящий трафик, а не IPv6: машина запрётся наглухо")
	}
	if strings.Contains(k, "localip=any") {
		t.Fatal("localip=any лишний и путает: семейство задаётся удалённым адресом")
	}
	if !strings.Contains(k, "remoteip=::/1,8000::/1") {
		t.Fatalf("нет покрытия пространства IPv6 двумя половинами: %s", k)
	}
	// `::/0` netsh отвергает, и это проверено вживую. Тест держит форму, которая
	// принимается, чтобы её не "упростили" обратно.
	if strings.Contains(k, "::/0") {
		t.Fatal("::/0 netsh не принимает: address prefixes is invalid")
	}
}

func TestSnyatieNesushchestvuyushchegoEtoUspeh(t *testing.T) {
	// Cleanup runs exactly when things have already gone wrong. A teardown that
	// fails because there was nothing to tear down is a teardown that stops the
	// recovery it was written for.
	prezhniy := vypolnit
	vypolnit = func([]string) (string, error) {
		return "No rules match the specified criteria.", errors.New("exit status 1")
	}
	defer func() { vypolnit = prezhniy }()

	if err := VernutIPv6(); err != nil {
		t.Fatalf("снятие отсутствующего правила дало ошибку: %v", err)
	}
}

func TestGlushitSnachalaSnimaet(t *testing.T) {
	// netsh does not update a rule by name, it adds a SECOND one with the same
	// name, and a later delete removes only one of them. Two runs would leave a
	// rule behind after disconnect.
	var poryadok []string
	prezhniy := vypolnit
	vypolnit = func(a []string) (string, error) {
		poryadok = append(poryadok, a[2])
		return "Ok.", nil
	}
	defer func() { vypolnit = prezhniy }()

	if err := GlushitIPv6(); err != nil {
		t.Fatal(err)
	}
	if len(poryadok) != 2 || poryadok[0] != "delete" || poryadok[1] != "add" {
		t.Fatalf("порядок вызовов %v, ожидался delete затем add", poryadok)
	}
}

// zakrytie подставляет ответы системы для ZakrytIPv6 и считает вызовы netsh.
//
// Срок ожидания фильтра ядра сжат: в настоящем он три секунды, и тест на
// запасной путь иначе стоил бы их каждый раз.
func zakrytie(t *testing.T, proba func() bool, estIPv6 bool, netsh func([]string) (string, error)) *[]string {
	t.Helper()
	var vyzovy []string
	prezhProba, prezhEst, prezhVyp := probaFiltra, estIPv6NaMashine, vypolnit
	prezhSrok, prezhShag := srokFiltraYadra, shagFiltraYadra
	t.Cleanup(func() {
		probaFiltra, estIPv6NaMashine, vypolnit = prezhProba, prezhEst, prezhVyp
		srokFiltraYadra, shagFiltraYadra = prezhSrok, prezhShag
		praviloZavedeno.Store(false)
	})
	praviloZavedeno.Store(false)
	srokFiltraYadra, shagFiltraYadra = 150*time.Millisecond, 10*time.Millisecond
	probaFiltra = proba
	estIPv6NaMashine = func() (bool, error) { return estIPv6, nil }
	vypolnit = func(a []string) (string, error) {
		vyzovy = append(vyzovy, a[2])
		return netsh(a)
	}
	return &vyzovy
}

func netshUspeh([]string) (string, error) { return "Ok.", nil }

// Главный случай жалобы 17.10.2026: ядро уже закрыло IPv6, а служба
// брандмауэра отвечает на netsh отказом. netsh не зовётся вовсе, ни при
// подъёме, ни при опускании, и подключение не падает.
func TestFiltrYadraZakryvaetBezNetsh(t *testing.T) {
	vyzovy := zakrytie(t, func() bool { return true }, true, func([]string) (string, error) {
		return "", errors.New("Неустранимая ошибка брандмауэра Защитника Windows (0x2).")
	})
	if err := ZakrytIPv6(); err != nil {
		t.Fatalf("закрытый ядром IPv6 назван отказом: %v", err)
	}
	if err := OtkrytIPv6(); err != nil {
		t.Fatalf("опускание без своего правила дало ошибку: %v", err)
	}
	if len(*vyzovy) != 0 {
		t.Fatalf("netsh звался при закрытом ядром IPv6: %v", *vyzovy)
	}
}

// Фильтр ядро ставит чуть позже адаптера. Ранний ответ «открыт» не повод
// заводить правило: дождаться фильтра.
func TestFiltrYadraDozhidaetsya(t *testing.T) {
	var n int
	vyzovy := zakrytie(t, func() bool { n++; return n >= 3 }, true, netshUspeh)
	if err := ZakrytIPv6(); err != nil {
		t.Fatal(err)
	}
	if len(*vyzovy) != 0 {
		t.Fatalf("правило заведено, не дождавшись фильтра ядра: %v", *vyzovy)
	}
}

// Без IPv6 у машины уходить по нему нечему, и netsh не нужен.
func TestBezIPv6NaMashineNetshNeNuzhen(t *testing.T) {
	vyzovy := zakrytie(t, func() bool { return false }, false, netshUspeh)
	if err := ZakrytIPv6(); err != nil {
		t.Fatal(err)
	}
	if len(*vyzovy) != 0 {
		t.Fatalf("netsh звался на машине без IPv6: %v", *vyzovy)
	}
}

// Ядро IPv6 не закрыло, а IPv6 у машины есть: запасной путь через netsh, и
// снимается ровно то, что заведено.
func TestZapasnoyPutCherezNetsh(t *testing.T) {
	vyzovy := zakrytie(t, func() bool { return false }, true, netshUspeh)
	if err := ZakrytIPv6(); err != nil {
		t.Fatal(err)
	}
	if strings.Join(*vyzovy, " ") != "delete add" {
		t.Fatalf("запасной путь позвал netsh так: %v", *vyzovy)
	}
	if err := OtkrytIPv6(); err != nil {
		t.Fatal(err)
	}
	if strings.Join(*vyzovy, " ") != "delete add delete" {
		t.Fatalf("своё правило не снято при опускании: %v", *vyzovy)
	}
	// Второе опускание правила уже не трогает.
	if err := OtkrytIPv6(); err != nil {
		t.Fatal(err)
	}
	if len(*vyzovy) != 3 {
		t.Fatalf("снятое правило снимается повторно: %v", *vyzovy)
	}
}

// Открытый IPv6, который не закрылся и запасным путём, это отказ: туннель с
// открытым IPv6 течёт молча.
func TestOtkrytyyIPv6BezPravilaEtoOtkaz(t *testing.T) {
	zakrytie(t, func() bool { return false }, true, func(a []string) (string, error) {
		if a[2] == "add" {
			return "Неустранимая ошибка брандмауэра Защитника Windows (0x2).", errors.New("exit status 1")
		}
		return "No rules match the specified criteria.", errors.New("exit status 1")
	})
	err := ZakrytIPv6()
	if err == nil {
		t.Fatal("открытый IPv6 без правила назван закрытым")
	}
	if !strings.Contains(err.Error(), ImyaPravilaIPv6) {
		t.Errorf("в отказе не названо правило: %v", err)
	}
	if praviloZavedeno.Load() {
		t.Error("незаведённое правило записано как своё")
	}
}

// Проверка утечек спрашивает факт: фильтр или отсутствие IPv6 у машины.
func TestIPv6ZakrytPoFaktu(t *testing.T) {
	sluchai := []struct {
		imya    string
		filtr   bool
		estIPv6 bool
		zhdom   bool
	}{
		{"фильтр стоит", true, true, true},
		{"IPv6 у машины нет", false, false, true},
		{"IPv6 есть, фильтра нет", false, true, false},
	}
	for _, s := range sluchai {
		zakrytie(t, func() bool { return s.filtr }, s.estIPv6, netshUspeh)
		got, err := IPv6Zakryt()
		if err != nil || got != s.zhdom {
			t.Errorf("%s: закрыт=%v, ошибка %v, ждали %v", s.imya, got, err, s.zhdom)
		}
	}
}

// Проба идёт на адрес для документации и ни на что другое: без фильтра пакет
// уходит в никуда, а не к чужому серверу.
func TestProbaIdyotNaAdresDokumentatsii(t *testing.T) {
	ap, err := netip.ParseAddrPort(adresProbyFiltra)
	if err != nil {
		t.Fatalf("адрес пробы не разбирается: %v", err)
	}
	if !netip.MustParsePrefix("2001:db8::/32").Contains(ap.Addr()) {
		t.Fatalf("проба идёт на %s, вне 2001:db8::/32", ap.Addr())
	}
}

func TestOtkazyProbyIPv6(t *testing.T) {
	obernut := func(errno syscall.Errno) error {
		return &net.OpError{Op: "dial", Net: "tcp6", Err: os.NewSyscallError("connectex", errno)}
	}
	sluchai := []struct {
		imya   string
		err    error
		zakryt bool
	}{
		{"фильтр WFP", obernut(windows.WSAEACCES), true},
		{"нет пути в интернет по IPv6", obernut(windows.WSAENETUNREACH), true},
		// Узел недоступен сообщает роутер, то есть пакет уже ушёл с машины.
		{"узел недоступен", obernut(windows.WSAEHOSTUNREACH), false},
		{"срок пробы", context.DeadlineExceeded, false},
		{"отказ соединения", obernut(windows.WSAECONNREFUSED), false},
	}
	for _, sl := range sluchai {
		if got := zakrytPoOtkazu(sl.err); got != sl.zakryt {
			t.Errorf("%s: закрыт %v, ждали %v", sl.imya, got, sl.zakryt)
		}
	}
}

func TestPravilaEstOtlichaetPustotuOtOshibki(t *testing.T) {
	prezhniy := vypolnit
	defer func() { vypolnit = prezhniy }()

	vypolnit = func([]string) (string, error) {
		return "No rules match the specified criteria.", errors.New("exit status 1")
	}
	est, err := pravilaIPv6Est()
	if err != nil || est {
		t.Fatalf("отсутствие правила: est=%v err=%v", est, err)
	}

	vypolnit = func([]string) (string, error) {
		return "Rule Name: " + ImyaPravilaIPv6 + "\nAction: Block", nil
	}
	est, err = pravilaIPv6Est()
	if err != nil || !est {
		t.Fatalf("наличие правила: est=%v err=%v", est, err)
	}

	vypolnit = func([]string) (string, error) {
		return "The following command was not found", errors.New("exit status 1")
	}
	if _, err = pravilaIPv6Est(); err == nil {
		t.Fatal("настоящий отказ netsh выдан за отсутствие правила")
	}
}
