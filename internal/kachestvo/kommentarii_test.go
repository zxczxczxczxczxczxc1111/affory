package kachestvo_test

import (
	"go/ast"
	"go/token"
	"go/types"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// Ш5 ворот приёмки, класс дефекта C: комментарий читается как описание
// работающего механизма и переживает его смерть. Пойман вычиткой глазами
// четыре раза, а глазами это не ворота.
//
// Полностью класс не автоматизируется: комментарий может врать про поведение, не
// называя ни одного имени. Здесь закрывается дешёвая половина, и только она:
// имя, названное в комментарии, обязано существовать.
//
// Что считается известным именем: объединение всего, что проект ОБЪЯВЛЯЕТ,
// всего, что он ИСПОЛЬЗУЕТ, и всех его СТРОКОВЫХ литералов.
//
// Второе не менее важно первого: `svc.ChangeRequest` и `types.Implements` наши
// комментарии называют законно, и список чужих библиотек вручную вести было бы
// бессмысленно.
//
// Третье добавлено после первого прогона, и оно не послабление. Имена команд
// канала (`setKillSwitch` и два десятка соседей), коды отказа и имена правил
// брандмауэра живут в проекте СТРОКАМИ, а не идентификаторами. Они существуют
// ровно так же, и комментарий, который их называет, прав. Без этого правила
// список исключений начал бы наполняться тем, что на самом деле есть, а такой
// список через месяц прикрывает и настоящие находки.
var (
	// Обратные кавычки это единственный надёжный признак: в прозе имена у нас
	// всегда в них, а без них разбор утонул бы в обычных словах.
	reKavychki = regexp.MustCompile("`([^`\n]+)`")
	// Похоже на имя Go: буквы, цифры, подчёркивания, точки для пакета, хвост
	// со скобками. Всё с дефисом, слэшем или пробелом отсеивается тут же, то
	// есть netsh-команды и пути к файлам сюда не попадают вовсе.
	reImya = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*(\(\))?$`)
	// Заглавная ВНУТРИ слова. Без этого условия в разбор уезжают `chrome`,
	// `check`, `block` и прочая проза, а с ним остаются ровно составные имена.
	reVnutriZaglavnaya = regexp.MustCompile(`[a-z0-9][A-Z]`)
)

// Исключения с причиной. Ключ это само слово из комментария.
var slovaIsklyucheniya = map[string]string{}

func TestKommentariyNazyvaetSushchestvuyushchee(t *testing.T) {
	rezhim := packages.NeedName | packages.NeedSyntax | packages.NeedTypes |
		packages.NeedTypesInfo | packages.NeedDeps | packages.NeedImports
	// Tests: true, в отличие от ворот на мёртвый экспорт. Там тесты мешали, тут
	// нужны: комментарий в тесте врёт ровно так же, как в проде.
	pkgs, err := packages.Load(&packages.Config{Mode: rezhim, Tests: true, Dir: "../.."}, korenProekta)
	if err != nil {
		t.Fatal(err)
	}
	if n := packages.PrintErrors(pkgs); n > 0 {
		t.Fatalf("пакеты не загрузились: %d ошибок", n)
	}

	izvestno := map[string]bool{}
	dobavit := func(o types.Object) {
		if o == nil {
			return
		}
		izvestno[o.Name()] = true
		if o.Pkg() != nil {
			izvestno[o.Pkg().Name()] = true
		}
	}
	for _, p := range pkgs {
		izvestno[p.Name] = true
		for _, o := range p.TypesInfo.Defs {
			dobavit(o)
		}
		for _, o := range p.TypesInfo.Uses {
			dobavit(o)
		}
		for _, f := range p.Syntax {
			ast.Inspect(f, func(n ast.Node) bool {
				if l, est := n.(*ast.BasicLit); est && l.Kind == token.STRING {
					if v, err := strconv.Unquote(l.Value); err == nil {
						izvestno[v] = true
					}
				}
				return true
			})
		}
	}

	type nahodka struct{ slovo, gde string }
	var nahodki []nahodka
	videli := map[string]bool{}

	for _, p := range pkgs {
		for _, f := range p.Syntax {
			put := p.Fset.Position(f.Pos()).Filename
			// Один и тот же файл приезжает в нескольких вариантах пакета при
			// Tests: true. Без этого отсева одна находка печаталась бы трижды.
			for _, gruppa := range f.Comments {
				for _, s := range gruppa.List {
					for _, m := range reKavychki.FindAllStringSubmatch(s.Text, -1) {
						slovo := strings.TrimSuffix(m[1], "()")
						if !podozritelno(slovo, izvestno) {
							continue
						}
						klyuch := slovo + "\x00" + put
						if videli[klyuch] {
							continue
						}
						videli[klyuch] = true
						gde := p.Fset.Position(s.Pos()).String()
						nahodki = append(nahodki, nahodka{slovo, gde})
					}
				}
			}
		}
	}

	sort.Slice(nahodki, func(i, j int) bool { return nahodki[i].slovo < nahodki[j].slovo })
	for _, n := range nahodki {
		if prichina, est := slovaIsklyucheniya[n.slovo]; est {
			t.Logf("пропущено %s: %s", n.slovo, prichina)
			continue
		}
		t.Errorf("%s: комментарий называет `%s`, а такого имени в проекте нет", n.gde, n.slovo)
	}
	t.Logf("известных имён %d, подозрительных упоминаний %d", len(izvestno), len(nahodki))
}

func podozritelno(slovo string, izvestno map[string]bool) bool {
	if !reImya.MatchString(slovo) || !reVnutriZaglavnaya.MatchString(slovo) {
		return false
	}
	// Точечная форма проверяется по ХВОСТУ: `ssylki.Slit` это про `Slit`, а
	// пакет и так известен.
	if i := strings.LastIndex(slovo, "."); i >= 0 {
		slovo = slovo[i+1:]
	}
	return !izvestno[slovo]
}
