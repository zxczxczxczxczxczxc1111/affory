package main

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/yadra"
)

// Задача 6.1. Опрос clash_api идёт ТОЛЬКО пока кто-то подписан: свёрнутое
// окно, которое опрашивает ядро раз в секунду, это трата батареи без читателя.
// Флаг подписки и есть вся задача.

func sluzhbaSoStatistikoy(t *testing.T) (*Sluzhba, *atomic.Int32) {
	t.Helper()
	s := podstavnaya(t, nil)
	var oprosov atomic.Int32
	s.periodStat = 5 * time.Millisecond
	s.snimokStat = func(ctx context.Context, adres, sekret, teg string) (yadra.Snimok, error) {
		oprosov.Add(1)
		return yadra.Snimok{Zaderzhka: 87 * time.Millisecond, EstZaderzhka: true, Otdano: 10, Prinyato: 20}, nil
	}
	// Туннель «поднят» напрямую: подъём тут не предмет проверки.
	s.mu.Lock()
	s.sost = protokol.SostPodnyat
	s.nesushchiyId = "nl"
	s.portClash = 9090
	s.sekretClash = "s"
	s.mu.Unlock()
	return s, &oprosov
}

func podpisat(s *Sluzhba, ctx context.Context, vkl bool) protokol.Kadr {
	telo, _ := json.Marshal(map[string]bool{"vkl": vkl})
	return s.Obrabotat(ctx, protokol.Kadr{Tip: "cmd", Id: 1, Imya: "subscribeStats", Telo: telo})
}

func TestBezPodpiskiNeOprashivaem(t *testing.T) {
	s, oprosov := sluzhbaSoStatistikoy(t)
	time.Sleep(40 * time.Millisecond)
	if n := oprosov.Load(); n != 0 {
		t.Fatalf("без подписчиков ядро опрошено %d раз", n)
	}
	_ = s
}

func TestPodpiskaDayotSobytiyaIOtpiskaIhGasit(t *testing.T) {
	s, oprosov := sluzhbaSoStatistikoy(t)
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
				Zaderzhka *int `json:"zaderzhka_ms"`
				Otdano    uint64
				Prinyato  uint64
			}
			if err := json.Unmarshal(k.Telo, &st); err != nil {
				t.Fatal(err)
			}
			if st.Zaderzhka == nil || *st.Zaderzhka != 87 || st.Otdano != 10 || st.Prinyato != 20 {
				t.Fatalf("событие с чужими числами: %s", k.Telo)
			}
			goto otpiska
		case <-srok:
			t.Fatal("события stats не пришло")
		}
	}
otpiska:
	if o := podpisat(s, ctx, false); o.Oshib != nil {
		t.Fatalf("отписка отвергнута: %+v", o.Oshib)
	}
	time.Sleep(20 * time.Millisecond)
	bylo := oprosov.Load()
	time.Sleep(40 * time.Millisecond)
	if stalo := oprosov.Load(); stalo != bylo {
		t.Fatalf("после отписки опрос продолжается: было %d, стало %d", bylo, stalo)
	}
}

func TestSmertSoedineniyaSnimaetPodpisku(t *testing.T) {
	// Интерфейс, убитый диспетчером задач, отписаться не успевает. Закрытие
	// соединения обязано считаться отпиской, иначе служба опрашивает ядро
	// раз в секунду для призрака до своей перезагрузки.
	s, oprosov := sluzhbaSoStatistikoy(t)
	id, _ := s.Podpisatsya()
	ctx := sPodpischikom(context.Background(), id)
	if o := podpisat(s, ctx, true); o.Oshib != nil {
		t.Fatalf("подписка отвергнута: %+v", o.Oshib)
	}
	time.Sleep(30 * time.Millisecond)
	if oprosov.Load() == 0 {
		t.Fatal("опрос не начался")
	}
	s.Otpisatsya(id)
	time.Sleep(20 * time.Millisecond)
	bylo := oprosov.Load()
	time.Sleep(40 * time.Millisecond)
	if stalo := oprosov.Load(); stalo != bylo {
		t.Fatalf("после смерти соединения опрос продолжается: было %d, стало %d", bylo, stalo)
	}
}

func TestNeizmerennayaZaderzhkaNeNol(t *testing.T) {
	// Договор запасного пути: ноль за неизмеренное на экране запрещён, значит
	// служба обязана не присылать поле вовсе, а не присылать ноль.
	s, _ := sluzhbaSoStatistikoy(t)
	s.snimokStat = func(ctx context.Context, adres, sekret, teg string) (yadra.Snimok, error) {
		return yadra.Snimok{Otdano: 1, Prinyato: 2}, nil
	}
	id, sob := s.Podpisatsya()
	defer s.Otpisatsya(id)
	ctx := sPodpischikom(context.Background(), id)
	podpisat(s, ctx, true)
	srok := time.After(2 * time.Second)
	for {
		select {
		case k := <-sob:
			if k.Imya != "stats" {
				continue
			}
			var st map[string]any
			_ = json.Unmarshal(k.Telo, &st)
			if _, est := st["zaderzhka_ms"]; est {
				t.Fatalf("неизмеренная задержка прислана: %s", k.Telo)
			}
			return
		case <-srok:
			t.Fatal("события stats не пришло")
		}
	}
}

func TestPodpiskaBezSoedineniyaOtvergaetsya(t *testing.T) {
	s, _ := sluzhbaSoStatistikoy(t)
	if o := podpisat(s, context.Background(), true); o.Oshib == nil {
		t.Fatal("подписка без соединения принята: гасить её будет некому")
	}
}
