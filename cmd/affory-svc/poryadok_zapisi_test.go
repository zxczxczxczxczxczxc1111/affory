package main

import (
	"math/rand/v2"
	"sync"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/sostoyanie"
)

// Н8 аудита 1.6.1. Снимок файла состояния снимался под s.mu, а писался уже
// без замка: два одновременных изменения ложились на диск в любом порядке, и
// более старый снимок затирал более новый. Последняя запись на диске обязана
// совпадать с тем, что служба держит в памяти.
func TestZapisiFaylaIdutVPoryadkeSnimkov(t *testing.T) {
	s := podstavnaya(t, nil)
	var mu sync.Mutex
	var poslednyaya sostoyanie.SostoyanieFayla
	s.zapisat = func(f sostoyanie.SostoyanieFayla) error {
		// Разная длительность записи и есть то, что переставляло их местами.
		time.Sleep(time.Duration(rand.IntN(3)) * time.Millisecond)
		mu.Lock()
		poslednyaya = f
		mu.Unlock()
		return nil
	}
	var wg sync.WaitGroup
	for i := 1; i <= 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = s.pravitSost(func(f *sostoyanie.SostoyanieFayla) { f.PolosaVverh = i })
		}()
	}
	wg.Wait()
	s.mu.Lock()
	vPamyati := s.snimok.PolosaVverh
	s.mu.Unlock()
	if poslednyaya.PolosaVverh != vPamyati {
		t.Fatalf("на диске %d, в памяти %d: старый снимок лёг поверх нового", poslednyaya.PolosaVverh, vPamyati)
	}
}
