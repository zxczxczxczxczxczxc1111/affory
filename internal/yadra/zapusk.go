package yadra

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/hranenie"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/sostoyanie"
)

// A watchdog is four things, and a delay formula is one of them. The other three
// are how you learn the process died, when to stop counting failures, and how to
// be told to go away.
type Yadro struct {
	cmd    *exec.Cmd
	imya   string
	smert  chan error // written by Wait, read by the watchdog
	podnyt time.Time
}

// Cores are separate processes on purpose: one of them dying should not take
// the service with it, and the service is the half that holds the firewall
// rules. A crashed core is an inconvenience; a crashed service with rules still
// standing is a machine with no internet.
func Zapustit(imya string, putKonfiga string) (*Yadro, error) {
	put := filepath.Join(sostoyanie.KatalogProgrammy(), imya)
	// Checked here rather than at install time only, because the interesting
	// moment is not "was it right when we put it there" but "is it right now".
	// A core swapped between install and launch is the whole scenario.
	if err := hranenie.Sverit(put); err != nil {
		return nil, fmt.Errorf("ядро %s не запущено: %w", imya, err)
	}
	cmd := exec.Command(put, "run", "-c", putKonfiga)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	// Вывод ядра идёт в журнал службы, а не в никуда. Без этого отказ подъёма
	// недиагностируем: служба говорит «адаптер не появился», а почему ядро не
	// поднялось, не знает никто. Ровно так пряталась находка 43.
	zh := &zhurnalYadra{imya: imya}
	cmd.Stdout = zh
	cmd.Stderr = zh
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("ядро %s не запустилось: %w", imya, err)
	}

	// Назначение в задание сразу после старта. Окно между Start и назначением
	// микроскопическое, но оно ЕСТЬ: правильный способ это CREATE_SUSPENDED
	// плюс ResumeThread, а Go дескриптор потока наружу не отдаёт вовсе. Цена
	// окна ограничена тем, что за него успел бы породить сам процесс, а ни
	// xray, ни sing-box детей не порождают.
	//
	// Отказ назначения это ОТКАЗ ЗАПУСКА, а не «запустимся как раньше».
	// Продолжить значило бы молча вернуться ровно к тому поведению, которое эта
	// задача чинит, и узнал бы об этом человек по осиротевшему туннелю.
	if err := vZadanie(cmd); err != nil {
		return nil, fmt.Errorf("ядро %s не запущено: %w", imya, err)
	}

	y := &Yadro{
		cmd:  cmd,
		imya: imya,
		// Buffered by one. The plan drew this channel unbuffered, which parks the
		// waiter goroutine forever when Ostanovit is called and nobody reads the
		// death: a leak per restart, invisible until the hundredth one.
		smert:  make(chan error, 1),
		podnyt: time.Now(),
	}
	go y.zhdat()
	return y, nil
}

func (y *Yadro) zhdat() {
	// cmd.Wait must be called exactly once, and it is the only honest source of
	// "the process is gone". Polling by PID lies: PIDs are reused, and a fresh
	// process with the old number reads as "still alive".
	y.smert <- y.cmd.Wait()
	close(y.smert)
}

func (y *Yadro) Ostanovit() error {
	if y.cmd == nil || y.cmd.Process == nil {
		return nil
	}
	if err := y.cmd.Process.Kill(); err != nil {
		return fmt.Errorf("ядро %s не убивается: %w", y.imya, err)
	}
	// Drain the death so zhdat finishes and the process handle is released.
	// Skipping this leaves a zombie handle and a goroutine per stop.
	<-y.smert
	return nil
}

// vzyatZadanie это шов. Задание процесса создаётся один раз и навсегда, и
// подменить его в тесте иначе нечем, а проверять надо именно отказ.
var vzyatZadanie = zadanieProcessa

// vZadanie назначает уже запущенный процесс в задание, а при отказе УБИВАЕТ его.
//
// Оставить процесс жить после отказа значит получить ровно ту сироту, ради
// которой всё это писалось, только сразу и без всякого убийства службы.
func vZadanie(cmd *exec.Cmd) error {
	z, err := vzyatZadanie()
	if err == nil {
		err = z.Prinyat(cmd.Process.Pid)
	}
	if err != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		return err
	}
	return nil
}
