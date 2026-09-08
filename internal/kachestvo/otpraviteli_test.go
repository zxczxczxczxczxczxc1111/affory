package kachestvo_test

import (
	"go/ast"
	"go/types"
	"sort"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// Ворота 04c: у каждого кода словаря есть ОТПРАВИТЕЛЬ.
//
// Соседние ворота проверяют другое. kody_test.go сверяет словарь со спекой в обе
// стороны, otkazy_ui_test.go сверяет спеку с экранами. Обе молчат про третье:
// код объявлен, экран написан, а слать его некому. Ровно так четыре кода
// (dns-resolve-failed, server-auth-failed, wintun-missing, firewall-disabled)
// прожили до полосы З с готовыми экранами, которых человек не видел ни разу.
//
// Счёт по КОНСТАНТАМ, а не по копиям строк, и это не украшение. Прежняя сверка
// словаря уже была слеплена дубликатом строки кода в internal/obnovlenie: код
// слался, а числился непосланным. Разбор по типам такого не умеет по
// построению.
//
// TestNetMyortvogoEksporta пропускает пакет protokol целиком и печатает счёт
// непосланных со ссылкой сюда: там вопрос «удалить ли», здесь «кто шлёт».
var kodyBezOtpravitelya = map[string]string{
	"health-snapshot-stale": "не кадр отказа, а ПОЛЕ ustarel в ответе getServerHealth " +
		"(cmd/affory-svc/zdorovie.go, porogSvezhestiSnimka): возраст приезжает " +
		"числом, и рисует его Glavnyy.tsx строкой «снимок ..., устарел». Карточка " +
		"при этом не блокируется, значит отказом это не является",
}

func TestUKazhdogoKodaEstOtpravitel(t *testing.T) {
	rezhim := packages.NeedName | packages.NeedSyntax | packages.NeedTypes |
		packages.NeedTypesInfo | packages.NeedDeps | packages.NeedImports
	// Tests: false намеренно, ровно как у ворот мёртвого экспорта. Отправитель
	// в тесте это не отправитель: путь отказа остаётся незакрытым, а ворота
	// зеленеют от собственной фикстуры.
	pkgs, err := packages.Load(&packages.Config{Mode: rezhim, Dir: "../.."}, korenProekta)
	if err != nil {
		t.Fatal(err)
	}
	if n := packages.PrintErrors(pkgs); n > 0 {
		t.Fatalf("пакеты не загрузились: %d ошибок", n)
	}

	obyavleny := map[string]string{}  // имя константы -> значение на проводе
	otpravlyayut := map[string]bool{} // имя константы, у которой есть вызывающий вне protokol
	for _, p := range pkgs {
		vSlovare := strings.HasSuffix(p.PkgPath, "/internal/protokol")
		if vSlovare {
			for _, imya := range p.Types.Scope().Names() {
				if !strings.HasPrefix(imya, "Kod") {
					continue
				}
				c, est := p.Types.Scope().Lookup(imya).(*types.Const)
				if !est || c.Val() == nil {
					continue
				}
				obyavleny[imya] = strings.Trim(c.Val().String(), `"`)
			}
			// Использование ВНУТРИ словаря отправителем не считается: словарь
			// сам себе кадров не шлёт.
			continue
		}
		for _, f := range p.Syntax {
			ast.Inspect(f, func(n ast.Node) bool {
				id, est := n.(*ast.Ident)
				if !est {
					return true
				}
				c, est := p.TypesInfo.Uses[id].(*types.Const)
				if !est || c.Pkg() == nil {
					return true
				}
				if strings.HasSuffix(c.Pkg().Path(), "/internal/protokol") &&
					strings.HasPrefix(c.Name(), "Kod") {
					otpravlyayut[c.Name()] = true
				}
				return true
			})
		}
	}

	// Якорь. Без него переезд пакета или смена префикса превращают ворота в
	// зелёное ничто: сломан разбор, а не отправители исчезли.
	if len(obyavleny) == 0 {
		t.Fatal("в пакете protokol не нашлось ни одного кода: сломан разбор, а не словарь пуст")
	}
	if len(otpravlyayut) == 0 {
		t.Fatal("ни у одного кода не нашлось отправителя: сломан разбор, а не продукт молчит")
	}

	var bez []string
	for imya, znachenie := range obyavleny {
		if otpravlyayut[imya] {
			continue
		}
		if _, mozhno := kodyBezOtpravitelya[znachenie]; mozhno {
			continue
		}
		bez = append(bez, imya+" ("+znachenie+")")
	}
	sort.Strings(bez)
	for _, imya := range bez {
		t.Errorf("код %s объявлен и не шлётся ниоткуда: экран написан, путь отказа не закрыт", imya)
	}

	// Исключение обязано ЖИТЬ. Протухшее оправдание молча прикрывает следующий
	// код, попавший под тот же ключ, и это главная болезнь таких списков.
	poZnacheniyu := map[string]string{}
	for imya, z := range obyavleny {
		poZnacheniyu[z] = imya
	}
	for znachenie, prichina := range kodyBezOtpravitelya {
		imya, est := poZnacheniyu[znachenie]
		if !est {
			t.Errorf("исключение %q протухло: такого кода в словаре больше нет (%s)", znachenie, prichina)
			continue
		}
		if otpravlyayut[imya] {
			t.Errorf("исключение %q протухло: отправитель у кода появился (%s)", znachenie, prichina)
		}
	}
	t.Logf("кодов в словаре %d, с отправителем %d, исключений %d",
		len(obyavleny), len(otpravlyayut), len(kodyBezOtpravitelya))
}
