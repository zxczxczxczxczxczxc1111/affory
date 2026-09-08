package yadra

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPortVydayotsyaSistemoy(t *testing.T) {
	// Two calls in a row must not return the same number, and neither must
	// return the hardcoded 10808 that four other VPN clients on this machine
	// also picked.
	a, err := VydatPort()
	if err != nil {
		t.Fatalf("порт не выдан: %v", err)
	}
	if a == 10808 || a == 10809 {
		t.Fatalf("выдан порт из чужого диапазона: %d", a)
	}
	b, err := VydatPort()
	if err != nil {
		t.Fatalf("второй порт не выдан: %v", err)
	}
	if a == b {
		t.Fatalf("система выдала тот же порт дважды: %d", a)
	}
}

func TestVladelecPortaNazyvaetProcess(t *testing.T) {
	// Слушатель наш собственный, значит владельцем обязан назваться бинарь
	// теста. Если это сломается, любой вердикт о перехвате станет шумом.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("слушатель не поднялся: %v", err)
	}
	defer l.Close()
	port := l.Addr().(*net.TCPAddr).Port

	imya, err := VladelecPorta(port)
	if err != nil {
		t.Fatalf("владелец не определён: %v", err)
	}
	if !strings.EqualFold(filepath.Base(imya), filepath.Base(os.Args[0])) {
		t.Fatalf("владельцем своего сокета назван %q, а ожидался %q", imya, os.Args[0])
	}
}

func TestPustoyPortBezVladeltsa(t *testing.T) {
	// Тишина это тишина, а не перехват. Назвать её занятостью значило бы
	// отправить человека искать злодея, которого нет: ровно эту ошибку тут
	// однажды уже сделали.
	svobodnyy, err := VydatPort()
	if err != nil {
		t.Fatalf("порт не выдан: %v", err)
	}
	imya, err := VladelecPorta(svobodnyy)
	if err != nil {
		t.Fatalf("пустой порт дал ошибку: %v", err)
	}
	if imya != "" {
		t.Fatalf("у пустого порта нашёлся владелец: %q", imya)
	}
}
