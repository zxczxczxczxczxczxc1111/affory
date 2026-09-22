package main

import (
	"context"
	"log"
	"net/netip"
	"time"
)

// Смена сети под поднятым туннелем (A4, 22.09.2026).
//
// Адрес местного резолвера выбирается ОДИН раз, при сборке конфига, и уезжает
// в ядро строкой. Горячей перезагрузки конфига у ядра нет. Машину переносят в
// другую сеть - адрес остаётся прежним и указывает в пустоту.
//
// Чем это стоит человеку, измерено в госте 22.09.2026. Через местный резолвер
// идёт ВЕСЬ российский набор (`rule_set=ru => route(mestnyy)`): в лаборатории с
// недоступным местным резолвером rt.ru, kinopoisk.ru, avito.ru и
// wildberries.ru не разрешились вовсе, каждый отказ по 12 секунд, а
// stackoverflow.com через туннель отвечал за 0.1 с. Запасного сервера у
// правила нет: ядро пишет `match[2] rule_set=ru => route(mestnyy)` и молчит.
//
// Туннель при этом ЖИВ: `auto_detect_interface` у ядра сам находит новый
// выход, внешний адрес остаётся прежним, и HTTP-проба наблюдателя зелёная.
// Поэтому заметить смену сети может только тот, кто спрашивает про неё прямо.

// perezapusSetiNeChashche это нижняя граница между переподъёмами по смене сети.
//
// Сеть дребезжит: переключение Wi-Fi на провод, пробуждение и смена профиля
// дают несколько изменений подряд. Без границы продукт рвал бы соединения на
// каждое из них, то есть чинил бы одно и ломал другое.
const perezapusSetiNeChashche = 30 * time.Second

// rezolverSmenilsya отвечает, стоит ли пересобирать конфиг под новую сеть.
//
// Отвечает false, когда резолвер НЕ ЧИТАЕТСЯ: сеть могли просто выдернуть, и
// переподъём на машине без сети это попытка подняться в пустоту. Туннель в
// этом случае разберётся обычным путём - пробой живости и восстановлением.
func (s *Sluzhba) rezolverSmenilsya() (netip.Addr, bool) {
	s.mu.Lock()
	bylo := s.rezolverKonfiga
	s.mu.Unlock()
	if !bylo.IsValid() {
		return netip.Addr{}, false
	}
	stalo, err := s.mestnyyRezolver()
	if err != nil {
		return netip.Addr{}, false
	}
	if stalo == bylo {
		return netip.Addr{}, false
	}
	return stalo, true
}

// perezapustitPodNovuyuSet пересобирает конфиг под новую сеть.
//
// Зовётся из наблюдателя своей горутиной на фоновом контексте: контекст
// наблюдателя отменяет Disconnect, который случится внутри, и переподъём умер
// бы в момент рождения - той же граблей, что уже описана у vosstanavlivat.
//
// Кандидат проверяется ядром ДО остановки рабочего подключения (A6), поэтому
// сеть, в которой новый конфиг не собирается, не стоит человеку туннеля.
func (s *Sluzhba) perezapustitPodNovuyuSet(stalo netip.Addr) {
	s.mu.Lock()
	bylo := s.rezolverKonfiga
	s.posledniyPerezapuskSeti = s.seychas()
	s.mu.Unlock()
	log.Printf("сеть сменилась: местный резолвер был %s, стал %s; пересобираю конфиг", bylo, stalo)

	if !s.zavestiFonovuyu() {
		return
	}
	go func() {
		defer s.fon.Done()
		if err := s.perepodklyuchit(s.fonCtx); err != nil {
			// Не авария: туннель либо цел (кандидат забракован до остановки),
			// либо уже поднимается заново. Наблюдатель и восстановление делают
			// свою работу дальше, а молчать про причину нельзя.
			log.Printf("переподъём под новую сеть не прошёл: %v", err)
			return
		}
		log.Printf("конфиг пересобран под новую сеть, местный резолвер %s", stalo)
	}()
}

// poraSmotretSet держит паузу между переподъёмами по смене сети.
func (s *Sluzhba) poraSmotretSet() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.seychas().Sub(s.posledniyPerezapuskSeti) >= s.perezapuskSetiNeChashche
}

// smotretSet это шаг наблюдателя. Отвечает true, когда переподъём запущен и
// наблюдателю пора уходить: туннель под ним сейчас опустят и поднимут заново.
func (s *Sluzhba) smotretSet(ctx context.Context) bool {
	if ctx.Err() != nil || !s.poraSmotretSet() {
		return false
	}
	stalo, smenilsya := s.rezolverSmenilsya()
	if !smenilsya {
		return false
	}
	s.perezapustitPodNovuyuSet(stalo)
	return true
}
