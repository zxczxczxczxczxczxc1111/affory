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
	vremen := putOtkata() + ".tmp"
	if err := os.WriteFile(vremen, telo, 0o600); err != nil {
		return fmt.Errorf("временный файл отката не записан: %w", err)
	}
	// Переименование поверх атомарно на NTFS, запись на месте нет. Разница
	// проявляется ровно один раз, в самый неудачный момент.
	if err := os.Rename(vremen, putOtkata()); err != nil {
		_ = os.Remove(vremen)
		return fmt.Errorf("откат не переименован: %w", err)
	}
	return nil
}

func ProchitatOtkat() (Otkat, error) {
	telo, err := os.ReadFile(putOtkata())
	if errors.Is(err, os.ErrNotExist) {
		return Otkat{}, ErrOtkataNet
	}
	if err != nil {
		return Otkat{}, fmt.Errorf("откат не прочитан: %w", err)
	}
	var o Otkat
	if err := json.Unmarshal(telo, &o); err != nil {
		return Otkat{}, fmt.Errorf("откат неразборчив: %w", err)
	}
	if len(o.Profili) == 0 {
		return Otkat{}, fmt.Errorf("откат пуст: возвращать нечего")
	}
	return o, nil
}

// UdalitOtkat зовётся ПОСЛЕ успешного возврата и только тогда.
//
// Файл, переживший неудачный возврат, это единственное, по чему следующий старт
// поймёт, что машина осталась запертой. Удалять его заранее значит выбросить
// карту ровно перед тем, как заблудиться.
func UdalitOtkat() error {
	if err := os.Remove(putOtkata()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("файл отката не удалён: %w", err)
	}
	return nil
}
