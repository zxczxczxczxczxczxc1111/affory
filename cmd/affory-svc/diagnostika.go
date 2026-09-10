package main

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/diagnostika"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/genkonfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/sostoyanie"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/yadra"
)

// Подробный журнал для отладки. Выключен по умолчанию, настройка живёт на
// диске: включать его заново после каждого падения службы значит не поймать ни
// одного падения.
//
// Состав полей выбран по разбору живой машины 10.09.2026, см. шапку пакета
// internal/diagnostika. Здесь только подключение: кто зовёт и с какой частотой.

const periodDiagnostikiPoUmolchaniyu = time.Second

// SetDiagnostics это НАСТРОЙКА, как и журнал соединений: пишется на диск и
// переживает сброс файла при отключении.
func (s *Sluzhba) SetDiagnostics(vkl bool) {
	s.pravitSost(func(f *sostoyanie.SostoyanieFayla) { f.Diagnostika = vkl })
}

func (s *Sluzhba) setDiagnostics(k protokol.Kadr) protokol.Kadr {
	var telo struct {
		Vkl bool `json:"vkl"`
	}
	if len(k.Telo) > 0 {
		if err := json.Unmarshal(k.Telo, &telo); err != nil {
			return otkaz(k.Id, k.Imya, protokol.KodProtocolMismatch, err.Error())
		}
	}
	s.SetDiagnostics(telo.Vkl)
	// Включение и выключение сами по себе событие: без них в ряду появляется
	// дыра, и читающий журнал не знает, туннель встал или журнал выключили.
	if telo.Vkl {
		_ = s.zhurnalDiag.Sobytie("подробный журнал включён")
	} else {
		_ = s.zhurnalDiag.Sobytie("подробный журнал выключен")
	}
	return otvet(k.Id, k.Imya, s.Status())
}

// deltaBayt переводит накопленные с подъёма ядра счётчики в прирост за такт.
//
// Накопленный итог прячет провал: разница двух больших чисел глазами не
// читается, а вопрос ровно про одну секунду. Первый такт после подъёма ядра
// даёт нули, и это честно: сравнивать не с чем.
func (s *Sluzhba) deltaBayt(vverh, vniz uint64, soedineniy int, zaderzhka time.Duration) diagnostika.Yadro {
	y := diagnostika.Yadro{Soedineniy: soedineniy, Zaderzhka: zaderzhka}
	// Счётчики сбрасываются вместе с ядром. Отрицательная дельта значит
	// перезапуск, и ноль честнее отрицательного числа.
	if vverh >= s.byloVverh && s.byloVverh > 0 {
		y.Vverh = int64(vverh - s.byloVverh)
	}
	if vniz >= s.byloVniz && s.byloVniz > 0 {
		y.Vniz = int64(vniz - s.byloVniz)
	}
	s.byloVverh, s.byloVniz = vverh, vniz
	return y
}

// sobiratDiagnostiku крутится всё время жизни службы и молчит, пока настройка
// выключена. Отдельная горутина на включение была бы лишней гонкой: настройку
// меняют редко, а такт дешёвый.
func (s *Sluzhba) sobiratDiagnostiku(ctx context.Context) {
	period := s.periodDiagnostiki
	if period <= 0 {
		period = periodDiagnostikiPoUmolchaniyu
	}
	t := time.NewTicker(period)
	defer t.Stop()
	skazal := false
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		s.mu.Lock()
		vkl := s.snimok.Diagnostika
		s.mu.Unlock()
		if !vkl {
			continue
		}
		srez := diagnostika.Snyat(s.istochnikiDiag)
		if err := s.zhurnalDiag.Pisat(srez); err != nil {
			// Один раз в журнал службы, а не раз в секунду: отказ здесь
			// повторяется, пока не починят диск.
			if !skazal {
				log.Printf("подробный журнал не пишется: %v", err)
				skazal = true
			}
			continue
		}
		skazal = false
	}
}

// nastoyashchieIstochniki собирает швы над системой и ядром.
//
// Ядро ищется по ВЛАДЕЛЬЦУ порта clash_api, а не по имени процесса: на машине
// человека живут чужие sing-box от других клиентов, и спутать их значит мерить
// чужое. Порт наш по построению, мы его сами выдали.
func (s *Sluzhba) nastoyashchieIstochniki() diagnostika.Istochniki {
	return diagnostika.Istochniki{
		Seychas:      time.Now,
		Deskriptorov: diagnostika.DeskriptorovProtsessa,
		Porty:        diagnostika.PortyEfemernye,
		Runtime:      diagnostika.Runtime,
		PidYadra: func() int {
			s.mu.Lock()
			port := s.portClash
			s.mu.Unlock()
			if port == 0 {
				return 0
			}
			pid, err := yadra.PidPorta(port)
			if err != nil {
				return 0
			}
			return pid
		},
		Yadro: func() (diagnostika.Yadro, error) {
			adres, sekret := s.dostupKKlash()
			if adres == "" {
				// Туннеля нет, мерить нечего. Не сбой: см. Snyat.
				return diagnostika.Yadro{}, nil
			}
			ctx, otmena := context.WithTimeout(context.Background(), 3*time.Second)
			defer otmena()
			s.mu.Lock()
			teg := genkonfig.TegKandidata(s.nesushchiyId)
			s.mu.Unlock()
			nachalo := time.Now()
			sn, err := s.snimokStat(ctx, adres, sekret, teg)
			if err != nil {
				return diagnostika.Yadro{}, err
			}
			soed, err := s.soedineniyaYadra(ctx, adres, sekret)
			if err != nil {
				return diagnostika.Yadro{}, err
			}
			return s.deltaBayt(sn.Otdano, sn.Prinyato, len(soed), time.Since(nachalo)), nil
		},
	}
}
