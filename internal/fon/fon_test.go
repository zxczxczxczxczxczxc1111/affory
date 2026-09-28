package fon

import (
	"bytes"
	"context"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// zhurnalTesta это приёмник журнала под своим замком: Zapustit пишет из своей
// горутины, тест читает из своей, и голый bytes.Buffer здесь гонка (-race в
// воротах).
type zhurnalTesta struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (z *zhurnalTesta) Write(p []byte) (int, error) {
	z.mu.Lock()
	defer z.mu.Unlock()
	return z.b.Write(p)
}

func (z *zhurnalTesta) String() string {
	z.mu.Lock()
	defer z.mu.Unlock()
	return z.b.String()
}

func perehvatitZhurnal(t *testing.T) *zhurnalTesta {
	t.Helper()
	z := &zhurnalTesta{}
	prezhniy := log.Writer()
	log.SetOutput(z)
	t.Cleanup(func() { log.SetOutput(prezhniy) })
	return z
}

func TestZapustitPerezhivaetPaniku(t *testing.T) {
	zhurnal := perehvatitZhurnal(t)
	gotovo := make(chan struct{})
	Zapustit("пробе", func() {
		defer close(gotovo)
		panic("бах")
	})
	select {
	case <-gotovo:
	case <-time.After(5 * time.Second):
		t.Fatal("горутина не отработала")
	}
	// Запись идёт после close(gotovo): ждём её, а не спим наугад.
	srok := time.Now().Add(5 * time.Second)
	for !strings.Contains(zhurnal.String(), "паника в пробе: бах") {
		if time.Now().After(srok) {
			t.Fatalf("паники нет в журнале: %q", zhurnal.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestVypolnitGovoritOPadenii(t *testing.T) {
	perehvatitZhurnal(t)
	if Vypolnit("пробе", func() {}) {
		t.Fatal("целая работа названа упавшей")
	}
	if !Vypolnit("пробе", func() { panic(1) }) {
		t.Fatal("паника не замечена")
	}
}

func TestSPovtoromPerezapuskaetRovnoOdinRaz(t *testing.T) {
	zhurnal := perehvatitZhurnal(t)
	var zahodov atomic.Int32
	SPovtorom(context.Background(), "цикле", time.Millisecond, func() {
		zahodov.Add(1)
		panic("снова")
	})
	if n := zahodov.Load(); n != 2 {
		t.Fatalf("заходов %d, нужно 2: первый и один повтор", n)
	}
	if !strings.Contains(zhurnal.String(), "больше не перезапускаю") {
		t.Fatalf("второй провал не объяснён в журнале: %q", zhurnal.String())
	}
}

func TestSPovtoromBezPanikiNePovtoryaet(t *testing.T) {
	var zahodov atomic.Int32
	SPovtorom(context.Background(), "цикле", time.Millisecond, func() { zahodov.Add(1) })
	if n := zahodov.Load(); n != 1 {
		t.Fatalf("заходов %d, нужен 1", n)
	}
}

func TestSPovtoromOtmenaSnimaetPovtor(t *testing.T) {
	perehvatitZhurnal(t)
	ctx, otmena := context.WithCancel(context.Background())
	var zahodov atomic.Int32
	SPovtorom(ctx, "цикле", time.Hour, func() {
		zahodov.Add(1)
		otmena()
		panic("при отключении")
	})
	if n := zahodov.Load(); n != 1 {
		t.Fatalf("после отмены был повтор: заходов %d", n)
	}
}
