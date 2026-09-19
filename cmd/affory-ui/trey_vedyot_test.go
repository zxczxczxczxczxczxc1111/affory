package main

import "testing"

// Пункт «Обновить до X» обязан доводить до места, где обновляются.
//
// 13.09.2026 живой отзыв: «когда обновление доступно, нельзя обновиться из
// трея, ничего не происходит». Пункт и правда открывал окно, но на той вкладке,
// где оно было брошено, обычно на «Подключение». Карточка обновления живёт в
// «Настройках», внизу, и человек не видел никакой связи между нажатием и тем,
// что показалось на экране.
func TestPunktObnovleniyaVedyotVNastroyki(t *testing.T) {
	pokazano := 0
	var kuda []string
	t_ := &Trey{
		pokazat: func() { pokazano++ },
		vesti:   func(vkladka string) { kuda = append(kuda, vkladka) },
	}

	t_.otkrytObnovlenie()

	if pokazano != 1 {
		t.Fatalf("окно показано %d раз", pokazano)
	}
	if len(kuda) != 1 || kuda[0] != vkladkaNastroyek {
		t.Fatalf("окно уехало на %v, а обновление живёт на %q", kuda, vkladkaNastroyek)
	}
}

// Трей живёт дольше, чем подписка окна на события, и указание вкладки не
// должно быть условием работы пункта: без шва он обязан просто показать окно.
func TestPunktObnovleniyaBezSvyaziSOknomNePadaet(t *testing.T) {
	pokazano := 0
	t_ := &Trey{pokazat: func() { pokazano++ }}

	t_.otkrytObnovlenie()

	if pokazano != 1 {
		t.Fatalf("окно показано %d раз", pokazano)
	}
}

// Щелчок по всплывашке приходит из чужой горутины, и показ окна обязан уехать
// на главный поток. Иначе HWND трогают откуда попало: ровно это уже стоило
// паники в трее 10.09.2026.
func TestShchelchokPoVsplyvashkeIdyotCherezGlavnyyPotok(t *testing.T) {
	pokazano := 0
	var kuda []string
	naGlavnom := 0
	t_ := &Trey{
		pokazat: func() { pokazano++ },
		vesti:   func(vkladka string) { kuda = append(kuda, vkladka) },
	}
	t_.naGlavnom = func(f func()) { naGlavnom++; f() }

	t_.OtkrytObnovlenieIzvne()

	if naGlavnom != 1 {
		t.Fatalf("на главный поток ушло %d вызовов", naGlavnom)
	}
	if pokazano != 1 {
		t.Fatalf("окно показано %d раз", pokazano)
	}
	if len(kuda) != 1 || kuda[0] != vkladkaNastroyek {
		t.Fatalf("окно уехало на %v, а обновление живёт на %q", kuda, vkladkaNastroyek)
	}
}

// До запуска приложения отправителя работы ещё нет, и щелчок не имеет права
// падать на этом: показывать в этот момент всё равно некуда.
func TestShchelchokBezOtpravitelyaNePadaet(t *testing.T) {
	pokazano := 0
	t_ := &Trey{pokazat: func() { pokazano++ }}

	t_.OtkrytObnovlenieIzvne()

	if pokazano != 1 {
		t.Fatalf("окно показано %d раз", pokazano)
	}
}
