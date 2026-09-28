package yadra

import (
	"os"
	"testing"
	"time"

	"go.uber.org/goleak"
)

const peremennayaZavisnut = "AFFORY_TEST_ZAVISNUT"

// Н9 аудита 1.6.1: зависший `sing-box check` держал подъём туннеля без конца.
func TestZavisshayaProverkaKonfigaNeDerzhitVechno(t *testing.T) {
	t.Setenv(peremennayaZavisnut, "1")
	srok := srokProverki
	t.Cleanup(func() { srokProverki = srok })
	srokProverki = 300 * time.Millisecond

	nachalo := time.Now()
	err := proveritKonfig(os.Args[0], "net-takogo.json")
	if err == nil {
		t.Fatal("зависшая проверка ответила успехом")
	}
	if proshlo := time.Since(nachalo); proshlo > 5*time.Second {
		t.Fatalf("проверка вернулась через %v при сроке %v", proshlo, srokProverki)
	}
}

// Утечка соединений 10.09.2026 закрыта не только починкой, но и МЕХАНИЗМОМ.
//
// Транспорт, брошенный с непустым пулом, держит по две горутины на соединение
// (readLoop и writeLoop persistConn), и именно они не дают сборщику мусора его
// забрать. Значит горутина, пережившая набор, это тот же дефект с другой
// стороны, и ловить его должен прогон, а не бдительность.
func TestMain(m *testing.M) {
	// Тестовый бинарь, запущенный с этой переменной, изображает зависшее ядро.
	if os.Getenv(peremennayaZavisnut) == "1" {
		time.Sleep(time.Hour)
		os.Exit(0)
	}
	goleak.VerifyTestMain(m,
		// Горутина ожидания процесса ядра: набор поднимает подставные процессы
		// и не всегда дожидается их смерти. К пулу соединений отношения не имеет.
		goleak.IgnoreTopFunction("os/exec.(*Cmd).watchCtx"),
	)
}
