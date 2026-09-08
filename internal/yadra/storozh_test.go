package yadra

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestStorozhNarastaet(t *testing.T) {
	// 1, 2, 4, 8, 16, then a ceiling of 30. An unbounded retry loop at zero
	// delay is not resilience, it is a bonfire made of CPU.
	hotim := []time.Duration{1, 2, 4, 8, 16, 30, 30}
	for i, zhdem := range hotim {
		if got := pauzaStorozha(i); got != zhdem*time.Second {
			t.Fatalf("попытка %d: пауза %v, ожидалась %v", i, got, zhdem*time.Second)
		}
	}
}

func TestStorozhNeUletaetVMinus(t *testing.T) {
	// The counter, not the result, is what gets clamped. 1<<63 overflows into a
	// negative duration, a negative sleep returns instantly, and the ceiling that
	// was meant to spare the CPU becomes the thing that pins it at 100 percent.
	for _, popytka := range []int{63, 64, 1000, -1} {
		p := pauzaStorozha(popytka)
		if p <= 0 {
			t.Fatalf("попытка %d дала паузу %v", popytka, p)
		}
		if p > 30*time.Second {
			t.Fatalf("попытка %d дала паузу %v, потолок 30с", popytka, p)
		}
	}
}

func TestStorozhPodnimaetUpavsheeYadro(t *testing.T) {
	// The claim "a killed core comes back" was verifiable only inside the guest.
	// Here the process is faked and the logic is real, which is the half that can
	// actually be wrong.
	staryy := zapuskatel
	defer func() { zapuskatel = staryy }()

	zapuski := make(chan *Yadro, 8)
	zapuskatel = func(imya, konfig string) (*Yadro, error) {
		y := &Yadro{imya: imya, smert: make(chan error, 1), podnyt: time.Now()}
		zapuski <- y
		return y, nil
	}

	ctx, otmena := context.WithCancel(context.Background())
	defer otmena()
	itog := make(chan error, 1)
	go func() { itog <- Storozhit(ctx, "xray.exe", "sb.json", nil) }()

	pervoe := <-zapuski
	pervoe.smert <- errors.New("ядро упало")

	select {
	case <-zapuski:
	case <-time.After(5 * time.Second):
		t.Fatal("сторож не поднял ядро заново")
	}

	otmena()
	select {
	case err := <-itog:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("сторож вернул %v, ожидалась отмена", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("сторож не остановился по отмене контекста")
	}
}

func TestStorozhSbrasyvaetSchyotchik(t *testing.T) {
	// A core that lived past minZhivoy and then died must not inherit the delay
	// of one that never started. Checked on the formula, not on the clock: the
	// alternative is a test that sleeps for a minute.
	if pauzaStorozha(0) != time.Second {
		t.Fatalf("сброшенный счётчик даёт %v, ожидалась секунда", pauzaStorozha(0))
	}
	if minZhivoy != 60*time.Second {
		t.Fatalf("порог здоровой работы %v, а пороги волны считаны от 60с", minZhivoy)
	}
}
