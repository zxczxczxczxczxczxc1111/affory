package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Разбор журнала живой машины 10.09.2026, полтора суток работы 1.0.2.
//
// 197 записей вида «clash_api не отвечает: ... dial tcp 127.0.0.1:51983:
// bind: An operation on a socket could not be performed because the system
// lacked sufficient buffer space» и 25 разрывов туннеля следом, ровно раз в
// 98 минут. Столько нужно утечке со скоростью 2.79 соединения в секунду,
// чтобы съесть 16384 эфемерных порта Windows.
//
// Цепочка целиком: порты кончились -> локальный сокет к clash_api не
// открывается -> замер падает дважды подряд -> наблюдатель считает, что
// туннель не несёт -> Connect заново, то есть перезапуск ядра -> у человека
// рвутся ВСЕ соединения на несколько секунд.
//
// Утечка починена в этой же волне, но спусковой крючок от неё не зависит:
// порты может съесть торрент, браузер или чужая программа. Отказ ЛОКАЛЬНОГО
// сокета не говорит о туннеле ничего.

func nehvatkaPortov() error {
	// Ровно то, что стоит в журнале: dial -> bind -> WSAENOBUFS.
	return fmt.Errorf("clash_api не отвечает: %w", &net.OpError{
		Op:  "dial",
		Net: "tcp",
		Err: os.NewSyscallError("bind", syscall.Errno(10055)),
	})
}

func TestNehvatkaPortovNeSchitaetsyaSmertyuTunnelya(t *testing.T) {
	s := podstavnaya(t, nil)
	var pervyy atomic.Bool
	var zamerov atomic.Int32

	s.zamerit = func(ctx context.Context, adres, sekret, teg string) (time.Duration, error) {
		if pervyy.CompareAndSwap(false, true) {
			return time.Millisecond, nil
		}
		zamerov.Add(1)
		return 0, nehvatkaPortov()
	}
	s.period = time.Millisecond
	s.provalov = 2

	if err := s.Connect(context.Background()); err != nil {
		t.Fatalf("подъём не прошёл: %v", err)
	}
	defer s.Disconnect()

	// Ждём заведомо больше, чем нужно на десяток провалов подряд.
	do := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(do) {
		if sost := s.Status().Sostoyanie; sost != protokol.SostPodnyat {
			t.Fatalf("туннель опущен из-за нехватки портов на машине: состояние %s."+
				" Локальный сокет к 127.0.0.1 не открылся, а рвут соединение человеку", sost)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if n := zamerov.Load(); n < 5 {
		t.Fatalf("замеров с отказом всего %d: тест не дошёл до порога и ничего не проверил", n)
	}
}

// Обратная половина, без неё первую можно «починить» отказом опускать туннель
// вообще. Настоящий отказ обязан по-прежнему опускать.
func TestNastoyashchiyOtkazVsyoeshchyoOpuskaetTunnel(t *testing.T) {
	s := podstavnaya(t, nil)
	var pervyy atomic.Bool

	s.zamerit = func(ctx context.Context, adres, sekret, teg string) (time.Duration, error) {
		if pervyy.CompareAndSwap(false, true) {
			return time.Millisecond, nil
		}
		return 0, errors.New("исходящий не отвечает (код 504)")
	}
	s.period = time.Millisecond
	s.provalov = 2

	if err := s.Connect(context.Background()); err != nil {
		t.Fatalf("подъём не прошёл: %v", err)
	}
	defer s.Disconnect()

	do := time.Now().Add(2 * time.Second)
	for time.Now().Before(do) {
		if s.Status().Sostoyanie == protokol.SostNeNeset {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("настоящий отказ туннеля больше не опускает его: наблюдатель ослеп")
}
