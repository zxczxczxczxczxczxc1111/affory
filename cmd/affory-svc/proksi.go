package main

import (
	"context"
	"log"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/set"
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
// Читаются кусты ВОШЕДШИХ ЛЮДЕЙ, а не свой. Служба живёт под LocalSystem, и её
// HKEY_CURRENT_USER это куст S-1-5-18, где прокси не бывает: до 21.09.2026
// сторож смотрел именно туда и не сработал ни разу за всё время.
func (s *Sluzhba) slediZaProksi(ctx context.Context) {
	soobshchali := ""
	for {
		lyudi, err := s.prochitatProksi()
		s.mu.Lock()
		nash := s.portProksiNash
		s.mu.Unlock()
		// Первый чужой по списку. Их бывает несколько (быстрая смена
		// пользователей, терминальный сервер), но предупреждение про один это
		// уже повод открыть настройки, а перечисление всех превратило бы
		// сообщение в простыню.
		chuzhoy := set.ProksiCheloveka{}
		for _, p := range lyudi {
			if p.Chuzhoy(nash) {
				chuzhoy = p
				break
			}
		}
		switch {
		case err != nil:
			log.Printf("состояние системного прокси не прочитано: %v", err)
		case chuzhoy.Adres != "" && chuzhoy.Adres != soobshchali:
			// Повтор при неизменившемся адресе гасится: одно и то же
			// предупреждение раз в минуту это способ научить человека его не
			// читать.
			soobshchali = chuzhoy.Adres
			log.Printf("чужой системный прокси %s у %s (%s)", chuzhoy.Adres, chuzhoy.Sid, protokol.KodForeignRegistryHijack)
			s.izvestit("proxyHijack", protokol.Oshibka{
				Kod:   protokol.KodForeignRegistryHijack,
				Tekst: "системный прокси " + chuzhoy.Adres + " перехватывает трафик мимо VPN",
			})
		case chuzhoy.Adres == "":
			soobshchali = ""
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(s.periodProksi):
		}
	}
}
