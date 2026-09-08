package main

import (
	"context"
	"log"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/set"
)

// checkExitIp и checkLeaks (задача 6.3, долг 2).
//
// Запрос адреса выхода идёт через локальный mixed-вход sing-box, а не из
// службы напрямую: собственные процессы службы по правилу петли ходят МИМО
// туннеля, и прямой запрос показал бы домашний адрес при любом состоянии
// туннеля. Эндпоинт из файла состояния (adres_proverki), умолчание
// set.AdresProverkiPoUmolchaniyu.

// sNaborom: адрес несущего сервера нужен только тексту checkLeaks. Фоновое
// обновление адреса выхода обходится без похода в хранилище.
func (s *Sluzhba) vhodProverki(sNaborom bool) set.VhodProverki {
	// Критерий «ядро живо» это ПУСТОЙ адрес clash_api, а не состояние. Разница
	// здесь не косметическая: по состоянию проверка утечек отказывалась
	// смотреть в podnimaetsya, то есть ровно там, где адаптер уже поднят
	// (komandy.go), а утечка вероятнее всего.
	//
	// Спрашивается ДО замка: dostupKKlash берёт тот же s.mu, и вложенный вызов
	// это тупик, а не медленный путь.
	adres, _ := s.dostupKKlash()
	s.mu.Lock()
	v := set.VhodProverki{
		Podnyat:       adres != "",
		PortProksi:    s.portProksiNash,
		Endpoint:      s.adresProverki,
		SprositVyhod:  s.sprositVyhod,
		IPv6Zaglushen: s.ipv6Zaglushen,
	}
	id := s.nesushchiyId
	s.mu.Unlock()
	// Набор читается ВНЕ замка: это поход в хранилище. Адрес несущего нужен
	// только для текста «это адрес сервера», отсутствие проверке не мешает.
	if sNaborom && id != "" {
		if n, err := s.nabor(); err == nil {
			for _, srv := range n.Servery {
				if srv.Id == id {
					v.AdresServera = srv.Host
				}
			}
		}
	}
	return v
}

func (s *Sluzhba) checkExitIp(ctx context.Context, k protokol.Kadr) protokol.Kadr {
	v := s.vhodProverki(false)
	if !v.Podnyat {
		adres, err := v.SprositVyhod(ctx, v.Endpoint, 0)
		if err != nil {
			return otkaz(k.Id, k.Imya, protokol.KodVyhodNeIzmeren, err.Error())
		}
		return otvet(k.Id, k.Imya, map[string]string{"adres": adres, "cherez": "napryamuyu"})
	}
	if v.PortProksi == 0 {
		return otkaz(k.Id, k.Imya, protokol.KodVyhodNeIzmeren, "локальный прокси не поднят, через туннель спросить нечем")
	}
	adres, err := v.SprositVyhod(ctx, v.Endpoint, v.PortProksi)
	if err != nil {
		return otkaz(k.Id, k.Imya, protokol.KodVyhodNeIzmeren, err.Error())
	}
	s.zapomnitAdresVyhoda(adres)
	return otvet(k.Id, k.Imya, map[string]string{"adres": adres, "cherez": "tunnel"})
}

func (s *Sluzhba) checkLeaks(ctx context.Context, k protokol.Kadr) protokol.Kadr {
	r := set.ProveritUtechki(ctx, s.vhodProverki(true))
	if r.AdresVyhoda != "" {
		s.zapomnitAdresVyhoda(r.AdresVyhoda)
	}
	return otvet(k.Id, k.Imya, r)
}

func (s *Sluzhba) zapomnitAdresVyhoda(adres string) {
	s.mu.Lock()
	s.adresVyhoda = adres
	s.mu.Unlock()
}

// obnovitAdresVyhoda дёргается подъёмом и сменой несущего (§5: по кнопке и при
// смене сервера, не по таймеру). В фоне и молча: экран узнает через stats.
func (s *Sluzhba) obnovitAdresVyhoda() {
	v := s.vhodProverki(false)
	if !v.Podnyat || v.PortProksi == 0 {
		return
	}
	ctx, otm := context.WithTimeout(s.fonCtx, 20*time.Second)
	defer otm()
	adres, err := v.SprositVyhod(ctx, v.Endpoint, v.PortProksi)
	if err != nil {
		log.Printf("адрес выхода не обновлён: %v", err)
		return
	}
	s.zapomnitAdresVyhoda(adres)
}
