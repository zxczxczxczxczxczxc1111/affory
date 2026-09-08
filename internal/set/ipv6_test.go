package set

import (
	"errors"
	"strings"
	"testing"
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
