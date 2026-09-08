package main

import (
	"context"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/skorost"
	"testing"
	"time"
)

func TestSpeedJobUsesOwnedProxyAndCancelsOnDisconnect(t *testing.T) {
	s := podstavnaya(t, nil)
	s.sost = protokol.SostPodnyat
	s.portProksiNash = 12345
	called := make(chan string, 1)
	exited := make(chan struct{})
	s.speedRunner = func(ctx context.Context, proxy, _ string, _ func(skorost.Progress)) skorost.Result {
		called <- proxy
		<-ctx.Done()
		close(exited)
		return skorost.Result{Download: 999, Upload: 999}
	}
	k := protokol.Kadr{Id: 1, Imya: "startSpeedTest"}
	if o := s.startSpeedTest(k); o.Oshib != nil {
		t.Fatal(o.Oshib)
	}
	if proxy := <-called; proxy != "127.0.0.1:12345" {
		t.Fatalf("wrong path: %s", proxy)
	}
	if o := s.startSpeedTest(k); o.Oshib == nil {
		t.Fatal("allowed overlapping jobs")
	}
	s.Disconnect()
	select {
	case <-exited:
	case <-time.After(time.Second):
		t.Fatal("download survived disconnect")
	}
	s.fon.Wait()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.speed.snapshot.Phase != "cancelled" || s.speed.snapshot.Result != nil {
		t.Fatalf("cancelled numbers escaped: %+v", s.speed.snapshot)
	}
}

func TestSpeedCannotFallbackOutsideMissingVPNProxy(t *testing.T) {
	s := podstavnaya(t, nil)
	s.sost = protokol.SostPodnyat
	s.portProksiNash = 0
	s.speedRunner = func(context.Context, string, string, func(skorost.Progress)) skorost.Result {
		t.Error("network touched")
		return skorost.Result{}
	}
	if o := s.startSpeedTest(protokol.Kadr{Id: 1, Imya: "startSpeedTest"}); o.Oshib == nil {
		t.Fatal("missing own proxy accepted")
	}
}

func TestIdleSpeedDoesNotStartVPNAndServiceShutdownCancels(t *testing.T) {
	s := podstavnaya(t, nil)
	called := make(chan string, 1)
	s.speedRunner = func(ctx context.Context, proxy, _ string, _ func(skorost.Progress)) skorost.Result {
		called <- proxy
		<-ctx.Done()
		return skorost.Result{}
	}
	if o := s.startSpeedTest(protokol.Kadr{Id: 1, Imya: "startSpeedTest"}); o.Oshib != nil {
		t.Fatal(o.Oshib)
	}
	if proxy := <-called; proxy != "" {
		t.Fatal("idle used VPN proxy")
	}
	if s.Status().Sostoyanie != protokol.SostVyklyuchen {
		t.Fatal("speed test started VPN")
	}
	s.Zavershit()
}
