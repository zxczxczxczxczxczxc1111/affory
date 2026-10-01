package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/fon"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/genkonfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/sboi"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/yadra"
)

// Пинг ПО КАЖДОМУ серверу, одним числом (01.10.2026).
//
// До этого чисел было два: tcping до узла мимо туннеля и задержка urltest из
// clash API. Второе это дозвон, рукопожатие протокола, TLS с целью и сам
// запрос, то есть три-пять кругов разом, и список показывал 166-797 мс там,
// где Discord и строка «Задержка» на главном экране показывали около 60.
// Владелец решил оставить одно число, то самое, которое человек видит в игре.
//
// Меряется так же, как «Задержка»: соединение через сервер прогревается
// запросом, который не считается, и меряется второй запрос по нему же (см.
// set.Otklik). Через какой сервер идти, называет логин входа замеров у ядра.
//
// Ядро для замера всегда временное и без TUN, при включённом VPN тоже. Его
// собственные соединения к серверам идут мимо туннеля, как и у боевого ядра,
// то есть путь замера тот же. Зато новые ключи меряются сразу, без
// переподключения, а боевой конфиг от замеров не зависит вовсе.

// Одновременных замеров. Каждый это настоящее соединение через сервер, и
// сорок разом мерили бы не серверы, а собственную очередь.
const odnovremennyhZamerov = 4

// Срок всего обхода. Окно ждёт ответа srokZamera (90 с), и замер обязан
// уложиться раньше: опоздавший ответ окно уже не прочитает.
const srokObhoda = 75 * time.Second

// Кругов на сервер после прогрева, в число идёт лучший (см.
// set.LuchshiyOtklikCherez). Каждый круг это один путь туда и обратно, так что
// три стоят около трёх пингов и укладываются в срок прибора с запасом.
const krugovPinga = 3

// Срок подъёма временного ядра: от старта до ответа clash_api.
const srokYadraZamera = 15 * time.Second

type zamerZaderzhki struct {
	Versiya   string `json:"versiya,omitempty"`
	Id        string `json:"id"`
	PingMs    *int64 `json:"ping_ms"`
	PingOtkaz string `json:"ping_otkaz,omitempty"`
}

// vremennoeYadro это поднятое для замера ядро без TUN.
type vremennoeYadro struct {
	zamer genkonfig.VhodZamera
	// isklyucheny это серверы, которых ядро не приняло: их исходящих в
	// конфиге нет, и мерить их нечем.
	isklyucheny map[string]bool
	ostanovit   func()
}

func (s *Sluzhba) measureDelays(ctx context.Context, k protokol.Kadr) protokol.Kadr {
	ctx, otm := context.WithTimeout(ctx, srokObhoda)
	defer otm()
	// Замеры идут по одному. Конфиг временного ядра лежит одним файлом, и
	// второй замер разом с первым (окно и консоль) переписывал бы его под
	// первым ядром. Ожидание входит в тот же срок обхода: окно ждёт ответа
	// не дольше srokZamera.
	select {
	case s.vorotaPinga <- struct{}{}:
		defer func() { <-s.vorotaPinga }()
	case <-ctx.Done():
		return otkazIz(k, protokol.KodYadroNeOtvechaet, errors.New("пинг не измерен: прежний замер так и не закончился"))
	}

	n, err := s.nabor()
	if err != nil {
		return otkazIz(k, protokol.KodSecretsUnreadable, err)
	}
	servery := n.Servery
	versii, err := s.versiiServerov(servery)
	if err != nil {
		return otkaz(k.Id, k.Imya, protokol.KodSecretsUnreadable, "не удалось определить версии серверов")
	}

	y, err := s.yadroZamera(ctx)
	if err != nil {
		return otkazIz(k, protokol.KodYadroNeOtvechaet, fmt.Errorf("пинг не измерен: %w", err))
	}
	defer y.ostanovit()

	zamery := make([]zamerZaderzhki, len(servery))
	var gruppa sync.WaitGroup
	vorota := make(chan struct{}, odnovremennyhZamerov)
	for i, srv := range servery {
		gruppa.Add(1)
		fon.Zapustit("замере пинга", func() {
			defer gruppa.Done()
			vorota <- struct{}{}
			defer func() { <-vorota }()
			zamery[i] = s.pingOdnogo(ctx, srv, y)
			zamery[i].Versiya = versii[srv.Id]
		})
	}
	gruppa.Wait()

	return otvet(k.Id, k.Imya, map[string]any{"zamery": zamery})
}

func (s *Sluzhba) pingOdnogo(ctx context.Context, srv protokol.Server, y vremennoeYadro) zamerZaderzhki {
	z := zamerZaderzhki{Id: srv.Id}
	if y.isklyucheny[srv.Id] {
		z.PingOtkaz = "ядро не принимает этот сервер"
		return z
	}
	// Опоздавший к сроку обхода сервер не мерится вовсе: иначе он получил бы
	// отказ «не ответил вовремя», хотя до него просто не дошла очередь.
	if ctx.Err() != nil {
		z.PingOtkaz = "не успели измерить: серверов много, попробуй ещё раз"
		return z
	}
	proksi := &url.URL{
		Scheme: "http",
		User:   url.UserPassword(genkonfig.PolzovatelZamera(srv.Id), y.zamer.Parol),
		Host:   net.JoinHostPort("127.0.0.1", strconv.Itoa(y.zamer.Port)),
	}
	s.mu.Lock()
	cel := s.adresOtklika
	s.mu.Unlock()
	d, err := s.zamerPinga(ctx, cel, proksi)
	if err != nil {
		z.PingOtkaz = sboi.DlyaCheloveka(err)
		return z
	}
	// Ноль на экране читается как «мгновенно» и ставит сервер первым.
	ms := max(d.Milliseconds(), 1)
	z.PingMs = &ms
	return z
}

// podnyatYadroZamera поднимает ядро без TUN со входом замеров.
//
// Конфиг проверяется ядром до запуска, как и боевой: чужая подписка однажды
// принесла сервер, который ядро отвергает, и без проверки одна такая запись
// оставила бы без пинга весь список. Отвергнутый сервер исключается, остальные
// меряются.
func (s *Sluzhba) podnyatYadroZamera(ctx context.Context) (vremennoeYadro, error) {
	put := filepath.Join(s.dirDannyh, "sing-box.zamer.json")
	ubrat := func() {
		if err := os.Remove(put); err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Printf("конфиг замера пинга не удалён: %v", err)
		}
	}
	isklyucheny := map[string]bool{}
	for {
		telo, portClash, sekret, _, zamer, err := s.sobratTunPolno(isklyucheny, true, true, true, true)
		if err != nil {
			return vremennoeYadro{}, err
		}
		if zamer.Port <= 0 {
			return vremennoeYadro{}, errors.New("вход замеров не собран")
		}
		// 0600 по той же причине, что и у боевого: в конфиге ключи серверов.
		if err := os.WriteFile(put, telo, 0o600); err != nil {
			return vremennoeYadro{}, fmt.Errorf("конфиг замера не записан: %w", err)
		}
		if s.proveritKonfig != nil {
			if err := s.proveritKonfig(put); err != nil {
				var o *yadra.OshibkaKonfiga
				id, est := "", false
				if errors.As(err, &o) && o.EstNomer {
					id, est = idServeraPoNomeru(telo, o.Nomer)
				}
				if !est || isklyucheny[id] {
					ubrat()
					return vremennoeYadro{}, fmt.Errorf("ядро не приняло конфиг замера: %w", err)
				}
				log.Printf("замер пинга: сервер %s исключён, ядро не приняло его исходящий: %s", id, o.Vyhod)
				isklyucheny[id] = true
				continue
			}
		}

		y, err := yadra.Zapustit(imyaYadraTun, put)
		if err != nil {
			ubrat()
			return vremennoeYadro{}, err
		}
		ostanovit := func() {
			if err := y.Ostanovit(); err != nil {
				log.Printf("ядро замера пинга не остановлено: %v", err)
			}
			ubrat()
		}
		zctx, otm := context.WithTimeout(ctx, srokYadraZamera)
		err = s.zhdatKlash(zctx, fmt.Sprintf("127.0.0.1:%d", portClash), sekret)
		otm()
		if err != nil {
			ostanovit()
			return vremennoeYadro{}, fmt.Errorf("ядро для замера не ответило: %w", err)
		}
		return vremennoeYadro{zamer: zamer, isklyucheny: isklyucheny, ostanovit: ostanovit}, nil
	}
}
