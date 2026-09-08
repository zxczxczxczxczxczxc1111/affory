package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/yadra"
)

// Команда measureDelays: задержка ПО КАЖДОМУ серверу, две цифры вместо одной.
//
// До неё клиент знал ровно одно число, задержку последней пробы urltest, одну
// на всё подключение и обновляемую раз в три минуты. Выбирать сервер по такому
// числу нельзя: оно про тот сервер, который уже выбран.
//
// Две цифры, а не одна, потому что они отвечают на разные вопросы. tcping это
// дорога до узла, realping это весь путь через туннель вместе с рукопожатием.
// Сервер, отвечающий на TCP мгновенно и не несущий ни байта, это обычный
// случай (просроченный ключ, чужой sid у REALITY), и по одной цифре он
// неотличим от далёкого, но исправного.

// TestZaderzhkiBezTunnelyaOtdayutTcpingANeOtkaz: главное свойство части Б.
// tcping туннеля не требует, значит кнопка полезна ДО подключения, то есть
// ровно тогда, когда человеку и надо выбрать, куда подключаться.
func TestZaderzhkiBezTunnelyaOtdayutTcpingANeOtkaz(t *testing.T) {
	l := zhivoyUzel(t)
	s := podstavnayaSUzlom(t, l)

	k := s.Obrabotat(context.Background(), protokol.Kadr{
		Tip: "komanda", Id: 1, Imya: "measureDelays",
	})
	if k.Oshib != nil {
		t.Fatalf("отказ при опущенном туннеле: %v", k.Oshib)
	}
	z := razobratZaderzhki(t, k)
	if len(z) != 1 {
		t.Fatalf("замеров не по числу серверов: %d", len(z))
	}
	if z[0].TcpingMs == nil {
		t.Fatal("tcping не измерен, хотя узел жив и туннель для него не нужен")
	}
	if *z[0].TcpingMs < 0 {
		t.Fatalf("tcping отрицательный: %d", *z[0].TcpingMs)
	}
	if z[0].RealpingMs != nil {
		t.Fatalf("realping измерен без туннеля: %v", *z[0].RealpingMs)
	}
	if z[0].RealpingOtkaz == "" {
		t.Fatal("realping не измерен и не объяснён: человеку нечего прочитать")
	}
}

// TestZaderzhkiMolchashchiyUzelEtoOtkazANeNol: ноль вместо отказа поставил бы
// мёртвый сервер ПЕРВЫМ по задержке, то есть ровно наверх списка.
func TestZaderzhkiMolchashchiyUzelEtoOtkazANeNol(t *testing.T) {
	l := zhivoyUzel(t)
	adres := l.Addr().String()
	l.Close()
	s := podstavnayaSAdresom(t, adres)

	k := s.Obrabotat(context.Background(), protokol.Kadr{
		Tip: "komanda", Id: 1, Imya: "measureDelays",
	})
	if k.Oshib != nil {
		t.Fatalf("вся команда отвергнута из-за одного мёртвого узла: %v", k.Oshib)
	}
	z := razobratZaderzhki(t, k)
	if z[0].TcpingMs != nil {
		t.Fatalf("мёртвый узел получил число: %v", *z[0].TcpingMs)
	}
	if z[0].TcpingOtkaz == "" {
		t.Fatal("мёртвый узел без причины отказа")
	}
}

// TestZaderzhkiSTunnelemZovutKlashPoTeguKazhdogo: realping идёт через ЯДРО, по
// тегу конкретного исходящего, а не через отдельную пробу. Проба мимо ядра
// мерила бы путь, которым трафик не пойдёт.
func TestZaderzhkiSTunnelemZovutKlashPoTeguKazhdogo(t *testing.T) {
	l := zhivoyUzel(t)
	s := podstavnayaSUzlom(t, l)
	s.mu.Lock()
	s.portClash, s.sekretClash = 9090, "sekret"
	s.mu.Unlock()

	var sprosheno []string
	s.zamerit = func(_ context.Context, _, _, teg string) (time.Duration, error) {
		sprosheno = append(sprosheno, teg)
		return 87 * time.Millisecond, nil
	}

	k := s.Obrabotat(context.Background(), protokol.Kadr{
		Tip: "komanda", Id: 1, Imya: "measureDelays",
	})
	if k.Oshib != nil {
		t.Fatalf("отказ при поднятом туннеле: %v", k.Oshib)
	}
	z := razobratZaderzhki(t, k)
	if z[0].RealpingMs == nil || *z[0].RealpingMs != 87 {
		t.Fatalf("realping не дошёл: %v", z[0].RealpingMs)
	}
	if z[0].TcpingMs != nil {
		t.Fatal("TCP при включённом TUN не является независимым замером")
	}
	if len(sprosheno) != 1 || sprosheno[0] == "" {
		t.Fatalf("ядро спрошено не по тегу сервера: %v", sprosheno)
	}
}

// TestZaderzhkiOtkazYadraNeSpisyvaetsyaNaServer: если молчит Clash API, виноваты
// МЫ, а не сервер. Свалить своё в «сервер плох» значит выкинуть исправный
// сервер из списка по собственной вине.
func TestZaderzhkiOtkazYadraNeSpisyvaetsyaNaServer(t *testing.T) {
	l := zhivoyUzel(t)
	s := podstavnayaSUzlom(t, l)
	s.mu.Lock()
	s.portClash, s.sekretClash = 9090, "sekret"
	s.mu.Unlock()
	s.zamerit = func(context.Context, string, string, string) (time.Duration, error) {
		return 0, errors.New("clash_api не отвечает")
	}

	k := s.Obrabotat(context.Background(), protokol.Kadr{
		Tip: "komanda", Id: 1, Imya: "measureDelays",
	})
	z := razobratZaderzhki(t, k)
	if z[0].TcpingMs != nil || z[0].TcpingOtkaz == "" {
		t.Fatal("TCP под TUN не должен выдавать локальное подтверждение за задержку узла")
	}
	if z[0].RealpingOtkaz == "" {
		t.Fatal("отказ ядра не назван")
	}
}

type zamerServera struct {
	Id            string `json:"id"`
	TcpingMs      *int64 `json:"tcping_ms"`
	TcpingOtkaz   string `json:"tcping_otkaz"`
	RealpingMs    *int64 `json:"realping_ms"`
	RealpingOtkaz string `json:"realping_otkaz"`
}

func razobratZaderzhki(t *testing.T, k protokol.Kadr) []zamerServera {
	t.Helper()
	if k.Telo == nil {
		t.Fatal("ответ без тела")
	}
	var o struct {
		Zamery []zamerServera `json:"zamery"`
	}
	if err := json.Unmarshal(k.Telo, &o); err != nil {
		t.Fatalf("ответ не разбирается: %v", err)
	}
	if len(o.Zamery) == 0 {
		t.Fatal("ответ без замеров")
	}
	return o.Zamery
}

func zhivoyUzel(t *testing.T) net.Listener {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	return l
}

func podstavnayaSUzlom(t *testing.T, l net.Listener) *Sluzhba {
	t.Helper()
	return podstavnayaSAdresom(t, l.Addr().String())
}

func podstavnayaSAdresom(t *testing.T, adres string) *Sluzhba {
	t.Helper()
	host, port, err := net.SplitHostPort(adres)
	if err != nil {
		t.Fatal(err)
	}
	nomer, err := net.LookupPort("tcp", port)
	if err != nil {
		t.Fatal(err)
	}
	s := podstavnaya(t, nil)
	srv := protokol.Server{
		Id: "u1", Imya: "узел", Transport: "hy2",
		Host: host, Port: nomer, Parol: "parol",
	}
	if err := s.pravitNabor(func(n *Nabor) error {
		n.Servery = []protokol.Server{srv}
		n.Vybran = srv.Id
		return nil
	}); err != nil {
		t.Fatalf("набор не записан: %v", err)
	}
	s.sobratAdresa = func() ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr(host)}, nil
	}
	// Умолчание боевого пути: настоящий tcping. Тесты, которым нужен туннель,
	// подменяют только s.zamerit.
	_ = yadra.Tcping
	return s
}
