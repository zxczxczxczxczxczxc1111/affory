package set

import (
	"os"
	"testing"
	"time"

	"go.uber.org/goleak"
)

const peremennayaZavisnut = "AFFORY_TEST_ZAVISNUT"

// Н9 аудита 1.6.1: зависший netsh держал подъём туннеля и снятие защиты без
// конца. Вызов обязан вернуться отказом за свой срок.
func TestZavisshiyNetshNeDerzhitVechno(t *testing.T) {
	t.Setenv(peremennayaZavisnut, "1")
	prog, srok := programmaNetsh, srokNetsh
	t.Cleanup(func() { programmaNetsh, srokNetsh = prog, srok })
	programmaNetsh = func() string { return os.Args[0] }
	srokNetsh = 300 * time.Millisecond

	nachalo := time.Now()
	_, err := vypolnitNetsh([]string{"advfirewall", "show", "allprofiles"})
	if err == nil {
		t.Fatal("зависший netsh ответил успехом")
	}
	if proshlo := time.Since(nachalo); proshlo > 5*time.Second {
		t.Fatalf("вызов вернулся через %v при сроке %v", proshlo, srokNetsh)
	}
}

// Тот же механизм, что и в internal/yadra: горутина, пережившая набор, это
// брошенный пул соединений с другой стороны. Ловит прогон, а не бдительность.
func TestMain(m *testing.M) {
	// Тестовый бинарь, запущенный с этой переменной, изображает зависший netsh.
	if os.Getenv(peremennayaZavisnut) == "1" {
		time.Sleep(time.Hour)
		os.Exit(0)
	}
	goleak.VerifyTestMain(m,
		// Резолвер Windows, а не наша утечка. TestNastoyashchiyRezolverNeRazreshaetInvalid
		// намеренно спрашивает живой резолвер про имя в .invalid, а net.Resolver
		// под Windows зовёт БЛОКИРУЮЩУЮ GetAddrInfoW в отдельной горутине, и
		// отменить её нечем: контекст возвращает управление, системный вызов
		// продолжает висеть. Исключение адресное, по кадру стека, а не по
		// «всё, что сидит в syscall»: иначе оно накрыло бы и настоящие утечки.
		goleak.IgnoreAnyFunction("net.(*Resolver).lookupIP.func1"),
	)
}
