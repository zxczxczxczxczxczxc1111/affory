package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/sostoyanie"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/yadra"
)

func sluzhbaSZhurnalomSoedineniy(t *testing.T) (*Sluzhba, string, *atomic.Int32) {
	t.Helper()
	s := podstavnaya(t, nil)
	d := t.TempDir()
	s.zhurnalSoed = sostoyanie.ZhurnalSoedineniyV(d)
	s.periodZhurnala = 5 * time.Millisecond
	var oprosov atomic.Int32
	s.soedineniyaYadra = func(ctx context.Context, adres, sekret string) ([]yadra.Soedinenie, error) {
		oprosov.Add(1)
		return []yadra.Soedinenie{
			{Id: "a1", Host: "discord.com", Adres: "203.0.113.21", Port: 443, Protsess: `C:\d.exe`, Vyhod: "srv-nl", Nachalo: time.Now()},
		}, nil
	}
	s.mu.Lock()
	s.sost = protokol.SostPodnyat
	s.portClash = 9090
	s.sekretClash = "s"
	s.mu.Unlock()
	return s, filepath.Join(d, "soedineniya.jsonl"), &oprosov
}

func TestZhurnalSoedineniyVyklyuchenPoUmolchaniyu(t *testing.T) {
	s, fayl, oprosov := sluzhbaSZhurnalomSoedineniy(t)
	if s.Status().Zhurnal {
		t.Fatal("журнал включён по умолчанию")
	}
	ctx, otmena := context.WithCancel(context.Background())
	defer otmena()
	go s.vestiZhurnal(ctx)
	time.Sleep(40 * time.Millisecond)
	if oprosov.Load() != 0 {
		t.Fatal("выключенный журнал опрашивает ядро")
	}
	if _, err := os.Stat(fayl); !os.IsNotExist(err) {
		t.Fatal("файл журнала появился при выключенном журнале")
	}
}

func TestSetJournalVklyuchaetZapisIOdnoSoedinenieOdinRaz(t *testing.T) {
	s, fayl, _ := sluzhbaSZhurnalomSoedineniy(t)
	var zapisano []sostoyanie.SostoyanieFayla
	s.zapisat = func(f sostoyanie.SostoyanieFayla) error { zapisano = append(zapisano, f); return nil }
	telo, _ := json.Marshal(map[string]bool{"vkl": true})
	if o := s.Obrabotat(context.Background(), protokol.Kadr{Tip: "cmd", Id: 1, Imya: "setJournal", Telo: telo}); o.Oshib != nil {
		t.Fatalf("setJournal отвергнут: %+v", o.Oshib)
	}
	if !s.Status().Zhurnal {
		t.Fatal("статус не показывает включённый журнал")
	}
	// Настройка, а не состояние: на диск, чтобы пережить перезапуск службы.
	if len(zapisano) == 0 || !zapisano[len(zapisano)-1].Zhurnal {
		t.Fatal("флаг журнала не записан на диск")
	}
	ctx, otmena := context.WithCancel(context.Background())
	defer otmena()
	go s.vestiZhurnal(ctx)
	time.Sleep(60 * time.Millisecond)
	b, err := os.ReadFile(fayl)
	if err != nil {
		t.Fatalf("файл журнала не появился: %v", err)
	}
	if n := strings.Count(string(b), "discord.com"); n != 1 {
		t.Fatalf("одно соединение записано %d раз: %q", n, string(b))
	}
	if !strings.Contains(string(b), `d.exe`) {
		t.Fatalf("колонки процесса нет: %q", string(b))
	}
}

func TestClearJournalSnosiFayl(t *testing.T) {
	s, fayl, _ := sluzhbaSZhurnalomSoedineniy(t)
	if err := os.WriteFile(fayl, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if o := s.Obrabotat(context.Background(), protokol.Kadr{Tip: "cmd", Id: 1, Imya: "clearJournal"}); o.Oshib != nil {
		t.Fatalf("clearJournal отвергнут: %+v", o.Oshib)
	}
	if _, err := os.Stat(fayl); !os.IsNotExist(err) {
		t.Fatal("файл остался")
	}
}

func TestSbrosSostoyaniyaNeGasitZhurnal(t *testing.T) {
	// Флаг это настройка: Disconnect стирает файл состояния целиком, и без
	// исключения журнал выключался бы каждым отключением.
	s, _, _ := sluzhbaSZhurnalomSoedineniy(t)
	s.SetJournal(true)
	if err := s.sbrositSost(); err != nil {
		t.Fatal(err)
	}
	if !s.Status().Zhurnal {
		t.Fatal("сброс состояния выключил журнал")
	}
}
