package obnovlenie

import (
	"os"
	"path/filepath"
	"testing"
)

// Окно, приславшее installUpdate, работает во время подмены. Файл, который
// нельзя переименовать поверх, отодвигается в сторону; хвост .ubrat не должен
// пережить успешную подмену и не должен мешать следующей.
func TestPodmenaNeOstavlyaetHvostovIOtodvigaetStaryy(t *testing.T) {
	p, prog := podmena(t)
	// Хвост от прошлого захода уже лежит: не должен ломать подмену.
	os.WriteFile(filepath.Join(prog, "affory-ui.exe.ubrat"), []byte("musor"), 0o600)
	if itog := p.Vypolnit(); !itog.Ok {
		t.Fatalf("подмена не удалась: %+v", itog)
	}
	z, _ := os.ReadDir(prog)
	for _, e := range z {
		if filepath.Ext(e.Name()) == ".ubrat" || filepath.Ext(e.Name()) == ".chast" {
			t.Fatalf("после подмены остался хвост %s", e.Name())
		}
	}
	if soderzhimoe(t, filepath.Join(prog, "affory-ui.exe")) != "novaya-ui" {
		t.Fatal("интерфейс не подменён")
	}
}
