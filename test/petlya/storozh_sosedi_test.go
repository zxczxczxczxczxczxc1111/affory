package petlya

import (
	"os"
	"slices"
	"strings"
	"testing"
)

// Сторож обязан пропускать короткие проверки соседних пакетов и при этом
// видеть всё, ради чего заведён. 26.09.2026 ворота упали на «ядра sing-box на
// машине изменились»: это `sing-box check` из genkonfig, который go test гонял
// одновременно с петлёй.
func TestStorozhOtlichaetSosedeyOtChuzhih(t *testing.T) {
	const svoy = 500
	vse := []processMashiny{
		{Pid: svoy, Roditel: 1, Imya: "petlya.test.exe"},
		{Pid: 600, Roditel: 1, Imya: "genkonfig.test.exe"},
		{Pid: 700, Roditel: 1, Imya: "sotaconnect.exe"},
		{Pid: 800, Roditel: 1, Imya: "affory-svc.exe"},

		{Pid: 11, Roditel: 600, Imya: "sing-box.exe"},  // проверка соседа
		{Pid: 12, Roditel: 700, Imya: "sing-box.exe"},  // личное ядро владельца
		{Pid: 13, Roditel: svoy, Imya: "sing-box.exe"}, // своё, обязано исчезнуть к концу
		{Pid: 14, Roditel: 800, Imya: "sing-box.exe"},  // запущено службой
		{Pid: 15, Roditel: 999, Imya: "sing-box.exe"},  // родитель уже умер
		{Pid: 16, Roditel: 600, Imya: "xray.exe"},      // не ядро sing-box
	}
	pidy := otobratYadra(vse, svoy)
	slices.Sort(pidy)
	if want := []int{12, 13, 14, 15}; !slices.Equal(pidy, want) {
		t.Fatalf("сторож следит за %v, а должен за %v", pidy, want)
	}
}

// Отбор держится на двух фактах живой системы: снимок видит процессы, и
// тестовый бинарник в нём зовётся `*.test.exe`. Если хоть один из них неверен,
// сторож молча перестаёт отличать соседей, и проверка выше это не поймает.
func TestSnimokProcessovVidnoSebya(t *testing.T) {
	vse, err := processyMashiny()
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(vse, func(p processMashiny) bool { return p.Pid == os.Getpid() })
	if i < 0 {
		t.Fatalf("в снимке %d процессов, а себя среди них нет", len(vse))
	}
	if !strings.HasSuffix(vse[i].Imya, ".test.exe") {
		t.Fatalf("тестовый бинарник в снимке зовётся %q, а отбор ждёт *.test.exe", vse[i].Imya)
	}
}
