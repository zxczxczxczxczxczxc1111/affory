package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/sostoyanie"
)

// Uninstall proper, spec §"Удаление": firewall rules → tunnel → service →
// autostart → data directory (keys, only if asked) → program directory.
// snyat() covers the first four; this file is the last two.

// flagSteretKlyuchi is the only spelling that erases the data directory.
// The interface passes it after the human answered the question; a typo
// means "keep", because losing keys to a misspelling is the worst reading.
const flagSteretKlyuchi = "--steret-klyuchi"

// popytokUdaleniya times four seconds of ping each: half a minute is longer
// than any sane window takes to close, and short enough that a directory the
// human genuinely locked (a shell sitting inside it) stops being retried.
const popytokUdaleniya = 8

func steretKlyuchiIz(args []string) bool {
	for _, a := range args[1:] {
		if a == flagSteretKlyuchi {
			return true
		}
	}
	return false
}

// snyatDannye removes the data directory when asked and leaves it alone
// otherwise. Absent directory is success either way.
func snyatDannye(dir string, steret bool) error {
	if !steret {
		return nil
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("каталог данных не удалён: %w", err)
	}
	return nil
}

// udalitKatalogProgrammy schedules the removal of the program directory from
// OUTSIDE this process: a running binary cannot delete itself, and the
// interface that launched us lives in the same directory. A detached cmd
// waits three seconds for both to exit, then removes the directory.
// katalogDlyaUdaleniya is a seam: the test aims the launch at a scratch dir.
var katalogDlyaUdaleniya = sostoyanie.KatalogProgrammy

func udalitKatalogProgrammy() error {
	dir := katalogDlyaUdaleniya()
	// Каталог освобождается ДО планирования уборки: иначе уборщик отрабатывает
	// свои восемь кругов о живое окно и уходит ни с чем.
	osvoboditKatalog(dir)
	// exec.Command would wrap the whole /c argument in quotes; cmd.exe strips
	// the outer pair and trips over the inner ones ("The filename, directory
	// name, or volume label syntax is incorrect"), so the raw command line is
	// handed over verbatim. Guest run 02.09.2026 caught this: the directory
	// stayed whole. timeout.exe refuses to run without a console, which a
	// detached process has not got, so ping does the waiting.
	// Попыток НЕСКОЛЬКО, а не одна. Каталог держит не только эта служба: рядом
	// лежит affory-ui.exe. Само окно уходит лишь тогда, когда снятие пошло
	// ЧЕРЕЗ НЕГО (most.go зовёт app.Quit), поэтому его теперь закрывает
	// osvoboditKatalog выше, а не надежда на то, что оно догадается. Повтор
	// остаётся ради всех остальных: антивирус, индексатор и просто открытая в
	// каталоге оболочка держат файл секунду-другую и никого не слушают. Цикл
	// прекращается сразу, как каталога не станет, поэтому обычный случай не
	// стал дольше ни на шаг.
	cmd := exec.Command("cmd.exe")
	cmd.Dir = filepath.Dir(dir) // not inside the directory being removed
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | 0x00000008,
		CmdLine: fmt.Sprintf(
			`cmd.exe /c for /l %%i in (1,1,%d) do (ping -n 4 127.0.0.1 >nul & rmdir /s /q "%s" 2>nul & if not exist "%s" exit)`,
			popytokUdaleniya, dir, dir),
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("удаление каталога программы не запланировано: %w", err)
	}
	return nil
}

// osvoboditKatalog просит закрыться всё, что запущено ИЗ каталога программы.
//
// Без этого снятие из командной строки не доводит дело до конца. Окно
// закрывается само, только когда снятие пошло ЧЕРЕЗ НЕГО (most.go зовёт
// app.Quit после ухода команды); про `affory-svc.exe uninstall`, набранный
// руками или запущенный оснасткой, оно не узнаёт никогда, живёт дальше и держит
// каталог своим образом. Уборщик отрабатывает все восемь кругов впустую, а
// uninstall к тому времени уже отчитался кодом 0.
//
// Ищем по ПУТИ ОБРАЗА, а не по имени процесса. Имя `affory-ui.exe` может носить
// что угодно на чужой машине, а убивать по имени значит однажды убить не своё.
//
// Сначала просьба (`taskkill` без /F шлёт окну WM_CLOSE), через три секунды
// принуждение: программу снимают, и окно, пережившее снятие, показывало бы
// кнопки к удалённому бинарю.
func osvoboditKatalog(dir string) {
	nashi := procesyIz(dir)
	if len(nashi) == 0 {
		return
	}
	for _, pid := range nashi {
		// Ошибка здесь не повод останавливаться: процесс мог закончиться сам
		// между снимком и просьбой, и это ровно то, чего мы добивались.
		_ = exec.Command("taskkill.exe", "/PID", strconv.FormatUint(uint64(pid), 10)).Run()
	}
	srok := time.Now().Add(3 * time.Second)
	for time.Now().Before(srok) {
		if len(procesyIz(dir)) == 0 {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	for _, pid := range procesyIz(dir) {
		h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, pid)
		if err != nil {
			continue
		}
		_ = windows.TerminateProcess(h, 1)
		_ = windows.CloseHandle(h)
	}
}

// procesyIz отдаёт pid процессов, чей образ лежит внутри dir. Свой процесс не
// в счёт: служба сама лежит там же и умрёт сразу после uninstall.
func procesyIz(dir string) []uint32 {
	snimok, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snimok)

	svoy := uint32(os.Getpid())
	prefiks := strings.ToLower(filepath.Clean(dir)) + string(filepath.Separator)
	var out []uint32
	// dwSize это договор, а не подсказка: Process32First отвергает нулевой.
	var zapis windows.ProcessEntry32
	zapis.Size = uint32(unsafe.Sizeof(zapis))
	for err = windows.Process32First(snimok, &zapis); err == nil; err = windows.Process32Next(snimok, &zapis) {
		if zapis.ProcessID == svoy || zapis.ProcessID == 0 {
			continue
		}
		put := putObraza(zapis.ProcessID)
		if put == "" {
			continue
		}
		if strings.HasPrefix(strings.ToLower(put), prefiks) {
			out = append(out, zapis.ProcessID)
		}
	}
	return out
}

// putObraza отдаёт полный путь образа процесса или пустую строку.
//
// QUERY_LIMITED_INFORMATION, а не QUERY_INFORMATION: первого хватает на путь и
// он даётся к процессам, к которым второе не даётся вовсе.
func putObraza(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	bufer := make([]uint16, windows.MAX_LONG_PATH)
	dlina := uint32(len(bufer))
	if err := windows.QueryFullProcessImageName(h, 0, &bufer[0], &dlina); err != nil || dlina == 0 {
		return ""
	}
	return windows.UTF16ToString(bufer[:dlina])
}
