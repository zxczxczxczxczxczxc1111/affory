package set

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// Тот же класс, что и в internal/yadra: транспорт, собранный на вызов, уносит
// соединение в свой пул простоя и хоронит его там навсегда. Здесь вызовов
// меньше, чем у опроса статистики, поэтому в замер 10.09.2026 они не
// попали заметной долей. Класс от этого не меняется: чинить надо КЛАСС, а не
// самый громкий его случай.

// schyotSoedineniy считает живые соединения на стороне сервера.
type schyotSoedineniy struct {
	mu     sync.Mutex
	zhivye map[net.Conn]bool
}

func novyySchyot() *schyotSoedineniy {
	return &schyotSoedineniy{zhivye: map[net.Conn]bool{}}
}

func (s *schyotSoedineniy) sostoyanie(c net.Conn, st http.ConnState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch st {
	case http.StateNew:
		s.zhivye[c] = true
	case http.StateClosed, http.StateHijacked:
		delete(s.zhivye, c)
	}
}

func (s *schyotSoedineniy) skolko() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.zhivye)
}

// zhdatSpada ждёт, пока живых соединений не станет не больше predel. Ожидание
// по УСЛОВИЮ, а не по секундомеру: закрытие идёт из чужой горутины, и
// фиксированная пауза либо врёт, либо тормозит набор.
func (s *schyotSoedineniy) zhdatSpada(predel int, srok time.Duration) int {
	do := time.Now().Add(srok)
	for {
		n := s.skolko()
		if n <= predel || time.Now().After(do) {
			return n
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestZagruzitSnimokNeOstavlyaetSoedineniy(t *testing.T) {
	const zaprosov = 20
	sch := novyySchyot()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"vremya":1}`))
	}))
	s.Config.ConnState = sch.sostoyanie
	defer s.Close()

	for i := 0; i < zaprosov; i++ {
		if _, err := ZagruzitSnimok(context.Background(), s.URL); err != nil {
			t.Fatalf("снимок %d не загрузился: %v", i, err)
		}
	}
	if n := sch.zhdatSpada(2, 2*time.Second); n > 2 {
		t.Fatalf("после %d загрузок снимка открыто %d соединений", zaprosov, n)
	}
}

func TestAdresVyhodaNeOstavlyaetSoedineniy(t *testing.T) {
	const zaprosov = 20
	sch := novyySchyot()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("203.0.113.7"))
	}))
	s.Config.ConnState = sch.sostoyanie
	defer s.Close()

	for i := 0; i < zaprosov; i++ {
		// Ноль портом значит «напрямую»: прокси в этом наборе поднимать нечем,
		// а утечка от наличия прокси не зависит, транспорт один и тот же.
		if _, err := AdresVyhoda(context.Background(), s.URL, 0); err != nil {
			t.Fatalf("адрес выхода %d не получен: %v", i, err)
		}
	}
	if n := sch.zhdatSpada(2, 2*time.Second); n > 2 {
		t.Fatalf("после %d проверок выхода открыто %d соединений", zaprosov, n)
	}
}
