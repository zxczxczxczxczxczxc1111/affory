package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/genkonfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/yadra"
)

// Цифры экрана идут и до того, как urltest назовёт несущего.
//
// Поймано приёмкой 13.09.2026: сразу после подключения подписчик не получал за
// четыре секунды НИ ОДНОГО события, а следующие четыре секунды приносили все
// четыре. Группа avto называет выбор не сразу (замерено: ответ появляется
// около 5.6 с), до тех пор nesushchiyId пуст, тег кандидата собирался из
// пустого идентификатора, и запрос уходил к /proxies/srv- . Ядро отвечало
// отказом, снимок не снимался, экран стоял пустой.
//
// Счётчики трафика от тега не зависят вовсе, они из /connections. От него
// зависит только задержка, а её отсутствие договор экрана допускает.
func TestStatistikaIdyotPokaNesushchiyNeNazvan(t *testing.T) {
	s, _ := sluzhbaSoStatistikoy(t)
	s.mu.Lock()
	s.nesushchiyId = ""
	s.mu.Unlock()

	var sprosheno = make(chan string, 8)
	s.snimokStat = func(_ context.Context, _, _, teg string) (yadra.Snimok, error) {
		select {
		case sprosheno <- teg:
		default:
		}
		// Ядро отвечает отказом на тег из пустого идентификатора, ровно как в
		// госте. Спрашивать его так служба не имеет права.
		if teg == genkonfig.TegKandidata("") {
			return yadra.Snimok{}, errors.New("исходящий srv- не описан ядром (код 404)")
		}
		return yadra.Snimok{Otdano: 10, Prinyato: 20}, nil
	}

	id, sob := s.Podpisatsya()
	defer s.Otpisatsya(id)
	ctx := sPodpischikom(context.Background(), id)
	if o := podpisat(s, ctx, true); o.Oshib != nil {
		t.Fatalf("подписка отвергнута: %+v", o.Oshib)
	}

	srok := time.After(2 * time.Second)
	for {
		select {
		case <-srok:
			var teg string
			select {
			case teg = <-sprosheno:
			default:
			}
			t.Fatalf("подписчик не получил ни одного события, пока несущий не назван; служба спрашивала тег %q", teg)
		case k := <-sob:
			if k.Imya == "stats" {
				return
			}
		}
	}
}

// Названный несущий по-прежнему спрашивается своим тегом: подмена группой
// нужна ТОЛЬКО пока имени нет.
func TestStatistikaSprashivaetNesushchegoKogdaOnNazvan(t *testing.T) {
	s, _ := sluzhbaSoStatistikoy(t)
	tegi := make(chan string, 4)
	s.snimokStat = func(_ context.Context, _, _, teg string) (yadra.Snimok, error) {
		select {
		case tegi <- teg:
		default:
		}
		return yadra.Snimok{Otdano: 1, Prinyato: 2}, nil
	}
	id, sob := s.Podpisatsya()
	defer s.Otpisatsya(id)
	ctx := sPodpischikom(context.Background(), id)
	if o := podpisat(s, ctx, true); o.Oshib != nil {
		t.Fatalf("подписка отвергнута: %+v", o.Oshib)
	}
	select {
	case <-sob:
	case <-time.After(2 * time.Second):
		t.Fatal("событий нет вовсе")
	}
	select {
	case teg := <-tegi:
		if teg != genkonfig.TegKandidata("nl") {
			t.Errorf("несущий назван nl, а спрошен тег %q", teg)
		}
	default:
		t.Fatal("тег не записан")
	}
	_ = protokol.SostPodnyat
}
