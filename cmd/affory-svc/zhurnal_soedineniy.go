package main

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/sostoyanie"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/yadra"
)

// Журнал соединений (задача 6.2): выключен по умолчанию, живёт сутки,
// стирается кнопкой. Берётся из clash_api, уровень журнала ядра не трогается.

const (
	periodZhurnalaPoUmolchaniyu = 5 * time.Second
	periodPodrezki              = time.Hour
	// Виденные id держатся в памяти, чтобы одно соединение не попадало в
	// журнал на каждый опрос. Потолок, а не бесконечный рост: соединений за
	// день бывают десятки тысяч, и без потолка карта ест память до перезапуска.
	predelVidennyh = 20000
)

// SetJournal это НАСТРОЙКА, а не состояние: пишется на диск и переживает
// сброс файла при отключении, как флаг подключения при старте.
func (s *Sluzhba) SetJournal(vkl bool) {
	s.pravitSost(func(f *sostoyanie.SostoyanieFayla) { f.Zhurnal = vkl })
}

func (s *Sluzhba) setJournal(k protokol.Kadr) protokol.Kadr {
	var telo struct {
		Vkl bool `json:"vkl"`
	}
	if len(k.Telo) > 0 {
		if err := json.Unmarshal(k.Telo, &telo); err != nil {
			return otkaz(k.Id, k.Imya, protokol.KodProtocolMismatch, err.Error())
		}
	}
	s.SetJournal(telo.Vkl)
	return otvet(k.Id, k.Imya, s.Status())
}

func (s *Sluzhba) clearJournal(k protokol.Kadr) protokol.Kadr {
	if err := s.zhurnalSoed.Ochistit(); err != nil {
		return otkaz(k.Id, k.Imya, protokol.KodZhurnalNeStyort, err.Error())
	}
	return otvet(k.Id, k.Imya, s.Status())
}

// vestiZhurnal живёт всё время работы службы. Чистка при старте и раз в час,
// опрос ядра только при включённом журнале и поднятом туннеле.
func (s *Sluzhba) vestiZhurnal(ctx context.Context) {
	if err := s.zhurnalSoed.Podrezat(time.Now()); err != nil {
		log.Printf("журнал соединений не подрезан при старте: %v", err)
	}
	opros := time.NewTicker(s.periodZhurnala)
	defer opros.Stop()
	podrezka := time.NewTicker(periodPodrezki)
	defer podrezka.Stop()
	videno := map[string]bool{}
	skazal := false
	for {
		select {
		case <-ctx.Done():
			return
		case <-podrezka.C:
			if err := s.zhurnalSoed.Podrezat(time.Now()); err != nil {
				log.Printf("журнал соединений не подрезан: %v", err)
			}
			continue
		case <-opros.C:
		}
		s.mu.Lock()
		vkl := s.snimok.Zhurnal
		s.mu.Unlock()
		// Критерий «ядро живо» это ПУСТОЙ адрес clash_api, а не состояние.
		// Прежде здесь стояли два разных критерия подряд: состояние решало,
		// смотреть ли, а адрес решал, у кого спрашивать.
		adres, sekret := s.dostupKKlash()
		zhivo := adres != ""
		if !vkl || !zhivo {
			// Новое ядро выдаёт новые id: карта прошлого подъёма только мешает.
			if !zhivo && len(videno) > 0 {
				videno = map[string]bool{}
			}
			continue
		}
		sp, err := s.soedineniyaYadra(ctx, adres, sekret)
		if err != nil {
			if !skazal {
				log.Printf("соединения для журнала не сняты: %v", err)
				skazal = true
			}
			continue
		}
		skazal = false
		if len(videno) > predelVidennyh {
			videno = map[string]bool{}
		}
		var novye []sostoyanie.Zapis
		for _, c := range sp {
			if videno[c.Id] {
				continue
			}
			videno[c.Id] = true
			novye = append(novye, sostoyanie.Zapis{
				Vremya: c.Nachalo, Protsess: c.Protsess, Host: c.Host, Adres: c.Adres,
				Port: c.Port, Vyhod: c.Vyhod, Pravilo: c.Pravilo,
			})
		}
		if err := s.zhurnalSoed.Dopisat(novye); err != nil {
			log.Printf("журнал соединений не дописан: %v", err)
		}
	}
}

var _ = yadra.Soedineniya
