package main

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// Задержка экрана это СВОЙ замер круга, а не число ядра.
//
// Ядро меряет urltest: дозвон через сервер с рукопожатием, TLS с целью и
// запрос - три-пять кругов разом. Экран показывал 169 мс там, где Discord
// показывал 65 (владелец, 21.09.2026), и человек читал исправную связь как
// беду. Если источник числа вернут к ядру, этот тест увидит чужую цифру.
func TestZaderzhkaEkranaBerotsyaIzSvoegoZamera(t *testing.T) {
	s, _ := sluzhbaSoStatistikoy(t)
	s.zamerOtklika = func(context.Context, string, int) (time.Duration, error) { return 61 * time.Millisecond, nil }

	id, sob := s.Podpisatsya()
	defer s.Otpisatsya(id)
	ctx := sPodpischikom(context.Background(), id)
	if o := podpisat(s, ctx, true); o.Oshib != nil {
		t.Fatalf("подписка отвергнута: %+v", o.Oshib)
	}
	srok := time.After(2 * time.Second)
	for {
		select {
		case k := <-sob:
			if k.Imya != "stats" {
				continue
			}
			var st struct {
				Zaderzhka *int64 `json:"zaderzhka_ms"`
			}
			if err := json.Unmarshal(k.Telo, &st); err != nil {
				t.Fatal(err)
			}
			if st.Zaderzhka == nil {
				continue // первый замер мог не успеть до первого события
			}
			if *st.Zaderzhka != 61 {
				t.Fatalf("на экране %d мс, а замер дал 61", *st.Zaderzhka)
			}
			return
		case <-srok:
			t.Fatal("события с задержкой не пришло")
		}
	}
}

// Замер идёт через ЛОКАЛЬНЫЙ ПРОКСИ, то есть тем же путём, что трафик
// приложений. Прямой запрос из службы уходит мимо туннеля по правилу петли и
// померил бы домашний канал, назвав его задержкой VPN.
func TestOtklikMeryaetsyaCherezLokalnyyProksi(t *testing.T) {
	s, _ := sluzhbaSoStatistikoy(t)
	porty := make(chan int, 4)
	s.mu.Lock()
	s.portProksiNash = 10877
	s.mu.Unlock()
	s.zamerOtklika = func(_ context.Context, _ string, port int) (time.Duration, error) {
		select {
		case porty <- port:
		default:
		}
		return 50 * time.Millisecond, nil
	}

	s.odinZamerOtklika(context.Background())
	select {
	case p := <-porty:
		if p != 10877 {
			t.Fatalf("замер пошёл в порт %d, а локальный прокси на 10877", p)
		}
	default:
		t.Fatal("замер не сделан вовсе")
	}
}

// Опущенный туннель это не отказ, а состояние: мерить нечего, и числа на
// экране быть не должно.
//
// Опущенность здесь задана ПОРТОМ clash_api, а не состоянием службы: таков
// критерий живого ядра во всей службе (kriteriy_zhivogo_yadra_test.go).
func TestBezTunnelyaOtklikNeMeryaetsya(t *testing.T) {
	s, _ := sluzhbaSoStatistikoy(t)
	var zvali atomic.Int32
	s.zamerOtklika = func(context.Context, string, int) (time.Duration, error) {
		zvali.Add(1)
		return 10 * time.Millisecond, nil
	}
	s.mu.Lock()
	s.portClash = 0
	s.mu.Unlock()

	s.odinZamerOtklika(context.Background())
	if n := zvali.Load(); n != 0 {
		t.Fatalf("при опущенном туннеле замер сделан %d раз", n)
	}
	if _, est := s.posledniyOtklik(); est {
		t.Fatal("при опущенном туннеле на экране осталось число")
	}
}

// Отказ замера ГАСИТ число, а не оставляет прежнее. Задержка, замершая на
// цифре десятиминутной давности, читается как живая, и по ней судят о связи,
// которой уже нет.
func TestOtkazZameraGasitChislo(t *testing.T) {
	s, _ := sluzhbaSoStatistikoy(t)
	s.zamerOtklika = func(context.Context, string, int) (time.Duration, error) { return 70 * time.Millisecond, nil }
	s.mu.Lock()
	s.portProksiNash = 10809
	s.mu.Unlock()

	s.odinZamerOtklika(context.Background())
	if ms, est := s.posledniyOtklik(); !est || ms != 70 {
		t.Fatalf("удачный замер не запомнен: %d мс, есть=%v", ms, est)
	}

	s.zamerOtklika = func(context.Context, string, int) (time.Duration, error) {
		return 0, errors.New("цель замера не ответила")
	}
	s.odinZamerOtklika(context.Background())
	if ms, est := s.posledniyOtklik(); est {
		t.Fatalf("после отказа на экране осталось %d мс", ms)
	}
}
