package hranenie

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
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
	err := sveritV(dannye, yadro)
	if err == nil {
		t.Fatal("подмена не замечена")
	}
	if !errors.Is(err, ErrFaylIzmenen) || errors.Is(err, ErrFaylPropal) {
		t.Fatalf("подмена должна быть ErrFaylIzmenen, получено: %v", err)
	}
}

// Ядро, унесённое антивирусом в карантин, это пропавший файл, и служба
// отличает его от подмены и от прочих отказов только по этой ошибке.
func TestSveritOtlichaetPropavshiyFayl(t *testing.T) {
	prog := t.TempDir()
	dannye := t.TempDir()
	yadro := filepath.Join(prog, "sing-box.exe")
	if err := os.WriteFile(yadro, []byte("ядро"), 0o600); err != nil {
		t.Fatalf("файл не создан: %v", err)
	}
	if err := snyatV(prog, dannye); err != nil {
		t.Fatalf("отпечатки не сняты: %v", err)
	}
	if err := os.Remove(yadro); err != nil {
		t.Fatalf("файл не удалён: %v", err)
	}
	err := sveritV(dannye, yadro)
	if !errors.Is(err, ErrFaylPropal) || errors.Is(err, ErrFaylIzmenen) {
		t.Fatalf("пропавший файл должен быть ErrFaylPropal, получено: %v", err)
	}
	if !strings.Contains(err.Error(), yadro) {
		t.Fatalf("в отказе нет пути к файлу, человеку нечего искать в карантине: %v", err)
	}
}

// Антивирус, который ещё держит файл, отвечает на открытие кодом 225 или 226,
// а не «файла нет»: это тоже пропажа, иначе человек снова видит «Повторить».
func TestAntivirusDerzhitFayl(t *testing.T) {
	for _, kod := range []error{windows.ERROR_VIRUS_INFECTED, windows.ERROR_VIRUS_DELETED, os.ErrNotExist} {
		err := &os.PathError{Op: "open", Path: "sing-box.exe", Err: kod}
		if !faylaNet(fmt.Errorf("файл sing-box.exe не открывается: %w", err)) {
			t.Errorf("%v не принят за пропажу", kod)
		}
	}
	if faylaNet(&os.PathError{Op: "open", Path: "sing-box.exe", Err: windows.ERROR_ACCESS_DENIED}) {
		t.Error("отказ в доступе принят за пропажу: его причина не обязательно антивирус")
	}
}

// Пропавший файл ОТПЕЧАТКОВ это не пропавшее ядро: карантин тут ни при чём, и
// текст про антивирус увёл бы человека не туда.
func TestPropavshieOtpechatkiNeYadro(t *testing.T) {
	err := sveritV(t.TempDir(), filepath.Join(t.TempDir(), "sing-box.exe"))
	if errors.Is(err, ErrFaylPropal) || errors.Is(err, ErrFaylIzmenen) {
		t.Fatalf("отказ без файла отпечатков принят за пропажу ядра: %v", err)
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
