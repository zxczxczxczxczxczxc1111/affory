package set

import (
	"bytes"
	"errors"
	"net/netip"
	"os"
	"strings"
	"testing"
)

// The grep guard the plan asked for. It only proves nobody typed the owner's home
// network into the source, which is worth exactly one line and not one more: a
// hardcoded 10.0.0.1 sails straight past it. The tests below are the real ones.
func TestResolverNeIzKonstanty(t *testing.T) {
	ish, err := os.ReadFile("sistema.go")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ish, []byte("192.168.")) {
		t.Fatal("адрес домашней сети вбит в исходник")
	}
}

func adr(s string) netip.Addr {
	a, err := netip.ParseAddr(s)
	if err != nil {
		panic(err)
	}
	return a
}

// A believable set: one physical adapter, one loopback, one adapter that is up but
// leads nowhere, and a tunnel that looks better than everything else on paper.
func obraztsy() []Adapter {
	return []Adapter{
		{Indeks: 1, Imya: "Loopback", Tip: tipPetli, Sostoyanie: sostoyanieVverh,
			Shlyuzy: []netip.Addr{adr("127.0.0.1")}, Metrika: 0, Umolchanie: true},
		{Indeks: 5, Imya: "Ethernet", Sostoyanie: sostoyanieVverh,
			Shlyuzy: []netip.Addr{adr("10.7.0.1")}, Resolvery: []netip.Addr{adr("10.7.0.1")},
			Metrika: 15, Umolchanie: true},
		{Indeks: 7, Imya: "Wi-Fi", Sostoyanie: sostoyanieVverh, Metrika: 3},
		{Indeks: 9, Imya: "Ethernet 2", Sostoyanie: 2,
			Shlyuzy: []netip.Addr{adr("10.9.0.1")}, Metrika: 1, Umolchanie: true},
		{Indeks: 10, Imya: "tun0", Opisanie: "sing-tun Tunnel", Sostoyanie: sostoyanieVverh,
			Shlyuzy: []netip.Addr{adr("172.19.0.2")}, Metrika: 0, Umolchanie: true},
	}
}

// chuzhoyVPN это Radmin VPN из жалобы 21.09.2026: поднят, не петля, шлюз своей
// частной сети есть, метрика лучше всех, маршрута наружу нет и DNS нет.
func chuzhoyVPN() Adapter {
	return Adapter{Indeks: 21, Imya: "Radmin VPN", Opisanie: "Radmin VPN Ethernet Adapter",
		Sostoyanie: sostoyanieVverh, Adresa: []netip.Addr{adr("26.13.0.7")},
		Shlyuzy: []netip.Addr{adr("26.0.0.1")}, Metrika: 1}
}

func TestVybratBeretMenshuyuMetriku(t *testing.T) {
	a, err := vybrat(obraztsy())
	if err != nil {
		t.Fatal(err)
	}
	// tun0 wins on metric, and that is CORRECT for this function: it answers
	// "where does traffic go now", not "what is the physical uplink". Wave 2.3
	// knows the tunnel index and excludes it explicitly.
	if a.Imya != "tun0" {
		t.Fatalf("выбран %q, ожидался tun0 по метрике 0", a.Imya)
	}
}

func TestVybratKromeIsklyuchaetTunnel(t *testing.T) {
	a, err := vybrat(obraztsy(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if a.Imya != "Ethernet" {
		t.Fatalf("выбран %q, ожидался Ethernet после исключения туннеля", a.Imya)
	}
}

func TestVybratIgnoriruetPetlyuBezShlyuzaIVyklyuchennye(t *testing.T) {
	// Loopback has a gateway and metric 0, so only the type check keeps it out.
	// Wi-Fi is up with a good metric and no gateway. Ethernet 2 has the best
	// metric of all and is down. Each one is a different way to be wrong.
	spisok := []Adapter{
		obraztsy()[0], obraztsy()[2], obraztsy()[3], obraztsy()[1],
	}
	a, err := vybrat(spisok)
	if err != nil {
		t.Fatal(err)
	}
	if a.Imya != "Ethernet" {
		t.Fatalf("выбран %q, ожидался единственный годный Ethernet", a.Imya)
	}
}

func TestVybratBezKandidatovOshibka(t *testing.T) {
	_, err := vybrat([]Adapter{obraztsy()[0], obraztsy()[2]})
	if err == nil {
		t.Fatal("пустой выбор обязан быть ошибкой, а не нулевым адаптером")
	}
}

// The seam that makes the three exported functions testable without a network.
func TestShlyuzIResolverCherezShov(t *testing.T) {
	prezhniy := perechislit
	perechislit = func() ([]Adapter, error) { return obraztsy(), nil }
	defer func() { perechislit = prezhniy }()

	g, err := ShlyuzKrome(10)
	if err != nil {
		t.Fatal(err)
	}
	if g.String() != "10.7.0.1" {
		t.Fatalf("шлюз %v, ожидался 10.7.0.1", g)
	}

	r, err := LokalnyyResolverKrome(10)
	if err != nil {
		t.Fatal(err)
	}
	if r.String() != "10.7.0.1" {
		t.Fatalf("резолвер %v, ожидался 10.7.0.1", r)
	}
}

func TestResolverOtsutstvuetEtoOshibka(t *testing.T) {
	// An adapter can carry a default route and still list no DNS server. Returning
	// a zero Addr here would put "invalid Addr" into a core config, and sing-box
	// would refuse to start with a message about something else entirely.
	prezhniy := perechislit
	perechislit = func() ([]Adapter, error) {
		return []Adapter{{Indeks: 5, Imya: "Ethernet", Sostoyanie: sostoyanieVverh,
			Shlyuzy: []netip.Addr{adr("10.7.0.1")}, Metrika: 15, Umolchanie: true}}, nil
	}
	defer func() { perechislit = prezhniy }()

	if _, err := LokalnyyResolver(); err == nil {
		t.Fatal("адаптер без резолвера обязан давать ошибку")
	}
}

// Жалоба 21.09.2026. Чужой адаптер выигрывал метрикой, не имел ни выхода
// наружу, ни DNS, и подъём туннеля падал на каждой попытке.
func TestChuzhoyVPNNeBeryotsyaZaKanal(t *testing.T) {
	spisok := append(obraztsy(), chuzhoyVPN())
	a, err := vybrat(spisok, 10)
	if err != nil {
		t.Fatal(err)
	}
	if a.Imya != "Ethernet" {
		t.Fatalf("выбран %q, ожидался Ethernet: у чужого VPN нет маршрута по умолчанию", a.Imya)
	}
}

func TestResolverBeryotsyaUSleduyushchegoKandidata(t *testing.T) {
	// Чужой адаптер здесь НЕСЁТ маршрут по умолчанию (так бывает у корпоративных
	// клиентов) и всё равно не объявляет DNS. Прежде это был отказ на весь
	// подъём; теперь берётся резолвер следующего кандидата.
	bez := chuzhoyVPN()
	bez.Umolchanie = true
	prezhniy := perechislit
	perechislit = func() ([]Adapter, error) { return append(obraztsy(), bez), nil }
	defer func() { perechislit = prezhniy }()

	r, err := LokalnyyResolverKrome(10)
	if err != nil {
		t.Fatal(err)
	}
	if r.String() != "10.7.0.1" {
		t.Fatalf("резолвер %v, ожидался 10.7.0.1 от Ethernet", r)
	}
}

func TestOshibkaResolveraNazyvaetVsehKandidatov(t *testing.T) {
	bez := chuzhoyVPN()
	bez.Umolchanie = true
	prezhniy := perechislit
	perechislit = func() ([]Adapter, error) { return []Adapter{bez}, nil }
	defer func() { perechislit = prezhniy }()

	_, err := LokalnyyResolver()
	if err == nil {
		t.Fatal("без резолвера у всех кандидатов обязан быть отказ")
	}
	if !errors.Is(err, ErrNetResolvera) || !strings.Contains(err.Error(), "Radmin VPN") {
		t.Fatalf("отказ %v не называет кандидата", err)
	}
}

// Windows складывает метрику маршрута с метрикой интерфейса, и продукт обязан
// складывать их так же: иначе адаптер с лучшей метрикой интерфейса и худшим
// маршрутом побеждает там, где система выбрала бы другой.
func TestVybratSkladyvaetMetrikiMarshrutaIIntefeysa(t *testing.T) {
	spisok := []Adapter{
		{Indeks: 5, Imya: "Ethernet", Sostoyanie: sostoyanieVverh, Umolchanie: true,
			Shlyuzy: []netip.Addr{adr("10.7.0.1")}, Metrika: 25, MetrikaMarshruta: 0},
		{Indeks: 7, Imya: "Wi-Fi", Sostoyanie: sostoyanieVverh, Umolchanie: true,
			Shlyuzy: []netip.Addr{adr("10.8.0.1")}, Metrika: 20, MetrikaMarshruta: 100},
	}
	a, err := vybrat(spisok)
	if err != nil {
		t.Fatal(err)
	}
	if a.Imya != "Ethernet" {
		t.Fatalf("выбран %q: 25+0 меньше, чем 20+100", a.Imya)
	}
}

func TestVesAdapteraNePerepolnyaetsya(t *testing.T) {
	// Метрика 0xFFFFFFFF у обоих полей это «система считает путь негодным».
	// В uint32 их сумма равна 4294967294 минус переполнение, то есть почти ноль,
	// и негодный адаптер становился бы лучшим.
	plohoy := Adapter{Indeks: 5, Imya: "Негодный", Sostoyanie: sostoyanieVverh, Umolchanie: true,
		Shlyuzy: []netip.Addr{adr("10.7.0.1")}, Metrika: 0xFFFFFFFF, MetrikaMarshruta: 0xFFFFFFFF}
	horoshiy := Adapter{Indeks: 7, Imya: "Ethernet", Sostoyanie: sostoyanieVverh, Umolchanie: true,
		Shlyuzy: []netip.Addr{adr("10.8.0.1")}, Metrika: 35}
	a, err := vybrat([]Adapter{plohoy, horoshiy})
	if err != nil {
		t.Fatal(err)
	}
	if a.Imya != "Ethernet" {
		t.Fatalf("выбран %q, ожидался Ethernet", a.Imya)
	}
}

func TestPometitUmolchanieOtkatPriOtkazeTablitsy(t *testing.T) {
	// Без запасного пути отказ таблицы маршрутов означал бы ноль кандидатов,
	// то есть починку одной жалобы ценой полной неработоспособности.
	a := Adapter{Indeks: 5, Shlyuzy: []netip.Addr{adr("10.7.0.1")}}
	pometitUmolchanie(&a, nil, errors.New("таблица недоступна"))
	if !a.Umolchanie {
		t.Fatal("при отказе таблицы шлюз обязан снова считаться признаком выхода")
	}

	b := Adapter{Indeks: 5, Shlyuzy: []netip.Addr{adr("10.7.0.1")}}
	pometitUmolchanie(&b, map[uint32]uint32{}, nil)
	if b.Umolchanie {
		t.Fatal("таблица прочитана и умолчания нет: шлюз не делает адаптер кандидатом")
	}

	c := Adapter{Indeks: 5, Shlyuzy: []netip.Addr{adr("10.7.0.1")}}
	pometitUmolchanie(&c, map[uint32]uint32{5: 256}, nil)
	if !c.Umolchanie || c.MetrikaMarshruta != 256 {
		t.Fatalf("метрика маршрута не перенесена: %+v", c)
	}
}
