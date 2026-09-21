package set

import (
	"context"
	"errors"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"
)

func TestZhdatAdapterNahoditPoImeni(t *testing.T) {
	prezhniy := perechislit
	perechislit = func() ([]Adapter, error) { return obraztsy(), nil }
	defer func() { perechislit = prezhniy }()

	a, err := ZhdatAdapterPolno(context.Background(), "tun0", nil)
	if err != nil {
		t.Fatal(err)
	}
	if a.Indeks != 10 {
		t.Fatalf("индекс %d, ожидался 10: без него нечего исключать при поиске канала под туннелем", a.Indeks)
	}
}

func TestZhdatAdapterNahoditPoOpisaniyu(t *testing.T) {
	// The core may name the adapter differently than we asked. Falling back to the
	// description turns "the tunnel did not come up" into "the tunnel came up
	// under another name", which is a completely different bug report.
	prezhniy := perechislit
	perechislit = func() ([]Adapter, error) { return obraztsy(), nil }
	defer func() { perechislit = prezhniy }()

	a, err := ZhdatAdapterPolno(context.Background(), "sovsem-drugoe-imya", nil)
	if err != nil {
		t.Fatal(err)
	}
	if a.Opisanie != "sing-tun Tunnel" {
		t.Fatalf("найден %q, ожидался адаптер с описанием sing-tun Tunnel", a.Opisanie)
	}
}

func TestZhdatAdapterZhdyotPoyavleniya(t *testing.T) {
	// The whole point of the function: the alias exists only AFTER sing-box has
	// raised it, so the first few polls must come back empty and be survived.
	var vyzovov atomic.Int32
	prezhniy := perechislit
	perechislit = func() ([]Adapter, error) {
		if vyzovov.Add(1) < 3 {
			return []Adapter{obraztsy()[1]}, nil // ещё нет туннеля
		}
		return obraztsy(), nil
	}
	defer func() { perechislit = prezhniy }()

	ctx, otm := context.WithTimeout(context.Background(), 5*time.Second)
	defer otm()
	if _, err := ZhdatAdapterPolno(ctx, "tun0", nil); err != nil {
		t.Fatal(err)
	}
	if vyzovov.Load() < 3 {
		t.Fatalf("опросов %d, ожидалось не меньше трёх", vyzovov.Load())
	}
}

func TestZhdatAdapterSdayotsyaPoSroku(t *testing.T) {
	prezhniy := perechislit
	perechislit = func() ([]Adapter, error) { return []Adapter{obraztsy()[1]}, nil }
	defer func() { perechislit = prezhniy }()

	ctx, otm := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer otm()
	_, err := ZhdatAdapterPolno(ctx, "tun0", nil)
	if !errors.Is(err, ErrAdapterNePoyavilsya) {
		t.Fatalf("ошибка %v, ожидалась ErrAdapterNePoyavilsya", err)
	}
	// Причина срока должна доезжать до зовущего: "не появился за отведённое
	// время" и "отменили руками" это разные новости для человека у экрана.
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("причина потеряна: %v", err)
	}
}

func TestZhdatAdapterIgnoriruetPodnyatyyNoVyklyuchennyy(t *testing.T) {
	prezhniy := perechislit
	perechislit = func() ([]Adapter, error) {
		a := obraztsy()[4]
		a.Sostoyanie = 2 // адаптер есть, но лежит
		return []Adapter{a}, nil
	}
	defer func() { perechislit = prezhniy }()

	ctx, otm := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer otm()
	if _, err := ZhdatAdapterPolno(ctx, "tun0", nil); err == nil {
		t.Fatal("выключенный адаптер принят за поднятый туннель")
	}
}

// chuzhoyTunnel это другой клиент на том же ядре: имя tun0 и адрес 172.19.0.1
// это УМОЛЧАНИЯ sing-box, а "wintun" в описании несёт всякий туннель на этом
// драйвере. Отличить его от нашего можно только по времени появления.
func chuzhoyTunnel() Adapter {
	return Adapter{Indeks: 30, Imya: "tun0", Opisanie: "WireGuard Tunnel #1 (wintun)",
		Sostoyanie: sostoyanieVverh, Adresa: []netip.Addr{adr("172.19.0.1")}}
}

func TestChuzhoyTunnelPodnyatyyRansheNeBeryotsyaZaSvoy(t *testing.T) {
	chuzhoy := chuzhoyTunnel()
	prezhniy := perechislit
	perechislit = func() ([]Adapter, error) { return []Adapter{obraztsy()[1], chuzhoy}, nil }
	defer func() { perechislit = prezhniy }()

	// Снимок снят ДО старта нашего ядра: чужой туннель в нём уже есть.
	bylo := SnimokAdapterov{chuzhoy.Indeks: true, 5: true}
	ctx, otm := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer otm()
	if _, err := ZhdatAdapterPolno(ctx, "tun0", bylo); err == nil {
		t.Fatal("чужой туннель принят за наш: его адрес получил бы правила брандмауэра")
	}

	// А наш, появившийся после снимка, берётся, хотя чужой лежит рядом и носит
	// то же имя.
	nash := obraztsy()[4]
	perechislit = func() ([]Adapter, error) { return []Adapter{obraztsy()[1], chuzhoy, nash}, nil }
	a, err := ZhdatAdapterPolno(context.Background(), "tun0", bylo)
	if err != nil {
		t.Fatal(err)
	}
	if a.Indeks != nash.Indeks {
		t.Fatalf("взят индекс %d, ожидался наш %d", a.Indeks, nash.Indeks)
	}
}

func TestSnimokBeryotTolkoPodnyatye(t *testing.T) {
	// Windows переиспользует индекс интерфейса. Наш вчерашний tun0, оставшийся
	// в системе выключенным, попав в снимок, занял бы собой место сегодняшнего.
	prezhniy := perechislit
	perechislit = func() ([]Adapter, error) {
		lezhit := obraztsy()[4]
		lezhit.Sostoyanie = 2
		return []Adapter{obraztsy()[1], lezhit}, nil
	}
	defer func() { perechislit = prezhniy }()

	s, err := SnyatSnimok()
	if err != nil {
		t.Fatal(err)
	}
	if s[10] {
		t.Fatal("выключенный адаптер попал в снимок")
	}
	if !s[5] {
		t.Fatal("поднятый адаптер в снимок не попал")
	}
}

func TestZhdatIscheznoveniyaPropuskaetKogdaAdapteraNet(t *testing.T) {
	prezhniy := perechislit
	perechislit = func() ([]Adapter, error) { return []Adapter{obraztsy()[1]}, nil }
	defer func() { perechislit = prezhniy }()

	nachalo := time.Now()
	ctx, otm := context.WithTimeout(context.Background(), 3*time.Second)
	defer otm()
	ZhdatIscheznoveniya(ctx, "tun0")
	if time.Since(nachalo) > time.Second {
		t.Fatal("ждали ухода адаптера, которого нет")
	}
}

func TestZhdatIscheznoveniyaDozhidaetsyaUhoda(t *testing.T) {
	// Гонка переподключения: свой туннель ещё в системе, снимок снимать рано.
	var vyzovov atomic.Int32
	prezhniy := perechislit
	perechislit = func() ([]Adapter, error) {
		if vyzovov.Add(1) < 3 {
			return obraztsy(), nil // tun0 ещё жив
		}
		return []Adapter{obraztsy()[1]}, nil
	}
	defer func() { perechislit = prezhniy }()

	ctx, otm := context.WithTimeout(context.Background(), 3*time.Second)
	defer otm()
	ZhdatIscheznoveniya(ctx, "tun0")
	if vyzovov.Load() < 3 {
		t.Fatalf("опросов %d, ожидалось не меньше трёх", vyzovov.Load())
	}
}

func TestZhdatIscheznoveniyaSdayotsyaPoSroku(t *testing.T) {
	// Чужой туннель с тем же именем не уйдёт никогда, и ждать его незачем.
	prezhniy := perechislit
	perechislit = func() ([]Adapter, error) { return obraztsy(), nil }
	defer func() { perechislit = prezhniy }()

	ctx, otm := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer otm()
	nachalo := time.Now()
	ZhdatIscheznoveniya(ctx, "tun0")
	if proshlo := time.Since(nachalo); proshlo > 2*time.Second {
		t.Fatalf("ждали %v вместо срока контекста", proshlo)
	}
}
