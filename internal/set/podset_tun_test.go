package set

import (
	"net/netip"
	"testing"
)

func bezMarshrutov(t *testing.T, marshruty ...netip.Prefix) {
	t.Helper()
	prezhniy := prefiksyMarshrutov
	prefiksyMarshrutov = func() ([]netip.Prefix, error) { return marshruty, nil }
	t.Cleanup(func() { prefiksyMarshrutov = prezhniy })
}

func TestPodsetTunNaChistoyMashineNeMenyaetsya(t *testing.T) {
	// Главное свойство правки: на машине без Docker и WSL адрес прежний, до
	// байта. Иначе она стоила бы переустановки правил у всех сразу.
	prezhniy := perechislit
	perechislit = func() ([]Adapter, error) { return obraztsy(), nil }
	defer func() { perechislit = prezhniy }()
	bezMarshrutov(t, netip.MustParsePrefix("192.168.0.0/24"))

	p, ushli := SvobodnayaPodsetTun(10)
	if p.String() != "172.19.0.1/30" || ushli {
		t.Fatalf("взята %s (ушли=%v), ожидалась прежняя 172.19.0.1/30", p, ushli)
	}
}

func TestPodsetTunUhoditOtSetiDocker(t *testing.T) {
	// Docker раздаёт из 172.17.0.0/16 … 172.31.0.0/16, и шлюзом такой сети
	// становится ровно 172.19.0.1. Маршрут на неё есть даже когда адаптер лежит.
	prezhniy := perechislit
	perechislit = func() ([]Adapter, error) { return obraztsy(), nil }
	defer func() { perechislit = prezhniy }()
	bezMarshrutov(t, netip.MustParsePrefix("172.19.0.0/16"))

	p, ushli := SvobodnayaPodsetTun(10)
	if !ushli {
		t.Fatal("занятая подсеть не распознана")
	}
	if p.Addr().String() == "172.19.0.1" {
		t.Fatalf("остались в занятой сети: %s", p)
	}
}

func TestPodsetTunUhoditOtChuzhogoAdaptera(t *testing.T) {
	// Адрес чужого адаптера считается занятым и без маршрута на его сеть.
	chuzhoy := Adapter{Indeks: 40, Imya: "vEthernet (WSL)", Sostoyanie: sostoyanieVverh,
		Adresa: []netip.Addr{adr("172.19.0.1")}}
	prezhniy := perechislit
	perechislit = func() ([]Adapter, error) { return append(obraztsy(), chuzhoy), nil }
	defer func() { perechislit = prezhniy }()
	bezMarshrutov(t)

	p, ushli := SvobodnayaPodsetTun(10)
	if !ushli || p.Addr().String() == "172.19.0.1" {
		t.Fatalf("взята %s (ушли=%v) при занятом адресе", p, ushli)
	}
}

func TestPodsetTunNeSchitaetSvoyTunnelZanyavshim(t *testing.T) {
	// Наш собственный туннель от прошлого подъёма исключается по индексу:
	// иначе каждое переподключение уводило бы адрес на новый.
	nash := Adapter{Indeks: 10, Imya: "tun0", Sostoyanie: sostoyanieVverh,
		Adresa: []netip.Addr{adr("172.19.0.1")}}
	prezhniy := perechislit
	perechislit = func() ([]Adapter, error) { return []Adapter{nash}, nil }
	defer func() { perechislit = prezhniy }()
	bezMarshrutov(t)

	p, ushli := SvobodnayaPodsetTun(10)
	if p.String() != "172.19.0.1/30" || ushli {
		t.Fatalf("взята %s (ушли=%v): свой туннель принят за чужой", p, ushli)
	}
}

func TestPodsetTunKogdaZanyatyVse(t *testing.T) {
	// Подняться с риском пересечения лучше, чем не подняться вовсе, но признак
	// обязан сказать правду: по нему пишется строка в журнал.
	prezhniy := perechislit
	perechislit = func() ([]Adapter, error) { return nil, nil }
	defer func() { perechislit = prezhniy }()
	bezMarshrutov(t, netip.MustParsePrefix("0.0.0.0/1"), netip.MustParsePrefix("128.0.0.0/1"))

	p, ushli := SvobodnayaPodsetTun()
	if p != PodsetiTun[0] || !ushli {
		t.Fatalf("взята %s (ушли=%v), ожидался первый кандидат с признаком", p, ushli)
	}
}
