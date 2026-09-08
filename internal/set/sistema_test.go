package set

import (
	"bytes"
	"net/netip"
	"os"
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
			Shlyuzy: []netip.Addr{adr("127.0.0.1")}, Metrika: 0},
		{Indeks: 5, Imya: "Ethernet", Sostoyanie: sostoyanieVverh,
			Shlyuzy: []netip.Addr{adr("10.7.0.1")}, Resolvery: []netip.Addr{adr("10.7.0.1")}, Metrika: 15},
		{Indeks: 7, Imya: "Wi-Fi", Sostoyanie: sostoyanieVverh, Metrika: 3},
		{Indeks: 9, Imya: "Ethernet 2", Sostoyanie: 2,
			Shlyuzy: []netip.Addr{adr("10.9.0.1")}, Metrika: 1},
		{Indeks: 10, Imya: "tun0", Opisanie: "sing-tun Tunnel", Sostoyanie: sostoyanieVverh,
			Shlyuzy: []netip.Addr{adr("172.19.0.2")}, Metrika: 0},
	}
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
			Shlyuzy: []netip.Addr{adr("10.7.0.1")}, Metrika: 15}}, nil
	}
	defer func() { perechislit = prezhniy }()

	if _, err := LokalnyyResolver(); err == nil {
		t.Fatal("адаптер без резолвера обязан давать ошибку")
	}
}
