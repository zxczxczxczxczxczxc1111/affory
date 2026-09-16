package kachestvo_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Выпуск, собранный без метки production, это ОТЛАДОЧНАЯ сборка Wails, и она
// уезжает людям выглядя обычной.
//
// 16.09.2026 в госте правый щелчок открывал меню Chromium, хотя окно просит
// DefaultContextMenuDisabled: true. Опция не при чём: Wails складывает её с
// режимом сборки по ИЛИ, и отладочный режим включает меню поверх просьбы окна
// (webview_window_windows.go, вызов PutAreDefaultContextMenusEnabled).
// Отладочным считается всё, что собрано без метки production
// (pkg/application/application_debug.go). Тем же тумблером включается панель
// разработчика и логгер в stdout, то есть выпуски 1.0.0-1.1.3 ушли людям с
// живой панелью разработчика.
//
// Прежние ворота этого не видели по построению: okno_test.go судит опцию, а
// опцию ничто не перебивает в исходнике, перебивает МЕТКА СБОРКИ. Поэтому
// ворота смотрят на команду сборки, а не на код.
const putSkriptaVypuska = "../../ustanovka/sobrat-reliz.ps1"

// Строка сборки окна: `go build ... ./cmd/affory-ui`. Метка ищется в той же
// строке, иначе ворота зазеленеют от слова production в соседнем комментарии.
var reSborkaOkna = regexp.MustCompile(`(?m)^.*\bgo build\b.*\./cmd/affory-ui\s*$`)

func TestVypuskOknaSobiraetsyaSMetkoyProduction(t *testing.T) {
	skript, err := os.ReadFile(filepath.FromSlash(putSkriptaVypuska))
	if err != nil {
		t.Fatalf("sobrat-reliz.ps1 не прочитан: %v", err)
	}
	stroki := reSborkaOkna.FindAllString(string(skript), -1)
	if len(stroki) == 0 {
		t.Fatal("в sobrat-reliz.ps1 не нашлось строки сборки ./cmd/affory-ui: " +
			"сломан разбор скрипта, а не сборка")
	}
	for _, s := range stroki {
		if !strings.Contains(s, "-tags production") {
			t.Errorf("окно собирается без -tags production, значит в отладочном режиме Wails "+
				"(правое меню и панель разработчика включены поверх наших опций):\n%s",
				strings.TrimSpace(s))
		}
	}
}
