package kachestvo_test

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/genkonfig"
)

// Четвёртая сторона словаря: транспорт, который продукт УЖЕ несёт, а экран
// назвать не умеет.
//
// Дыра найдена 05.09.2026 при добавлении anytls и tuic. Комментарий в самом
// `Servery.tsx` описывает цену точно: «транспорт, которого нет в этой карте,
// показывается пустой ячейкой, и это читается как сломанная запись, а не как
// новая». Описание было, проверки не было, и оба прошлых расширения набора
// (03.09.2026, пять транспортов) держались на том, что автор не забыл.
//
// Сверяется РЕЕСТР ЯДРА с картой экрана, а не список со списком: реестр
// genkonfig.Izvestnyy это единственный судья того, что ядро действительно
// умеет, и любой новый транспорт проходит через него по построению.
const putServeryTSX = "../../cmd/affory-ui/frontend/src/ekrany/Servery.tsx"

// Ключи карты TRANSPORT: и голые (`xhttp:`), и в кавычках (`"reality-tcp":`).
// Ловить одну форму значит объявить половину карты отсутствующей.
//
// Якорь это начало строки ИЛИ запятая, а не только начало строки: карта
// записана по несколько пар в строку, и версия с якорем `^` нашла первый ключ
// каждой строки, а остальные объявила отсутствующими. Первый прогон этого теста
// обвинил экран в десяти пропусках, из которых настоящими были два.
// Флаг `(?m)` обязателен: без него `^` в Go означает начало ВСЕГО текста, а не
// строки, и ключ, стоящий первым в своей строке, не находится вовсе. Так
// потерялся httpupgrade во втором прогоне.
var reKlyuchTransporta = regexp.MustCompile(`(?m)(?:^|[,{])\s*(?:"([a-z0-9-]+)"|([a-z0-9-]+))\s*:\s*"`)

func TestEkranNazyvaetVseTransportyYadra(t *testing.T) {
	b, err := os.ReadFile(filepath.FromSlash(putServeryTSX))
	if err != nil {
		t.Fatalf("Servery.tsx не прочитан: %v", err)
	}
	// Карта берётся куском, а не файлом целиком: в файле есть и другие объекты
	// со строковыми полями, и разбор по всему файлу зазеленел бы на них.
	tekst := string(b)
	nachalo := strings.Index(tekst, "const TRANSPORT")
	if nachalo < 0 {
		t.Fatal("в Servery.tsx нет карты TRANSPORT: сломан разбор, а не экран")
	}
	konec := strings.Index(tekst[nachalo:], "};")
	if konec < 0 {
		t.Fatal("карта TRANSPORT не закрыта: сломан разбор, а не экран")
	}
	karta := tekst[nachalo : nachalo+konec]

	naEkrane := map[string]bool{}
	for _, m := range reKlyuchTransporta.FindAllStringSubmatch(karta, -1) {
		imya := m[1]
		if imya == "" {
			imya = m[2]
		}
		naEkrane[imya] = true
	}
	if len(naEkrane) == 0 {
		t.Fatal("в карте TRANSPORT не нашлось ни одного ключа: сломан разбор словаря, а не экран")
	}

	var netu []string
	for _, transport := range genkonfig.VseTransporty() {
		if !naEkrane[transport] {
			netu = append(netu, transport)
		}
	}
	sort.Strings(netu)
	if len(netu) > 0 {
		t.Fatalf("ядро несёт транспорты, которых экран назвать не может: %s\n"+
			"в списке серверов они покажутся пустой ячейкой, то есть сломанной записью",
			strings.Join(netu, ", "))
	}
}
