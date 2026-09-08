package petlya_test

import (
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/test/petlya"
)

// Снимок машины обязан быть непустым, иначе сторож охраняет пустоту.
//
// Это не придирка: сторож, который молча ничего не измерил, опаснее его
// отсутствия, потому что по нему перестают проверять. Ровно так 07.09.2026 вёл
// себя разбор пина, читавший не ту отметку времени.
func TestSnimokMashinyNePustoy(t *testing.T) {
	s := petlya.SnyatSostoyanie()
	if s.MarshrutPoUmolchaniyu == "" {
		t.Fatal("маршрут по умолчанию не снят: сторож не увидит, что интернет увели")
	}
	if s.Oshibka != "" {
		t.Fatalf("снимок снят с ошибкой: %s", s.Oshibka)
	}
}

// Сверка обязана ЗАМЕЧАТЬ то, ради чего заведена: чужой процесс ядра, уехавший
// маршрут, включённый системный прокси.
func TestSverkaZamechaetIzmeneniya(t *testing.T) {
	do := petlya.Sostoyanie{
		PidyYadra:             []int{100, 200},
		MarshrutPoUmolchaniyu: "192.0.2.1",
		SistemnyyProksi:       "",
	}
	sluchai := []struct {
		imya  string
		posle petlya.Sostoyanie
		zhdem string
	}{
		{"появилось чужое ядро", petlya.Sostoyanie{PidyYadra: []int{100, 200, 300}, MarshrutPoUmolchaniyu: "192.0.2.1"}, "ядр"},
		{"пропало чужое ядро", petlya.Sostoyanie{PidyYadra: []int{100}, MarshrutPoUmolchaniyu: "192.0.2.1"}, "ядр"},
		{"уехал маршрут", petlya.Sostoyanie{PidyYadra: []int{100, 200}, MarshrutPoUmolchaniyu: "198.51.100.1"}, "маршрут"},
		{"включился прокси", petlya.Sostoyanie{PidyYadra: []int{100, 200}, MarshrutPoUmolchaniyu: "192.0.2.1", SistemnyyProksi: "127.0.0.1:10809"}, "прокси"},
	}
	for _, s := range sluchai {
		t.Run(s.imya, func(t *testing.T) {
			raznica := petlya.Sverit(do, s.posle)
			if raznica == "" {
				t.Fatalf("сторож не заметил: %s", s.imya)
			}
			if !containsRus(raznica, s.zhdem) {
				t.Fatalf("сторож сказал %q, а должен был про %q", raznica, s.zhdem)
			}
		})
	}
}

// Контроль: на одинаковых снимках сторож обязан МОЛЧАТЬ, иначе он не сторож, а
// генератор ложных тревог, и его выключат первым же спокойным днём.
func TestSverkaMolchitNaOdinakovom(t *testing.T) {
	s := petlya.Sostoyanie{PidyYadra: []int{100, 200}, MarshrutPoUmolchaniyu: "192.0.2.1"}
	if raznica := petlya.Sverit(s, s); raznica != "" {
		t.Fatalf("сторож поднял тревогу на пустом месте: %s", raznica)
	}
	// Порядок PID приходит от системы и не гарантирован.
	drugoyPoryadok := petlya.Sostoyanie{PidyYadra: []int{200, 100}, MarshrutPoUmolchaniyu: "192.0.2.1"}
	if raznica := petlya.Sverit(s, drugoyPoryadok); raznica != "" {
		t.Fatalf("сторож считает разным порядок тех же PID: %s", raznica)
	}
}

func containsRus(s, podstroka string) bool {
	return len(s) >= len(podstroka) && (len(podstroka) == 0 || indexOf(s, podstroka) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
