package set

import (
	"net/netip"
	"slices"
	"testing"
)

// Режим «весь трафик» убивал Radmin VPN и Hamachi молча: их сети публичные, в
// список частных не входят, а политика Block режет всё неразрешённое.
func TestChuzhieSetiRazreshayutsyaTolkoPriZhivomAdaptere(t *testing.T) {
	prezhniy := perechislit
	defer func() { perechislit = prezhniy }()

	// Машина без чужих туннелей: дыры в публичных /8 быть не должно.
	perechislit = func() ([]Adapter, error) { return obraztsy(), nil }
	if s := SetiChuzhihTunneley(10); len(s) != 0 {
		t.Fatalf("на машине без чужих туннелей разрешены %v", s)
	}

	// Появился Radmin: его сеть и только она.
	radmin := Adapter{Indeks: 21, Imya: "Radmin VPN", Sostoyanie: sostoyanieVverh,
		Adresa: []netip.Addr{adr("26.13.0.7")}}
	perechislit = func() ([]Adapter, error) { return append(obraztsy(), radmin), nil }
	s := SetiChuzhihTunneley(10)
	if !slices.Contains(s, "26.0.0.0/8") {
		t.Fatalf("сеть Radmin не разрешена: %v", s)
	}
	if slices.Contains(s, "25.0.0.0/8") || slices.Contains(s, "100.64.0.0/10") {
		t.Fatalf("разрешены сети, которых на машине нет: %v", s)
	}
}

func TestChuzhieSetiNeSchitayutNashTunnel(t *testing.T) {
	// Наш туннель мог бы взять адрес из CGNAT, и тогда мы разрешили бы
	// 100.64.0.0/10 сами себе, приняв себя за чужого.
	nash := Adapter{Indeks: 10, Imya: "tun0", Sostoyanie: sostoyanieVverh,
		Adresa: []netip.Addr{adr("100.64.0.1")}}
	prezhniy := perechislit
	perechislit = func() ([]Adapter, error) { return []Adapter{nash}, nil }
	defer func() { perechislit = prezhniy }()

	if s := SetiChuzhihTunneley(10); len(s) != 0 {
		t.Fatalf("наш собственный туннель принят за чужой: %v", s)
	}
}
