package set

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// Подставной прокси: отвечает сам на любой запрос, потому что проверяем мы не
// проксирование, а то, КАКОЙ из двух кругов попал в число.
func podstavnoyProksi(t *testing.T, otvet func(n int, w http.ResponseWriter)) (port int, schyot func() int, adresa func() []string) {
	t.Helper()
	var mu sync.Mutex
	var n int
	var otkuda []string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		n++
		nomer := n
		otkuda = append(otkuda, r.RemoteAddr)
		mu.Unlock()
		otvet(nomer, w)
	}))
	t.Cleanup(s.Close)
	adres := strings.TrimPrefix(s.URL, "http://")
	dvoetochie := strings.LastIndex(adres, ":")
	p := 0
	for _, c := range adres[dvoetochie+1:] {
		p = p*10 + int(c-'0')
	}
	return p, func() int {
			mu.Lock()
			defer mu.Unlock()
			return n
		}, func() []string {
			mu.Lock()
			defer mu.Unlock()
			return append([]string(nil), otkuda...)
		}
}

// Главное свойство: в число попадает ВТОРОЙ круг.
//
// Первый круг здесь нарочно долгий - так выглядит рукопожатие протокола и TLS,
// из-за которого экран показывал 169 мс там, где Discord показывал 65. Если
// замер начнёт считать первый круг или оба, тест это увидит.
func TestOtklikMeryaetVtoroyKrug(t *testing.T) {
	port, schyot, _ := podstavnoyProksi(t, func(n int, w http.ResponseWriter) {
		if n == 1 {
			time.Sleep(300 * time.Millisecond)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	d, err := Otklik(context.Background(), "http://cel.invalid/generate_204", port)
	if err != nil {
		t.Fatalf("замер не прошёл: %v", err)
	}
	if d > 150*time.Millisecond {
		t.Fatalf("в число попал прогревочный круг: %v", d)
	}
	if n := schyot(); n != 2 {
		t.Fatalf("кругов сделано %d, а нужно ровно два", n)
	}
}

// Второй круг обязан идти по соединению первого. Иначе он померит рукопожатие
// заново, и весь приём теряет смысл молча: число останется правдоподобным.
func TestOtklikIdyotPoTomuZheSoedineniyu(t *testing.T) {
	port, _, adresa := podstavnoyProksi(t, func(_ int, w http.ResponseWriter) {
		w.WriteHeader(http.StatusNoContent)
	})

	if _, err := Otklik(context.Background(), "http://cel.invalid/generate_204", port); err != nil {
		t.Fatalf("замер не прошёл: %v", err)
	}
	a := adresa()
	if len(a) != 2 {
		t.Fatalf("кругов %d, а нужно два: %v", len(a), a)
	}
	if a[0] != a[1] {
		t.Fatalf("второй круг пошёл по новому соединению: %s, затем %s", a[0], a[1])
	}
}

// Отказ цели это НЕ отклик. Чужой клиент на нашем порту отвечает мгновенно, и
// его отказ встал бы на экран как отличная задержка.
func TestOtklikNeSchitaetOtkazUdachey(t *testing.T) {
	port, _, _ := podstavnoyProksi(t, func(_ int, w http.ResponseWriter) {
		w.WriteHeader(http.StatusProxyAuthRequired)
	})

	if _, err := Otklik(context.Background(), "http://cel.invalid/generate_204", port); err == nil {
		t.Fatal("отказ 407 принят за успешный замер")
	}
}

// Без локального прокси мерить нечем, и это отказ с причиной, а не ноль.
func TestOtklikBezProksiOtkazyvaet(t *testing.T) {
	if _, err := Otklik(context.Background(), "", 0); err == nil {
		t.Fatal("замер без прокси отдал успех")
	}
}
