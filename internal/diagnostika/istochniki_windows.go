//go:build windows

package diagnostika

import (
	"encoding/binary"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Настоящие источники. Каждый отвечает фактом или ошибкой; решать, что делать
// с отказом, будет Snyat.

var (
	iphlpapi                = windows.NewLazySystemDLL("iphlpapi.dll")
	procGetExtendedTcpTable = iphlpapi.NewProc("GetExtendedTcpTable")
	kernel32                = windows.NewLazySystemDLL("kernel32.dll")
	procGetProcessHandleCnt = kernel32.NewProc("GetProcessHandleCount")
)

const (
	// Все соединения, а не только слушатели: течь съедает порты именно
	// исходящими, и слушателей среди них нет ни одного.
	tcpTableOwnerPidAll = 5
	razmerStrokiTcp     = 24
)

// DeskriptorovProtsessa отвечает, сколько дескрипторов держит процесс. pid 0
// значит свой.
//
// Утечку 1.0.2 пришлось ловить `handle64.exe` со стороны, потому что изнутри её
// не видел никто. Своё число стоит один системный вызов.
func DeskriptorovProtsessa(pid int) (int, error) {
	var h windows.Handle
	if pid == 0 {
		h = windows.CurrentProcess()
	} else {
		// LIMITED_INFORMATION хватает и не требует прав отладки: ядро запущено
		// службой, и просить больше, чем нужно, ради счётчика незачем.
		var err error
		h, err = windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
		if err != nil {
			return 0, fmt.Errorf("процесс %d не открывается: %w", pid, err)
		}
		defer windows.CloseHandle(h)
	}
	var n uint32
	r, _, err := procGetProcessHandleCnt.Call(uintptr(h), uintptr(unsafe.Pointer(&n)))
	if r == 0 {
		return 0, fmt.Errorf("счётчик дескрипторов %d не читается: %w", pid, err)
	}
	return int(n), nil
}

// diapazonEfemernyh кэшируется: границы задаются один раз на систему, а netsh
// это внешний процесс, и звать его раз в секунду значит мерить прибором,
// который сам дороже измеряемого.
var (
	razOdin      sync.Once
	nachaloPorta int
	vsegoPortov  int
)

func diapazonEfemernyh() (int, int) {
	razOdin.Do(func() {
		// Умолчание Windows Vista и новее. Ставится ДО опроса: если netsh не
		// ответит, лучше считать по умолчанию, чем не считать вовсе.
		nachaloPorta, vsegoPortov = 49152, 16384
		vyvod, err := zapustitNetsh()
		if err != nil {
			return
		}
		n, v, ok := razobratDiapazon(vyvod)
		if ok {
			nachaloPorta, vsegoPortov = n, v
		}
	})
	return nachaloPorta, vsegoPortov
}

// razobratDiapazon вытаскивает начало и число портов из вывода netsh. Вынесен
// отдельно ровно ради теста: netsh отвечает на языке системы, и держаться за
// слова нельзя, только за числа.
func razobratDiapazon(vyvod string) (nachalo, vsego int, ok bool) {
	var chisla []int
	for _, stroka := range strings.Split(vyvod, "\n") {
		polya := strings.Fields(stroka)
		if len(polya) == 0 {
			continue
		}
		hvost := polya[len(polya)-1]
		n, err := strconv.Atoi(strings.TrimSpace(hvost))
		if err != nil {
			continue
		}
		chisla = append(chisla, n)
	}
	// Ровно два числа в выводе: начальный порт и количество. Больше или
	// меньше значит формат не тот, и угадывать нельзя.
	if len(chisla) != 2 || chisla[0] <= 0 || chisla[1] <= 0 {
		return 0, 0, false
	}
	return chisla[0], chisla[1], true
}

// PortyEfemernye считает, сколько портов из динамического диапазона занято.
//
// WSAENOBUFS в журнале это уже последствие. Занятость видна за двадцать минут
// до первого отказа, и именно это число отвечает на вопрос «кто съел порты»
// раньше, чем начнутся разрывы.
func PortyEfemernye() (zanyato, vsego int, err error) {
	nachalo, vsego := diapazonEfemernyh()
	konec := nachalo + vsego - 1
	stroki, err := tablicaTcp()
	if err != nil {
		return 0, vsego, err
	}
	for _, s := range stroki {
		p := int(portIzSetevogo(s.LokPort))
		if p >= nachalo && p <= konec {
			zanyato++
		}
	}
	return zanyato, vsego, nil
}

// Runtime отдаёт горутины и память. Вторая половина той же утечки: транспорт на
// запрос оставлял не только сокет, но и две горутины на каждый.
func Runtime() (int, uint64) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return runtime.NumGoroutine(), m.Sys
}

type strokaTcp struct {
	LokPort uint32
}

func portIzSetevogo(v uint32) uint16 {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], v)
	return binary.BigEndian.Uint16(b[:2])
}

func tablicaTcp() ([]strokaTcp, error) {
	var razmer uint32
	r, _, _ := procGetExtendedTcpTable.Call(0, uintptr(unsafe.Pointer(&razmer)), 0,
		uintptr(windows.AF_INET), tcpTableOwnerPidAll, 0)
	if r != uintptr(windows.ERROR_INSUFFICIENT_BUFFER) && r != 0 {
		return nil, fmt.Errorf("таблица TCP не измеряется: код %d", r)
	}
	if razmer == 0 {
		return nil, nil
	}
	bufer := make([]byte, razmer)
	r, _, _ = procGetExtendedTcpTable.Call(uintptr(unsafe.Pointer(&bufer[0])),
		uintptr(unsafe.Pointer(&razmer)), 0,
		uintptr(windows.AF_INET), tcpTableOwnerPidAll, 0)
	if r != 0 {
		return nil, fmt.Errorf("таблица TCP не читается: код %d", r)
	}
	kol := binary.LittleEndian.Uint32(bufer[:4])
	stroki := make([]strokaTcp, 0, kol)
	for i := uint32(0); i < kol; i++ {
		nach := 4 + int(i)*razmerStrokiTcp
		if nach+razmerStrokiTcp > len(bufer) {
			// Таблица сжалась между двумя вызовами. Обрезать правильно: читать
			// за буфером ради полноты это способ уронить службу.
			break
		}
		stroki = append(stroki, strokaTcp{LokPort: binary.LittleEndian.Uint32(bufer[nach+8:])})
	}
	return stroki, nil
}

// netsh зовётся с погашенным окном: служба живёт в сеансе 0, но консоль всё
// равно мелькнёт, если однажды окажется в сеансе человека.
func zapustitNetsh() (string, error) {
	cmd := exec.Command("netsh", "int", "ipv4", "show", "dynamicport", "tcp")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	vyvod, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("netsh не ответил: %w", err)
	}
	return string(vyvod), nil
}
