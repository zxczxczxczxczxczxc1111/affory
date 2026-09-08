package main

import (
	"encoding/json"
	"errors"
	"net/netip"
	"sort"
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/genkonfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/set"
)

// Сверка СОСТАВА двух списков со ВХОДОМ, а не одного списка с другим.
//
// Сравнение двух обёрток над одним сборщиком истинно всегда: пустой список,
// потерянный кандидат и забытый адрес подписки проходят одинаково. Поэтому оба
// списка строятся настоящими генераторами и сверяются с тем, что подали на вход.
//
// Тест живёт здесь, а не в internal/set, намеренно: заставить set импортировать
// genkonfig запрещено долгом №3 того же файла, а cmd/affory-svc видит оба
// пакета по своей природе.

func adresaProby() []netip.Addr {
	return []netip.Addr{
		netip.MustParseAddr("192.0.2.225"),
		netip.MustParseAddr("203.0.113.1"),
		netip.MustParseAddr("203.0.113.2"),
		netip.MustParseAddr("198.51.100.9"), // адрес подписки
	}
}

func serveryProby() []protokol.Server {
	return []protokol.Server{
		{Id: "nl", Transport: "reality-tcp", Host: "192.0.2.225", Port: 443,
			Uuid: "11111111-2222-3333-4444-555555555555",
			// 43 символа base64url: check силён в значениях и отверг бы короче.
			PublicKey: "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8", ShortId: "01ab",
			Sni: "www.example.com"},
		{Id: "k1", Transport: "ws", Host: "203.0.113.1", Port: 443,
			Uuid: "11111111-2222-3333-4444-555555555555", Put: "/ws", Sni: "www.example.com"},
		{Id: "k2", Transport: "grpc", Host: "203.0.113.2", Port: 443,
			Uuid: "11111111-2222-3333-4444-555555555555", Put: "gun", Sni: "www.example.com"},
	}
}

func vhodS(adresa []netip.Addr, servery []protokol.Server) genkonfig.Vhod {
	return genkonfig.Vhod{
		Server:         servery[0],
		Servery:        servery,
		Kandidaty:      adresa,
		Resolver:       netip.MustParseAddr("10.7.0.1"),
		PutiProtsessov: []string{`C:\x\sing-box.exe`, `C:\x\affory-svc.exe`},
		ClashApi:       genkonfig.ClashApi{Adres: "127.0.0.1", Port: 9090, Sekret: "s"},
	}
}

// ipCidrIz достаёт адреса правила петли из готового конфига sing-box.
func ipCidrIz(t *testing.T, v genkonfig.Vhod) []string {
	t.Helper()
	b, err := genkonfig.SingBox(v)
	if err != nil {
		t.Fatal(err)
	}
	var k struct {
		Route struct {
			Rules []map[string]any `json:"rules"`
		} `json:"route"`
	}
	if err := json.Unmarshal(b, &k); err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, p := range k.Route.Rules {
		seti, est := p["ip_cidr"].([]any)
		if !est {
			continue
		}
		for _, s := range seti {
			str, _ := s.(string)
			// Многоадресные и широковещательные это отдельное правило, не наше.
			if strings.HasPrefix(str, "224.") || strings.HasPrefix(str, "255.") {
				continue
			}
			out = append(out, strings.TrimSuffix(strings.TrimSuffix(str, "/32"), "/128"))
		}
	}
	sort.Strings(out)
	return out
}

// remoteIpIz достаёт адреса разрешающего правила брандмауэра.
func remoteIpIz(t *testing.T, r set.Razreshyonnoe) []string {
	t.Helper()
	for _, kom := range set.KomandyRazresheniya(r) {
		if kom[0] != set.PravAllowSrv {
			continue
		}
		for _, arg := range kom {
			if !strings.HasPrefix(arg, "remoteip=") {
				continue
			}
			out := strings.Split(strings.TrimPrefix(arg, "remoteip="), ",")
			sort.Strings(out)
			return out
		}
	}
	return nil
}

func ravnyMnozhestva(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	x, y := append([]string(nil), a...), append([]string(nil), b...)
	sort.Strings(x)
	sort.Strings(y)
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}

func TestSostavSpiskovProtivVhoda(t *testing.T) {
	adresa, servery := adresaProby(), serveryProby()
	petlya := ipCidrIz(t, vhodS(adresa, servery))
	razresh := remoteIpIz(t, set.Razreshyonnoe{
		AdresTun: netip.MustParseAddr("172.19.0.1"), Kandidaty: adresa,
	})
	ozhidaem := []string{"192.0.2.225", "203.0.113.1", "203.0.113.2", "198.51.100.9"}

	if !ravnyMnozhestva(petlya, ozhidaem) {
		t.Fatalf("правило петли: %v, ожидалось %v", petlya, ozhidaem)
	}
	if !ravnyMnozhestva(razresh, ozhidaem) {
		t.Fatalf("разрешающие правила: %v, ожидалось %v", razresh, ozhidaem)
	}
}

func TestUbrannyyAdresIschezaetIzOboih(t *testing.T) {
	// Отрицательный случай. Без него тест выше проходит и на списке, вбитом
	// в код гвоздями.
	adresa := adresaProby()[:2]
	servery := serveryProby()[:1]
	if soderzhitStroku(ipCidrIz(t, vhodS(adresa, servery)), "203.0.113.2") {
		t.Fatal("убранный адрес остался в правиле петли")
	}
	razresh := remoteIpIz(t, set.Razreshyonnoe{
		AdresTun: netip.MustParseAddr("172.19.0.1"), Kandidaty: adresa,
	})
	if soderzhitStroku(razresh, "203.0.113.2") {
		t.Fatal("убранный адрес остался в разрешающих правилах")
	}
}

func TestOdinIstochnikDayotOdinakovyeSpiski(t *testing.T) {
	// Сборщик один, и это проверяется на его СОБСТВЕННОМ выходе, а не на
	// совпадении двух обёрток: адреса берутся из SobratAdresa и подаются в оба
	// генератора. Расхождение здесь означало бы либо петлю, либо туннель,
	// отрезающий сам себя в запертом режиме.
	servery := serveryProby()
	adresa, err := set.SobratAdresa(servery, "")
	if err != nil {
		t.Fatal(err)
	}
	petlya := ipCidrIz(t, vhodS(adresa, servery))
	razresh := remoteIpIz(t, set.Razreshyonnoe{
		AdresTun: netip.MustParseAddr("172.19.0.1"), Kandidaty: adresa,
	})
	if !ravnyMnozhestva(petlya, razresh) {
		t.Fatalf("списки разошлись: петля %v, брандмауэр %v", petlya, razresh)
	}
	if len(petlya) != len(servery) {
		t.Fatalf("адресов %v, а серверов %d", petlya, len(servery))
	}
}

func soderzhitStroku(s []string, chto string) bool {
	for _, v := range s {
		if v == chto {
			return true
		}
	}
	return false
}

func TestObaPotrebitelyaIdutCherezOdinSbornik(t *testing.T) {
	// Тест ловит возврат к самодеятельности: если завтра одно из двух мест снова
	// начнёт разбирать адрес само, приметный адрес не доедет ровно до него, и
	// расхождение станет видно здесь, а не через неделю в виде петли.
	primetnyy := netip.MustParseAddr("198.51.100.77")
	s := podstavnaya(t, nil)
	// Приметный адрес добавляется К адресу сервера, а не вместо него: инвариант
	// задачи 3.6 справедливо отвергает конфиг, где адреса кандидата нет в
	// правиле петли, и подмена «вместо» проверяла бы этот инвариант, а не шов.
	servery, err := s.serveryDlyaPodyoma()
	if err != nil {
		t.Fatal(err)
	}
	adrServera := netip.MustParseAddr(servery[0].Host)
	s.sobratAdresa = func() ([]netip.Addr, error) {
		return []netip.Addr{adrServera, primetnyy}, nil
	}

	telo, _, _, err := s.sobratTun(nil)
	if err != nil {
		t.Fatalf("конфиг туннеля не собран: %v", err)
	}
	if !strings.Contains(string(telo), primetnyy.String()) {
		t.Fatal("правило петли собрано мимо общего сборщика адресов")
	}

	r, err := s.spisokRazreshyonnogo(set.Adapter{
		Adresa: []netip.Addr{netip.MustParseAddr("172.19.0.1")}, Indeks: 1,
	})
	if err != nil {
		t.Fatalf("список разрешённого не собран: %v", err)
	}
	if !soderzhitStroku(remoteIpIz(t, r), primetnyy.String()) {
		t.Fatal("разрешающие правила собраны мимо общего сборщика адресов")
	}
}

func TestOtkazSbornikaOstanavlivaetOboih(t *testing.T) {
	// Неразрешившееся имя обязано ОСТАНОВИТЬ подъём, а не тихо сузить список.
	// Дыра в правиле петли не видна ничем, пока однажды не станет петлёй.
	s := podstavnaya(t, nil)
	s.sobratAdresa = func() ([]netip.Addr, error) { return nil, errors.New("имя не разрешилось") }

	if _, _, _, err := s.sobratTun(nil); err == nil {
		t.Fatal("конфиг туннеля собрался при отказе сборщика адресов")
	}
	if _, err := s.spisokRazreshyonnogo(set.Adapter{
		Adresa: []netip.Addr{netip.MustParseAddr("172.19.0.1")}, Indeks: 1,
	}); err == nil {
		t.Fatal("список разрешённого собрался при отказе сборщика адресов")
	}
}
