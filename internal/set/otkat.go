package set

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/sostoyanie"
)

// Прежнее состояние снимается ДО изменения и кладётся на диск.
//
// Change first, remember later is how you end up with a machine that has no
// internet and no idea what it looked like before. Файл читается человеком без
// интернета и программой при следующем старте, поэтому он простой JSON, а не
// хитрая структура.
const ImyaFaylaOtkata = "firewall-do-affory.json"

var ErrOtkataNet = errors.New("файла отката нет")

type ProfilDo struct {
	Imya      string `json:"imya"` // domain | private | public
	Vklyuchen bool   `json:"vklyuchen"`
	Politika  string `json:"politika"` // например BlockInbound,AllowOutbound
}

type Otkat struct {
	Zapisan time.Time  `json:"zapisan"`
	Profili []ProfilDo `json:"profili"`

	// Namerenno отличает "человек включил режим сам" от "режим остался после
	// нештатной смерти службы". Без этого признака задача 2.6 распечатывает
	// машину, которую заперли осознанно: правило "политика Block есть, туннеля
	// нет, значит осиротело" накрывает оба случая одинаково.
	Namerenno bool `json:"namerenno"`

	// Pravila это имена правил, которые мы РЕАЛЬНО завели. Фиксированного
	// списка тут мало: правил по процессам столько, сколько процессов, и они
	// нумерованные. Снятие по угаданному списку однажды оставит висеть лишнее,
	// а маску по префиксу применять нельзя, она заденет чужое похожее имя.
	Pravila []string `json:"pravila,omitempty"`
}

// Шов ради тестов. Без него любой тест отката пишет в живой
// C:\ProgramData\Affory, то есть в ту самую папку, поведение с которой он и
// проверяет.
var katalogDannyh = sostoyanie.KatalogDannyh

func putOtkata() string { return filepath.Join(katalogDannyh(), ImyaFaylaOtkata) }

func ZapisatOtkat(o Otkat) error {
	o.Zapisan = time.Now()
	telo, err := json.MarshalIndent(o, "", "  ")
	if err != nil {
		return fmt.Errorf("откат не сериализуется: %w", err)
	}
	// Пропавшее питание не должно оставить вместо файла мусор: тогда машина
	// не знала бы, что заперта (Н3 аудита 1.6.1). Прежняя версия остаётся
	// рядом, если она годна.
	godno := func(b []byte) bool { return razobratOtkat(b, new(Otkat)) == nil }
	if err := sostoyanie.ZapisatNadyozhno(putOtkata(), telo, godno); err != nil {
		return fmt.Errorf("откат не записан: %w", err)
	}
	return nil
}

func razobratOtkat(telo []byte, o *Otkat) error {
	*o = Otkat{}
	if err := json.Unmarshal(telo, o); err != nil {
		return fmt.Errorf("откат неразборчив: %w", err)
	}
	if len(o.Profili) == 0 {
		return fmt.Errorf("откат пуст: возвращать нечего")
	}
	return nil
}

func ProchitatOtkat() (Otkat, error) {
	var o Otkat
	_, _, err := sostoyanie.ProchitatSZapasom(putOtkata(), func(telo []byte) error {
		return razobratOtkat(telo, &o)
	})
	if errors.Is(err, os.ErrNotExist) {
		return Otkat{}, ErrOtkataNet
	}
	if err != nil {
		return Otkat{}, err
	}
	return o, nil
}

// UdalitOtkat зовётся ПОСЛЕ успешного возврата и только тогда.
//
// Файл, переживший неудачный возврат, это единственное, по чему следующий старт
// поймёт, что машина осталась запертой. Удалять его заранее значит выбросить
// карту ровно перед тем, как заблудиться. Прежняя версия уходит вместе с ним:
// иначе снятый замок воскрес бы из копии.
func UdalitOtkat() error {
	if err := sostoyanie.UdalitSZapasom(putOtkata()); err != nil {
		return fmt.Errorf("файл отката не удалён: %w", err)
	}
	return nil
}
