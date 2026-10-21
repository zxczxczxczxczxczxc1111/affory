package main

import (
	"bytes"
	"log"
	"strings"
	"testing"
	"unicode/utf8"

	"golang.org/x/sys/windows"
)

func perehvatitZhurnal(t *testing.T) *bytes.Buffer {
	t.Helper()
	var b bytes.Buffer
	bylo := log.Writer()
	log.SetOutput(&b)
	t.Cleanup(func() { log.SetOutput(bylo) })
	return &b
}

// Страница пишет в журнал то, что видел человек: ключ из текста ошибки в
// журнал не попадает, многострочный стек остаётся одной записью.
func TestZhurnalStranicyVychishchaetSsylki(t *testing.T) {
	b := perehvatitZhurnal(t)
	(&most{}).ZapisatVZhurnal("отказ: vless://tayna@198.51.100.7:443#x\n    at App.tsx:10")
	zapis := b.String()
	if strings.Contains(zapis, "tayna") || !strings.Contains(zapis, "страница: отказ: <ссылка>") {
		t.Fatalf("запись %q", zapis)
	}
	if strings.Count(zapis, "\n") != 2 || !strings.Contains(zapis, "\n        at App.tsx:10") {
		t.Fatalf("стек не сдвинут под запись: %q", zapis)
	}
}

func TestPodpisVersiiRazlichaetWindows11(t *testing.T) {
	for _, s := range []struct {
		v    windows.OsVersionInfoEx
		zhdu string
	}{
		{windows.OsVersionInfoEx{MajorVersion: 10, BuildNumber: 26200}, "Affory 1.10.0, Windows 11 (26200)"},
		{windows.OsVersionInfoEx{MajorVersion: 10, BuildNumber: 19045}, "Affory 1.10.0, Windows 10 (19045)"},
	} {
		if got := podpisVersii("1.10.0", &s.v); got != s.zhdu {
			t.Errorf("сборка %d: %q, ждала %q", s.v.BuildNumber, got, s.zhdu)
		}
	}
}

// Стек React бывает в десятки килобайт: запись режется по границе буквы.
func TestZhurnalStranicyObrezaetDlinnoe(t *testing.T) {
	b := perehvatitZhurnal(t)
	(&most{}).ZapisatVZhurnal(strings.Repeat("я", predelStrokiStranicy))
	zapis := b.String()
	if !strings.HasSuffix(zapis, " (обрезано)\n") || !utf8.ValidString(zapis) {
		t.Fatalf("обрезка дала %q…", zapis[len(zapis)-40:])
	}
	if len(zapis) > predelStrokiStranicy+200 {
		t.Fatalf("запись %d байт", len(zapis))
	}
}
