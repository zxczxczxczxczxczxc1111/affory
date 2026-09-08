package main

import (
	"context"
	"log"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Раз в минуту, а не раз в секунду: перехват это не гонка, а состояние, и
// человек всё равно не читает журнал чаще.
//
// Значение ПО УМОЛЧАНИЮ, а живёт расписание полем службы. Пакетной переменной
// оно быть не может: её читает фоновая горутина, и тест, укорачивающий период,
// писал бы в неё, пока горутина читает. Детектор гонок такое находит первым же
// прогоном, а без детектора оно годами выглядит исправным.
const periodProksiPoUmolchaniyu = time.Minute

// slediZaProksi называет чужой перехват вслух и НИЧЕГО не трогает.
//
// Affory под TUN реестр не пишет вовсе, поэтому любой включённый системный
// прокси чужой по построению. Снимать его молча нельзя: это чужая настройка,
// её мог поставить человек или корпоративная политика. Наше дело назвать.
//
// Практический смысл предупреждения: пока запись стоит, браузер пойдёт в ЧУЖОЙ
// прокси мимо нашего туннеля, и внешний адрес окажется не тем, который
// показывает интерфейс. Это самая обидная форма утечки, потому что всё выглядит
// работающим.
func (s *Sluzhba) slediZaProksi(ctx context.Context) {
	soobshchali := ""
	for {
		p, err := s.prochitatProksi()
		s.mu.Lock()
		nash := s.portProksiNash
		s.mu.Unlock()
		switch {
		case err != nil:
			log.Printf("состояние системного прокси не прочитано: %v", err)
		case p.Chuzhoy(nash) && p.Adres != soobshchali:
			// Повтор при неизменившемся адресе гасится: одно и то же
			// предупреждение раз в минуту это способ научить человека его не
			// читать.
			soobshchali = p.Adres
			log.Printf("чужой системный прокси %s (%s)", p.Adres, protokol.KodForeignRegistryHijack)
			s.izvestit("proxyHijack", protokol.Oshibka{
				Kod:   protokol.KodForeignRegistryHijack,
				Tekst: "системный прокси " + p.Adres + " перехватывает трафик мимо туннеля",
			})
		case !p.Chuzhoy(nash):
			soobshchali = ""
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(s.periodProksi):
		}
	}
}
