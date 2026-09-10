package yadra

import (
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"
	"testing"
)

// Разбор журнала 10.09.2026. Полтора суток работы дали 197 записей вида
//
//	статистика не снята: clash_api не отвечает: Get "http://127.0.0.1:51983/...":
//	dial tcp 127.0.0.1:51983: bind: An operation on a socket could not be
//	performed because the system lacked sufficient buffer space or because a
//	queue was full
//
// и 25 разрывов туннеля следом. Это WSAENOBUFS: у машины кончились эфемерные
// порты (их 16384), а утечка соединений съедала 2.79 в секунду. 16384/2.79 это
// 98 минут, и ровно с таким периодом служба рвала туннель.
//
// Утечка починена, но спусковой крючок остался: отказ ЛОКАЛЬНОГО сокета к
// 127.0.0.1 неотличим от «туннель не несёт», и любой другой пожиратель портов
// на машине даст тот же ложный перезапуск ядра.
//
// Опознаётся именно исчерпание РЕСУРСОВ, а не любой отказ соединения:
// connection refused значит, что ядро не слушает, и это настоящая авария.

func TestResursyMashinyOpoznayutsyaVCepochke(t *testing.T) {
	// Ровно та ошибка, что стоит в журнале: dial -> bind -> WSAENOBUFS.
	vnutr := &net.OpError{
		Op:  "dial",
		Net: "tcp",
		Err: os.NewSyscallError("bind", syscall.Errno(10055)),
	}
	err := fmt.Errorf("clash_api не отвечает: %w", vnutr)

	if !ResursyMashiny(err) {
		t.Fatalf("нехватка портов не опознана: %v", err)
	}
}

func TestOtkazYadraNeSchitaetsyaNehvatkoyResursov(t *testing.T) {
	// Ядро не слушает. Это НАСТОЯЩАЯ авария, и путать её с нехваткой портов
	// нельзя: иначе смерть ядра перестанет поднимать туннель обратно.
	vnutr := &net.OpError{
		Op:  "dial",
		Net: "tcp",
		Err: os.NewSyscallError("connectex", syscall.Errno(10061)), // WSAECONNREFUSED
	}
	err := fmt.Errorf("clash_api не отвечает: %w", vnutr)

	if ResursyMashiny(err) {
		t.Fatal("отказ соединения принят за нехватку ресурсов: смерть ядра перестанет чиниться")
	}
}

func TestObychnayaOshibkaNeResursy(t *testing.T) {
	if ResursyMashiny(errors.New("ответ clash_api не разбирается")) {
		t.Fatal("посторонняя ошибка принята за нехватку ресурсов")
	}
}
