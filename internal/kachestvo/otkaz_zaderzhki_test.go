package kachestvo_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Экран серверов отличает «замер не вышел» от «мерить было нечем» по ПОДСТРОКЕ
// в тексте отказа, который пишет служба. Связь держится на совпадении двух
// строк в разных языках, и переименование в службе ломает вид молча: строка
// поедет, а окно продолжит рисовать «VPN недоступен» там, где человек просто
// не подключён.
//
// Найдено 13.09.2026 при сведении словаря: служба сменила «туннель опущен» на
// «VPN отключён», окно осталось со старой подстрокой, и ни один тест не
// покраснел.
const (
	putZaderzhek = "../../cmd/affory-svc/zaderzhki.go"
	putServeryTS = "../../cmd/affory-ui/frontend/src/ekrany/Servery.tsx"
)

func TestOknoUznayotOtkazZaderzhkiSluzhby(t *testing.T) {
	sluzhba, err := os.ReadFile(filepath.FromSlash(putZaderzhek))
	if err != nil {
		t.Fatalf("не прочитан %s: %v", putZaderzhek, err)
	}
	okno, err := os.ReadFile(filepath.FromSlash(putServeryTS))
	if err != nil {
		t.Fatalf("не прочитан %s: %v", putServeryTS, err)
	}

	// Подстрока берётся из окна, а ищется в службе: окно решает, что именно оно
	// узнаёт, и оно же обязано остаться правым.
	const marker = `realping_otkaz.includes("`
	i := strings.Index(string(okno), marker)
	if i < 0 {
		t.Fatalf("в %s больше нет разбора realping_otkaz по подстроке: проверка устарела", putServeryTS)
	}
	hvost := string(okno)[i+len(marker):]
	konec := strings.Index(hvost, `"`)
	if konec <= 0 {
		t.Fatalf("подстрока в %s не закрыта кавычкой", putServeryTS)
	}
	podstroka := hvost[:konec]

	if !strings.Contains(string(sluzhba), podstroka) {
		t.Errorf("окно ищет в отказе задержки %q, а служба такого не пишет: %s", podstroka, putZaderzhek)
	}
}
