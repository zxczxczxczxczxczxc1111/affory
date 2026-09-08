package kanal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

func telo(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("тело не сериализуется: %v", err)
	}
	return b
}

func TestKlientOtvergaetChuzhuyuVersiyu(t *testing.T) {
	// Two halves from different releases compile fine and misbehave subtly. The
	// version field turns that into one loud, boring error.
	otvet := protokol.Kadr{Tip: "otvet", Imya: "hello", Telo: telo(t, map[string]any{"protocol": 99})}
	if err := proveritHello(otvet); err == nil {
		t.Fatal("чужая версия принята")
	} else if !strings.Contains(err.Error(), protokol.KodProtocolMismatch) {
		t.Fatalf("не тот код: %v", err)
	}
}

func TestKlientPrinimaetSvoyuVersiyu(t *testing.T) {
	// The mirror of the test above. Without it a proveritHello that rejects
	// everything would pass the suite and fail every real connection.
	otvet := protokol.Kadr{Tip: "otvet", Imya: "hello", Telo: telo(t, map[string]any{"protocol": protokol.Versiya})}
	if err := proveritHello(otvet); err != nil {
		t.Fatalf("своя же версия отвергнута: %v", err)
	}
}

// Находка 25. Канала НЕТ это не захват канала.
//
// Прежний тест требовал ровно обратного и тем закреплял ложь: несуществующее
// имя давало pipe-squatted, то есть человек при остановленной службе читал
// обвинение чужой программы в захвате. По спеке §9.1 это no-admin: «служба не
// установлена ИЛИ не отвечает», экран первого запуска с кнопкой установки.
func TestOtsutstvuyushchiyKanalEtoNeZahvat(t *testing.T) {
	err := proveritVladeltsaPoImeni(`\\.\pipe\affory-net-takogo`)
	if err == nil {
		t.Fatal("несуществующий канал прошёл проверку")
	}
	if strings.Contains(err.Error(), protokol.KodPipeSquatted) {
		t.Fatalf("остановленная служба обвиняет чужую программу: %v", err)
	}
	if !strings.Contains(err.Error(), protokol.KodNoAdmin) {
		t.Fatalf("не тот код: %v", err)
	}
}

// КОНТРОЛЬ 1: имя, которое ДЕРЖИТ не наша служба, обязано остаться захватом.
//
// Здесь срабатывает ветка «владелец не читается»: у канала одна свободная
// ячейка, её занимает сам слушатель, и запрос дескриптора получает
// ERROR_PIPE_BUSY. Для человека это ровно то же событие: наше имя занято.
func TestZanyatoeImyaEtoZahvat(t *testing.T) {
	imya := `\\.\pipe\affory-proba-zahvata`
	l, err := slushatImenem(imya, "D:P(A;;GA;;;WD)")
	if err != nil {
		t.Skipf("канал не завёлся, проверять нечего: %v", err)
	}
	defer l.Close()

	err = proveritVladeltsaPoImeni(imya)
	if err == nil {
		t.Fatal("занятое имя прошло проверку: скваттинг не ловится вовсе")
	}
	if !strings.Contains(err.Error(), protokol.KodPipeSquatted) {
		t.Fatalf("занятое имя дало не pipe-squatted: %v", err)
	}
}

// КОНТРОЛЬ 2: владелец, который читается и НЕ является LocalSystem.
//
// Отдельным тестом, потому что контроль выше до этой ветки не доходит, и без
// него правка про отсутствующий канал прошла бы вместе с превращением всей
// проверки владельца в no-admin. Цель это обычный файл: проверка работает по
// SE_FILE_OBJECT, а файл во временном каталоге принадлежит текущему
// пользователю гарантированно, тогда как канал с нужным владельцем в тесте не
// завести.
func TestVladelecNeSystemEtoZahvat(t *testing.T) {
	fayl := filepath.Join(t.TempDir(), "proba.txt")
	if err := os.WriteFile(fayl, []byte("proba"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := proveritVladeltsaPoImeni(fayl)
	if err == nil {
		t.Fatal("чужой владелец прошёл проверку")
	}
	if !strings.Contains(err.Error(), protokol.KodPipeSquatted) {
		t.Fatalf("чужой владелец дал не pipe-squatted: %v", err)
	}
}
