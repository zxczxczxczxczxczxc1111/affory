package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/hranenie"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/sostoyanie"
)

// След состава подписок в журнале службы.
//
// Заведён 20.09.2026, когда разбор жалобы «подписка возвращается после каждого
// обновления» третий раз подряд кончился ничем. Кончился он не из-за сложности
// механизма, а из-за того, что наблюдать было НЕЧЕГО: список подписок живёт
// внутри блоба DPAPI, меняется восемью путями, и ни один из них про себя не
// говорит. Единственным свидетелем оказалась побочная строка про
// неразрешившиеся адреса, и то лишь когда подписка запасная.
//
// Отсюда три правила этого файла:
//
//  1. Наружу идут УЗЛЫ, а не адреса. Адрес подписки это пропуск к ключам, и
//     журнал читают глазами, копируют в переписку и прикладывают к отчётам.
//  2. Строка пишется на ИЗМЕНЕНИЕ состава, а не на каждую запись набора. Набор
//     пишется расписанием раз в 12 часов и каждым нажатием человека; строка на
//     каждую запись утонула бы в собственном шуме за сутки.
//  3. Рядом с составом стоит отметка файла секретов: время и короткий хеш. Если
//     запись однажды вернётся, эта пара скажет, вернулась ли она вместе с
//     ОТКАТОМ файла к прежнему содержимому, или файл новый, а запись в нём
//     появилась заново. Два этих случая чинятся в разных местах, и без отметки
//     их не различить.

// skazatRedko пишет строку в журнал не чаще раза в период.
//
// Нужна ровно одному месту: починкам на ЧТЕНИИ набора. Чтение идёт на каждую
// команду интерфейса, и починка, которая не доезжает до диска, повторяется на
// каждом из них. Без сита такая находка утопила бы журнал за сутки, а с ситом
// она остаётся видимой: сито режет повтор, а не событие.
func skazatRedko(tekst string) {
	const period = 10 * time.Minute
	teper := time.Now()
	if bylo, est := kogdaSkazano.Load(tekst); est {
		if teper.Sub(bylo.(time.Time)) < period {
			return
		}
	}
	kogdaSkazano.Store(tekst, teper)
	log.Print(tekst)
}

var kogdaSkazano sync.Map

// opisatPodpiski собирает состав в одну строку для журнала.
func opisatPodpiski(n Nabor) string {
	if len(n.Podpiski) == 0 {
		return "подписок нет"
	}
	chasti := make([]string, 0, len(n.Podpiski))
	for _, z := range n.Podpiski {
		rol := "запас"
		if z.Id == n.Aktivnaya {
			rol = "активная"
		}
		uzel := UzelPodpiski(z.Adres)
		if uzel == "" {
			uzel = "<адрес не разбирается>"
		}
		chasti = append(chasti, fmt.Sprintf("%s(%s, ключей %d)", uzel, rol, len(z.Servery)))
	}
	return strings.Join(chasti, ", ")
}

// sostavPodpisok отдаёт отсортированные идентификаторы: по ним сравнивают, а не
// по строке описания, в которой меняется ещё и число ключей.
func sostavPodpisok(n Nabor) []string {
	id := make([]string, 0, len(n.Podpiski))
	for _, z := range n.Podpiski {
		id = append(id, z.Id)
	}
	sort.Strings(id)
	return id
}

func sostavySovpadayut(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// sledSekretov это отметка файла секретов: когда записан и чем является.
//
// Хеш короткий намеренно. Полные 64 знака в журнале никто не сверяет глазами, а
// двенадцати хватает, чтобы отличить откат к прежнему файлу от новой записи.
func sledSekretov() string {
	put := filepath.Join(sostoyanie.KatalogDannyh(), hranenie.ImyaSekretov)
	st, err := os.Stat(put)
	if err != nil {
		return "файла секретов нет"
	}
	telo, err := os.ReadFile(put)
	if err != nil {
		return fmt.Sprintf("записан %s, прочитать не удалось: %v",
			st.ModTime().Format("15:04:05"), err)
	}
	sum := sha256.Sum256(telo)
	return fmt.Sprintf("записан %s, %d байт, sha256 %s",
		st.ModTime().Format("15:04:05"), len(telo), hex.EncodeToString(sum[:6]))
}
