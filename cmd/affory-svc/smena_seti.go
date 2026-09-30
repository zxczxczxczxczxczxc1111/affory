package main

import (
	"context"
	"log"
	"net/netip"
	"slices"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/fon"
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

// mestnyyBezTunnelya спрашивает резолвер СОСЕДНЕЙ сети, исключая свой туннель.
//
// Исключение обязательно, и это измерено, а не предположено. Адаптер tun0
// несёт маршрут по умолчанию и объявляет СВОЙ адрес сервером имён (172.19.0.2),
// то есть проходит по всем признакам кандидата. Без исключения продукт видит
// собственный туннель как новую сеть: 22.09.2026 в госте это дало переподъём
// каждые тридцать секунд по кругу, с разрывом всех соединений на каждом.
// Конфиг при этом собирался на опущенном туннеле, то есть снова с домашним
// резолвером, и следующий тик снова объявлял смену сети.
//
// Туннель ещё не поднят - индекса нет, исключать нечего.
func (s *Sluzhba) mestnyyBezTunnelya() (netip.Addr, error) {
	s.mu.Lock()
	indeks := s.tun.Indeks
	s.mu.Unlock()
	if indeks == 0 {
		return s.mestnyyRezolver()
	}
	return s.mestnyyRezolver(indeks)
}

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
	stalo, err := s.mestnyyBezTunnelya()
	if err != nil {
		return netip.Addr{}, false
	}
	if stalo == bylo {
		return netip.Addr{}, false
	}
	// В конфиг мог уехать не первый DNS, а первый ответивший (M9 аудита
	// 1.8.0). Пока он среди объявленных адаптерами, сеть та же: иначе
	// молчащий первый DNS гонял бы переподъём по кругу.
	if slices.Contains(s.dopolnitRezolvery(stalo), bylo) {
		return netip.Addr{}, false
	}
	return stalo, true
}

// Худший случай выбора резолвера, когда молчат все: probRezolverovMaks
// вопросов по srokProbyRezolvera.
const (
	srokProbyRezolvera = time.Second
	probRezolverovMaks = 4
)

// dopolnitRezolvery отдаёт DNS физического канала: первым pervyy, тот же,
// что у mestnyyBezTunnelya, дальше остальные объявленные адаптерами.
func (s *Sluzhba) dopolnitRezolvery(pervyy netip.Addr) []netip.Addr {
	s.mu.Lock()
	indeks := s.tun.Indeks
	s.mu.Unlock()
	var krome []uint32
	if indeks != 0 {
		krome = []uint32{indeks}
	}
	spisok := []netip.Addr{pervyy}
	vse, err := s.mestnyeRezolvery(krome...)
	if err != nil {
		// Первый уже прочитан: без остальных выбор беднее, но он есть.
		log.Printf("список DNS адаптеров не прочитан, остаётся первый: %v", err)
		return spisok
	}
	for _, r := range vse {
		if !slices.Contains(spisok, r) {
			spisok = append(spisok, r)
		}
	}
	return spisok
}

// rezolverDlyaKonfiga берёт первый ответивший DNS физического канала (M9
// аудита 1.8.0). Windows при молчащем первом DNS сама уходит на второй, а
// ядро с одним первым оставалось без имён, и туннель не поднимался.
//
// Никто не ответил: первый, как было. Молчат все чаще в выдернутой сети,
// чем при мёртвых DNS, и отказ здесь ронял бы подъём, который прежде доходил
// до пробы и говорил о причине сам.
func (s *Sluzhba) rezolverDlyaKonfiga() (netip.Addr, error) {
	pervyy, err := s.mestnyyBezTunnelya()
	if err != nil {
		return netip.Addr{}, err
	}
	spisok := s.dopolnitRezolvery(pervyy)
	if len(spisok) == 1 {
		// Выбирать не из чего, и вопрос только задержал бы подъём.
		return pervyy, nil
	}
	for i, r := range spisok {
		if i == probRezolverovMaks {
			break
		}
		ctx, otmena := context.WithTimeout(context.Background(), srokProbyRezolvera)
		err := s.sprositRezolver(ctx, r)
		otmena()
		if err == nil {
			if i > 0 {
				log.Printf("местный резолвер %s молчит, в конфиг идёт %s", pervyy, r)
			}
			return r, nil
		}
		log.Printf("DNS %s не ответил: %v", r, err)
	}
	return pervyy, nil
}

// perezapustitPodNovuyuSet пересобирает конфиг под новую сеть и отвечает, ушёл
// ли туннель в переподъём.
//
// Кандидат проверяется ядром ДО остановки рабочего подключения (A6), и
// проверяется ЗДЕСЬ, в горутине наблюдателя (А2 аудита 1.8.0). Прежде проверка
// шла в фоне, а наблюдатель уходил сразу: забракованный кандидат оставлял
// старый туннель работать без единой пробы, без реакции на следующую смену
// сети и без восстановления. Теперь наблюдатель уходит, только когда туннель
// действительно опускают.
//
// Сам переподъём идёт своей горутиной на фоновом контексте: контекст
// наблюдателя отменяет Disconnect, который случится внутри, и переподъём умер
// бы в момент рождения - той же граблей, что уже описана у vosstanavlivat. Его
// провал уходит в восстановление (smenitPodklyuchenie).
func (s *Sluzhba) perezapustitPodNovuyuSet(stalo netip.Addr) bool {
	s.mu.Lock()
	bylo := s.rezolverKonfiga
	s.posledniyPerezapuskSeti = s.seychas()
	s.mu.Unlock()
	log.Printf("сеть сменилась: местный резолвер был %s, стал %s; пересобираю конфиг", bylo, stalo)

	if err := s.proveritKandidata(); err != nil {
		// Туннель цел, наблюдатель остаётся при нём. Повтор после паузы
		// perezapuskSetiNeChashche: сеть могла ещё не договорить.
		log.Printf("конфиг под новую сеть не принят, туннель остаётся прежним: %v", err)
		return false
	}
	if !s.zavestiFonovuyu() {
		return false
	}
	fon.Zapustit("переподъёме под новую сеть", func() {
		defer s.fon.Done()
		s.muPerepodyom.Lock()
		defer s.muPerepodyom.Unlock()
		if err := s.smenitPodklyuchenie(s.fonCtx, true); err != nil {
			log.Printf("переподъём под новую сеть не прошёл: %v", err)
			return
		}
		log.Printf("конфиг пересобран под новую сеть, местный резолвер %s", stalo)
	})
	return true
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
	return s.perezapustitPodNovuyuSet(stalo)
}
