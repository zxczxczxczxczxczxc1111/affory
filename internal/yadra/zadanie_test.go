package yadra

import (
	"errors"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

// dolgiyProcess запускает процесс, который сам по себе не закончится за время
// теста. ping с сотней пакетов есть на любой Windows и не требует ни сети, ни
// прав.
func dolgiyProcess(t *testing.T) *exec.Cmd {
	t.Helper()
	c := exec.Command("ping.exe", "-n", "100", "127.0.0.1")
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := c.Start(); err != nil {
		t.Fatalf("подопытный процесс не запустился: %v", err)
	}
	t.Cleanup(func() {
		_ = c.Process.Kill()
		_, _ = c.Process.Wait()
	})
	return c
}

// umer ждёт смерти процесса не дольше срока.
func umer(c *exec.Cmd, srok time.Duration) bool {
	gotovo := make(chan struct{})
	go func() { _, _ = c.Process.Wait(); close(gotovo) }()
	select {
	case <-gotovo:
		return true
	case <-time.After(srok):
		return false
	}
}

func TestYadraUmirayutSRoditelem(t *testing.T) {
	// Объекта задания в проекте не было вовсе: SysProcAttr нёс один HideWindow.
	// Убийство службы оставляло ядра живыми при поднятом туннеле, то есть
	// машину, трафик которой идёт через выход, которым никто не управляет.
	//
	// Проверяется несущее свойство: закрытие ДЕСКРИПТОРА задания убивает всё,
	// что в нём. Смерть родителя это тот же механизм, потому что смерть
	// процесса закрывает его дескрипторы, и отдельного кода для неё нет.
	z, err := NovoeZadanie()
	if err != nil {
		t.Fatalf("задание не создалось: %v", err)
	}
	c := dolgiyProcess(t)
	if err := z.Prinyat(c.Process.Pid); err != nil {
		t.Fatalf("процесс не назначен: %v", err)
	}
	if umer(c, 300*time.Millisecond) {
		t.Fatal("процесс умер до закрытия задания, тест ничего не доказывает")
	}
	if err := z.zakryt(); err != nil {
		t.Fatalf("задание не закрылось: %v", err)
	}
	if !umer(c, 5*time.Second) {
		t.Fatal("процесс пережил закрытие задания")
	}
}

func TestChuzhoyProcessZadanieNeTrogaet(t *testing.T) {
	// Контроль. Без него «процесс умер» ничего не значит: он мог умереть сам,
	// от таймаута, от чего угодно. Проверка обязана уметь показать разницу.
	z, err := NovoeZadanie()
	if err != nil {
		t.Fatalf("задание не создалось: %v", err)
	}
	svoy := dolgiyProcess(t)
	chuzhoy := dolgiyProcess(t)
	if err := z.Prinyat(svoy.Process.Pid); err != nil {
		t.Fatalf("процесс не назначен: %v", err)
	}
	if err := z.zakryt(); err != nil {
		t.Fatalf("задание не закрылось: %v", err)
	}
	if !umer(svoy, 5*time.Second) {
		t.Fatal("назначенный процесс выжил")
	}
	if umer(chuzhoy, time.Second) {
		t.Fatal("процесс ВНЕ задания тоже умер: проверка не отличает своих от чужих")
	}
}

func TestZadanieBezFlagaNeSchitaetsya(t *testing.T) {
	// Задание БЕЗ KILL_ON_JOB_CLOSE выглядит ровно так же: создаётся, принимает
	// процессы, не жалуется. Отличается оно только тем, ради чего заводилось.
	// Этот тест фиксирует, что проверка выше меряет именно флаг, а не сам факт
	// существования задания.
	goloe, err := goloeZadanieDlyaTesta()
	if err != nil {
		t.Fatalf("голое задание не создалось: %v", err)
	}
	c := dolgiyProcess(t)
	if err := goloe.Prinyat(c.Process.Pid); err != nil {
		t.Fatalf("процесс не назначен: %v", err)
	}
	if err := goloe.zakryt(); err != nil {
		t.Fatalf("задание не закрылось: %v", err)
	}
	if umer(c, time.Second) {
		t.Fatal("процесс умер без флага убийства: значит флаг не при чём")
	}
}

func TestZapuskBezZadaniyaOtkazyvaet(t *testing.T) {
	// Отказ задания это отказ запуска, а не «запустимся как раньше». И процесс,
	// который уже стартовал, обязан быть убит: иначе отказ порождает ровно ту
	// сироту, ради которой всё это писалось, только сразу.
	bylo := vzyatZadanie
	vzyatZadanie = func() (*Zadanie, error) { return nil, errors.New("задания нет") }
	defer func() { vzyatZadanie = bylo }()

	c := dolgiyProcess(t)
	if err := vZadanie(c); err == nil {
		t.Fatal("отказ задания не стал отказом запуска")
	}
	if !umer(c, 5*time.Second) {
		t.Fatal("процесс пережил отказ назначения: сирота получена сразу")
	}
}
