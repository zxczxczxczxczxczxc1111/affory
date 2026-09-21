package set

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Алиас TUN-адаптера появляется только ПОСЛЕ того, как его поднял sing-box, и
// это не мелочь: на нём держится порядок постановки правил брандмауэра в задаче
// 2.5. Правило, поставленное до появления адаптера, ссылается в пустоту.
const IntervalOprosaTun = 250 * time.Millisecond

var ErrAdapterNePoyavilsya = errors.New("TUN-адаптер не появился")

// Опознание по описанию. Замерено 31.08.2026 в госте: sing-tun поднимает
// адаптер с именем tun0 и описанием "sing-tun Tunnel". Слова WireGuard там нет
// вовсе, хотя драйвер внутри именно wintun, поэтому поиск по нему находил бы
// ноль совпадений и молча.
var opisaniyaTun = []string{"sing-tun", "wintun", "wireguard"}

// SnimokAdapterov это индексы адаптеров, поднятых ДО старта нашего ядра.
//
// Нужен ровно затем, чтобы не принять чужой туннель за свой. Признаки, по
// которым мы себя опознаём, чужому клиенту доступны все: имя tun0 и адрес
// 172.19.0.1/30 это УМОЛЧАНИЯ самого sing-box, то есть у любого другого клиента
// на том же ядре (Hiddify, Karing, NekoBox) они ровно такие же, а описание
// "wintun"/"wireguard" несёт вообще всякий туннель на этом драйвере.
// Единственный признак, который чужой адаптер подделать не может, это ВРЕМЯ:
// наш появляется после того, как мы запустили ядро.
type SnimokAdapterov map[uint32]bool

// ZhdatIscheznoveniya ждёт, пока адаптер с этим именем пропадёт из системы.
//
// Зовётся ПЕРЕД снимком и закрывает гонку переподключения. Windows
// переиспользует индекс интерфейса, и наш собственный туннель, который ещё не
// успел исчезнуть после Disconnect, попал бы в снимок как «был раньше», а
// новый адаптер получил бы тот же индекс и был бы принят за чужой. Тогда
// исправный подъём отваливался бы по сроку ожидания.
//
// Не дождались значит адаптер не наш: наш уходит вместе с ядром. Такой
// остаётся в снимке и считается чужим, а ядро поднимет свой под другим именем,
// и его найдёт поиск по описанию.
func ZhdatIscheznoveniya(ctx context.Context, imya string) {
	for {
		spisok, err := perechislit()
		if err != nil {
			return
		}
		est := false
		for _, a := range spisok {
			if a.Imya == imya && a.Sostoyanie == sostoyanieVverh {
				est = true
				break
			}
		}
		if !est {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(IntervalOprosaTun):
		}
	}
}

// SnyatSnimok запоминает поднятые адаптеры. Зовётся ДО запуска ядра.
//
// Опущенные адаптеры в снимок НЕ идут намеренно: Windows переиспользует индекс
// интерфейса, и наш вчерашний tun0, оставшийся в системе выключенным, занял бы
// собой место сегодняшнего.
func SnyatSnimok() (SnimokAdapterov, error) {
	spisok, err := perechislit()
	if err != nil {
		return nil, err
	}
	s := make(SnimokAdapterov, len(spisok))
	for _, a := range spisok {
		if a.Sostoyanie == sostoyanieVverh {
			s[a.Indeks] = true
		}
	}
	return s, nil
}

// ZhdatAdapterPolno отдаёт адаптер целиком.
//
// Отступление от плановой подписи, и намеренное: зовущему нужен ИНДЕКС, чтобы
// исключить туннель при поиске канала под ним (см. AktivnyyAdapterKrome).
// Возвращать одно имя значит заставить его тут же искать этот адаптер второй
// раз, уже по имени, в системе, которая за это время могла измениться.
// bylo это снимок, снятый ДО старта ядра. Пустой снимок значит «снять не
// удалось»: тогда ищем как прежде, по имени и описанию среди всех, и это хуже,
// но лучше отказа на ровном месте. Вызывающий говорит об этом в журнал.
func ZhdatAdapterPolno(ctx context.Context, imya string, bylo SnimokAdapterov) (Adapter, error) {
	for {
		if spisok, err := perechislit(); err == nil {
			if a, ok := naytiTun(spisok, imya, bylo); ok {
				return a, nil
			}
		}
		select {
		case <-ctx.Done():
			return Adapter{}, fmt.Errorf("%w: %s: %w", ErrAdapterNePoyavilsya, imya, ctx.Err())
		case <-time.After(IntervalOprosaTun):
		}
	}
}

// naytiTun ищет сначала по точному имени, потом по описанию, и в обоих случаях
// ТОЛЬКО среди адаптеров, которых при старте ядра ещё не было.
//
// Имя задаём мы сами через конфиг ядра, поэтому оно точнее. Описание это
// запасной путь на случай, если ядро назовёт адаптер иначе, чем попросили: без
// него отказ выглядел бы как "туннель не поднялся", хотя он поднят и работает.
//
// Отсев по снимку добавлен 21.09.2026. Оба признака чужому туннелю доступны:
// "wintun" в описании несёт каждый клиент на этом драйвере, а имя tun0 это
// умолчание sing-box, то есть имя чужого клиента на том же ядре. Принятый за
// свой чужой адаптер уводил за собой всё: к его адресу привязывались правила
// брандмауэра, его индекс исключался из поиска канала под туннелем, а наш
// настоящий туннель при этом оставался без правил.
func naytiTun(spisok []Adapter, imya string, bylo SnimokAdapterov) (Adapter, bool) {
	novyy := func(a Adapter) bool { return a.Sostoyanie == sostoyanieVverh && !bylo[a.Indeks] }
	for _, a := range spisok {
		if novyy(a) && a.Imya == imya {
			return a, true
		}
	}
	for _, a := range spisok {
		if !novyy(a) {
			continue
		}
		o := strings.ToLower(a.Opisanie)
		for _, obr := range opisaniyaTun {
			if strings.Contains(o, obr) {
				return a, true
			}
		}
	}
	return Adapter{}, false
}
