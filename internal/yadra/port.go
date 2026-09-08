package yadra

import (
	"encoding/binary"
	"fmt"
	"net"
	"unsafe"

	"golang.org/x/sys/windows"
)

// VydatPort asks the OS for a free port instead of nailing one down. This
// machine has hosted nekoray, Throne, Amnezia and Sota, all in the same range.
// Прежде на этом порту жил SOCKS второго ядра, и занятый порт означал бы тихий
// разговор с ЧУЖИМ прокси, то есть зелёный туннель через чужой сервер. Ядро
// одно с 01.09.2026, и порт теперь под clash_api, но диапазон тот же.
//
// The window between release and the core's own bind is a real race and it is
// accepted knowingly. Закрывает его не надежда, а VladelecPorta ниже: чужой
// слушатель нашего секрета не знает, и его 401 называется перехватом, а не
// нашей ошибкой.
func VydatPort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("свободный порт не выделяется: %w", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// GetExtendedTcpTable is NOT in golang.org/x/sys/windows (checked by grep on
// v0.47.0; MIB_TCPROW_OWNER_PID is not there either). Declared by hand, same as
// ImpersonateNamedPipeClient in task 1.3. Third time this project trusted a
// plausible-looking name from somebody else's library and found nothing.
var (
	iphlpapi                = windows.NewLazySystemDLL("iphlpapi.dll")
	procGetExtendedTcpTable = iphlpapi.NewProc("GetExtendedTcpTable")
)

const (
	tcpTableOwnerPidListener = 3
	razmerStroki             = 24 // dwState, local addr/port, remote addr/port, pid
)

type strokaTcp struct {
	Sostoyanie  uint32
	LokAdres    uint32
	LokPort     uint32
	UdalAdres   uint32
	UdalPort    uint32
	VladeletPid uint32
}

// VladelecPorta называет ПОЛНЫЙ путь процесса, который слушает порт, или
// пустую строку, если не слушает никто.
//
// Прежде здесь было две функции сверки, по pid и по пути, и обе решали за
// вызывающего, перехват это или нет. После ухода на одно ядро их не вызывал
// НИКТО, при том что комментарий в тесте утверждал обратное: механизм защиты
// был объявлен живым и выключен. Теперь функция одна, отвечает фактом, а
// вердикт выносит тот, кто знает, чего ждал.
func VladelecPorta(port int) (string, error) {
	stroki, err := slushateli()
	if err != nil {
		return "", err
	}
	for _, s := range stroki {
		if int(portIzSetevogo(s.LokPort)) == port {
			return imyaProcessa(s.VladeletPid), nil
		}
	}
	return "", nil
}

// The port sits in the low two bytes in network order. Reading it as a plain
// uint32 gives numbers like 34825 for port 10808 and looks like a real result.
func portIzSetevogo(v uint32) uint16 {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], v)
	return binary.BigEndian.Uint16(b[:2])
}

func slushateli() ([]strokaTcp, error) {
	var razmer uint32
	// First call with a nil buffer only to learn the size. It returns
	// ERROR_INSUFFICIENT_BUFFER and that is the success path here.
	r, _, _ := procGetExtendedTcpTable.Call(0, uintptr(unsafe.Pointer(&razmer)), 0,
		uintptr(windows.AF_INET), tcpTableOwnerPidListener, 0)
	if r != uintptr(windows.ERROR_INSUFFICIENT_BUFFER) && r != 0 {
		return nil, fmt.Errorf("таблица TCP не измеряется: код %d", r)
	}
	if razmer == 0 {
		return nil, nil
	}
	bufer := make([]byte, razmer)
	r, _, _ = procGetExtendedTcpTable.Call(uintptr(unsafe.Pointer(&bufer[0])),
		uintptr(unsafe.Pointer(&razmer)), 0,
		uintptr(windows.AF_INET), tcpTableOwnerPidListener, 0)
	if r != 0 {
		return nil, fmt.Errorf("таблица TCP не читается: код %d", r)
	}
	kol := binary.LittleEndian.Uint32(bufer[:4])
	stroki := make([]strokaTcp, 0, kol)
	for i := uint32(0); i < kol; i++ {
		nach := 4 + int(i)*razmerStroki
		if nach+razmerStroki > len(bufer) {
			// The table shrank between the two calls. Truncating is correct:
			// reading past the buffer to be thorough is how this turns into a
			// crash in the half that holds the firewall rules.
			break
		}
		var s strokaTcp
		s.Sostoyanie = binary.LittleEndian.Uint32(bufer[nach:])
		s.LokAdres = binary.LittleEndian.Uint32(bufer[nach+4:])
		s.LokPort = binary.LittleEndian.Uint32(bufer[nach+8:])
		s.UdalAdres = binary.LittleEndian.Uint32(bufer[nach+12:])
		s.UdalPort = binary.LittleEndian.Uint32(bufer[nach+16:])
		s.VladeletPid = binary.LittleEndian.Uint32(bufer[nach+20:])
		stroki = append(stroki, s)
	}
	return stroki, nil
}

// Best effort, and deliberately so: the name is for the human reading the error.
// Failing to get it must not turn a clear diagnosis into an unrelated errno.
func imyaProcessa(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "имя недоступно"
	}
	defer windows.CloseHandle(h)
	bufer := make([]uint16, windows.MAX_PATH)
	dlina := uint32(len(bufer))
	if err := windows.QueryFullProcessImageName(h, 0, &bufer[0], &dlina); err != nil {
		return "имя недоступно"
	}
	return windows.UTF16ToString(bufer[:dlina])
}
