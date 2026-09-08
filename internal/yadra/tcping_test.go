package yadra_test

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/yadra"
)

// TestTcpingMeryaetZhivoyUzel: соединение до живого слушателя это число, а не
// отказ. Число малое, потому что слушатель на петле, но проверяется не оно, а
// сам факт: замер состоялся и вернул положительную длительность.
func TestTcpingMeryaetZhivoyUzel(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()

	host, port, err := net.SplitHostPort(l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	d, err := yadra.Tcping(context.Background(), host, atoi(t, port), time.Second)
	if err != nil {
		t.Fatalf("живой узел объявлен мёртвым: %v", err)
	}
	if d <= 0 {
		t.Fatalf("замер вернул неположительное время: %v", d)
	}
}

// TestTcpingMolchashchiyUzelEtoOtkaz: закрытый порт обязан быть отказом, а не
// нулём. Ноль, выданный за измерение, поставил бы мёртвый сервер ПЕРВЫМ в
// списке по задержке.
func TestTcpingMolchashchiyUzelEtoOtkaz(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	host, port, err := net.SplitHostPort(l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	l.Close() // порт освобождён, значит соединение получит отказ

	if _, err := yadra.Tcping(context.Background(), host, atoi(t, port), time.Second); err == nil {
		t.Fatal("закрытый порт принят за исправный узел")
	} else if !errors.Is(err, yadra.ErrUzelMolchit) {
		t.Fatalf("причина не названа своим именем: %v", err)
	}
}

// TestTcpingUvazhaetOtmenu: отменённый замер не держит очередь. Кнопка
// «проверить» на сорока серверах без этого висела бы до последнего срока.
func TestTcpingUvazhaetOtmenu(t *testing.T) {
	ctx, otmena := context.WithCancel(context.Background())
	otmena()
	// 203.0.113.0/24 это TEST-NET-3, туда не отвечает никто и никогда.
	nachalo := time.Now()
	if _, err := yadra.Tcping(ctx, "203.0.113.9", 443, 30*time.Second); err == nil {
		t.Fatal("отменённый замер вернул успех")
	}
	if proshlo := time.Since(nachalo); proshlo > 3*time.Second {
		t.Fatalf("отмена не замечена, ждали %v", proshlo)
	}
}

func atoi(t *testing.T, s string) int {
	t.Helper()
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			t.Fatalf("порт %q не число", s)
		}
		n = n*10 + int(r-'0')
	}
	return n
}
