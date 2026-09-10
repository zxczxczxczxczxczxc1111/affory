package diagnostika

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"
)

// chistimyy это то, что умеет опустошить себя: zhurnaly.Fayl умеет, буфер в
// тесте нет. Интерфейсом, а не типом, чтобы пакет не тянул зависимость ради
// одной кнопки.
type chistimyy interface{ Ochistit() error }

// ImyaZhurnala лежит рядом с остальными журналами и ротируется тем же
// zhurnaly.Fayl: 10 МБ на файл, три файла. При строке в секунду это примерно
// сутки на файл, то есть трое суток истории при том же потолке в 30 МБ, что и
// у соседей.
const ImyaZhurnala = "diagnostika.jsonl"

// Zhurnal пишет срезы построчно. Нулевой указатель это ВЫКЛЮЧЕННЫЙ журнал, а
// не ошибка: звать его будут каждую секунду независимо от настройки, и
// заставлять каждое место проверять nil значит однажды забыть.
type Zhurnal struct {
	mu      sync.Mutex
	kuda    io.Writer
	seychas func() time.Time
}

func NovyyZhurnal(kuda io.Writer) *Zhurnal {
	return &Zhurnal{kuda: kuda, seychas: time.Now}
}

// Pisat кладёт срез строкой JSON.
func (z *Zhurnal) Pisat(s Srez) error {
	if z == nil || z.kuda == nil {
		return nil
	}
	b, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("срез не сериализуется: %w", err)
	}
	b = append(b, '\n')
	z.mu.Lock()
	defer z.mu.Unlock()
	if _, err := z.kuda.Write(b); err != nil {
		return fmt.Errorf("журнал диагностики: %w", err)
	}
	return nil
}

// Ochistit опустошает файл, если тот умеет опустошаться. Кнопка «Очистить
// журнал» обещает стереть журналы, и подробный входит в это обещание.
func (z *Zhurnal) Ochistit() error {
	if z == nil || z.kuda == nil {
		return nil
	}
	z.mu.Lock()
	defer z.mu.Unlock()
	c, umeet := z.kuda.(chistimyy)
	if !umeet {
		return nil
	}
	return c.Ochistit()
}

// Sobytie пишет строку вне очереди: разрыв случается между секундами, и ждать
// своего тика значит потерять порядок относительно чисел.
func (z *Zhurnal) Sobytie(tekst string) error {
	if z == nil || z.kuda == nil {
		return nil
	}
	return z.Pisat(Srez{Vremya: z.seychas(), Sobytie: tekst})
}
