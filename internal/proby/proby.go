// Package proby это отдельные пробы сетевых слоёв (A3, 22.09.2026).
//
// Наблюдатель туннеля спрашивал ровно одно: отвечает ли HTTP через выбранный
// исходящий. Такая проба зелена при сломанном DNS и при съеденном UDP, потому
// что идёт другим путём. 22.09.2026 это показано в госте: местный резолвер
// недоступен, российские имена не разрешаются по 12 секунд на имя, а продукт
// всё это время отвечает «поднят» и ошибки не показывает.
//
// Здесь слои разведены: имя, местный резолвер, UDP и сам туннель проверяются
// порознь, и каждая проба отвечает за себя. Тяжёлого постоянного замера тут
// нет намеренно: пробы идут по запросу человека и один раз перед аварийным
// разрывом, чтобы назвать причину.
package proby

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/sboi"
)

// Itog это ответ одной пробы. Nil-ошибка значит «прошло».
type Itog struct {
	Proshlo    bool
	Dlitelnost time.Duration
	Podrobno   string
}

func poluchilos(nach time.Time, podrobno string) Itog {
	return Itog{Proshlo: true, Dlitelnost: time.Since(nach), Podrobno: podrobno}
}

func nePoluchilos(nach time.Time, podrobno string) Itog {
	return Itog{Proshlo: false, Dlitelnost: time.Since(nach), Podrobno: podrobno}
}

// SrokProby одинаков у всех проб: человек ждёт ответа, а не точности до
// миллисекунды. Три секунды это вдвое больше, чем отвечает живой резолвер в
// домашней сети, и вчетверо меньше, чем ждёт Windows до своего отказа.
const SrokProby = 3 * time.Second

// Прямого вопроса к адресу местного резолвера здесь НЕТ, и это измерено.
//
// Такая проба была, и на поднятом туннеле она врала всегда: 22.09.2026 в госте
// прямой вопрос к живому 192.168.0.1 не вернулся вовсе (12.2 с), потому что
// пакет уходит в TUN и его забирает ядро. Слой краснел на здоровой сети, то
// есть пугал человека поломкой, которой нет.
//
// Путь мимо VPN меряется тем же способом, каким по нему ходят сайты: именем
// через системный резолвер (Imya ниже). Тот же опыт: при живом местном
// резолвере семь российских имён отвечали за 0 с, при мёртвом - ни одно, по 12
// секунд на имя, а stackoverflow.com через туннель отвечал за 0.1 с.

// Imya спрашивает имя ТЕМ ЖЕ путём, которым его спрашивают программы человека:
// через системный резолвер. На поднятом туннеле это путь через ядро целиком, с
// его правилами и его выбором сервера.
func Imya(ctx context.Context, imya string) Itog {
	nach := time.Now()
	ctx, otmena := context.WithTimeout(ctx, SrokProby)
	defer otmena()
	adresa, err := net.DefaultResolver.LookupHost(ctx, imya)
	if err != nil {
		return nePoluchilos(nach, fmt.Sprintf("%s не находится по имени: %v", imya, korotko(err)))
	}
	return poluchilos(nach, fmt.Sprintf("%s это %s", imya, adresa[0]))
}

// ItogUDP это ответ пробы UDP. Потери и разброс собираются серией: один пакет
// про UDP не говорит ничего, а постоянный тяжёлый замер съедает канал.
type ItogUDP struct {
	Itog
	Otpravleno int
	Poluchheno int
	Poter      int
	Sredniy    time.Duration
	Razbros    time.Duration
}

// PaketovUDP и ShagUDP: десять пакетов с шагом в сотню миллисекунд это секунда
// замера и хватает, чтобы отличить «UDP не ходит вовсе» от «UDP теряет».
const (
	PaketovUDP = 10
	ShagUDP    = 100 * time.Millisecond
)

// UDP проверяет, ходит ли UDP тем путём, которым идут голос и видео.
//
// Запросами STUN, с которых начинается любой звонок в браузере: пакет в 20
// байт, и публичный сервер отвечает сразу. Потери здесь это потери НАШЕГО пути
// до него, а не качество самого сервера.
//
// portProksi больше нуля значит «через VPN»: пакеты идут через прокси-вход
// ядра (SOCKS5 UDP ASSOCIATE). Прямо из службы их нести нельзя: свои процессы
// службы правило процессов ведёт мимо туннеля, и проба мерила бы путь без VPN
// (С5 аудита 1.6.1). DNS для пробы не годится по той же линии: пакет на порт 53
// через вход ядра забирает DNS самого ядра, и мерился бы он, а не UDP.
func UDP(ctx context.Context, cel string, portProksi int) ItogUDP {
	nach := time.Now()
	itog := ItogUDP{Otpravleno: PaketovUDP}
	var put putUDP
	var err error
	if portProksi > 0 {
		put, err = otkrytSocksUDP(ctx, portProksi, cel)
	} else {
		put, err = otkrytPryamoyUDP(ctx, cel)
	}
	if err != nil {
		itog.Poter = PaketovUDP
		itog.Itog = nePoluchilos(nach, "путь для UDP не открылся: "+korotko(err))
		return itog
	}
	defer put.Close()

	zamery := make([]time.Duration, 0, PaketovUDP)
	var posledniy error
	for i := 0; i < PaketovUDP; i++ {
		if ctx.Err() != nil {
			break
		}
		if i > 0 {
			select {
			case <-ctx.Done():
			case <-time.After(ShagUDP):
			}
		}
		t, err := odinSTUN(ctx, put)
		if err != nil {
			posledniy = err
			continue
		}
		zamery = append(zamery, t)
	}
	itog.Poluchheno = len(zamery)
	itog.Poter = itog.Otpravleno - itog.Poluchheno
	if len(zamery) == 0 {
		itog.Itog = nePoluchilos(nach, fmt.Sprintf("ни один пакет не вернулся: %v", korotko(posledniy)))
		return itog
	}
	itog.Sredniy, itog.Razbros = sredniyIRazbros(zamery)
	// Проба СЧИТАЕТ, а не судит: решение «рвать или нет» остаётся снаружи.
	// Потеря половины пакетов это ещё рабочая мобильная сеть, и обрывать по ней
	// связь значит обрывать её на ровном месте.
	itog.Itog = poluchilos(nach, fmt.Sprintf("получено %d из %d, среднее %s, разброс %s",
		itog.Poluchheno, itog.Otpravleno, Millisekundy(itog.Sredniy), Millisekundy(itog.Razbros)))
	return itog
}

// odinSTUN шлёт один запрос и ждёт ответ с ТЕМ ЖЕ номером: опоздавший ответ
// на прошлый пакет уже посчитан потерей и в замер не идёт.
func odinSTUN(ctx context.Context, put putUDP) (time.Duration, error) {
	srok := time.Now().Add(SrokProby)
	if d, est := ctx.Deadline(); est && d.Before(srok) {
		srok = d
	}
	zapros, nomer, err := zaprosSTUN()
	if err != nil {
		return 0, err
	}
	nach := time.Now()
	if err := put.otpravit(zapros); err != nil {
		return 0, err
	}
	for {
		otv, err := put.poluchit(srok)
		if err != nil {
			return 0, err
		}
		if otvetSTUN(otv, nomer) {
			return time.Since(nach), nil
		}
	}
}

func sredniyIRazbros(zamery []time.Duration) (time.Duration, time.Duration) {
	var summa time.Duration
	for _, z := range zamery {
		summa += z
	}
	sredniy := summa / time.Duration(len(zamery))
	if len(zamery) < 2 {
		return sredniy, 0
	}
	var kvadraty float64
	for _, z := range zamery {
		d := float64(z - sredniy)
		kvadraty += d * d
	}
	return sredniy, time.Duration(math.Sqrt(kvadraty / float64(len(zamery))))
}

// korotko убирает из ошибки сети её длинный хвост: человеку в подробностях
// пробы нужна причина, а не трасса вызовов сокета.
func korotko(err error) string {
	if err == nil {
		return "без ответа"
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		switch {
		case dnsErr.IsTimeout:
			return "нет ответа за отведённый срок"
		case dnsErr.IsNotFound:
			return "имя не найдено"
		}
		return sboi.DlyaCheloveka(err)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "нет ответа за отведённый срок"
	}
	return sboi.DlyaCheloveka(err)
}

// Millisekundy печатает длительность по-русски.
//
// time.Duration печатает себя сам и всегда латиницей: «41ms», «1.2s». В
// журнале это неважно, а на экране рядом с русским текстом читается
// машинно (владелец, 23.09.2026). Округление до миллисекунды здесь и
// нужно: доли миллисекунды на экране не значат ничего.
func Millisekundy(d time.Duration) string {
	ms := d.Round(time.Millisecond).Milliseconds()
	if ms < 1000 {
		return fmt.Sprintf("%d мс", ms)
	}
	return fmt.Sprintf("%.1f с", d.Seconds())
}
