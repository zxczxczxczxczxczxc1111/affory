package yadra

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sync"
	"testing"
	"time"
)

// Красный тест утечки соединений с clash_api.
//
// Замер владельца 10.09.2026: у службы 46 156 дескрипторов, 3155 установленных
// соединений с локальным ядром, +41 дескриптор за 14 секунд. Опрос статистики
// делает два запроса в секунду, наблюдатель ещё примерно один: 2.9 запроса в
// секунду против 2.93 наблюдаемых дескрипторов. Совпадение до второго знака
// значит «один запрос оставляет ровно одно соединение».
//
// Здесь это проверяется на своём сервере, без ядра и без сети наружу.
func TestZaprosNeOstavlyaetSoedineniyaOtkrytymi(t *testing.T) {
	const zaprosov = 30

	var mu sync.Mutex
	zhivyh := map[net.Conn]bool{}

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	s.Config.ConnState = func(c net.Conn, st http.ConnState) {
		mu.Lock()
		defer mu.Unlock()
		switch st {
		case http.StateNew:
			zhivyh[c] = true
		case http.StateClosed, http.StateHijacked:
			delete(zhivyh, c)
		}
	}
	defer s.Close()

	for i := 0; i < zaprosov; i++ {
		if _, _, err := zaprosS(context.Background(), http.MethodGet, s.URL, "sekret", nil, predelTela); err != nil {
			t.Fatalf("запрос %d не прошёл: %v", i, err)
		}
	}

	mu.Lock()
	otkryto := len(zhivyh)
	mu.Unlock()

	// Один клиент с общим пулом держит одно соединение и переиспользует его.
	// Всё, что больше горстки, это утечка ровно того вида, что съела 46 тысяч
	// дескрипторов на машине владельца.
	if otkryto > 2 {
		t.Fatalf("после %d запросов открыто %d соединений: каждый запрос оставляет своё",
			zaprosov, otkryto)
	}
}

// Второй след той же утечки: каждое брошенное соединение держит две горутины
// (readLoop и writeLoop persistConn). Они и есть причина, по которой транспорт
// не собирается сборщиком мусора: горутина ссылается на него.
func TestZaprosNeOstavlyaetGorutin(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer s.Close()

	// Разогрев, чтобы не считать разовые горутины сервера.
	for i := 0; i < 3; i++ {
		_, _, _ = zaprosS(context.Background(), http.MethodGet, s.URL, "s", nil, predelTela)
	}
	do := runtime.NumGoroutine()
	for i := 0; i < 30; i++ {
		_, _, _ = zaprosS(context.Background(), http.MethodGet, s.URL, "s", nil, predelTela)
	}
	time.Sleep(200 * time.Millisecond)
	posle := runtime.NumGoroutine()
	if posle-do > 10 {
		t.Fatalf("30 запросов оставили %d горутин (было %d, стало %d)", posle-do, do, posle)
	}
}

// Третье место того же класса: замер полосы собирает свой клиент на каждый
// замер и не закрывает его пул. Потоки замера намеренно не дожидаются, поэтому
// ждать здесь надо по УСЛОВИЮ, а не секундомером.
func TestZamerPolosyNeOstavlyaetSoedineniy(t *testing.T) {
	var mu sync.Mutex
	zhivyh := map[net.Conn]bool{}
	blob := make([]byte, 256*1024)

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(blob)
	}))
	s.Config.ConnState = func(c net.Conn, st http.ConnState) {
		mu.Lock()
		defer mu.Unlock()
		switch st {
		case http.StateNew:
			zhivyh[c] = true
		case http.StateClosed, http.StateHijacked:
			delete(zhivyh, c)
		}
	}
	defer s.Close()

	skolko := func() int {
		mu.Lock()
		defer mu.Unlock()
		return len(zhivyh)
	}

	if _, err := ZamerPolosy(context.Background(), VhodPolosy{
		Adres: s.URL, Potokov: 4, Srok: 300 * time.Millisecond,
	}); err != nil {
		t.Fatalf("замер не прошёл: %v", err)
	}

	do := time.Now().Add(3 * time.Second)
	for skolko() > 0 && time.Now().Before(do) {
		time.Sleep(10 * time.Millisecond)
	}
	if n := skolko(); n > 0 {
		t.Fatalf("через три секунды после замера открыто %d соединений: пул замера никто не закрыл", n)
	}
}
