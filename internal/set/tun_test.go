package set

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestZhdatAdapterNahoditPoImeni(t *testing.T) {
	prezhniy := perechislit
	perechislit = func() ([]Adapter, error) { return obraztsy(), nil }
	defer func() { perechislit = prezhniy }()

	a, err := ZhdatAdapterPolno(context.Background(), "tun0")
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

	a, err := ZhdatAdapterPolno(context.Background(), "sovsem-drugoe-imya")
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
	if _, err := ZhdatAdapterPolno(ctx, "tun0"); err != nil {
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
	_, err := ZhdatAdapterPolno(ctx, "tun0")
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
	if _, err := ZhdatAdapterPolno(ctx, "tun0"); err == nil {
		t.Fatal("выключенный адаптер принят за поднятый туннель")
	}
}
