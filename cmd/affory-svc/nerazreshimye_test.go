package main

import (
	"context"
	"errors"
	"net/netip"
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/set"
)

// Один мёртвый хост в подписке НЕ ДОЛЖЕН запрещать работу целиком.
//
// Найдено живым прогоном на стенде 01.09.2026: в подписке из одиннадцати узлов
// один перестал резолвиться, и killswitch on отказал целиком со словами
// «адреса кандидатов не собраны». Человек видит отказ и не понимает, при чём
// тут сервер, которым он не пользуется.
//
// Решение владельца: исключать из списка с уведомлением. Дыры в правиле петли
// это не делает: к серверу, которого нет в DNS, ядро всё равно не пойдёт.
func TestMyortvyyHostNeZapreshchaetRezhim(t *testing.T) {
	s := podstavnaya(t, nil)
	zhivoy := netip.MustParseAddr("192.0.2.225")
	s.sobratAdresa = func() ([]netip.Addr, error) {
		return []netip.Addr{zhivoy}, &set.OshibkaRazresheniya{Imena: []string{"podpiska.example"}}
	}

	r, err := s.spisokRazreshyonnogo(set.Adapter{
		Indeks: 10, Imya: "tun0",
		Adresa: []netip.Addr{netip.MustParseAddr("172.19.0.1")},
	})
	if err != nil {
		t.Fatalf("режим отказал из-за одного мёртвого хоста: %v", err)
	}
	if len(r.Kandidaty) != 1 || r.Kandidaty[0] != zhivoy {
		t.Fatalf("разрешившийся адрес потерялся: %v", r.Kandidaty)
	}
}

// Уведомление обязательно: молча выкинуть сервер значит оставить человека с
// подпиской, которая тихо стала короче.
func TestNerazreshivshiysyaHostUezzhaetSobytiem(t *testing.T) {
	s := podstavnaya(t, nil)
	s.sobratAdresa = func() ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("192.0.2.225")},
			&set.OshibkaRazresheniya{Imena: []string{"podpiska.example"}}
	}

	id, sob := s.Podpisatsya()
	defer s.Otpisatsya(id)

	if _, err := s.spisokRazreshyonnogo(set.Adapter{
		Indeks: 10, Imya: "tun0",
		Adresa: []netip.Addr{netip.MustParseAddr("172.19.0.1")},
	}); err != nil {
		t.Fatal(err)
	}

	for {
		select {
		case k := <-sob:
			if k.Imya == "serversUnreachable" && strings.Contains(string(k.Telo), "podpiska.example") {
				return
			}
		default:
			t.Fatal("события об исключённом сервере не было: подписка молча стала короче")
		}
	}
}

// Обратная сторона: если не разрешилось НИЧЕГО, это настоящий отказ. Пустой
// список кандидатов означает правило петли без единого адреса, то есть
// запертую машину без выхода к своему же серверу.
func TestKogdaNeRazreshilosNichegoEtoOtkaz(t *testing.T) {
	s := podstavnaya(t, nil)
	s.sobratAdresa = func() ([]netip.Addr, error) {
		return nil, &set.OshibkaRazresheniya{Imena: []string{"a.example", "b.example"}}
	}

	_, err := s.spisokRazreshyonnogo(set.Adapter{
		Indeks: 10, Imya: "tun0",
		Adresa: []netip.Addr{netip.MustParseAddr("172.19.0.1")},
	})
	if err == nil {
		t.Fatal("пустой список кандидатов принят: режим запрёт машину без выхода к серверу")
	}
	if !errors.Is(err, set.ErrImyaNeRazreshilos) {
		t.Fatalf("причина потеряна: %v", err)
	}
}

// Полный отказ резолвера человек читал как поломку брандмауэра.
//
// Ветка setKillSwitch в диспетчере ставила firewall-failed на ЛЮБОЙ отказ, а
// «не разрешилось ни одно имя» приходило туда голой ошибкой из
// kandidatySIsklyucheniem. Человек шёл чинить netsh при исправном netsh, а
// готовый экран «проверь сеть» не показывался никогда: кода dns-resolve-failed
// не производил никто.
func TestPolnyyOtkazDNSNeNazyvayetsyaPolomkoyBrandmauera(t *testing.T) {
	s, _, _ := sKillSwitch(t)
	// Не разрешилось НИ ОДНО имя: это отказ резолвера, а не netsh.
	s.sobratAdresa = func() ([]netip.Addr, error) {
		return nil, &set.OshibkaRazresheniya{Imena: []string{"a.example", "b.example"}}
	}
	if err := s.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}

	o := s.Obrabotat(ctxAdmina(), protokol.Kadr{
		Tip: "cmd", Id: 1, Imya: "setKillSwitch", Telo: []byte(`{"vkl":true}`),
	})
	if o.Oshib == nil {
		t.Fatal("режим включился при молчащем резолвере")
	}
	if kod := o.Oshib.Kod; kod != protokol.KodDnsResolveFailed {
		t.Errorf("код отказа %q, ожидали dns-resolve-failed", kod)
	}
}

// Обратная сторона той же ветки: часть имён не разрешилась это ПРЕЖНИЙ случай,
// сервер исключается, режим включается. Без этого судьи починка З1 могла бы
// назвать отказом DNS любое исключение одного мёртвого хоста.
func TestChastichnyyOtkazDNSNeNazyvayetsyaOtkazomRezolvera(t *testing.T) {
	s, vklyucheno, _ := sKillSwitch(t)
	s.sobratAdresa = func() ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("192.0.2.225")},
			&set.OshibkaRazresheniya{Imena: []string{"a.example"}}
	}
	if err := s.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	o := s.Obrabotat(ctxAdmina(), protokol.Kadr{
		Tip: "cmd", Id: 1, Imya: "setKillSwitch", Telo: []byte(`{"vkl":true}`),
	})
	if o.Oshib != nil {
		t.Fatalf("один мёртвый хост запретил режим: %v", o.Oshib)
	}
	if len(*vklyucheno) != 1 {
		t.Fatalf("вызовов включения %d, ожидался один", len(*vklyucheno))
	}
}
