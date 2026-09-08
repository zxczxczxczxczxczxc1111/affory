package main

import (
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// putOboloshki берётся ПОЛНЫМ путём модуля, а не относительным шаблоном.
//
// Замерено: go test запускается из каталога самого пакета, и "./cmd/affory-ui/..."
// оттуда не резолвится. Ошибка при этом приезжает не в err, а внутрь p.Errors,
// поэтому t.Fatal(err) молчит, список пакетов оказывается пустым, цикл не
// находит ничего и тест зелен ВСЕГДА. Это класс «приёмочный инструмент врёт»:
// граница считается охраняемой, а не охраняется вовсе.
const putOboloshki = "github.com/zxczxczxczxczxczxc1111/affory/cmd/affory-ui/..."

// Пакеты, которых оболочке иметь нельзя ни прямо, ни через посредника.
//
// Оболочка живёт БЕЗ прав администратора. Затащив сюда genkonfig или set,
// половина логики переезжает в процесс, который не имеет права её исполнять, и
// разделение службы и интерфейса становится театром.
var zapreshchenoOboloshke = []string{
	"internal/genkonfig",
	"internal/set",
	"internal/hranenie",
	"internal/yadra",
	"internal/sostoyanie",
}

func TestOboloshkaNeTyanetLishnego(t *testing.T) {
	pak, err := packages.Load(&packages.Config{
		// NeedName нужен ради PkgPath: без него путь пуст и сверять нечего.
		// NeedDeps вместе с NeedImports заполняет карту импортов ТРАНЗИТИВНО,
		// поэтому обход ниже видит и посредников. Поля p.Deps у packages.Package
		// не существует, проверено сборкой на x/tools v0.49.0.
		Mode: packages.NeedName | packages.NeedImports | packages.NeedDeps,
	}, putOboloshki)
	if err != nil {
		t.Fatal(err)
	}
	// ДВА заслона против молчаливой зелени, и оба обязательны.
	//
	// Первый: ошибки загрузки лежат в p.Errors, а не в err, и без этой строки
	// сломанный шаблон читается как «запрещённого не нашли».
	if n := packages.PrintErrors(pak); n > 0 {
		t.Fatalf("пакеты не загрузились, ошибок %d: судить о границе нечем", n)
	}
	// Второй: ноль пакетов это тоже «ничего не нашли», и выглядит как успех.
	if len(pak) == 0 {
		t.Fatalf("по шаблону %s не загрузилось ни одного пакета", putOboloshki)
	}

	// Транзитивный обход. Прямой проверки мало: оболочка, тянущая set через
	// промежуточный пакет, проехала бы мимо.
	packages.Visit(pak, nil, func(p *packages.Package) {
		for _, z := range zapreshchenoOboloshke {
			if strings.Contains(p.PkgPath, z) {
				t.Errorf("оболочка тянет %s (запрещено: %s)", p.PkgPath, z)
			}
		}
	})
}

// Сторож самого сторожа: список запрещённого не имеет права опустеть.
//
// Пустой список делает проверку выше зелёной при любом импорте, и заметить это
// нечем: тест по-прежнему называется так же и по-прежнему проходит.
func TestSpisokZapreshchennogoNePust(t *testing.T) {
	if len(zapreshchenoOboloshke) == 0 {
		t.Fatal("список запрещённого пуст: граница объявлена и не охраняется")
	}
}
