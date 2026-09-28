package sostoyanie

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func razobratJSON(b []byte) error {
	var v any
	return json.Unmarshal(b, &v)
}

func prochitatStroku(t *testing.T, put string) string {
	t.Helper()
	b, err := os.ReadFile(put)
	if err != nil {
		t.Fatalf("%s не читается: %v", filepath.Base(put), err)
	}
	return string(b)
}

func TestNadyozhnayaZapisDerzhitPrezhnyuyuVersiyu(t *testing.T) {
	put := filepath.Join(t.TempDir(), "f.json")
	for _, telo := range []string{`{"n":1}`, `{"n":2}`} {
		if err := ZapisatNadyozhno(put, []byte(telo), json.Valid); err != nil {
			t.Fatal(err)
		}
	}
	if s := prochitatStroku(t, put); s != `{"n":2}` {
		t.Fatalf("на месте %q", s)
	}
	if s := prochitatStroku(t, put+RasshirenieZapasa); s != `{"n":1}` {
		t.Fatalf("в запасе %q, а нужна прежняя версия", s)
	}
	zapisi, err := os.ReadDir(filepath.Dir(put))
	if err != nil {
		t.Fatal(err)
	}
	for _, z := range zapisi {
		if strings.HasSuffix(z.Name(), ".tmp") {
			t.Fatalf("временный файл остался: %s", z.Name())
		}
	}
}

func TestBityyFaylChitaetsyaIzZapasa(t *testing.T) {
	put := filepath.Join(t.TempDir(), "f.json")
	for _, telo := range []string{`{"n":1}`, `{"n":2}`} {
		if err := ZapisatNadyozhno(put, []byte(telo), json.Valid); err != nil {
			t.Fatal(err)
		}
	}
	// Ровно то, что оставляет пропавшее питание без WRITE_THROUGH: имя на
	// месте, байтов нет.
	if err := os.WriteFile(put, make([]byte, 16), 0o600); err != nil {
		t.Fatal(err)
	}
	telo, izZapasa, err := ProchitatSZapasom(put, razobratJSON)
	if err != nil || !izZapasa || string(telo) != `{"n":1}` {
		t.Fatalf("прочитано %q, из запаса %v, ошибка %v", telo, izZapasa, err)
	}
}

func TestBityyFaylNeZatiraetZapas(t *testing.T) {
	put := filepath.Join(t.TempDir(), "f.json")
	if err := ZapisatNadyozhno(put, []byte(`{"n":1}`), json.Valid); err != nil {
		t.Fatal(err)
	}
	if err := ZapisatNadyozhno(put, []byte(`{"n":2}`), json.Valid); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(put, []byte("мусор"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ZapisatNadyozhno(put, []byte(`{"n":3}`), json.Valid); err != nil {
		t.Fatal(err)
	}
	// Вторую версию испортили на месте, значит годным запасом остаётся первая.
	if s := prochitatStroku(t, put+RasshirenieZapasa); s != `{"n":1}` {
		t.Fatalf("в запасе %q: битая версия затёрла годную", s)
	}
}

func TestUdalyonnyyFaylNeVoskresaetIzZapasa(t *testing.T) {
	put := filepath.Join(t.TempDir(), "f.json")
	for _, telo := range []string{`{"n":1}`, `{"n":2}`} {
		if err := ZapisatNadyozhno(put, []byte(telo), json.Valid); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Remove(put); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ProchitatSZapasom(put, razobratJSON); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("удалённый файл прочитан: %v", err)
	}
	if err := UdalitSZapasom(put); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(put + RasshirenieZapasa); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("запас пережил удаление")
	}
	if err := UdalitSZapasom(put); err != nil {
		t.Fatalf("удаление несуществующего вернуло ошибку: %v", err)
	}
}

// Файл, открытый чужим читателем, заменяется, как только его отпустят.
func TestZapisPerezhivaetZanyatuyuTsel(t *testing.T) {
	put := filepath.Join(t.TempDir(), "f.json")
	if err := ZapisatNadyozhno(put, []byte(`{"n":1}`), json.Valid); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(put)
	if err != nil {
		t.Fatal(err)
	}
	otpustit := time.AfterFunc(300*time.Millisecond, func() { _ = f.Close() })
	defer otpustit.Stop()
	if err := ZapisatNadyozhno(put, []byte(`{"n":2}`), json.Valid); err != nil {
		t.Fatalf("запись поверх занятого файла упала вместо ожидания: %v", err)
	}
	if s := prochitatStroku(t, put); s != `{"n":2}` {
		t.Fatalf("на месте %q", s)
	}
}
