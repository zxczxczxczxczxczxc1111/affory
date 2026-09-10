package main

import (
	"strings"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Индикатор обновления в трее. Значка поверх иконки нет намеренно: четыре
// иконки отвечают на вопрос «жив или нет», и подмешивать в этот же канал
// вторую новость значит сделать хуже оба ответа. Обновление называется
// словами, в подсказке и отдельной строкой меню.

func TestBezObnovleniyaStrokiNet(t *testing.T) {
	v := vidDlya(protokol.StatusOtvet{Sostoyanie: protokol.SostPodnyat})
	if v.obnovlenie {
		t.Fatalf("обновления нет, а вид говорит, что есть")
	}
	if v.punktObnovleniya != "" {
		t.Fatalf("строка обновления не пуста: %q", v.punktObnovleniya)
	}
	if strings.Contains(v.podskazka, "обновление") {
		t.Fatalf("подсказка говорит про обновление без обновления: %q", v.podskazka)
	}
}

func TestObnovlenieNazyvaetsyaNomerom(t *testing.T) {
	st := protokol.StatusOtvet{
		Sostoyanie: protokol.SostPodnyat,
		Obnovlenie: &protokol.ObnovlenieOtvet{
			Versiya:   "1.0.3",
			Razmer:    12 << 20,
			Provereno: time.Now(),
		},
	}
	v := vidDlya(st)
	if !v.obnovlenie {
		t.Fatalf("обновление есть, а вид молчит")
	}
	// Номер обязателен в обеих строках: «доступно обновление» без версии не
	// даёт человеку решить, нужна ли она ему прямо сейчас.
	if !strings.Contains(v.punktObnovleniya, "1.0.3") {
		t.Fatalf("строка меню без номера версии: %q", v.punktObnovleniya)
	}
	if !strings.Contains(v.podskazka, "1.0.3") {
		t.Fatalf("подсказка без номера версии: %q", v.podskazka)
	}
	// Состояние туннеля из подсказки не вытесняется: обновление это приписка,
	// а не замена основной новости.
	if !strings.Contains(v.podskazka, "подключено") {
		t.Fatalf("подсказка потеряла состояние: %q", v.podskazka)
	}
}

// Вид сравнивается одним ==, и появление обновления обязано это сравнение
// ломать: иначе трей не перерисуется и строка не появится до смены состояния.
func TestObnovlenieMenyaetVid(t *testing.T) {
	bez := vidDlya(protokol.StatusOtvet{Sostoyanie: protokol.SostPodnyat})
	s := vidDlya(protokol.StatusOtvet{
		Sostoyanie: protokol.SostPodnyat,
		Obnovlenie: &protokol.ObnovlenieOtvet{Versiya: "1.0.3"},
	})
	if bez == s {
		t.Fatalf("вид с обновлением и без него неразличим: %+v", bez)
	}
}

// Иконка от обновления не зависит: она отвечает только за здоровье туннеля.
func TestIkonkaNeZavisitOtObnovleniya(t *testing.T) {
	for _, sost := range []protokol.Sostoyanie{
		protokol.SostPodnyat, protokol.SostVyklyuchen,
		protokol.SostOtkaz, protokol.SostPodnimaetsya,
	} {
		bez := vidDlya(protokol.StatusOtvet{Sostoyanie: sost})
		s := vidDlya(protokol.StatusOtvet{
			Sostoyanie: sost,
			Obnovlenie: &protokol.ObnovlenieOtvet{Versiya: "1.0.3"},
		})
		if bez.ikonka != s.ikonka {
			t.Fatalf("%s: обновление сменило иконку с %q на %q", sost, bez.ikonka, s.ikonka)
		}
	}
}
