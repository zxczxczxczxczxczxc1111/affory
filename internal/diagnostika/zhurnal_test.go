package diagnostika

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

// Строка на срез, JSON без переносов внутри. Формат выбран не из моды: три
// журнала волны 1.0.3 разбирались регулярками, а лежавший рядом jsonl читался
// в одну строку кода.

func TestSrezPishetsyaOdnoyStrokoyJSON(t *testing.T) {
	var b bytes.Buffer
	z := NovyyZhurnal(&b)
	s := Srez{
		Vremya:              time.Date(2026, 9, 10, 18, 0, 0, 0, time.UTC),
		DeskriptorovSluzhby: 3155,
		PortovZanyato:       16380,
		PortovVsego:         16384,
		BaytVniz:            1 << 20,
	}
	if err := z.Pisat(s); err != nil {
		t.Fatalf("запись: %v", err)
	}
	if err := z.Pisat(s); err != nil {
		t.Fatalf("вторая запись: %v", err)
	}
	stroki := strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
	if len(stroki) != 2 {
		t.Fatalf("строк %d, ждали 2: %q", len(stroki), b.String())
	}
	var nazad Srez
	if err := json.Unmarshal([]byte(stroki[0]), &nazad); err != nil {
		t.Fatalf("строка не разбирается обратно: %v", err)
	}
	if nazad.DeskriptorovSluzhby != 3155 || nazad.PortovZanyato != 16380 {
		t.Fatalf("прочитано не то: %+v", nazad)
	}
}

// Событие пишется той же строкой и вне очереди: разрыв случается между
// секундами, и ждать своего тика значит потерять его порядок относительно
// чисел.
func TestSobytiePishetsyaSoSvoimTekstom(t *testing.T) {
	var b bytes.Buffer
	z := NovyyZhurnal(&b)
	if err := z.Sobytie("туннель разорван: нехватка портов, простой 2 с"); err != nil {
		t.Fatalf("событие: %v", err)
	}
	var s Srez
	if err := json.Unmarshal([]byte(strings.TrimRight(b.String(), "\n")), &s); err != nil {
		t.Fatalf("событие не разбирается: %v", err)
	}
	if !strings.Contains(s.Sobytie, "нехватка портов") {
		t.Fatalf("текст события потерян: %+v", s)
	}
	if s.Vremya.IsZero() {
		t.Fatalf("событие без времени бесполезно")
	}
}

type slomannyy struct{}

func (slomannyy) Write([]byte) (int, error) { return 0, errors.New("диск отвалился") }

// Отказ записи возвращается, а не глотается: журнал, который молча ничего не
// пишет, хуже выключенного, потому что на него рассчитывают.
func TestOtkazZapisiVozvrashchaetsya(t *testing.T) {
	z := NovyyZhurnal(slomannyy{})
	if err := z.Pisat(Srez{}); err == nil {
		t.Fatalf("диск отвалился, а запись отчиталась успехом")
	}
}

// Выключенный журнал это не ошибка, а покой: писать некуда, и звать его всё
// равно будут каждую секунду.
func TestVyklyuchennyyZhurnalMolchit(t *testing.T) {
	var z *Zhurnal
	if err := z.Pisat(Srez{}); err != nil {
		t.Fatalf("выключенный журнал ругается: %v", err)
	}
	if err := z.Sobytie("что-то"); err != nil {
		t.Fatalf("выключенный журнал ругается на событие: %v", err)
	}
}
