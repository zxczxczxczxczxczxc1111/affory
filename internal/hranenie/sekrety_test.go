package hranenie_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/hranenie"
)

// pohoronen отвечает, появился ли рядом переименованный блоб.
func pohoronen(t *testing.T, dir string) []string {
	t.Helper()
	zapisi, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var nashli []string
	for _, z := range zapisi {
		if strings.HasPrefix(z.Name(), hranenie.ImyaSekretov+".mertvyy-") {
			nashli = append(nashli, z.Name())
		}
	}
	return nashli
}

func TestKrugovoyReys(t *testing.T) {
	// The only test in редакция 1 checked that a broken encryptor returns an
	// error. An implementation that ALWAYS fails passed it, and `go test ./...`
	// went green. The property that matters is the opposite one: what we wrote
	// comes back.
	sh := hranenie.NovyyV(t.TempDir())
	if err := sh.Sohranit([]byte("sekret")); err != nil {
		t.Fatal(err)
	}
	nazad, err := sh.Zagruzit()
	if err != nil {
		t.Fatal(err)
	}
	if string(nazad) != "sekret" {
		t.Fatalf("вернулось %q", nazad)
	}
}

func TestKrugovoyReysCherezNastoyashchiyDPAPI(t *testing.T) {
	// The round trip above goes through the same process. This one proves the
	// blob on disk is genuinely DPAPI and not, say, base64 with a comment
	// promising encryption: it must not contain the plaintext at all.
	dir := t.TempDir()
	sh := hranenie.NovyyV(dir)
	const tayna = "OCHEN-ZAMETNAYA-STROKA"
	if err := sh.Sohranit([]byte(tayna)); err != nil {
		t.Fatal(err)
	}
	syroe, err := os.ReadFile(filepath.Join(dir, hranenie.ImyaSekretov))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(syroe), tayna) {
		t.Fatal("секрет лежит на диске открытым текстом")
	}
	if len(syroe) == 0 {
		t.Fatal("блоб пуст")
	}
}

func TestOtsutstvieFaylaNeOshibka(t *testing.T) {
	// First run has no blob. Treating that as a failure would show
	// secrets-unreadable to a человек who has simply not added a server yet.
	nazad, err := hranenie.NovyyV(t.TempDir()).Zagruzit()
	if err != nil {
		t.Fatalf("отсутствие файла принято за поломку: %v", err)
	}
	if nazad != nil {
		t.Fatalf("на пустом месте вернулось %q", nazad)
	}
}

func TestVremennyyOtkazNeHoronitBlob(t *testing.T) {
	// The service starts before logon, and CryptUnprotectData is an RPC to LSASS
	// that fails then for reasons other than a dead key. Renaming on the first
	// failure turns a temporary refusal into a verdict.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, hranenie.ImyaSekretov), []byte("blob"), 0o600); err != nil {
		t.Fatal(err)
	}
	popytok := 0
	sh := hranenie.SShovom(dir, func(b []byte) ([]byte, error) {
		popytok++
		if popytok < 3 {
			return nil, errors.New("LSASS ещё не готов")
		}
		return []byte("sekret"), nil
	})
	sh.Spat = func(time.Duration) {}

	nazad, err := sh.Zagruzit()
	if err != nil {
		t.Fatal(err)
	}
	if string(nazad) != "sekret" {
		t.Fatalf("вернулось %q", nazad)
	}
	if popytok < 3 {
		t.Fatalf("попыток %d: повторов не было", popytok)
	}
	if p := pohoronen(t, dir); len(p) != 0 {
		t.Fatalf("блоб похоронен после ВРЕМЕННОГО отказа: %v", p)
	}
}

func TestNastoyashchayaSmertHoronitBlob(t *testing.T) {
	// The mirror case. Without it the rule above could be «never bury», and a
	// permanently unreadable blob would keep the client refusing to start with
	// no way out except deleting files by hand.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, hranenie.ImyaSekretov), []byte("blob"), 0o600); err != nil {
		t.Fatal(err)
	}
	sh := hranenie.SShovom(dir, func(b []byte) ([]byte, error) {
		return nil, errors.New("ключ мёртв")
	})
	sh.Spat = func(time.Duration) {}

	if _, err := sh.Zagruzit(); !errors.Is(err, hranenie.ErrSekretyNechitaemy) {
		t.Fatalf("мёртвый блоб не опознан: %v", err)
	}
	if p := pohoronen(t, dir); len(p) != 1 {
		t.Fatalf("похоронено %v, ожидался ровно один", p)
	}
	// Место обязано освободиться, иначе следующий запуск упрётся в тот же блоб.
	if _, err := os.Stat(filepath.Join(dir, hranenie.ImyaSekretov)); !os.IsNotExist(err) {
		t.Fatal("мёртвый блоб остался на месте")
	}
}

func TestSuffiksSoderzhitVremyaANeTolkoDatu(t *testing.T) {
	// Two deaths in one day with a date-only suffix collide, and the second
	// rename overwrites the first. The survivor is then the WRONG blob, and
	// nothing in the логи says so.
	dir := t.TempDir()
	mig := time.Date(2026, 9, 1, 13, 45, 7, 0, time.UTC)
	for i := 0; i < 2; i++ {
		if err := os.WriteFile(filepath.Join(dir, hranenie.ImyaSekretov), []byte("blob"), 0o600); err != nil {
			t.Fatal(err)
		}
		sh := hranenie.SShovom(dir, func(b []byte) ([]byte, error) {
			return nil, errors.New("ключ мёртв")
		})
		sh.Spat = func(time.Duration) {}
		// Второй отказ в те же сутки, но на минуту позже.
		sh.Chasy = func() time.Time { return mig.Add(time.Duration(i) * time.Minute) }
		if _, err := sh.Zagruzit(); !errors.Is(err, hranenie.ErrSekretyNechitaemy) {
			t.Fatal(err)
		}
	}
	if p := pohoronen(t, dir); len(p) != 2 {
		t.Fatalf("похоронено %v, ожидались два: второй отказ перетёр первый", p)
	}
}

func TestChislOPopytokKonechno(t *testing.T) {
	// A retry loop with no ceiling hangs the service start instead of failing it,
	// and «висит» is the one outcome no screen in §9.1 covers.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, hranenie.ImyaSekretov), []byte("blob"), 0o600); err != nil {
		t.Fatal(err)
	}
	popytok := 0
	sh := hranenie.SShovom(dir, func(b []byte) ([]byte, error) {
		popytok++
		return nil, errors.New("ключ мёртв")
	})
	sh.Spat = func(time.Duration) {}
	if _, err := sh.Zagruzit(); err == nil {
		t.Fatal("ожидался отказ")
	}
	if popytok != hranenie.PopytokRasshifrovki {
		t.Fatalf("попыток %d, ожидалось %d", popytok, hranenie.PopytokRasshifrovki)
	}
}

func TestPauzaNarastaet(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, hranenie.ImyaSekretov), []byte("blob"), 0o600); err != nil {
		t.Fatal(err)
	}
	var pauzy []time.Duration
	sh := hranenie.SShovom(dir, func(b []byte) ([]byte, error) {
		return nil, errors.New("ключ мёртв")
	})
	sh.Spat = func(d time.Duration) { pauzy = append(pauzy, d) }
	_, _ = sh.Zagruzit()
	if len(pauzy) != hranenie.PopytokRasshifrovki-1 {
		t.Fatalf("пауз %d, ожидалось %d", len(pauzy), hranenie.PopytokRasshifrovki-1)
	}
	for i := 1; i < len(pauzy); i++ {
		if pauzy[i] <= pauzy[i-1] {
			t.Fatalf("пауза не нарастает: %v", pauzy)
		}
	}
}

func TestZapisAtomarna(t *testing.T) {
	// Same rule as the state file: a half-written blob fails every later read and
	// looks exactly like a dead key, which is the one diagnosis that buries it.
	dir := t.TempDir()
	sh := hranenie.NovyyV(dir)
	if err := sh.Sohranit([]byte("odin")); err != nil {
		t.Fatal(err)
	}
	if err := sh.Sohranit([]byte("dva")); err != nil {
		t.Fatal(err)
	}
	zapisi, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, z := range zapisi {
		if strings.HasSuffix(z.Name(), ".tmp") {
			t.Fatalf("временный файл остался: %s", z.Name())
		}
	}
	nazad, err := sh.Zagruzit()
	if err != nil || string(nazad) != "dva" {
		t.Fatalf("перезапись не доехала: %q %v", nazad, err)
	}
}

// Guest run 02.09.2026: `subscription set` followed at once by `refresh`
// failed the rename of sekrety.dat.tmp over sekrety.dat with "The process
// cannot access the file": on Windows a file somebody holds open (Defender
// scanning the fresh write, our own reader a moment earlier) cannot be
// replaced, and one refusal there costs the human every server. The rename
// must retry for a short while instead of giving up on the first sharing
// violation.
func TestSohranitPerezhivaetZanyatyyFayl(t *testing.T) {
	dir := t.TempDir()
	s := hranenie.NovyyV(dir)
	put := filepath.Join(dir, hranenie.ImyaSekretov)
	if err := s.Sohranit([]byte("odin")); err != nil {
		t.Fatalf("первая запись: %v", err)
	}

	// Hold the target open the way a scanner does, release after 300 ms.
	f, err := os.Open(put)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(300 * time.Millisecond)
		f.Close()
	}()

	if err := s.Sohranit([]byte("dva")); err != nil {
		t.Fatalf("запись поверх занятого файла упала вместо ожидания: %v", err)
	}
	telo, err := s.Zagruzit()
	if err != nil || string(telo) != "dva" {
		t.Fatalf("после записи прочитано %q, %v", telo, err)
	}
	if _, err := os.Stat(put + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("временный файл остался")
	}
}
