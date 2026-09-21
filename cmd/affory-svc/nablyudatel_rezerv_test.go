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
