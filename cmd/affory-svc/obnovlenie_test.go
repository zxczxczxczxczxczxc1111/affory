package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/obnovlenie"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Задача 6.5. Служба принимает архив, который уже лежит на диске, сверяет
// sha256 и запускает подменщика. Сам подменщик проверен в internal/obnovlenie.

func arhivSborki(t *testing.T, fayly map[string]string, pravilnyyHesh bool) string {
	t.Helper()
	d := t.TempDir()
	put := filepath.Join(d, "affory.zip")
	f, err := os.Create(put)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	for imya, telo := range fayly {
		z, _ := w.Create(imya)
		z.Write([]byte(telo))
	}
	w.Close()
	f.Close()
	b, _ := os.ReadFile(put)
	h := sha256.Sum256(b)
	hesh := hex.EncodeToString(h[:])
	if !pravilnyyHesh {
		hesh = "00" + hesh[2:]
	}
	os.WriteFile(put+".sha256", []byte(hesh+"  affory.zip\n"), 0o600)
	return put
}

func TestInstallUpdateOtvergaetChuzhoyHesh(t *testing.T) {
	s := podstavnaya(t, nil)
	zapuskov := 0
	s.zapustitPodmenshchika = func(prog, novaya string) error { zapuskov++; return nil }
	put := arhivSborki(t, map[string]string{"affory-svc.exe": "n"}, false)
	telo, _ := json.Marshal(map[string]string{"path": put})
	o := s.Obrabotat(ctxAdmina(), protokol.Kadr{Tip: "cmd", Id: 1, Imya: "installUpdate", Telo: telo})
	if o.Oshib == nil || o.Oshib.Kod != protokol.KodArhivNegoden {
		t.Fatalf("архив с чужим хешем принят: %+v", o)
	}
	if zapuskov != 0 {
		t.Fatal("подменщик запущен для негодного архива")
	}
}

func TestInstallUpdateBezPutiOtkazyvaet(t *testing.T) {
	s := podstavnaya(t, nil)
	o := s.Obrabotat(ctxAdmina(), protokol.Kadr{Tip: "cmd", Id: 1, Imya: "installUpdate"})
	if o.Oshib == nil || o.Oshib.Kod != protokol.KodArhivNegoden {
		t.Fatalf("пустое тело принято: %+v", o)
	}
}

func TestInstallUpdateTrebuetAdmina(t *testing.T) {
	// Подмена файлов в Program Files это расширение области поражения: без
	// прав любой процесс от пользователя подложил бы свою службу.
	s := podstavnaya(t, nil)
	put := arhivSborki(t, map[string]string{"affory-svc.exe": "n"}, true)
	telo, _ := json.Marshal(map[string]string{"path": put})
	o := s.Obrabotat(t.Context(), protokol.Kadr{Tip: "cmd", Id: 1, Imya: "installUpdate", Telo: telo})
	if o.Oshib == nil || o.Oshib.Kod != protokol.KodTrebuetsyaAdmin {
		t.Fatalf("без прав: %+v", o)
	}
}

func TestStatusPokazyvaetIshodObnovleniyaIZabiraetFayl(t *testing.T) {
	// Исход пишет подменщик ПОСЛЕ того, как новая служба уже поднялась и
	// ответила ему, поэтому читать файл на старте поздно: он появляется через
	// секунду после старта. Читается при status, файл забирается один раз.
	s := podstavnaya(t, nil)
	s.dirDannyh = t.TempDir()
	if err := obnovlenie.ZapisatItog(s.dirDannyh, obnovlenie.Itog{Kod: obnovlenie.KodOtkat, Tekst: "новая служба молчит"}); err != nil {
		t.Fatal(err)
	}
	o := s.Obrabotat(t.Context(), protokol.Kadr{Tip: "cmd", Id: 1, Imya: "status"})
	var st protokol.StatusOtvet
	if err := json.Unmarshal(o.Telo, &st); err != nil {
		t.Fatal(err)
	}
	if st.Oshib == nil || st.Oshib.Kod != obnovlenie.KodOtkat {
		t.Fatalf("статус не показал откат: %+v", st.Oshib)
	}
	if _, est, _ := obnovlenie.ProchitatItog(s.dirDannyh); est {
		t.Fatal("файл исхода пережил показ")
	}
}
