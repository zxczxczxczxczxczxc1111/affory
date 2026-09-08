package kachestvo_test

import (
	"go/types"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// Правило файла `kody.go` звучит так: «ни одного кода сверх без строки в §9.1
// спеки». Правило было, проверки не было, и 02.09.2026 сверка руками нашла ТРИ
// расхождения разом: заголовок говорил «двадцать два» при двадцати четырёх, а у
// `tunnel-not-carrying` и `name-resolve-failed` строки в спеке не было вовсе,
// хотя оба живут в проде с волны 3.
//
// Сверка идёт в ОБЕ стороны. Односторонняя ловит только забытую строку и
// пропускает обратное: строку в спеке про код, которого в коде нет, то есть
// обещанный экран, которому нечем появиться.
const putSpeki = "../../../vpn/docs/superpowers/specs/2026-08-31-affory-klient-design.md"

// Исключение ОДНО и оно объявлено самим кодом: `not-implemented` это леса,
// которые обязаны исчезнуть к волне 6, и строки в спеке у них быть не должно.
var kodyBezStroki = map[string]string{
	"not-implemented": "леса: команда, которой ещё нет, отвечает вместо того чтобы висеть; " +
		"по правилу самого kody.go обязана исчезнуть к волне 6",
}

func TestKodyProtokolaOpisanyVSpeke(t *testing.T) {
	syroe, err := os.ReadFile(filepath.FromSlash(putSpeki))
	if err != nil {
		t.Skipf("спека недоступна (%v): сверять не с чем, и притворяться, что сверили, хуже", err)
	}
	// Разбор ограничен разделом §9.1, а не всей спекой. Первый заход этого не
	// делал и притащил `geosite`, `httpupgrade` и `pktmon` из соседних таблиц:
	// строка вида «| `слово` |» в markdown встречается где угодно.
	spec, est := razdel91(string(syroe))
	if !est {
		t.Fatal("в спеке не найден раздел 9.1: сверять не с чем")
	}

	vKode := kodyIzPaketa(t)
	if len(vKode) == 0 {
		t.Fatal("в пакете protokol не нашлось ни одного кода: сверка ничего не проверяет")
	}

	// Строка таблицы §9.1 начинается с кода в обратных кавычках. Разбор именно
	// по началу строки, а не по всему тексту: код, упомянутый в прозе, это не
	// описание экрана, а ссылка на него.
	reStroka := regexp.MustCompile("(?m)^[|] `([a-z0-9-]+)`")
	vSpeke := map[string]bool{}
	for _, m := range reStroka.FindAllStringSubmatch(spec, -1) {
		vSpeke[m[1]] = true
	}
	if len(vSpeke) == 0 {
		t.Fatal("в спеке не нашлось ни одной строки таблицы отказов: разбор сломан, а не спека пуста")
	}

	var netVSpeke, netVKode []string
	for kod := range vKode {
		if _, mozhno := kodyBezStroki[kod]; mozhno {
			continue
		}
		if !vSpeke[kod] {
			netVSpeke = append(netVSpeke, kod)
		}
	}
	for kod := range vSpeke {
		if !vKode[kod] {
			netVKode = append(netVKode, kod)
		}
	}
	sort.Strings(netVSpeke)
	sort.Strings(netVKode)

	for _, kod := range netVSpeke {
		t.Errorf("код %q есть в kody.go и НЕ описан в §9.1: экран для него никто не проектировал", kod)
	}
	for _, kod := range netVKode {
		t.Errorf("код %q описан в §9.1 и НЕ объявлен в kody.go: обещан экран, которому нечем появиться", kod)
	}
	for kod, prichina := range kodyBezStroki {
		if !vKode[kod] {
			t.Errorf("исключение %q протухло: такого кода в проекте больше нет (%s)", kod, prichina)
		}
	}
	t.Logf("кодов в коде %d, строк в §9.1 %d, исключений %d",
		len(vKode), len(vSpeke), len(kodyBezStroki))
}

// kodyIzPaketa берёт ЗНАЧЕНИЯ констант, а не имена: в спеке живут именно
// значения на проводе, а имя в Go может отличаться от строки как угодно.
func kodyIzPaketa(t *testing.T) map[string]bool {
	t.Helper()
	rezhim := packages.NeedName | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedSyntax
	pkgs, err := packages.Load(&packages.Config{Mode: rezhim, Dir: "../.."},
		"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol")
	if err != nil {
		t.Fatal(err)
	}
	if n := packages.PrintErrors(pkgs); n > 0 {
		t.Fatalf("пакет protokol не загрузился: %d ошибок", n)
	}
	itog := map[string]bool{}
	for _, p := range pkgs {
		for _, imya := range p.Types.Scope().Names() {
			// Только Kod*. Состояния (Sost*) это НЕ отказы, они живут в своём
			// разделе спеки, и требовать им строку в таблице отказов значило бы
			// заводить ворота, которые обязаны быть красными всегда.
			if !strings.HasPrefix(imya, "Kod") {
				continue
			}
			c, est := p.Types.Scope().Lookup(imya).(*types.Const)
			if !est || c.Val() == nil {
				continue
			}
			znachenie := strings.Trim(c.Val().String(), `"`)
			if znachenie != "" {
				itog[znachenie] = true
			}
		}
	}
	return itog
}

// razdel91 вырезает текст от заголовка §9.1 до следующего заголовка.
func razdel91(ves string) (string, bool) {
	const zagolovok = "### 9.1."
	i := strings.Index(ves, zagolovok)
	if i < 0 {
		return "", false
	}
	hvost := ves[i+len(zagolovok):]
	perevod := string(rune(10))
	for _, granitsa := range []string{perevod + "### ", perevod + "## "} {
		if j := strings.Index(hvost, granitsa); j >= 0 {
			hvost = hvost[:j]
		}
	}
	return hvost, true
}
