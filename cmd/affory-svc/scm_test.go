package main

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sys/windows/svc"
)

// Г2 аудита 1.8.0: служба, докладывающая ход, ждётся дольше голого срока.
func TestZhdatSostoyaniyaProdlevaetsyaOtmetkoyHoda(t *testing.T) {
	var zaprosov atomic.Int32
	sprosit := func() (svc.Status, error) {
		n := zaprosov.Add(1)
		if n >= 10 {
			return svc.Status{State: svc.Running}, nil
		}
		return svc.Status{State: svc.StartPending, CheckPoint: uint32(n), WaitHint: 50}, nil
	}
	// Голый срок 20 мс, путь до Running 10 шагов по 5 мс: без продления отказ.
	if err := zhdatSostoyaniyaPo(sprosit, svc.Running, 20*time.Millisecond, time.Second, 5*time.Millisecond); err != nil {
		t.Fatalf("служба с растущей отметкой объявлена зависшей: %v", err)
	}
}

func TestZhdatSostoyaniyaBezHodaSdayotsya(t *testing.T) {
	sprosit := func() (svc.Status, error) {
		return svc.Status{State: svc.StartPending, CheckPoint: 1, WaitHint: 10}, nil
	}
	nach := time.Now()
	err := zhdatSostoyaniyaPo(sprosit, svc.Running, 30*time.Millisecond, time.Second, 5*time.Millisecond)
	if err == nil {
		t.Fatal("служба без хода дождалась")
	}
	if d := time.Since(nach); d > 500*time.Millisecond {
		t.Fatalf("ждали %v при сроке 30 мс", d)
	}
}

func TestZhdatSostoyaniyaDerzhitPotolok(t *testing.T) {
	var n atomic.Uint32
	sprosit := func() (svc.Status, error) {
		return svc.Status{State: svc.StartPending, CheckPoint: n.Add(1), WaitHint: 1000}, nil
	}
	nach := time.Now()
	if err := zhdatSostoyaniyaPo(sprosit, svc.Running, 10*time.Millisecond, 100*time.Millisecond, 5*time.Millisecond); err == nil {
		t.Fatal("вечно растущая отметка ждалась без конца")
	}
	if d := time.Since(nach); d > time.Second {
		t.Fatalf("потолок 100 мс, ждали %v", d)
	}
}

func TestZhdatSostoyaniyaUpavshayaSluzhbaOtkazSrazu(t *testing.T) {
	sprosit := func() (svc.Status, error) { return svc.Status{State: svc.Stopped, Win32ExitCode: 1}, nil }
	nach := time.Now()
	err := zhdatSostoyaniyaPo(sprosit, svc.Running, time.Second, time.Second, 5*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "остановилась") {
		t.Fatalf("упавшая служба: %v", err)
	}
	if d := time.Since(nach); d > 200*time.Millisecond {
		t.Fatalf("отказ через %v, а не сразу", d)
	}
}

func TestDokladSCMZamolkaetPoKomande(t *testing.T) {
	st := make(chan svc.Status, 16)
	hvatit := dokladyvatSCM(st, svc.StartPending)
	pervyy := <-st
	if pervyy.State != svc.StartPending || pervyy.CheckPoint != 1 || pervyy.WaitHint == 0 {
		t.Fatalf("первый доклад %+v", pervyy)
	}
	hvatit()
	time.Sleep(shagDokladaSCM + 200*time.Millisecond)
	select {
	case s := <-st:
		t.Fatalf("доклад после hvatit: %+v", s)
	default:
	}
}
