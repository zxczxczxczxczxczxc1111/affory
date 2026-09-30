package main

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Г3 аудита 1.8.0. Остановка службы меняла поколение подъёма, но не гасила
// его контекст: подъём из восстановления досиживал пробу до её срока, и
// остановка ждала его на fon.Wait.
func TestOstanovkaNeZhdyotPodyomaIzVosstanovleniya(t *testing.T) {
	s := podstavnaya(t, nil)
	var vProbe atomic.Bool
	s.zamerit = func(ctx context.Context, _, _, _ string) (time.Duration, error) {
		vProbe.Store(true)
		// Как живое ядро с молчащим сервером: ответ только по сроку пробы.
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(5 * time.Second):
			return 0, context.DeadlineExceeded
		}
	}
	s.otstupy = []time.Duration{time.Millisecond}
	s.postavit(protokol.SostNeNeset, nil)
	s.zapustitVosstanovlenie()
	dozhdatsya(t, "подъём из восстановления дошёл до пробы", vProbe.Load)

	nach := time.Now()
	s.Zavershit()
	if d := time.Since(nach); d > 2*time.Second {
		t.Fatalf("остановка ждала идущий подъём %v", d)
	}
}

// M4 аудита 1.8.0: хранилище, не прочитанное при старте, дочитывается в фоне,
// и режим в статусе берётся из набора, а не остаётся умолчанием.
func TestNeprochitannoeKhranilishcheDochityvaetsya(t *testing.T) {
	prezhnie := pauzyDochityvaniya
	pauzyDochityvaniya = []time.Duration{time.Millisecond, time.Millisecond}
	t.Cleanup(func() { pauzyDochityvaniya = prezhnie })
	s := podstavnaya(t, nil)
	var chteniy atomic.Int32
	s.nabor = func() (Nabor, error) {
		if chteniy.Add(1) == 1 {
			return Nabor{}, errors.New("тест: шифрование Windows не отвечает")
		}
		return Nabor{Rezhim: protokol.RezhimRuchnoy}, nil
	}
	s.mu.Lock()
	s.rezhim, s.rezhimNeProchitan = protokol.RezhimAvto, true
	s.mu.Unlock()
	s.DochitatKhranilishche()
	dozhdatsya(t, "режим из набора", func() bool {
		return s.Status().RezhimMarshruta == protokol.RezhimRuchnoy
	})
}

// Г6 аудита 1.8.0. Наблюдатель, упавший паникой второй раз, больше не
// перезапускается, и туннель оставался без присмотра: умри он, никто бы не
// заметил.
func TestNablyudatelPosleVtoroyPanikiOtdayotTunnelVosstanovleniyu(t *testing.T) {
	s := podstavnaya(t, nil)
	s.period = time.Millisecond
	s.pauzaPaniki = time.Millisecond
	s.otstupy = []time.Duration{time.Hour}
	var slomat atomic.Bool
	prezhniy := s.zamerit
	s.zamerit = func(ctx context.Context, adres, sekret, teg string) (time.Duration, error) {
		if slomat.Load() {
			panic("тест: наблюдатель туннеля")
		}
		return prezhniy(ctx, adres, sekret, teg)
	}
	if err := s.Connect(context.Background()); err != nil {
		t.Fatalf("подъём: %v", err)
	}
	slomat.Store(true)
	dozhdatsya(t, "туннель опущен после второй паники", func() bool {
		return s.Status().Sostoyanie == protokol.SostNeNeset
	})
	s.mu.Lock()
	idyot, port := s.vosstIdyot, s.portClash
	s.mu.Unlock()
	if !idyot {
		t.Fatal("туннель опущен, а восстановление не запущено: он не вернётся сам")
	}
	if port != 0 {
		t.Fatal("состояние ne-neset, а ядро не опущено")
	}
}
