package main

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

var vseSostoyaniya = []protokol.Sostoyanie{
	protokol.SostSluzhbaMolchit, protokol.SostVyklyuchen, protokol.SostPodnimaetsya,
	protokol.SostPodnyat, protokol.SostNeNeset, protokol.SostVosstanavl, protokol.SostOtkaz,
}

// Spec §8.4: four icons (connected, connecting, disconnected, error) for
// seven states, every state gets one, and every icon is real bytes that were
// embedded, not a name that resolves to nothing at runtime.
func TestKazhdoeSostoyanieImeetIkonku(t *testing.T) {
	videli := map[string]bool{}
	for _, s := range vseSostoyaniya {
		imya := ikonkaSostoyaniya(s)
		b, est := ikonki[imya]
		if !est || len(b) == 0 {
			t.Fatalf("состояние %s указывает на иконку %q, которой нет", s, imya)
		}
		videli[imya] = true
	}
	for _, imya := range []string{"connected", "connecting", "disconnected", "error"} {
		if !videli[imya] {
			t.Fatalf("иконка %s не назначена ни одному состоянию", imya)
		}
	}
	if len(ikonki) != 4 {
		t.Fatalf("иконок %d, а состояний трея четыре", len(ikonki))
	}
}

// The tray text and the main screen text are the same words: the spec says
// icon answers "alive or not", text answers "what exactly". Two sources of
// those words would drift; the Go map is checked against podpisi.ts.
func TestPodpisiTreyaSovpadayutSEkranom(t *testing.T) {
	put := filepath.Join("frontend", "src", "ekrany", "podpisi.ts")
	tekst, err := os.ReadFile(put)
	if err != nil {
		t.Fatal(err)
	}
	blok := regexp.MustCompile(`(?s)export const podpis[^{]*\{(.*?)\n\};`).FindStringSubmatch(string(tekst))
	if blok == nil {
		t.Fatal("в podpisi.ts не найдена карта podpis")
	}
	re := regexp.MustCompile(`"?([a-z-]+)"?:\s*"([^"]+)"`)
	naEkrane := map[string]string{}
	for _, m := range re.FindAllStringSubmatch(blok[1], -1) {
		naEkrane[m[1]] = m[2]
	}
	if len(naEkrane) != len(vseSostoyaniya) {
		t.Fatalf("на экране %d подписей, состояний %d", len(naEkrane), len(vseSostoyaniya))
	}
	for _, s := range vseSostoyaniya {
		if got := podpisTreya(s); got != naEkrane[string(s)] {
			t.Errorf("%s: трей говорит %q, экран %q", s, got, naEkrane[string(s)])
		}
	}
}
