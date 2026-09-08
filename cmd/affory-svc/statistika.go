package main

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/genkonfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/yadra"
)

// Статистика (задача 6.1): событие stats раз в секунду, только при подписке.
//
// Подписка привязана к СОЕДИНЕНИЮ, а не к флагу службы. Интерфейс, убитый
// диспетчером задач, отписаться не успевает, и флаг остался бы поднятым до
// перезапуска службы; закрытие соединения (Otpisatsya) снимает подписку само.

const periodStatPoUmolchaniyu = time.Second

// Идентификатор подписчика едет в контексте соединения тем же способом, что и
// допуск: у службы одно поле на всех, а соединений много.
type klyuchPodpischika struct{}

func sPodpischikom(ctx context.Context, id uint64) context.Context {
	return context.WithValue(ctx, klyuchPodpischika{}, id)
}

func podpischikIz(ctx context.Context) uint64 {
	id, _ := ctx.Value(klyuchPodpischika{}).(uint64)
	return id
}

func (s *Sluzhba) subscribeStats(ctx context.Context, k protokol.Kadr) protokol.Kadr {
	var telo struct {
		Vkl bool `json:"vkl"`
	}
	if len(k.Telo) > 0 {
		if err := json.Unmarshal(k.Telo, &telo); err != nil {
			return otkaz(k.Id, k.Imya, protokol.KodProtocolMismatch, err.Error())
		}
	}
	id := podpischikIz(ctx)
	if id == 0 {
		// Гасить такую подписку было бы некому.
		return otkaz(k.Id, k.Imya, protokol.KodProtocolMismatch, "подписка вне соединения")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if telo.Vkl {
		s.statPodp[id] = true
		s.zapustitOprosStat()
	} else {
		delete(s.statPodp, id)
		s.pogasitOprosStat()
	}
	return otvet(k.Id, k.Imya, map[string]bool{"vkl": telo.Vkl})
}

// Под s.mu.
func (s *Sluzhba) zapustitOprosStat() {
	if s.statOtmena != nil || len(s.statPodp) == 0 {
		return
	}
	ctx, otmena := context.WithCancel(s.fonCtx)
	s.statOtmena = otmena
	go s.oprashivatStat(ctx)
}

// Под s.mu. Последний отписавшийся гасит опрос.
func (s *Sluzhba) pogasitOprosStat() {
	if len(s.statPodp) > 0 || s.statOtmena == nil {
		return
	}
	s.statOtmena()
	s.statOtmena = nil
}

func (s *Sluzhba) oprashivatStat(ctx context.Context) {
	t := time.NewTicker(s.periodStat)
	defer t.Stop()
	skazal := false
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		s.mu.Lock()
		teg := genkonfig.TegKandidata(s.nesushchiyId)
		adresVyhoda := s.adresVyhoda
		s.mu.Unlock()
		// Критерий «ядро живо» это ПУСТОЙ адрес clash_api, а не состояние. Два
		// разных критерия подряд в одной функции (состояние здесь, адрес
		// строкой ниже) это два ответа на один вопрос, и расходились они молча.
		adres, sekret := s.dostupKKlash()
		if adres == "" {
			continue
		}
		sn, err := s.snimokStat(ctx, adres, sekret, teg)
		if err != nil {
			// Один раз в журнал, а не раз в секунду: ошибка тут повторяется, пока
			// не изменится состояние, и забить журнал ею проще простого.
			if !skazal {
				log.Printf("статистика не снята: %v", err)
				skazal = true
			}
			continue
		}
		skazal = false
		st := map[string]any{"otdano": sn.Otdano, "prinyato": sn.Prinyato, "adres_vyhoda": adresVyhoda}
		// Неизмеренная задержка не присылается вовсе: ноль за неизмеренное на
		// экране запрещён договором запасного пути.
		if sn.EstZaderzhka {
			st["zaderzhka_ms"] = sn.Zaderzhka.Milliseconds()
		}
		s.izvestit("stats", st)
	}
}

var _ = yadra.Statistika
