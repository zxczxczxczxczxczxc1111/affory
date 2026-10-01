package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/fon"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/genkonfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/sostoyanie"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/yadra"
)

// Проверка сервера до TUN (Б1 аудита 1.8.0, находка H3).
//
// Автоматический подъём при мёртвом сервере поднимал TUN, и на 15-30 секунд
// весь трафик машины уходил в туннель, который не несёт: восстановление делало
// это каждые несколько секунд, то есть интернет пропадал ровно тогда, когда
// VPN и так не работал. Теперь служба сначала поднимает то же ядро с тем же
// набором исходящих, но без входов, и меряет сервер через его clash API. Сеть
// человека проверка не трогает: TUN нет, трафик машины идёт как шёл.
//
// Меряется ровно тем путём, каким пойдёт трафик, поэтому годится для всех
// протоколов, включая hy2 и tuic, у которых tcping бессмыслен.

// errProverkaNeSostoyalas: проверить сервер нечем. Это не приговор серверу, и
// подъём тогда идёт обычным путём: он сам назовёт настоящую причину, если она
// есть.
var errProverkaNeSostoyalas = errors.New("проверка сервера не состоялась")

// Срок всей проверки: старт ядра до 5 с и замер до 5 с, с запасом.
const srokProverkiServera = 20 * time.Second

// Одновременных замеров в группе авто. Больше незачем: нужен первый живой.
const odnovremennyhProverok = 8

func putKonfigaProverki() string {
	return sostoyanie.KatalogDannyh() + `\sing-box.proverka-servera.json`
}

// tekstServerMolchit: что человек видит, пока восстановление ждёт сервер.
const tekstServerMolchit = "Сервер не отвечает. VPN не включён, интернет идёт напрямую; попробую снова сам."

// proveritServerDoTun это ворота автоматического подъёма.
//
// Ручному подъёму проверка не положена: человек нажал «Подключить» и ждёт
// попытки, а не догадки о ней. Запертой машине тоже: сети мимо туннеля у неё
// нет и так, отнимать нечего, а лишние 10 секунд до подъёма это 10 секунд без
// интернета вовсе.
//
// Контекст проверки заводится от Background и отменяется через s.otmena, как у
// самого подъёма: гасит его отключение, а не конец чужого контекста.
func (s *Sluzhba) proveritServerDoTun(ctx context.Context, avto bool, moyo int) error {
	if !avto {
		return nil
	}
	s.mu.Lock()
	if s.zaslonAktiven {
		s.mu.Unlock()
		return nil
	}
	if s.pokolenieP != moyo || ctx.Err() != nil {
		s.mu.Unlock()
		return errPodyomOtmenyon
	}
	pctx, otm := context.WithCancel(context.Background())
	s.otmena = otm
	s.mu.Unlock()
	err := s.proveritServer(pctx)
	otm()

	if s.podyomOtmenyon(moyo) {
		return errPodyomOtmenyon
	}
	switch {
	case err == nil:
		return nil
	case errors.Is(err, errProverkaNeSostoyalas):
		log.Printf("%v, поднимаю туннель без неё", err)
		return nil
	}
	log.Printf("сервер не отвечает, туннель не поднимаю: %v", err)
	s.postavit(protokol.SostOtkaz, &protokol.Oshibka{Kod: protokol.KodAllServersDown, Tekst: tekstServerMolchit})
	return err
}

// proveritServerBezTun отвечает, несёт ли трафик хоть один сервер, через
// который пойдёт подъём. nil значит «да», errProverkaNeSostoyalas значит «не
// знаю», любая другая ошибка значит «нет».
func (s *Sluzhba) proveritServerBezTun(ctx context.Context) error {
	ctx, otm := context.WithTimeout(ctx, srokProverkiServera)
	defer otm()

	telo, portClash, sekret, _, _, err := s.sobratTunPolno(nil, true, true, true, false)
	if err != nil {
		return fmt.Errorf("%w: конфиг не собран: %v", errProverkaNeSostoyalas, err)
	}
	tegi, err := genkonfig.TegiProverki(telo)
	if err != nil {
		return fmt.Errorf("%w: %v", errProverkaNeSostoyalas, err)
	}
	put := putKonfigaProverki()
	// 0600 по той же причине, что и у боевого: в конфиге ключи серверов.
	if err := os.WriteFile(put, telo, 0o600); err != nil {
		return fmt.Errorf("%w: конфиг не записан: %v", errProverkaNeSostoyalas, err)
	}
	defer func() {
		if err := os.Remove(put); err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Printf("конфиг проверки сервера не удалён: %v", err)
		}
	}()

	y, err := yadra.Zapustit(imyaYadraTun, put)
	if err != nil {
		return fmt.Errorf("%w: %v", errProverkaNeSostoyalas, err)
	}
	defer func() {
		if err := y.Ostanovit(); err != nil {
			log.Printf("ядро проверки сервера не остановлено: %v", err)
		}
	}()

	adres := fmt.Sprintf("127.0.0.1:%d", portClash)
	if err := s.zhdatKlash(ctx, adres, sekret); err != nil {
		if ctx.Err() != nil && !errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return ctx.Err()
		}
		return fmt.Errorf("%w: ядро не ответило: %v", errProverkaNeSostoyalas, err)
	}
	return s.zameritLyuboy(ctx, adres, sekret, tegi)
}

// zameritLyuboy меряет кандидатов разом и отвечает успехом на первом живом.
// Группе авто нужен один несущий сервер, а не все: urltest выберет его сам.
func (s *Sluzhba) zameritLyuboy(ctx context.Context, adres, sekret string, tegi []string) error {
	if len(tegi) == 0 {
		return fmt.Errorf("%w: мерить нечего", errProverkaNeSostoyalas)
	}
	ctx, otm := context.WithCancel(ctx)
	defer otm()

	var (
		mu          sync.Mutex
		zhivoy      bool
		poslednyaya error
		gruppa      sync.WaitGroup
	)
	vorota := make(chan struct{}, odnovremennyhProverok)
	for _, teg := range tegi {
		gruppa.Add(1)
		fon.Zapustit("проверке сервера", func() {
			defer gruppa.Done()
			select {
			case vorota <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-vorota }()
			_, err := s.zamerit(ctx, adres, sekret, teg)
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				zhivoy = true
				otm()
				return
			}
			if !zhivoy {
				poslednyaya = fmt.Errorf("%s: %w", teg, err)
			}
		})
	}
	gruppa.Wait()
	if zhivoy {
		return nil
	}
	if poslednyaya == nil {
		return ctx.Err()
	}
	return poslednyaya
}
