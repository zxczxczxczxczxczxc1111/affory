package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/set"
)

// Задача 6.4. getServerHealth отдаёт снимок состояния сервера, который лежит
// рядом с подпиской. Адрес выводится из адреса подписки, наружу не отдаётся.

func sPodpiskoy(t *testing.T, podpiska string) *Sluzhba {
	t.Helper()
	s := podstavnaya(t, nil)
	n, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}
	n.Podpiska = podpiska
	if err := s.zapisatNabor(n); err != nil {
		t.Fatal(err)
	}
	return s
}

type zdorovieOtvet struct {
	Vremya   int64    `json:"vremya"`
	VozrastS int64    `json:"vozrast_s"`
	Ustarel  bool     `json:"ustarel"`
	Trevogi  []string `json:"trevogi"`
	DneySert *int     `json:"dney_do_konca_sertifikata_maski"`
	DneyHy2  *int     `json:"dney_do_konca_sertifikata_hy2"`
	XrayVer  string   `json:"xray_versiya"`
}

func TestGetServerHealthVyvoditAdresIzPodpiskiISchitaetVozrast(t *testing.T) {
	s := sPodpiskoy(t, "https://primer.example/0123456789abcdef0123456789abcdef")
	seychas := time.Unix(1756900000+7200, 0)
	s.seychas = func() time.Time { return seychas }
	var sprosili string
	dney := 5
	dneyHy2 := 6
	s.zagruzitSnimok = func(ctx context.Context, adres string) (set.Snimok, error) {
		sprosili = adres
		return set.Snimok{Vremya: 1756900000, Trevogi: []string{"сертификат маски истекает"}, DneySertifikata: &dney, DneySertifikataHy2: &dneyHy2, XrayVersiya: "25.1.1"}, nil
	}
	o := s.Obrabotat(t.Context(), protokol.Kadr{Tip: "cmd", Id: 1, Imya: "getServerHealth"})
	if o.Oshib != nil {
		t.Fatalf("отказ: %+v", o.Oshib)
	}
	if sprosili != "https://primer.example/0123456789abcdef0123456789abcdef.sostoyanie.json" {
		t.Fatalf("спросили %q", sprosili)
	}
	var z zdorovieOtvet
	if err := json.Unmarshal(o.Telo, &z); err != nil {
		t.Fatal(err)
	}
	if z.VozrastS != 7200 || z.Ustarel || len(z.Trevogi) != 1 || z.DneySert == nil || *z.DneySert != 5 || z.DneyHy2 == nil || *z.DneyHy2 != 6 || z.XrayVer != "25.1.1" {
		t.Fatalf("ответ %+v", z)
	}
	// Адрес подписки это секрет: в теле ответа его быть не должно.
	if json.Valid(o.Telo) && containsBytes(o.Telo, "primer.example") {
		t.Fatal("адрес подписки утёк в ответ")
	}
}

func TestGetServerHealthPomechaetStaryySnimok(t *testing.T) {
	s := sPodpiskoy(t, "https://primer.example/abc")
	s.seychas = func() time.Time { return time.Unix(1756900000+3*86400, 0) }
	s.zagruzitSnimok = func(ctx context.Context, adres string) (set.Snimok, error) {
		return set.Snimok{Vremya: 1756900000, Trevogi: []string{}}, nil
	}
	o := s.Obrabotat(t.Context(), protokol.Kadr{Tip: "cmd", Id: 1, Imya: "getServerHealth"})
	if o.Oshib != nil {
		t.Fatalf("отказ: %+v", o.Oshib)
	}
	var z zdorovieOtvet
	json.Unmarshal(o.Telo, &z)
	if !z.Ustarel {
		t.Fatal("снимок трёхдневной давности не помечен устаревшим")
	}
}

func TestGetServerHealthBezPodpiskiIBezFaylaOtkazyvaet(t *testing.T) {
	s := podstavnaya(t, nil)
	o := s.Obrabotat(t.Context(), protokol.Kadr{Tip: "cmd", Id: 1, Imya: "getServerHealth"})
	if o.Oshib == nil || o.Oshib.Kod != protokol.KodHealthSnapshotMissing {
		t.Fatalf("без подписки: %+v", o)
	}
	s = sPodpiskoy(t, "https://primer.example/abc")
	s.zagruzitSnimok = func(ctx context.Context, adres string) (set.Snimok, error) { return set.Snimok{}, set.ErrSnimkaNet }
	o = s.Obrabotat(t.Context(), protokol.Kadr{Tip: "cmd", Id: 1, Imya: "getServerHealth"})
	if o.Oshib == nil || o.Oshib.Kod != protokol.KodHealthSnapshotMissing {
		t.Fatalf("без файла: %+v", o)
	}
}

func containsBytes(b []byte, s string) bool {
	return len(s) > 0 && len(b) >= len(s) && string(b) != "" && indexOf(string(b), s) >= 0
}

func indexOf(h, n string) int {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return i
		}
	}
	return -1
}
