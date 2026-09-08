package yadra

import (
	"fmt"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Zadanie это объект задания Windows с KILL_ON_JOB_CLOSE.
//
// Смысл ровно один: ядра не должны переживать смерть службы. Замерено в госте
// 01.09.2026: после Stop-Process над службой xray.exe оставался жив, туннель
// стоял, и трафик машины шёл через выход, которым никто не управляет. Это хуже
// отсутствия VPN, потому что выглядит как работающий VPN.
//
// Механика: дескриптор задания держит НАШ процесс. Умирает процесс, закрываются
// все его дескрипторы, закрывается задание, и ядра убиваются ядром системы. Ни
// одного нашего обработчика при этом не выполняется, поэтому это работает и при
// убийстве службы, и при её падении, и при отладочном закрытии окна.
type Zadanie struct {
	h windows.Handle
}

// NovoeZadanie создаёт задание, из которого нельзя выйти живым.
func NovoeZadanie() (*Zadanie, error) {
	h, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("объект задания не создан: %w", err)
	}
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(h,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		// Задание БЕЗ флага хуже отсутствия задания: оно есть, процессы в него
		// назначаются, и всё выглядит сделанным, а сироты остаются. Дескриптор
		// закрывается, ошибка отдаётся наверх.
		_ = windows.CloseHandle(h)
		return nil, fmt.Errorf("флаг убийства не выставлен: %w", err)
	}
	return &Zadanie{h: h}, nil
}

// Prinyat назначает процесс в задание.
//
// Дескриптор открывается ПО PID, и это безопасно ровно потому, что вызывающий
// держит дескриптор процесса открытым (exec.Cmd не отпускает его до Wait).
// Номер занят, пока жив дескриптор, поэтому переиспользования номера здесь быть
// не может. Без этого условия открытие по номеру назначало бы в задание чужой
// процесс, и убивали бы мы тоже чужой.
func (z *Zadanie) Prinyat(pid int) error {
	if z == nil || z.h == 0 {
		return nil
	}
	p, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return fmt.Errorf("процесс %d не открыт: %w", pid, err)
	}
	defer windows.CloseHandle(p)
	if err := windows.AssignProcessToJobObject(z.h, p); err != nil {
		return fmt.Errorf("процесс %d не назначен в задание: %w", pid, err)
	}
	return nil
}

// zakryt закрывает задание и тем самым убивает всё, что в нём.
func (z *Zadanie) zakryt() error {
	if z == nil || z.h == 0 {
		return nil
	}
	h := z.h
	z.h = 0
	return windows.CloseHandle(h)
}

// Задание процесса ровно одно, потому что процесс ровно один. Глобальность
// здесь описывает факт операционной системы, а не удобство: два задания у одной
// службы означали бы, что часть ядер переживает её смерть, а часть нет, и
// разбираться в этом пришлось бы по осиротевшему туннелю.
var (
	odnoZadanie   sync.Once
	zadanieProc   *Zadanie
	oshibZadaniya error
)

func zadanieProcessa() (*Zadanie, error) {
	odnoZadanie.Do(func() { zadanieProc, oshibZadaniya = NovoeZadanie() })
	return zadanieProc, oshibZadaniya
}

// goloeZadanieDlyaTesta создаёт задание БЕЗ флага убийства.
//
// Нужно ровно одному тесту, который доказывает, что проверка меряет флаг, а не
// сам факт существования задания. Живёт здесь, рядом с настоящим конструктором:
// копия этих четырёх строк в тесте однажды разъедется с оригиналом, и тест
// начнёт проверять несуществующую разницу.
func goloeZadanieDlyaTesta() (*Zadanie, error) {
	h, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("объект задания не создан: %w", err)
	}
	return &Zadanie{h: h}, nil
}
