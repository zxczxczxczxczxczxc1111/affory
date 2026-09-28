package main

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// podstavnoySlushatel отдаёт на Accept заданные отказы по порядку, а потом
// ждёт закрытия, как настоящий слушатель без клиентов.
type podstavnoySlushatel struct {
	mu        sync.Mutex
	otkazy    []error
	zakryt    chan struct{}
	zakrytOdn sync.Once
}

func novyySlushatel(otkazy ...error) *podstavnoySlushatel {
	return &podstavnoySlushatel{otkazy: otkazy, zakryt: make(chan struct{})}
}

func (l *podstavnoySlushatel) Accept() (net.Conn, error) {
	l.mu.Lock()
	if len(l.otkazy) > 0 {
		e := l.otkazy[0]
		l.otkazy = l.otkazy[1:]
		l.mu.Unlock()
		return nil, e
	}
	l.mu.Unlock()
	<-l.zakryt
	return nil, net.ErrClosed
}

func (l *podstavnoySlushatel) Close() error {
	l.zakrytOdn.Do(func() { close(l.zakryt) })
	return nil
}

func (l *podstavnoySlushatel) Addr() net.Addr { return nil }

// podmenitSlushatelya отдаёт слушателей из списка по очереди и считает
// открытия. Паузы укорочены: иначе десять отказов подряд ждали бы минуту.
func podmenitSlushatelya(t *testing.T, slushateli ...net.Listener) *atomic.Int32 {
	t.Helper()
	var otkrytiy atomic.Int32
	prezhniy, prezhnyayaPauza := slushatKanal, nachaloPauzyKanala
	t.Cleanup(func() { slushatKanal, nachaloPauzyKanala = prezhniy, prezhnyayaPauza })
	nachaloPauzyKanala = time.Millisecond
	slushatKanal = func() (net.Listener, error) {
		n := int(otkrytiy.Add(1))
		if n > len(slushateli) {
			return nil, errors.New("тест: слушателей больше нет")
		}
		return slushateli[n-1], nil
	}
	return &otkrytiy
}

// zapustitPriyom крутит Obsluzhivat в фоне и отдаёт отмену с ожиданием выхода.
func zapustitPriyom(t *testing.T, s *Sluzhba) (vyhod chan error, otmena context.CancelFunc) {
	t.Helper()
	ctx, otmena := context.WithCancel(context.Background())
	vyhod = make(chan error, 1)
	go func() { vyhod <- s.Obsluzhivat(ctx) }()
	return vyhod, otmena
}

func dozhdatsya(t *testing.T, chto string, uslovie func() bool) {
	t.Helper()
	srok := time.Now().Add(10 * time.Second)
	for !uslovie() {
		if time.Now().After(srok) {
			t.Fatalf("не дождались: %s", chto)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// Н6 аудита 1.6.1: любой отказ Accept гасил службу вместе с туннелем, хотя у
// go-winio слушатель после такого отказа жив.
func TestVremennyyOtkazPriyomaNeGasitSluzhbu(t *testing.T) {
	perehvatitZhurnal(t)
	l := novyySlushatel(errors.New("тест: клиент ушёл посреди подключения"))
	otkrytiy := podmenitSlushatelya(t, l)
	vyhod, otmena := zapustitPriyom(t, podstavnaya(t, nil))

	dozhdatsya(t, "отказ выбран", func() bool {
		l.mu.Lock()
		defer l.mu.Unlock()
		return len(l.otkazy) == 0
	})
	select {
	case err := <-vyhod:
		t.Fatalf("приём вышел на временном отказе: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if n := otkrytiy.Load(); n != 1 {
		t.Errorf("слушатель открывался %d раз, а живой менять незачем", n)
	}
	otmena()
	if err := <-vyhod; err != nil {
		t.Errorf("остановка службы вернула ошибку: %v", err)
	}
}

func TestMertvyySlushatelOtkryvaetsyaZanovo(t *testing.T) {
	perehvatitZhurnal(t)
	otkrytiy := podmenitSlushatelya(t, novyySlushatel(net.ErrClosed), novyySlushatel())
	vyhod, otmena := zapustitPriyom(t, podstavnaya(t, nil))

	dozhdatsya(t, "второе открытие", func() bool { return otkrytiy.Load() == 2 })
	select {
	case err := <-vyhod:
		t.Fatalf("приём вышел вместо нового слушателя: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	otmena()
	if err := <-vyhod; err != nil {
		t.Errorf("остановка службы вернула ошибку: %v", err)
	}
}

func TestOtkazyPodryadMenyayutSlushatelya(t *testing.T) {
	perehvatitZhurnal(t)
	otkazy := make([]error, maksOtkazovPriyoma)
	for i := range otkazy {
		otkazy[i] = errors.New("тест: отказ приёма")
	}
	otkrytiy := podmenitSlushatelya(t, novyySlushatel(otkazy...), novyySlushatel())
	vyhod, otmena := zapustitPriyom(t, podstavnaya(t, nil))

	dozhdatsya(t, "смена слушателя", func() bool { return otkrytiy.Load() == 2 })
	otmena()
	if err := <-vyhod; err != nil {
		t.Errorf("остановка службы вернула ошибку: %v", err)
	}
}

// Отказ открыть канал при СТАРТЕ по-прежнему роняет службу: имя занято значит,
// что кто-то выдаёт себя за нас.
func TestOtkazOtkrytKanalPriStarteVozvrashchaetsya(t *testing.T) {
	podmenitSlushatelya(t) // ни одного слушателя: первое же открытие отказывает
	if err := podstavnaya(t, nil).Obsluzhivat(context.Background()); err == nil {
		t.Fatal("служба стартовала без канала")
	}
}
