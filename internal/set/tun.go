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

// ZhdatAdapterPolno отдаёт адаптер целиком.
//
// Отступление от плановой подписи, и намеренное: зовущему нужен ИНДЕКС, чтобы
// исключить туннель при поиске канала под ним (см. AktivnyyAdapterKrome).
// Возвращать одно имя значит заставить его тут же искать этот адаптер второй
// раз, уже по имени, в системе, которая за это время могла измениться.
func ZhdatAdapterPolno(ctx context.Context, imya string) (Adapter, error) {
	for {
		if spisok, err := perechislit(); err == nil {
			if a, ok := naytiTun(spisok, imya); ok {
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

// naytiTun ищет сначала по точному имени, потом по описанию.
//
// Имя задаём мы сами через конфиг ядра, поэтому оно точнее. Описание это
// запасной путь на случай, если ядро назовёт адаптер иначе, чем попросили: без
// него отказ выглядел бы как "туннель не поднялся", хотя он поднят и работает.
func naytiTun(spisok []Adapter, imya string) (Adapter, bool) {
	for _, a := range spisok {
		if a.Sostoyanie == sostoyanieVverh && a.Imya == imya {
			return a, true
		}
	}
	for _, a := range spisok {
		if a.Sostoyanie != sostoyanieVverh {
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
