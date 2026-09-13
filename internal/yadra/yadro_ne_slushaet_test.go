package yadra

import (
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"
	"testing"
)

// Ошибка приезжает сюда из-под fmt.Errorf, url.Error, net.OpError и
// os.SyscallError разом: проверяется именно такая обёртка, а не голый код.
func TestYadroNeSlushaetUznayotOtvergnutoeSoedinenie(t *testing.T) {
	vnutri := &net.OpError{
		Op:  "dial",
		Net: "tcp",
		Err: os.NewSyscallError("connectex", wsaeConnRefused),
	}
	err := fmt.Errorf("clash_api не отвечает: %w", vnutri)
	if !YadroNeSlushaet(err) {
		t.Error("отвергнутое соединение не опознано как отсутствие ядра")
	}
}

// Нехватка ресурсов машины это НЕ отсутствие ядра: там ядро живо и несёт, и
// перепутать их значит записать режим поверх живого трафика.
func TestYadroNeSlushaetNePutaetSNehvatkoyResursov(t *testing.T) {
	err := fmt.Errorf("clash_api не отвечает: %w", &net.OpError{
		Op: "dial", Err: os.NewSyscallError("socket", wsaeNoBufs)})
	if YadroNeSlushaet(err) {
		t.Error("нехватка портов принята за отсутствие ядра")
	}
}

// Ответ ядра с отказом не несёт кода Winsock вовсе.
func TestYadroNeSlushaetNePutaetSOtkazomYadra(t *testing.T) {
	if YadroNeSlushaet(errors.New("ядро не приняло выбор: 404")) {
		t.Error("отказ живого ядра принят за отсутствие ядра")
	}
	if YadroNeSlushaet(syscall.ECONNRESET) {
		t.Error("оборванное соединение принято за отсутствие ядра")
	}
}
