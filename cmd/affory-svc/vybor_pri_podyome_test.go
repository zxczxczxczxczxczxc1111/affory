package main

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/genkonfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Ядро ЗАПОМИНАЕТ выбранный исходящий и перебивает им `default` из конфига.
//
// Документация sing-box про store_selected: «включено по умолчанию, когда задан
// cache_file.enabled», а он у нас задан. Значит выбор человека, уехавший в
// конфиг полем default, при старте ядра проигрывает тому, что ядро выбирало в
// прошлый раз, и переписать это может только clash_api.
//
// Найдено живым прогоном 04.09.2026 и воспроизведено дважды на чистом наборе из
// четырёх серверов: просили один сервер, несущим выходил кэшированный. Экран
// при этом показывал верный vybran_id, то есть разойтись набору и трафику было
// нечем помешать, а человеку нечем заметить.
func podnyatISobratVybory(t *testing.T, n Nabor) []string {
	t.Helper()
	s := podstavnaya(t, nil)
	if err := s.zapisatNabor(n); err != nil {
		t.Fatalf("набор не записан: %v", err)
	}
	var mu sync.Mutex
	var tegi []string
	s.postavitVybor = func(_ context.Context, _, _, gruppa, teg string) error {
		mu.Lock()
		defer mu.Unlock()
		if gruppa != genkonfig.TegSelector {
			t.Errorf("выбор ставится группе %q, а селектор это %q", gruppa, genkonfig.TegSelector)
		}
		tegi = append(tegi, teg)
		return nil
	}
	if err := s.Connect(context.Background()); err != nil {
		t.Fatalf("подъём: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	return append([]string(nil), tegi...)
}

func TestPodyomNavyazyvaetYadruRuchnoyVybor(t *testing.T) {
	tegi := podnyatISobratVybory(t, Nabor{
		Servery: []protokol.Server{serverProby(), vtoroyServer()},
		Vybran:  "de",
		Rezhim:  protokol.RezhimRuchnoy,
	})
	zhdyom := genkonfig.TegKandidata("de")
	if len(tegi) == 0 {
		t.Fatalf("подъём не поставил выбор в ядре: трафик пойдёт через то, что ядро запомнило, а не через %q", zhdyom)
	}
	if tegi[len(tegi)-1] != zhdyom {
		t.Fatalf("последним поставлен %q, а выбран %q", tegi[len(tegi)-1], zhdyom)
	}
}

func TestPodyomNavyazyvaetYadruAvto(t *testing.T) {
	// Зеркало, и оно не для симметрии: кэш ядра держит КОНКРЕТНЫЙ сервер, и в
	// авто он точно так же перебил бы группу urltest. Автоматический режим тогда
	// молча выродился бы в один навсегда выбранный сервер.
	tegi := podnyatISobratVybory(t, Nabor{
		Servery: []protokol.Server{serverProby(), vtoroyServer()},
		Rezhim:  protokol.RezhimAvto,
	})
	if len(tegi) == 0 {
		t.Fatal("подъём в авто не поставил группу: ядро останется на запомненном сервере")
	}
	if tegi[len(tegi)-1] != genkonfig.TegAvto {
		t.Fatalf("последним поставлен %q, а в авто ждём %q", tegi[len(tegi)-1], genkonfig.TegAvto)
	}
}

// Не доехавший до ядра выбор это ЗАКРЫТЫЙ отказ, а не предупреждение.
//
// Два теста выше доказывают, что подъём выбор СТАВИТ. Этот доказывает, что он
// делает с неудачей: поднятый туннель через чужой сервер хуже неподнятого.
// Человек выбрал страну, и отдать ему другую молча значит соврать ровно в том,
// ради чего программу ставят.
func TestPodyomPadaetEsliVyborNeDoehal(t *testing.T) {
	s := podstavnaya(t, nil)
	if err := s.zapisatNabor(Nabor{
		Servery: []protokol.Server{serverProby(), vtoroyServer()},
		Vybran:  "de",
		Rezhim:  protokol.RezhimRuchnoy,
	}); err != nil {
		t.Fatalf("набор не записан: %v", err)
	}
	s.postavitVybor = func(context.Context, string, string, string, string) error {
		return errors.New("ядро не приняло PUT")
	}

	o := s.Obrabotat(ctxAdmina(), protokol.Kadr{Tip: "cmd", Id: 1, Imya: "connect"})
	if o.Oshib == nil {
		t.Fatal("подъём объявлен удачным, хотя выбор до ядра не доехал")
	}
	if kod := o.Oshib.Kod; kod != protokol.KodPereklyuchenieNeDoehalo {
		t.Errorf("код отказа %q, ожидали %q", kod, protokol.KodPereklyuchenieNeDoehalo)
	}
	if sost := s.Status().Sostoyanie; sost != protokol.SostOtkaz {
		t.Errorf("состояние после отказа %q, ожидали %q", sost, protokol.SostOtkaz)
	}
}
