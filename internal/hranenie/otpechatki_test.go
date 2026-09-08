package hranenie

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSveritLovitPodmenu(t *testing.T) {
	// The project has no certificate and does not need one for a single machine.
	// What it needs is to notice that the binary it is about to run is not the
	// binary that was installed.
	prog := t.TempDir()
	dannye := t.TempDir()
	yadro := filepath.Join(prog, "sing-box.exe")
	if err := os.WriteFile(yadro, []byte("настоящее ядро"), 0o600); err != nil {
		t.Fatalf("файл не создан: %v", err)
	}

	if err := snyatV(prog, dannye); err != nil {
		t.Fatalf("отпечатки не сняты: %v", err)
	}
	if err := sveritV(dannye, yadro); err != nil {
		t.Fatalf("свой же файл не признан: %v", err)
	}

	// One byte is enough. A check that only catches wholesale replacement is a
	// check that catches nothing interesting.
	if err := os.WriteFile(yadro, []byte("настоящее ядрО"), 0o600); err != nil {
		t.Fatalf("файл не переписан: %v", err)
	}
	if err := sveritV(dannye, yadro); err == nil {
		t.Fatal("подмена не замечена")
	}
}

func TestSveritOtvergaetNeznakomyyFayl(t *testing.T) {
	// A file nobody measured is not "probably fine". It is a file that appeared
	// in the program directory after installation, which is the whole scenario.
	prog := t.TempDir()
	dannye := t.TempDir()
	if err := os.WriteFile(filepath.Join(prog, "sing-box.exe"), []byte("ядро"), 0o600); err != nil {
		t.Fatalf("файл не создан: %v", err)
	}
	if err := snyatV(prog, dannye); err != nil {
		t.Fatalf("отпечатки не сняты: %v", err)
	}
	chuzhoy := filepath.Join(prog, "chuzhoy.exe")
	if err := os.WriteFile(chuzhoy, []byte("не наше"), 0o600); err != nil {
		t.Fatalf("файл не создан: %v", err)
	}
	err := sveritV(dannye, chuzhoy)
	if err == nil {
		t.Fatal("незнакомый файл прошёл сверку")
	}
	if !strings.Contains(err.Error(), "нет в отпечатках") {
		t.Fatalf("причина отказа невнятная: %v", err)
	}
}

func TestBezOtpechatkovSverkaNeMolchit(t *testing.T) {
	// No fingerprint file means the install never ran or somebody deleted it.
	// Passing silently here would make the whole mechanism decorative.
	if err := sveritV(t.TempDir(), filepath.Join(t.TempDir(), "sing-box.exe")); err == nil {
		t.Fatal("сверка без файла отпечатков прошла молча")
	}
}
