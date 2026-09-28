package main

import (
	"os"
	"path/filepath"
	"testing"
)

// Ошибка проверки в сторону «нет» страшнее обратной: окно отказалось бы
// открываться на машине, где всё работает. Машина, где идут тесты окна,
// рантайм имеет, иначе не открылось бы и само окно, поэтому здесь проверка
// обязана его найти.
func TestWebView2NaydenNaEtoyMashine(t *testing.T) {
	if !webView2Est() {
		t.Fatal("WebView2 не найден на машине, где он стоит: окно отказалось бы открываться у всех")
	}
}

func TestWebView2GodenPoPapke(t *testing.T) {
	koren := t.TempDir()
	papka := func(imya string, sBibliotekoy bool) string {
		p := filepath.Join(koren, imya)
		bib := filepath.Join(p, "EBWebView", "x64")
		if err := os.MkdirAll(bib, 0o700); err != nil {
			t.Fatal(err)
		}
		if sBibliotekoy {
			if err := os.WriteFile(filepath.Join(bib, "EmbeddedBrowserWebView.dll"), []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		return p
	}
	for _, c := range []struct {
		imya         string
		sBibliotekoy bool
		goden        bool
	}{
		{"120.0.2210.91", true, true},
		{"86.0.616.0", true, true},
		{"86.0.615.99", true, false},
		{"85.0.1.0", true, false},
		{"121.0.2277.83", false, false},
		{"EBWebView", true, false},
	} {
		if got := webView2Goden(papka(c.imya, c.sBibliotekoy)); got != c.goden {
			t.Errorf("%s с библиотекой=%v: годен=%v, ждали %v", c.imya, c.sBibliotekoy, got, c.goden)
		}
	}
}

func TestRazborVersiiWebView2(t *testing.T) {
	for _, c := range []struct {
		s  string
		v  [4]int
		ok bool
	}{
		{"120.0.2210.91", [4]int{120, 0, 2210, 91}, true},
		{"120", [4]int{120, 0, 0, 0}, true},
		{"1.2.3.4.5", [4]int{}, false},
		{"", [4]int{}, false},
		{"120.0.x.1", [4]int{}, false},
	} {
		v, ok := razobratVersiyuWebView2(c.s)
		if ok != c.ok || (ok && v != c.v) {
			t.Errorf("%q: %v %v, ждали %v %v", c.s, v, ok, c.v, c.ok)
		}
	}
}
