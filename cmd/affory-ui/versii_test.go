package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// Точная версия у рантайма Wails и «хоть какая-то» у остальных.
//
// `latest` в зависимости означает, что вчерашняя сборка и сегодняшняя это
// разные программы, а выясняется это в тот день, когда что-то ломается. В
// veloce `@wailsio/runtime` стоит именно так, и повторять это незачем.
// Предрелизный хвост разрешён: у @wailsio/runtime ВСЕ опубликованные 3.x
// это alpha и beta, точная строка под beta.16 выглядит как `3.0.0-beta.16`.
// Без хвоста тест был бы красным навсегда, и это нашёл разбор 02.09.2026.
var tochnaya = regexp.MustCompile(`^\d+\.\d+\.\d+(-[0-9A-Za-z.]+)?$`)

func TestVersiiFrontendaZakrepleny(t *testing.T) {
	// Relative to the package dir: go test chdirs here, and the frontend
	// lives inside cmd/affory-ui because go:embed refuses to climb "..".
	syroe, err := os.ReadFile(filepath.FromSlash("frontend/package.json"))
	if err != nil {
		t.Fatalf("package.json не прочитан: %v", err)
	}
	var p struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(syroe, &p); err != nil {
		t.Fatalf("package.json не разобран: %v", err)
	}

	v := p.Dependencies["@wailsio/runtime"]
	if v == "" {
		t.Fatal("@wailsio/runtime не объявлен вовсе")
	}
	if !tochnaya.MatchString(v) {
		t.Errorf("версия рантайма не закреплена точно: %q", v)
	}

	// У остальных достаточно запрета на плавающее: диапазон `^` мы принимаем,
	// `latest` и `*` нет. Требовать точную версию у всего значит завести себе
	// ручное обновление двадцати строк, а выигрыш только у рантайма.
	vsyo := map[string]string{}
	for k, val := range p.Dependencies {
		vsyo[k] = val
	}
	for k, val := range p.DevDependencies {
		vsyo[k] = val
	}
	if len(vsyo) == 0 {
		t.Fatal("зависимостей нет вовсе: проверять нечего, а значит проверка зелена зря")
	}
	for imya, val := range vsyo {
		if val == "latest" || val == "*" || val == "" {
			t.Errorf("зависимость %s плавает: %q", imya, val)
		}
	}
}
