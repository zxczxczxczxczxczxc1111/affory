package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Сверка часов машины (1.10.0). Сама сверка в set.RashozhdenieChasov, здесь
// когда её звать, где держать ответ и как он меняет отказ подключения.
//
// Порог 120 с: VMess отвергает запрос старше 90 с, и всё, что больше двух
// минут, уже точно мешает. Меньшее расхождение называть незачем, человек
// начнёт чинить исправное.

const (
	porogChasov               = 120 * time.Second
	periodChasovPoUmolchaniyu = 6 * time.Hour
	// Служба стартует раньше сети, и первые сверки не удаются. Пять повторов
	// раз в минуту, потом обычный период.
	povtorChasov     = time.Minute
	povtorovChasov   = 5
	srokSverkiChasov = 5 * time.Second
	// Отказ подключения сверяет часы заново, если прошлой сверке больше
	// десяти минут: часы могли сбить руками минуту назад.
	svezhestChasov = 10 * time.Minute
)

// ErrChasySbity оборачивает отказ подъёма, случившийся при сбитых часах.
// Ответ команды и состояние называют его одним кодом через
// kodPodklyucheniya.
var ErrChasySbity = errors.New("часы компьютера сбиты")

// sveritChasy сверяет часы и запоминает ответ. Отказ сверки прошлый ответ
// не стирает: лучше знать вчерашнее расхождение, чем никакое.
func (s *Sluzhba) sveritChasy(ctx context.Context) (time.Duration, error) {
	ctx, otmena := context.WithTimeout(ctx, srokSverkiChasov)
	defer otmena()
	s.mu.Lock()
	s.chasyPopytka = s.seychas()
	s.mu.Unlock()
	d, err := s.zameritChasy(ctx)
	if err != nil {
		return 0, err
	}
	s.mu.Lock()
	byli := !s.chasyZamereno.IsZero() && sbity(s.chasyRashozhdenie)
	s.chasyRashozhdenie, s.chasyZamereno = d, s.seychas()
	s.mu.Unlock()
	if sbity(d) != byli || sbity(d) {
		log.Printf("часы компьютера %s точного времени", opisatRashozhdenie(d))
	}
	if sbity(d) != byli {
		s.izvestit("state", s.Status())
	}
	return d, nil
}

// raspisanieChasov сверяет часы при старте службы и дальше раз в periodChasov,
// а пока они сбиты, раз в минуту.
func (s *Sluzhba) raspisanieChasov(ctx context.Context) {
	neudach := 0
	for {
		pauza := s.periodChasov
		d, err := s.sveritChasy(ctx)
		switch {
		case err != nil:
			// В журнал первая неудача из серии: сеть без доступа к адресам
			// сверки дала бы строку каждые шесть часов навсегда, это терпимо,
			// а каждую минуту нет.
			if neudach == 0 {
				log.Printf("часы не сверены: %v", err)
			}
			neudach++
			if neudach <= povtorovChasov {
				pauza = povtorChasov
			}
		case sbity(d):
			// Сбитые часы сверяются раз в минуту: человек поправит их по
			// строке на главном, и строка обязана уйти сразу, а не через шесть
			// часов.
			pauza = povtorChasov
			neudach = 0
		default:
			neudach = 0
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(pauza):
		}
	}
}

// chasySbityDlyaOtkaza отвечает, сбиты ли часы, на момент отказа подъёма.
func (s *Sluzhba) chasySbityDlyaOtkaza() (time.Duration, bool) {
	s.mu.Lock()
	d, kogda, popytka := s.chasyRashozhdenie, s.chasyZamereno, s.chasyPopytka
	s.mu.Unlock()
	// Сбитые по прошлой сверке часы сверяются заново всегда: их могли
	// поправить минуту назад. Но не чаще раза в минуту: без сети сверка ждёт
	// свой срок до конца, и восстановление платило бы им за каждый круг.
	nuzhno := kogda.IsZero() || s.seychas().Sub(kogda) > svezhestChasov || sbity(d)
	if nuzhno && s.seychas().Sub(popytka) >= povtorChasov {
		// Контекст свой, а не команды: команда к этому моменту могла кончиться
		// по сроку, а вопрос про часы от неё не зависит.
		svezhee, err := s.sveritChasy(context.Background())
		switch {
		case err == nil:
			d = svezhee
		case kogda.IsZero():
			return 0, false
		}
	}
	return d, sbity(d)
}

// sUchetomChasov подменяет отказ подъёма на clock-skew, если часы сбиты.
// Причина пробы остаётся в тексте: если часы ни при чём, человеку есть за что
// зацепиться и после того, как он их поправит.
func (s *Sluzhba) sUchetomChasov(kod, tekst string, err error) (string, string, error) {
	d, sb := s.chasySbityDlyaOtkaza()
	if !sb {
		return kod, tekst, err
	}
	tekst = fmt.Sprintf("Часы компьютера %s точного времени, а серверы VPN не принимают подключение со сбитыми часами. "+
		"Включи в настройках Windows «Установить время автоматически». Прежняя причина: %s", opisatRashozhdenie(d), tekst)
	return protokol.KodChasy, tekst, fmt.Errorf("%w: %w", ErrChasySbity, err)
}

// chasyDlyaStatusa отдаёт расхождение в секундах, только если оно выше
// порога. Зовётся под s.mu.
func (s *Sluzhba) chasyDlyaStatusa() int {
	if s.chasyZamereno.IsZero() || !sbity(s.chasyRashozhdenie) {
		return 0
	}
	return int(s.chasyRashozhdenie / time.Second)
}

func sbity(d time.Duration) bool { return d > porogChasov || d < -porogChasov }

// opisatRashozhdenie: «отстают на 7 мин от», «спешат на 3 ч от». Без
// склонений: сокращения «мин», «ч», «сут.» не склоняются.
func opisatRashozhdenie(d time.Duration) string {
	kuda := "спешат на"
	if d < 0 {
		kuda, d = "отстают на", -d
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%s %d с от", kuda, int(d/time.Second))
	case d < time.Hour:
		return fmt.Sprintf("%s %d мин от", kuda, int(d/time.Minute))
	case d < 24*time.Hour:
		return fmt.Sprintf("%s %d ч от", kuda, int(d/time.Hour))
	}
	return fmt.Sprintf("%s %d сут. от", kuda, int(d/(24*time.Hour)))
}
