package main

import (
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Разбор 10.09.2026. Жалоба владельца: «если водить мышкой по меню в трее,
// оно зависает, иногда меняет цвет на белый».
//
// Устройство Wails v3.0.0-beta.16, прочитанное по исходникам: SystemTray.SetMenu
// не обновляет меню, а УНИЧТОЖАЕТ его и строит заново (windowsSystemTray.updateMenu
// зовёт DestroyMenu). Открытое меню трея это TrackPopupMenuEx, который крутит свой
// модальный цикл сообщений на главном потоке и честно диспатчит PostMessage,
// которым InvokeSync доставляет вызов. Значит DestroyMenu прилетает по меню,
// нарисованному прямо сейчас под курсором.
//
// Спусковой крючок был свой: экран спрашивает status раз в пять секунд, каждый
// ответ доходил до Obnovit, а Obnovit звал SetMenu безусловно. Подержал меню
// открытым дольше пяти секунд, получил разрушение.
//
// Отсюда два требования, и оба проверяются здесь.

// stend собирает Trey без живого Wails и считает, сколько раз тот пошёл в трей.
type stendTreya struct {
	t          *Trey
	risovaniy  int
	posledniy  vidTreya
}

func novyyStend() *stendTreya {
	s := &stendTreya{}
	s.t = &Trey{
		// На главном потоке в наборе мы и так: звать сразу это честная модель
		// InvokeAsync, у которой очередь пуста.
		naGlavnom: func(f func()) { f() },
		risovat: func(v vidTreya) {
			s.risovaniy++
			s.posledniy = v
		},
	}
	return s
}

func TestTreyNeTrogaetMenyuBezIzmeneniy(t *testing.T) {
	s := novyyStend()
	st := protokol.StatusOtvet{Sostoyanie: protokol.SostPodnyat}

	s.t.Obnovit(st)
	if s.risovaniy != 1 {
		t.Fatalf("первый статус нарисован %d раз, ожидался один", s.risovaniy)
	}
	// Пять ответов status подряд без единого изменения. Ровно это и приходило
	// раз в пять секунд, пока человек держал меню открытым.
	for i := 0; i < 5; i++ {
		s.t.Obnovit(st)
	}
	if s.risovaniy != 1 {
		t.Fatalf("состояние не менялось, а трей тронут %d раз: каждое касание"+
			" это DestroyMenu по нарисованному меню", s.risovaniy)
	}
}

func TestTreyRisuetNastoyashcheeIzmenenie(t *testing.T) {
	s := novyyStend()
	s.t.Obnovit(protokol.StatusOtvet{Sostoyanie: protokol.SostVyklyuchen})
	s.t.Obnovit(protokol.StatusOtvet{Sostoyanie: protokol.SostPodnyat})
	if s.risovaniy != 2 {
		t.Fatalf("состояние сменилось, а трей тронут %d раз вместо двух", s.risovaniy)
	}
	if s.posledniy.sostoyanie != podpisTreya(protokol.SostPodnyat) {
		t.Fatalf("нарисована подпись %q, а состояние поднято", s.posledniy.sostoyanie)
	}
}

func TestTreyNePeresobiraetMenyuPodKursorom(t *testing.T) {
	s := novyyStend()
	s.t.Obnovit(protokol.StatusOtvet{Sostoyanie: protokol.SostVyklyuchen})
	bylo := s.risovaniy

	s.t.MenyuOtkrylos()
	s.t.Obnovit(protokol.StatusOtvet{Sostoyanie: protokol.SostPodnyat})
	if s.risovaniy != bylo {
		t.Fatalf("меню открыто, а трей пересобран: ровно это и белит меню"+
			" под курсором (нарисовано %d раз вместо %d)", s.risovaniy, bylo)
	}

	s.t.MenyuZakrylos()
	if s.risovaniy != bylo+1 {
		t.Fatalf("меню закрыто, а отложенное изменение не применено:"+
			" нарисовано %d раз вместо %d", s.risovaniy, bylo+1)
	}
	if s.posledniy.sostoyanie != podpisTreya(protokol.SostPodnyat) {
		t.Fatalf("после закрытия нарисовано %q, а ждали подписи поднятого туннеля",
			s.posledniy.sostoyanie)
	}
}

// Пока меню открыто, состояние может смениться дважды. Применять надо
// ПОСЛЕДНЕЕ, а не копить очередь пересборок: промежуточное состояние никто уже
// не увидит, а лишний DestroyMenu стоит ровно столько же, сколько нужный.
func TestTreyPrimenyaetPosledneeOtlozhennoe(t *testing.T) {
	s := novyyStend()
	s.t.Obnovit(protokol.StatusOtvet{Sostoyanie: protokol.SostVyklyuchen})
	bylo := s.risovaniy

	s.t.MenyuOtkrylos()
	s.t.Obnovit(protokol.StatusOtvet{Sostoyanie: protokol.SostPodnimaetsya})
	s.t.Obnovit(protokol.StatusOtvet{Sostoyanie: protokol.SostPodnyat})
	s.t.MenyuZakrylos()

	if s.risovaniy != bylo+1 {
		t.Fatalf("две смены при открытом меню дали %d пересборок вместо одной",
			s.risovaniy-bylo)
	}
	if s.posledniy.sostoyanie != podpisTreya(protokol.SostPodnyat) {
		t.Fatalf("применено %q, а последним было поднятое", s.posledniy.sostoyanie)
	}
}
