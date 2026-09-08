package set

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestPutNormalizuetsya(t *testing.T) {
	// Four spellings of the same file, one rule. Anything else is a rule that
	// works on the machine where it was written and nowhere else: process_path
	// is compared as a string by the core.
	dir := t.TempDir()
	put := filepath.Join(dir, "Program Files Test", "Steam.exe")
	if err := os.MkdirAll(filepath.Dir(put), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(put, []byte("MZ"), 0o644); err != nil {
		t.Fatal(err)
	}
	etalon, err := NormalizovatPut(put)
	if err != nil {
		t.Fatalf("сам путь не нормализовался: %v", err)
	}
	if strings.HasPrefix(etalon, `\\?\`) {
		t.Fatalf("префикс %s не снят: %s", `\\?\`, etalon)
	}
	if !strings.EqualFold(etalon, put) {
		t.Fatalf("нормализованный %q не тот же файл, что %q", etalon, put)
	}

	napisaniya := map[string]string{
		"строчные":         strings.ToLower(put),
		"прописные":        strings.ToUpper(put),
		"прямые слэши":     strings.ReplaceAll(put, `\`, `/`),
		"короткое имя 8.3": korotkoeImya(t, put),
	}
	for imya, n := range napisaniya {
		got, err := NormalizovatPut(n)
		if err != nil {
			t.Errorf("%s (%q): %v", imya, n, err)
			continue
		}
		if got != etalon {
			t.Errorf("%s: %q нормализовалось в %q, эталон %q", imya, n, got, etalon)
		}
	}
}

func TestNesushchestvuyushchiyPutOtvergaetsya(t *testing.T) {
	// A rule for a file that is not there matches nothing and looks like a
	// working exclusion. The service must know at setRules time, not never.
	if _, err := NormalizovatPut(filepath.Join(t.TempDir(), "net-takogo.exe")); err == nil {
		t.Fatal("путь к несуществующему файлу принят")
	}
}

func TestKatalogEtoNeProtsess(t *testing.T) {
	if _, err := NormalizovatPut(t.TempDir()); err == nil {
		t.Fatal("каталог принят за путь процесса")
	}
}

func korotkoeImya(t *testing.T, put string) string {
	t.Helper()
	p, err := windows.UTF16PtrFromString(put)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n, err := windows.GetShortPathName(p, &buf[0], uint32(len(buf)))
	if err != nil {
		t.Fatal(err)
	}
	return windows.UTF16ToString(buf[:n])
}
