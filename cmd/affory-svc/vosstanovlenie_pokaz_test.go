package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Приёмка 1.9.0 в госте, 30.09.2026. Пока служба сама восстанавливала
// туннель, наружу уходил круг podnimaetsya, otkaz и vyklyuchen: трей
// предлагал «Повторить» и «Подключить», а «выключено» читалось как «VPN
// выключен и сам не встанет», хотя попытки шли без человека.

// sobytiyaStatusa копит статусы из событий state. vzglyanut отдаёт
// накопленное на ходу, sobrat отписывается и отдаёт всё.
func sobytiyaStatusa(s *Sluzhba) (vzglyanut, sobrat func() []protokol.StatusOtvet) {
	id, c := s.Podpisatsya()
	var mu sync.Mutex
	var vse []protokol.StatusOtvet
	gotovo := make(chan struct{})
	go func() {
		defer close(gotovo)
		for k := range c {
			if k.Imya != "state" {
				continue
			}
			var st protokol.StatusOtvet
			if err := json.Unmarshal(k.Telo, &st); err != nil {
				continue
			}
			mu.Lock()
			vse = append(vse, st)
			mu.Unlock()
		}
	}()
	vzglyanut = func() []protokol.StatusOtvet {
		mu.Lock()
		defer mu.Unlock()
		return append([]protokol.StatusOtvet(nil), vse...)
	}
	sobrat = func() []protokol.StatusOtvet {
		s.Otpisatsya(id)
		<-gotovo
		return vzglyanut()
	}
	return vzglyanut, sobrat
}

// sobytiyaSostoyaniya то же, но одними состояниями.
func sobytiyaSostoyaniya(s *Sluzhba) func() []protokol.Sostoyanie {
	_, sobrat := sobytiyaStatusa(s)
	return func() []protokol.Sostoyanie {
		var vse []protokol.Sostoyanie
		for _, st := range sobrat() {
			vse = append(vse, st.Sostoyanie)
		}
		return vse
	}
}

// pervayaPrichinaVosstanovleniya ждёт первое событие vosstanavlivaetsya с
// причиной. Причина живёт рядом с состоянием, и следующая попытка быстро
// сменяет её своей: опросом её можно проскочить, потоком событий нет.
func pervayaPrichinaVosstanovleniya(t *testing.T, vzglyanut func() []protokol.StatusOtvet) *protokol.Oshibka {
	t.Helper()
	var prichina *protokol.Oshibka
	dozhdatsya(t, "восстановление с причиной", func() bool {
		for _, st := range vzglyanut() {
			if st.Sostoyanie == protokol.SostVosstanavl && st.Oshib != nil {
				prichina = st.Oshib
				return true
			}
		}
		return false
	})
	return prichina
}

func TestVosstanovlenieNaruzhuOdnoSostoyanie(t *testing.T) {
	s, proverok, _ := sluzhbaSProverkoy(t, errors.New("тест: сервер лёг"))
	s.otstupy = []time.Duration{time.Millisecond}
	s.postavit(protokol.SostNeNeset, nil)
	sobrat := sobytiyaSostoyaniya(s)
	s.zapustitVosstanovlenie()
	dozhdatsya(t, "три попытки восстановления", func() bool { return proverok.Load() >= 3 })
	st := s.Status()
	vse := sobrat()
	if st.Sostoyanie != protokol.SostVosstanavl {
		t.Fatalf("посреди восстановления снаружи %s", st.Sostoyanie)
	}
	if st.Oshib == nil || st.Oshib.Kod != protokol.KodAllServersDown {
		t.Fatalf("причина последнего отказа не видна: %+v", st.Oshib)
	}
	if len(vse) == 0 {
		t.Fatal("ни одного события за три попытки")
	}
	for _, x := range vse {
		if x != protokol.SostVosstanavl {
			t.Fatalf("наружу ушло %s посреди восстановления: %v", x, vse)
		}
	}
}

func TestVosstanovlenieNazyvaetSleduyushchuyuPopytku(t *testing.T) {
	s, _, _ := sluzhbaSProverkoy(t, errors.New("тест: сервер лёг"))
	s.otstupy = []time.Duration{time.Hour}
	s.postavit(protokol.SostOtkaz, &protokol.Oshibka{Kod: protokol.KodAllServersDown, Tekst: "тест: сервер лёг"})
	nach := time.Now()
	s.zapustitVosstanovlenie()
	dozhdatsya(t, "время следующей попытки", func() bool { return s.Status().SledPopytka != nil })
	st := s.Status()
	if st.Sostoyanie != protokol.SostVosstanavl {
		t.Fatalf("на паузе восстановления снаружи %s", st.Sostoyanie)
	}
	if d := st.SledPopytka.Sub(nach); d < 59*time.Minute || d > 61*time.Minute {
		t.Fatalf("следующая попытка через %v, пауза час", d)
	}
	if st.Oshib == nil || st.Oshib.Kod != protokol.KodAllServersDown {
		t.Fatalf("причина прошлого отказа пропала на паузе: %+v", st.Oshib)
	}
}

func TestOtklyuchitSnimaetVosstanovlenieSrazu(t *testing.T) {
	s, _, _ := sluzhbaSProverkoy(t, errors.New("тест: сервер лёг"))
	s.otstupy = []time.Duration{time.Hour}
	s.postavit(protokol.SostOtkaz, &protokol.Oshibka{Kod: protokol.KodAllServersDown, Tekst: "тест: сервер лёг"})
	s.zapustitVosstanovlenie()
	// Человек жмёт «Отключить» посреди паузы: там цикл и стоит почти всё время.
	dozhdatsya(t, "пауза восстановления", func() bool { return s.Status().SledPopytka != nil })
	sobrat := sobytiyaSostoyaniya(s)
	s.Otklyuchit()
	st := s.Status()
	vse := sobrat()
	if st.Sostoyanie != protokol.SostVyklyuchen || st.Oshib != nil || st.SledPopytka != nil {
		t.Fatalf("после «Отключить» %s, ошибка %+v, попытка %v", st.Sostoyanie, st.Oshib, st.SledPopytka)
	}
	for _, x := range vse {
		if x != protokol.SostVyklyuchen {
			t.Fatalf("«Отключить» показало по пути %s: %v", x, vse)
		}
	}
}

func TestPosleVosstanovleniyaOtkazSnovaOtkaz(t *testing.T) {
	s := podstavnaya(t, nil)
	var ostalos atomic.Int32
	ostalos.Store(2)
	s.proveritServer = func(context.Context) error {
		if ostalos.Add(-1) >= 0 {
			return errors.New("тест: сервер ещё лежит")
		}
		return nil
	}
	s.otstupy = []time.Duration{time.Millisecond}
	s.postavit(protokol.SostNeNeset, nil)
	s.zapustitVosstanovlenie()
	dozhdatsya(t, "туннель вернулся", func() bool { return s.Status().Sostoyanie == protokol.SostPodnyat })
	dozhdatsya(t, "восстановление закончилось", func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		return !s.vosstIdyot
	})
	if st := s.Status(); st.Sostoyanie != protokol.SostPodnyat || st.SledPopytka != nil {
		t.Fatalf("после возврата %s, попытка %v", st.Sostoyanie, st.SledPopytka)
	}
	// Ручной отказ вне восстановления называется отказом, как прежде.
	s.Otklyuchit()
	s.nabor = func() (Nabor, error) { return Nabor{}, nil }
	if err := s.Connect(context.Background()); err == nil {
		t.Fatal("подъём без серверов ответил успехом")
	}
	if got := s.Status().Sostoyanie; got != protokol.SostOtkaz {
		t.Fatalf("ручной отказ снаружи %s", got)
	}
}

// Status это то, что служба ПОКАЗЫВАЕТ: во время восстановления там
// vosstanavlivaetsya вместо внутреннего круга. Решать по нему нельзя:
// postavit(s.Status().Sostoyanie, ...) записал бы маску внутрь, и цикл
// остановился бы на собственной проверке состояния. Решения идут через vnutri.
func TestResheniyaNeChitayutPokaznoySostoyanie(t *testing.T) {
	fayly, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range fayly {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, stroka := range strings.Split(string(b), "\n") {
			if strings.Contains(stroka, "Status().Sostoyanie") || strings.Contains(stroka, "Status().Oshib") {
				t.Errorf("%s:%d решает по показному статусу: %s", f, i+1, strings.TrimSpace(stroka))
			}
		}
	}
}
