package main

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/pomehi"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Чужие программы, которые ломают VPN или могут ему мешать (1.10.0). Поиск
// живёт в пакете pomehi, здесь только когда его звать и куда класть ответ.
//
// Зовётся при подъёме, в отказе «данные не пошли» и раз в periodPomeh, чтобы
// строка на главном экране уходила сама, когда программу закрыли.

const periodPomehPoUmolchaniyu = 30 * time.Second

// obnovitPomehi проверяет машину заново, запоминает ответ и отдаёт его.
// Окно узнаёт о перемене в группе «ломает» сразу, а не через опрос.
func (s *Sluzhba) obnovitPomehi() []pomehi.Nahodka {
	if s.proveritPomehi == nil {
		return nil
	}
	s.mu.Lock()
	svoi := pomehi.Svoi{KatalogProgrammy: s.dirProgrammy}
	if s.tun.Indeks != 0 {
		svoi.Adaptery = []uint32{s.tun.Indeks}
	}
	if s.sost != protokol.SostVyklyuchen {
		svoi.ImyaAdaptera = imyaAdapteraTun
	}
	s.mu.Unlock()

	naydeno, err := s.proveritPomehi(svoi)
	opis := opisatPomehi(naydeno)
	tekstOshibki := ""
	if err != nil {
		tekstOshibki = err.Error()
	}

	s.mu.Lock()
	bylo := opisatPomehi(pomehi.Lomayushchie(s.pomehiNaydeno))
	bylOpis, bylaOshibka := s.pomehiOpis, s.pomehiOshibka
	s.pomehiNaydeno = naydeno
	s.pomehiOpis, s.pomehiOshibka = opis, tekstOshibki
	s.mu.Unlock()

	// В журнал только перемены: проверка идёт раз в 30 с, и одна и та же строка
	// за сутки заняла бы три тысячи строк.
	if opis != bylOpis {
		if opis == "" {
			log.Printf("помехи: не найдено")
		} else {
			log.Printf("помехи: %s", opis)
		}
	}
	if tekstOshibki != bylaOshibka && tekstOshibki != "" {
		log.Printf("помехи: проверка неполная: %s", tekstOshibki)
	}
	if opisatPomehi(pomehi.Lomayushchie(naydeno)) != bylo {
		s.izvestit("state", s.Status())
	}
	return naydeno
}

// opisatPomehi это строка для журнала и выгрузки: группа, название, признак.
func opisatPomehi(n []pomehi.Nahodka) string {
	chasti := make([]string, 0, len(n))
	for _, x := range n {
		chasti = append(chasti, string(x.Gruppa)+" "+x.Imya+" ("+x.Priznak+")")
	}
	return strings.Join(chasti, "; ")
}

// pomehiDlyaStatusa отдаёт группу «ломает» в виде провода. Зовётся под s.mu.
func (s *Sluzhba) pomehiDlyaStatusa() []protokol.Pomeha {
	var itog []protokol.Pomeha
	for _, x := range pomehi.Lomayushchie(s.pomehiNaydeno) {
		itog = append(itog, protokol.Pomeha{Imya: x.Imya, Vid: string(x.Vid)})
	}
	return itog
}

// podskazkaPomeh это хвост отказа «данные не пошли»: антивирус и VPN-клиенты,
// о которых окно молчало до отказа. Проверка свежая: отказ случается редко,
// а прошлый снимок мог устареть на полминуты.
func (s *Sluzhba) podskazkaPomeh() string {
	return pomehi.Podskazka(s.obnovitPomehi(), s.dirProgrammy)
}

// raspisaniePomeh крутится всё время жизни службы.
func (s *Sluzhba) raspisaniePomeh(ctx context.Context) {
	period := s.periodPomeh
	if period <= 0 {
		period = periodPomehPoUmolchaniyu
	}
	s.obnovitPomehi()
	t := time.NewTicker(period)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		s.obnovitPomehi()
	}
}
