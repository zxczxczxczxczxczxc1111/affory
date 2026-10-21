package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

func chasyOtvechayut(d time.Duration) func(context.Context) (time.Duration, error) {
	return func(context.Context) (time.Duration, error) { return d, nil }
}

// В статус уходит только расхождение выше порога: минута разницы подключению
// не мешает, и строка о ней на главном экране была бы шумом.
func TestStatusNesyotTolkoSbityeChasy(t *testing.T) {
	s := podstavnaya(t, nil)
	s.zameritChasy = chasyOtvechayut(time.Minute)
	if _, err := s.sveritChasy(context.Background()); err != nil {
		t.Fatal(err)
	}
	if r := s.Status().ChasyRashozhdenie; r != 0 {
		t.Fatalf("минута расхождения ушла в статус: %d", r)
	}
	s.zameritChasy = chasyOtvechayut(-7 * time.Minute)
	if _, err := s.sveritChasy(context.Background()); err != nil {
		t.Fatal(err)
	}
	if r := s.Status().ChasyRashozhdenie; r != -420 {
		t.Fatalf("расхождение в статусе %d, ожидалось -420", r)
	}
}

// Сбитые часы главнее причины пробы: сервер с ними отказывает при любом ключе,
// и «проверь подписку» отправило бы человека чинить исправное.
func TestOtkazPriSbityhChasahEtoClockSkew(t *testing.T) {
	s := podstavnaya(t, errors.New("исходящий не отвечает (код 504)"))
	s.zameritChasy = chasyOtvechayut(-7 * time.Minute)
	staryy := zhdatPodyoma
	zhdatPodyoma = 900 * time.Millisecond
	defer func() { zhdatPodyoma = staryy }()

	err := s.Connect(context.Background())
	if !errors.Is(err, ErrChasySbity) {
		t.Fatalf("отказ %v не помечен сбитыми часами", err)
	}
	st := s.Status()
	if st.Oshib == nil || st.Oshib.Kod != protokol.KodChasy {
		t.Fatalf("код состояния %+v, ожидался %s", st.Oshib, protokol.KodChasy)
	}
	if !strings.Contains(st.Oshib.Tekst, "отстают на 7 мин") || !strings.Contains(st.Oshib.Tekst, "данные через него не пошли") {
		t.Errorf("текст отказа не называет часы или прежнюю причину: %s", st.Oshib.Tekst)
	}
	// Ответ команды и состояние называют отказ одним кодом.
	if k := kodPodklyucheniya(err, st.Oshib); k != protokol.KodChasy {
		t.Fatalf("ответ команды %s, состояние %s", k, st.Oshib.Kod)
	}
}

// Верные часы отказ не трогают.
func TestVernyeChasyNeTrogayutOtkaz(t *testing.T) {
	s := podstavnaya(t, errors.New("исходящий не отвечает (код 504)"))
	staryy := zhdatPodyoma
	zhdatPodyoma = 900 * time.Millisecond
	defer func() { zhdatPodyoma = staryy }()

	err := s.Connect(context.Background())
	if errors.Is(err, ErrChasySbity) {
		t.Fatalf("верные часы пометили отказ: %v", err)
	}
	if st := s.Status(); st.Oshib == nil || st.Oshib.Kod == protokol.KodChasy {
		t.Fatalf("код состояния %+v", st.Oshib)
	}
}

// Старая сверка перед отказом обновляется: часы могли поправить минуту назад.
// Неудача свежей сверки оставляет прошлый ответ, а без прошлого молчит.
func TestSverkaPriOtkazeSvezhaya(t *testing.T) {
	s := podstavnaya(t, nil)
	s.zameritChasy = chasyOtvechayut(-7 * time.Minute)
	if _, err := s.sveritChasy(context.Background()); err != nil {
		t.Fatal(err)
	}
	seychas := time.Now()
	s.seychas = func() time.Time { return seychas.Add(svezhestChasov + time.Minute) }
	s.zameritChasy = chasyOtvechayut(0)
	if _, sb := s.chasySbityDlyaOtkaza(); sb {
		t.Fatal("поправленные часы названы сбитыми по старой сверке")
	}

	s.zameritChasy = func(context.Context) (time.Duration, error) { return 0, errors.New("сети нет") }
	s.mu.Lock()
	s.chasyRashozhdenie, s.chasyZamereno = 0, time.Time{}
	s.mu.Unlock()
	if _, sb := s.chasySbityDlyaOtkaza(); sb {
		t.Fatal("часы названы сбитыми без единой сверки")
	}
}

// Сбитые по прошлой сверке часы при отказе сверяются заново, даже если сверке
// всего две минуты: человек мог поправить их по строке на главном, и отказ
// не должен снова назвать часы причиной.
func TestSbityeChasyPereproveryayutsyaPriOtkaze(t *testing.T) {
	s := podstavnaya(t, nil)
	s.zameritChasy = chasyOtvechayut(-7 * time.Minute)
	if _, err := s.sveritChasy(context.Background()); err != nil {
		t.Fatal(err)
	}
	seychas := time.Now()
	s.seychas = func() time.Time { return seychas.Add(2 * time.Minute) }
	s.zameritChasy = chasyOtvechayut(0)
	if _, sb := s.chasySbityDlyaOtkaza(); sb {
		t.Fatal("поправленные две минуты назад часы названы сбитыми")
	}
	if r := s.Status().ChasyRashozhdenie; r != 0 {
		t.Fatalf("строка о часах осталась в статусе: %d", r)
	}
}

// Неудачная сверка повторяется не чаще раза в минуту: без сети она ждёт свой
// срок до конца, и каждый круг восстановления платил бы за неё.
func TestNeudachnayaSverkaNeChasheMinuty(t *testing.T) {
	s := podstavnaya(t, nil)
	zvonkov := 0
	s.zameritChasy = func(context.Context) (time.Duration, error) {
		zvonkov++
		return 0, errors.New("сети нет")
	}
	seychas := time.Now()
	s.seychas = func() time.Time { return seychas }
	s.chasySbityDlyaOtkaza()
	s.chasySbityDlyaOtkaza()
	if zvonkov != 1 {
		t.Fatalf("за одну минуту сверок %d, ожидалась одна", zvonkov)
	}
	s.seychas = func() time.Time { return seychas.Add(povtorChasov) }
	s.chasySbityDlyaOtkaza()
	if zvonkov != 2 {
		t.Fatalf("через минуту сверок %d, ожидалось две", zvonkov)
	}
}

func TestOpisatRashozhdenie(t *testing.T) {
	for d, nado := range map[time.Duration]string{
		-7 * time.Minute:          "отстают на 7 мин от",
		3*time.Hour + time.Minute: "спешат на 3 ч от",
		-50 * time.Hour:           "отстают на 2 сут. от",
		-30 * time.Second:         "отстают на 30 с от",
	} {
		if got := opisatRashozhdenie(d); got != nado {
			t.Errorf("%s: %q, ожидалось %q", d, got, nado)
		}
	}
}
