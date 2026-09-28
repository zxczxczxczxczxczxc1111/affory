package main

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

func TestOtkazOdnogoSaytaNeRvyotZhivoyTunnel(t *testing.T) {
	s := podstavnaya(t, nil)
	var probes, reserve atomic.Int32
	s.zamerit = func(context.Context, string, string, string) (time.Duration, error) {
		if probes.Add(1) == 1 {
			return time.Millisecond, nil
		}
		return 0, errors.New("gstatic timeout")
	}
	s.zameritRezerv = func(context.Context, string, string, string) (time.Duration, error) {
		reserve.Add(1)
		return time.Millisecond, nil
	}
	s.period = 10 * time.Millisecond
	if err := s.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && reserve.Load() < 3 {
		if s.Status().Sostoyanie != protokol.SostPodnyat {
			t.Fatal("отказ одного сайта оборвал работающий туннель")
		}
		time.Sleep(time.Millisecond)
	}
	if reserve.Load() < 3 {
		t.Fatal("перед аварийным отключением независимая цель не проверена")
	}
}

// С6 аудита 1.6.1. В «авто» группа держится за мёртвый сервер до своей
// следующей пробы, и наблюдатель рвал туннель раньше, чем она успевала уйти.
// Перемер группы выбирает живой сервер, и туннель остаётся.
func TestAvtoPeremeryaetGruppuPeredRazryvom(t *testing.T) {
	s := podstavnaya(t, nil)
	var probes, peremerov atomic.Int32
	var peremerena atomic.Bool
	s.zamerit = func(context.Context, string, string, string) (time.Duration, error) {
		if probes.Add(1) == 1 || peremerena.Load() {
			return time.Millisecond, nil
		}
		return 0, errors.New("мёртвый сервер")
	}
	s.zameritRezerv = func(context.Context, string, string, string) (time.Duration, error) {
		return 0, errors.New("мёртвый сервер")
	}
	s.peremeritAvto = func(context.Context, string, string) (bool, error) {
		peremerov.Add(1)
		peremerena.Store(true)
		return true, nil
	}
	s.period = 10 * time.Millisecond
	if err := s.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && probes.Load() < 10 {
		if s.Status().Sostoyanie != protokol.SostPodnyat {
			t.Fatal("туннель порван, хотя перемер группы нашёл живой сервер")
		}
		time.Sleep(time.Millisecond)
	}
	if peremerov.Load() != 1 {
		t.Fatalf("перемеров группы %d, ждали один", peremerov.Load())
	}
}

// Перемер не помог: туннель рвётся, как и прежде.
func TestAvtoBezZhivyhServerovRvyotTunnel(t *testing.T) {
	s := podstavnaya(t, nil)
	var probes atomic.Int32
	s.zamerit = func(context.Context, string, string, string) (time.Duration, error) {
		if probes.Add(1) == 1 {
			return time.Millisecond, nil
		}
		return 0, errors.New("мёртвый сервер")
	}
	s.peremeritAvto = func(context.Context, string, string) (bool, error) { return true, nil }
	s.period = 10 * time.Millisecond
	if err := s.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	dozhdatsya(t, "разрыв после бесполезного перемера", func() bool {
		return s.Status().Sostoyanie != protokol.SostPodnyat
	})
}

func TestDisconnectOtmenyaetRezervnuyuProbu(t *testing.T) {
	s := podstavnaya(t, nil)
	var probes atomic.Int32
	entered := make(chan struct{})
	s.zamerit = func(context.Context, string, string, string) (time.Duration, error) {
		if probes.Add(1) == 1 {
			return time.Millisecond, nil
		}
		return 0, errors.New("primary timeout")
	}
	s.zameritRezerv = func(ctx context.Context, _, _, _ string) (time.Duration, error) {
		close(entered)
		<-ctx.Done()
		return 0, ctx.Err()
	}
	s.period = time.Millisecond
	if err := s.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("резервная проба не запустилась")
	}
	s.Disconnect()
	if got := s.Status().Sostoyanie; got != protokol.SostVyklyuchen {
		t.Fatalf("отмена пробы превратилась в аварийное восстановление: %s", got)
	}
}
