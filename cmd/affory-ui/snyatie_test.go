package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
)

func podstavnoeSnyatie(t *testing.T, kod uint32, oshibka error, prichina string) *[]string {
	t.Helper()
	prezhneeZapusk, prezhnyayaPrichina := zapustitSnyatie, prochitatPrichinu
	var zvali []string
	zapustitSnyatie = func(args string) (uint32, error) {
		zvali = append(zvali, args)
		return kod, oshibka
	}
	prochitatPrichinu = func() string { return prichina }
	prezhneeZakrytie := zakrytOkno
	zakrytOkno = func(*most) {}
	t.Cleanup(func() {
		zapustitSnyatie, prochitatPrichinu = prezhneeZapusk, prezhnyayaPrichina
		zakrytOkno = prezhneeZakrytie
	})
	return &zvali
}

// Упавшее снятие обязано доехать до человека словами службы, а не молчанием.
// До 21.09.2026 окно закрывалось, не посмотрев на код, и «удалил» означало
// «нажал кнопку».
func TestUpavsheeSnyatieOtdayotPrichinuSluzhby(t *testing.T) {
	podstavnoeSnyatie(t, 1, nil, "снятие не удалось: остановка не принята")

	err := (&most{}).UdalitProgrammu(true)
	if err == nil {
		t.Fatal("отказ снятия пропущен как успех")
	}
	if !strings.Contains(err.Error(), "остановка не принята") {
		t.Fatalf("причина службы потеряна: %v", err)
	}
}

// Причины на диске может не быть: служба падала так, что не успела её
// положить. Тогда человеку хотя бы код, а не тишина.
func TestOtkazBezPrichinyNazyvaetKod(t *testing.T) {
	podstavnoeSnyatie(t, 7, nil, "")

	err := (&most{}).UdalitProgrammu(false)
	if err == nil || !strings.Contains(err.Error(), "7") {
		t.Fatalf("код отказа не назван: %v", err)
	}
}

// Флаг стирания ключей это единственное написание, которое служба понимает.
func TestFlagKlyuchevyyEdetTolkoKogdaSoglasilis(t *testing.T) {
	zvali := podstavnoeSnyatie(t, 0, nil, "")
	if err := (&most{}).UdalitProgrammu(false); err != nil {
		t.Fatalf("успешное снятие обернулось отказом: %v", err)
	}
	if len(*zvali) != 1 || (*zvali)[0] != "uninstall" {
		t.Fatalf("без согласия ушло %q", *zvali)
	}

	zvali = podstavnoeSnyatie(t, 0, nil, "")
	if err := (&most{}).UdalitProgrammu(true); err != nil {
		t.Fatalf("успешное снятие обернулось отказом: %v", err)
	}
	if len(*zvali) != 1 || (*zvali)[0] != "uninstall --steret-klyuchi" {
		t.Fatalf("с согласием ушло %q", *zvali)
	}
}

// Отказ запуска (человек не дал прав) едет как есть: подменять его кодом
// возврата значит терять единственное, что человек может исправить.
func TestOtkazZapuskaEdetKakEst(t *testing.T) {
	podstavnoeSnyatie(t, 0, errors.New("права не выданы: запрос отклонён"), "")

	err := (&most{}).UdalitProgrammu(true)
	if err == nil || !strings.Contains(err.Error(), "права не выданы") {
		t.Fatalf("отказ прав потерян: %v", err)
	}
}

// Файл причины пишется в UTF-16LE без BOM: так его кладёт служба и так же
// читает установщик. Чтение проверяется на настоящем файле рядом с бинарём
// теста, потому что prichinaSnyatiya ищет его именно там.
func TestPrichinaChitaetsyaVUtf16(t *testing.T) {
	svoy, err := os.Executable()
	if err != nil {
		t.Skip("свой путь недоступен")
	}
	put := filepath.Join(filepath.Dir(svoy), imyaFaylaPrichiny)
	tekst := "данные не удалены: каталог занят"
	shiroko := utf16.Encode([]rune(tekst))
	bayty := make([]byte, 0, len(shiroko)*2)
	for _, w := range shiroko {
		bayty = append(bayty, byte(w), byte(w>>8))
	}
	if err := os.WriteFile(put, bayty, 0o600); err != nil {
		t.Skipf("каталог теста недоступен на запись: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(put) })

	if got := prichinaSnyatiya(); got != tekst {
		t.Fatalf("причина прочитана как %q", got)
	}
}
