package kachestvo_test

import (
	"os"
	"path/filepath"
	"testing"
)

// Б7 аудита 1.6.1. Сверки со спекой пропускались молча, когда спеки не было на
// месте, и ворота печатали ok за проверку, которой не было. Ворота задают путь
// переменной AFFORY_SPEKA: при заданной переменной недоступная спека это
// провал. Без переменной (публичный репозиторий, CI) остаётся пропуск: спека
// живёт в частном репозитории.

func speka() (dannye []byte, yavno bool, err error) {
	put := os.Getenv("AFFORY_SPEKA")
	yavno = put != ""
	if !yavno {
		put = filepath.FromSlash(putSpeki)
	}
	dannye, err = os.ReadFile(put)
	return dannye, yavno, err
}

// trebovatSpeku отдаёт текст спеки или останавливает тест: провалом, если путь
// задан явно, пропуском, если нет.
func trebovatSpeku(t *testing.T) string {
	t.Helper()
	dannye, yavno, err := speka()
	if err != nil {
		if yavno {
			t.Fatalf("спека по AFFORY_SPEKA недоступна (%v): ворота требуют сверки", err)
		}
		t.Skipf("спека недоступна (%v): сверять не с чем, и притворяться, что сверили, хуже", err)
	}
	return string(dannye)
}

func TestYavnyyPutKSpekeNeProshchaetOtsutstviya(t *testing.T) {
	t.Setenv("AFFORY_SPEKA", filepath.Join(t.TempDir(), "net-takoy.md"))
	if _, yavno, err := speka(); !yavno || err == nil {
		t.Fatalf("явный путь к несуществующей спеке: явно=%v, ошибка=%v", yavno, err)
	}
	t.Setenv("AFFORY_SPEKA", "")
	if _, yavno, _ := speka(); yavno {
		t.Fatal("пустая переменная принята за явный путь")
	}
}
