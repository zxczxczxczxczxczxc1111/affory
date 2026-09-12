package obnovlenie

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Отказ переименования бывает ПРЕХОДЯЩИМ.
//
// 13.09.2026 обновление 1.0.3 на 1.1.0 на живой машине встало на
// `rename affory-ui.exe affory-ui.exe.ubrat: Access is denied`, причём и
// подмена, и следом откат. Запущенный exe тут ни при чём: он переименовывается
// и был проверен опытом на той же машине при живом окне. Файл держал кто-то
// посторонний доли секунды (Defender вырезан, индексатор Windows Search жив и
// просыпается ровно на появление новых exe в каталоге).
//
// Одна попытка на такой отказ превращает минутное обновление в сломанную
// установку, которую человек чинит установщиком руками.
func TestZanyatyyFaylOtodvigaetsyaSoVtoroyPopytki(t *testing.T) {
	p, prog := podmena(t)
	skoryeOtstupy(t)
	otkazov := 3
	zanyat(t, filepath.Join(prog, "affory-ui.exe")+".ubrat", &otkazov)

	if itog := p.Vypolnit(); !itog.Ok {
		t.Fatalf("подмена не пережила преходящий отказ: %+v", itog)
	}
	if soderzhimoe(t, filepath.Join(prog, "affory-ui.exe")) != "novaya-ui" {
		t.Fatal("интерфейс не подменён")
	}
	if otkazov > 0 {
		t.Fatalf("повторов не было: осталось %d заготовленных отказов", otkazov)
	}
}

// Занятый насовсем файл обязан отменить подмену ДО первого изменения.
//
// Иначе выходит худший из исходов: часть файлов новая, часть прежняя, откат
// спотыкается о тот же занятый файл и машина остаётся без рабочей программы.
// Сухая проба стоит одного переименования туда и обратно.
func TestNavsegdaZanyatyyFaylOtmenyaetPodmenuNeTronuvFaylov(t *testing.T) {
	p, prog := podmena(t)
	skoryeOtstupy(t)
	vsegda := -1
	zanyat(t, filepath.Join(prog, "affory-ui.exe")+".proba", &vsegda)

	itog := p.Vypolnit()
	if itog.Ok {
		t.Fatal("подмена прошла, хотя файл занят насовсем")
	}
	if !strings.Contains(itog.Tekst, "affory-ui.exe") {
		t.Fatalf("в отказе не названо, что именно занято: %q", itog.Tekst)
	}
	if soderzhimoe(t, filepath.Join(prog, "affory-svc.exe")) != "staraya" {
		t.Fatal("служба подменена, хотя подмена отменена: каталог остался в полусостоянии")
	}
	z, _ := os.ReadDir(prog)
	for _, e := range z {
		if ext := filepath.Ext(e.Name()); ext == ".chast" || ext == ".ubrat" || ext == ".proba" {
			t.Fatalf("отменённая подмена оставила хвост %s", e.Name())
		}
	}
}

// zanyat подменяет переименование: пока osталось отказов, вызов с этим целевым
// именем отвечает отказом доступа. Отрицательное число значит «навсегда».
func zanyat(t *testing.T, kuda string, ostalos *int) {
	t.Helper()
	prezhnee := pereimenovat
	pereimenovat = func(iz, v string) error {
		if v == kuda && *ostalos != 0 {
			if *ostalos > 0 {
				*ostalos--
			}
			return &os.LinkError{Op: "rename", Old: iz, New: v, Err: errors.New("Access is denied.")}
		}
		return prezhnee(iz, v)
	}
	t.Cleanup(func() { pereimenovat = prezhnee })
}

// skoryeOtstupy убирает ожидание из прогона: проверяется повтор, а не часы.
func skoryeOtstupy(t *testing.T) {
	t.Helper()
	prezhniy := otstupOtodviganiya
	otstupOtodviganiya = time.Millisecond
	t.Cleanup(func() { otstupOtodviganiya = prezhniy })
}
