package zhurnaly

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFaylPadeniyaPovorachivaetProshloe(t *testing.T) {
	katalog := t.TempDir()

	// Первый запуск: падений не было.
	f, proshloe, err := OtkrytFaylPadeniya(katalog, "padenie.log")
	if err != nil {
		t.Fatal(err)
	}
	if !proshloe.IsZero() {
		t.Fatalf("падение названо там, где его не было: %v", proshloe)
	}
	if _, err := f.WriteString("fatal error: concurrent map writes\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()

	// Второй запуск после падения: стек уехал в .1, время названо.
	f, proshloe, err = OtkrytFaylPadeniya(katalog, "padenie.log")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	if proshloe.IsZero() {
		t.Fatal("прошлое падение не названо")
	}
	stek, err := os.ReadFile(filepath.Join(katalog, "padenie.log.1"))
	if err != nil || string(stek) != "fatal error: concurrent map writes\n" {
		t.Fatalf("стек прошлого падения: %q, %v", stek, err)
	}
	if st, _ := os.Stat(filepath.Join(katalog, "padenie.log")); st.Size() != 0 {
		t.Fatal("текущий файл падений не пуст после поворота")
	}

	// Третий запуск без падения: молчит, .1 цел.
	f, proshloe, err = OtkrytFaylPadeniya(katalog, "padenie.log")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	if !proshloe.IsZero() {
		t.Fatal("пустой файл принят за падение")
	}
	if _, err := os.Stat(filepath.Join(katalog, "padenie.log.1")); err != nil {
		t.Fatalf("стек прошлого падения пропал: %v", err)
	}
}
