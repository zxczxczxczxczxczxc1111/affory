package main

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/set"
)

// Порт локального прокси занят ЧУЖОЙ программой.
//
// Это не выдумка ради полноты: на рабочей машине 10809 держит другой клиент, и
// оба туннеля одновременно не живут. Важно, чем это кончается. sing-box падает
// на бинде ЦЕЛИКОМ, то есть занятый прокси-порт унёс бы туннель, который сам по
// себе исправен. Значит выбор такой: прокси есть только если порт свободен, а
// туннель поднимается в любом случае.
func TestZanyatyyPortProksiNeLomaetTunnel(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("слушатель не поднялся: %v", err)
	}
	defer l.Close()
	zanyatyy := l.Addr().(*net.TCPAddr).Port

	if p := portProksi(zanyatyy); p != 0 {
		t.Errorf("занятый порт %d отдан конфигу как %d: ядро упадёт на бинде и унесёт туннель", zanyatyy, p)
	}
}

func TestSvobodnyyPortProksiBeryotsya(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("слушатель не поднялся: %v", err)
	}
	svobodnyy := l.Addr().(*net.TCPAddr).Port
	l.Close()

	if p := portProksi(svobodnyy); p != svobodnyy {
		t.Errorf("свободный порт %d не взят: отдано %d", svobodnyy, p)
	}
}

// Наблюдатель за чужим прокси не был покрыт НИЧЕМ до 02.09.2026, при том что
// это фоновая горутина, которая читает реестр и шлёт код протокола. Нашлось это
// не вычиткой, а воротами Ш1: они спросили, кто читает пакетную переменную с
// расписанием, и выяснилось, что читает горутина, а проверяет её никто.
//
// Расписание живёт полем службы, а не пакетной переменной, ровно ради этого
// теста: общая переменная, которую правит тест и читает чужая горутина, это
// гонка, которую детектор находит на первом же прогоне (шаг 3 ворот Ш8).
func TestChuzhoyProksiNazyvaetsyaOdinRaz(t *testing.T) {
	s := podstavnaya(t, nil)
	s.periodProksi = 2 * time.Millisecond
	s.prochitatProksi = func() (set.Proksi, error) {
		return set.Proksi{Vklyuchen: true, Adres: "127.0.0.1:8888"}, nil
	}

	id, sob := s.Podpisatsya()
	defer s.Otpisatsya(id)

	ctx, otmena := context.WithCancel(context.Background())
	t.Cleanup(otmena)
	go s.slediZaProksi(ctx)

	srok := time.After(3 * time.Second)
	nashli := 0
	for nashli == 0 {
		select {
		case k := <-sob:
			if k.Imya == "proxyHijack" &&
				strings.Contains(string(k.Telo), protokol.KodForeignRegistryHijack) {
				nashli++
			}
		case <-srok:
			t.Fatal("о чужом прокси не сказали ни разу: перехват мимо туннеля прошёл молча")
		}
	}

	// Гашение повтора: адрес не менялся, значит второго предупреждения быть не
	// должно, сколько бы кругов ни прошло. Ждём заведомо дольше периода.
	time.Sleep(50 * time.Millisecond)
	for {
		select {
		case k := <-sob:
			if k.Imya == "proxyHijack" {
				t.Fatal("предупреждение повторилось при неизменном адресе: так учат его не читать")
			}
		default:
			return
		}
	}
}
