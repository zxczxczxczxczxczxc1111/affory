package main

import (
	"context"
	"sync"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/genkonfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/yadra"
)

// Замер задержки ПО КАЖДОМУ серверу, двумя разными числами.
//
// До этой команды клиент знал ровно одно число: задержку последней пробы
// urltest, одну на всё подключение, обновляемую раз в три минуты. Выбирать
// сервер по такому числу нельзя, оно про тот сервер, который УЖЕ выбран.
//
// Чисел два, и они отвечают на разные вопросы:
//
//	tcping   — дорога до узла, мимо туннеля. Туннель для него НЕ нужен, значит
//	           кнопка полезна до подключения, то есть тогда, когда человеку и
//	           надо выбрать, куда подключаться;
//	realping — весь путь через туннель, вместе с протоколом и рукопожатием.
//	           Спрашивается у ЯДРА по тегу конкретного исходящего: проба мимо
//	           ядра мерила бы путь, которым трафик не пойдёт.
//
// Одно число вместо двух врёт про оба. Сервер, отвечающий на TCP мгновенно и не
// несущий ни байта, это обычный случай: просроченный ключ, чужой sid у REALITY,
// отвергнутое рукопожатие. По одной цифре «через туннель» он неотличим от
// далёкого, но исправного, и человек чинит не то.

// Одновременных замеров. `/delay` это НАСТОЯЩИЙ запрос через этот исходящий, и
// сорок штук разом мерили бы не серверы, а собственную очередь.
const odnovremennyhZamerov = 4

// Срок одного tcping. Дольше ждать незачем: узел, до которого TCP идёт больше
// трёх секунд, для выбора всё равно негоден.
const srokTcping = 3 * time.Second

type zamerZaderzhki struct {
	Id            string `json:"id"`
	TcpingMs      *int64 `json:"tcping_ms"`
	TcpingOtkaz   string `json:"tcping_otkaz,omitempty"`
	RealpingMs    *int64 `json:"realping_ms"`
	RealpingOtkaz string `json:"realping_otkaz,omitempty"`
}

func (s *Sluzhba) measureDelays(ctx context.Context, k protokol.Kadr) protokol.Kadr {
	n, err := s.nabor()
	if err != nil {
		return otkaz(k.Id, k.Imya, protokol.KodSecretsUnreadable, err.Error())
	}
	servery := n.Servery

	// Снимок доступа к ядру берётся ОДИН раз и до замеров, тем же способом, что
	// и везде. Читать его в каждой горутине значило бы, что половина замеров
	// пойдёт по старому адресу, если туннель опустят посреди прохода. Пустой
	// адрес означает опущенный туннель, это штатный случай.
	adresKlash, sekret := s.dostupKKlash()

	zamery := make([]zamerZaderzhki, len(servery))
	var gruppa sync.WaitGroup
	vorota := make(chan struct{}, odnovremennyhZamerov)
	for i, srv := range servery {
		gruppa.Add(1)
		go func(i int, srv protokol.Server) {
			defer gruppa.Done()
			vorota <- struct{}{}
			defer func() { <-vorota }()
			zamery[i] = s.zamerOdnogo(ctx, srv, adresKlash, sekret)
		}(i, srv)
	}
	gruppa.Wait()

	return otvet(k.Id, k.Imya, map[string]any{"zamery": zamery})
}

func (s *Sluzhba) zamerOdnogo(ctx context.Context, srv protokol.Server, adresKlash, sekret string) zamerZaderzhki {
	z := zamerZaderzhki{Id: srv.Id}

	if d, err := yadra.Tcping(ctx, srv.Host, srv.Port, srokTcping); err != nil {
		z.TcpingOtkaz = err.Error()
	} else {
		ms := d.Milliseconds()
		z.TcpingMs = &ms
	}

	// Отсутствие туннеля это НЕ отказ сервера, и текст обязан это говорить.
	// Иначе человек прочитает «сервер не отвечает» там, где не подключён он сам.
	if adresKlash == "" {
		z.RealpingOtkaz = "туннель опущен: через него мерить нечего"
		return z
	}
	if d, err := s.zamerit(ctx, adresKlash, sekret, genkonfig.TegKandidata(srv.Id)); err != nil {
		z.RealpingOtkaz = err.Error()
	} else {
		ms := d.Milliseconds()
		z.RealpingMs = &ms
	}
	return z
}
