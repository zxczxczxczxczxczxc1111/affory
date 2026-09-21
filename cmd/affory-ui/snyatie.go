package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Ожидание снятия и чтение его причины.
//
// Прежде окно звало ShellExecute и тут же закрывалось: вызов возвращается,
// как только человек ответил на запрос прав, а кода возврата не видит никто.
// Снятие, упавшее на середине, выглядело для человека законченным. 21.09.2026
// на живой машине это стоило целой жалобы: человек нажал «удалить и стереть
// ключи», окно закрылось, а на машине остались и служба, и данные, и правило
// сервиса, которое потом вернулось вместе с переустановкой.
//
// Поэтому окно теперь ЖДЁТ. При успехе его всё равно закроет само снятие
// (оно освобождает каталог программы), при отказе человек получает причину
// теми же словами, что получил бы установщик.

// srokSnyatiya это потолок ожидания. Снятие останавливает службу и ждёт SCM,
// то есть десятки секунд в худшем случае; две минуты покрывают их с запасом и
// не превращают зависшее снятие в зависшее окно.
const srokSnyatiya = 120

const (
	seeMaskNoCloseProcess = 0x00000040
	seeMaskNoAsync        = 0x00000100
	seeMaskFlagNoUI       = 0x00000400
)

// shellExecuteInfo это SHELLEXECUTEINFOW. В x/sys/windows v0.47.0 её нет:
// пакет знает ShellExecute, но не расширенную форму, а нам нужен дескриптор
// запущенного процесса.
type shellExecuteInfo struct {
	cbSize         uint32
	fMask          uint32
	hwnd           windows.Handle
	lpVerb         *uint16
	lpFile         *uint16
	lpParameters   *uint16
	lpDirectory    *uint16
	nShow          int32
	hInstApp       windows.Handle
	lpIDList       uintptr
	lpClass        *uint16
	hkeyClass      windows.Handle
	dwHotKey       uint32
	hIconOrMonitor windows.Handle
	hProcess       windows.Handle
}

var (
	shell32           = windows.NewLazySystemDLL("shell32.dll")
	procShellExecuteE = shell32.NewProc("ShellExecuteExW")
)

// Швы для теста: настоящие ходят в shell32 и в каталог живой программы.
var (
	zapustitSnyatie   = zapustitSPravamiIZhdat
	prochitatPrichinu = prichinaSnyatiya
	// Закрытие окна тоже шов: в тесте приложения Wails нет вовсе, и настоящий
	// Quit падает на пустом указателе прежде, чем тест успеет что-то сказать.
	zakrytOkno = func(m *most) { m.app.Quit() }
)

// zapustitSPravamiIZhdat запускает соседний affory-svc.exe с правами и ждёт
// его конца. Отдаёт код возврата.
func zapustitSPravamiIZhdat(argumenty string) (uint32, error) {
	put, err := putSluzhebnogoBinarya()
	if err != nil {
		return 0, err
	}
	verb, err := windows.UTF16PtrFromString("runas")
	if err != nil {
		return 0, err
	}
	fayl, err := windows.UTF16PtrFromString(put)
	if err != nil {
		return 0, err
	}
	args, err := windows.UTF16PtrFromString(argumenty)
	if err != nil {
		return 0, err
	}

	svedeniya := shellExecuteInfo{
		fMask:        seeMaskNoCloseProcess | seeMaskNoAsync | seeMaskFlagNoUI,
		lpVerb:       verb,
		lpFile:       fayl,
		lpParameters: args,
		nShow:        windows.SW_HIDE,
	}
	svedeniya.cbSize = uint32(unsafe.Sizeof(svedeniya))

	vyshlo, _, oshibka := procShellExecuteE.Call(uintptr(unsafe.Pointer(&svedeniya)))
	if vyshlo == 0 {
		if errors.Is(oshibka, windows.ERROR_CANCELLED) {
			return 0, errors.New("права не выданы: запрос отклонён")
		}
		return 0, fmt.Errorf("снятие не запустилось: %w", oshibka)
	}
	if svedeniya.hProcess == 0 {
		// Дескриптора нет: ждать нечего, и это не отказ. Возвращаем ноль и
		// оставляем поведение прежним, то есть без вести.
		return 0, nil
	}
	defer windows.CloseHandle(svedeniya.hProcess)

	sostoyanie, err := windows.WaitForSingleObject(svedeniya.hProcess, srokSnyatiya*1000)
	if err != nil {
		return 0, fmt.Errorf("ожидание снятия не удалось: %w", err)
	}
	if sostoyanie == uint32(windows.WAIT_TIMEOUT) {
		return 0, fmt.Errorf("снятие не ответило за %d секунд", srokSnyatiya)
	}
	var kod uint32
	if err := windows.GetExitCodeProcess(svedeniya.hProcess, &kod); err != nil {
		return 0, fmt.Errorf("код снятия не прочитан: %w", err)
	}
	return kod, nil
}

func putSluzhebnogoBinarya() (string, error) {
	svoy, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("свой путь не читается: %w", err)
	}
	put := filepath.Join(filepath.Dir(svoy), "affory-svc.exe")
	if _, err := os.Stat(put); err != nil {
		return "", fmt.Errorf("рядом с программой нет affory-svc.exe: %w", err)
	}
	return put, nil
}

// prichinaSnyatiya читает файл, который служба кладёт рядом с собой, падая.
//
// Кодировка UTF-16LE без BOM: так его пишет polozhitPrichinu и так его читает
// установщик. Пустая строка значит, что причины нет, и тогда человеку остаётся
// код возврата, а не выдуманное объяснение.
func prichinaSnyatiya() string {
	svoy, err := os.Executable()
	if err != nil {
		return ""
	}
	bayty, err := os.ReadFile(filepath.Join(filepath.Dir(svoy), imyaFaylaPrichiny))
	if err != nil || len(bayty) < 2 {
		return ""
	}
	shiroko := make([]uint16, 0, len(bayty)/2)
	for i := 0; i+1 < len(bayty); i += 2 {
		shiroko = append(shiroko, uint16(bayty[i])|uint16(bayty[i+1])<<8)
	}
	return strings.TrimSpace(string(utf16.Decode(shiroko)))
}

// imyaFaylaPrichiny повторяет имя из службы. Копия, а не общая константа:
// внутренний пакет ради одной строки завёл бы зависимость окна от подробностей
// установки, а имя файла это договор между службой и тем, кто её запускал.
const imyaFaylaPrichiny = "ustanovka-prichina.txt"
