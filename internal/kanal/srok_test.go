package kanal

import (
	"net"
	"sync"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// medlennayaSluzhba отвечает на любой кадр через задержку.
//
// net.Pipe вместо настоящего именованного канала: проверяется срок ожидания, а
// не транспорт, и поднимать ради этого канал с проверкой владельца значит
// проверять заодно всё остальное.
func medlennayaSluzhba(t *testing.T, zaderzhka time.Duration) *Klient {
	t.Helper()
	moy, ih := net.Pipe()
	k := &Klient{
		c:        moy,
		zhdut:    make(map[uint64]chan protokol.Kadr),
		sobytiya: make(chan protokol.Kadr, 8),
		gotovo:   make(chan struct{}),
	}
	go k.chitat()
	// Ответ уезжает в ОТДЕЛЬНУЮ горутину. Сон прямо в цикле чтения останавливает
	// приём следующих кадров, а net.Pipe синхронен: вторая команда тогда не
	// уходит вовсе и падает по сроку ЗАПИСИ, то есть тест меряет не то.
	var pishet sync.Mutex
	go func() {
		for {
			kadr, err := ChitatKadr(ih)
			if err != nil {
				return
			}
			go func(k protokol.Kadr) {
				time.Sleep(zaderzhka)
				pishet.Lock()
				defer pishet.Unlock()
				_ = PisatKadr(ih, protokol.Kadr{Tip: "otvet", Id: k.Id, Imya: k.Imya})
			}(kadr)
		}
	}()
	t.Cleanup(func() { _ = moy.Close(); _ = ih.Close() })
	return k
}

func TestDolgayaKomandaPerezhivaetSrokBystroy(t *testing.T) {
	// Замерено на стенде 01.09.2026: при едином сроке в пять секунд connect
	// отвечал «служба не ответила», а туннель в это время был поднят, оба ядра
	// жили и внешний адрес был адресом сервера. Отказ на сработавшей команде
	// отправляет человека чинить исправное.
	//
	// Оба случая гоняются ПАРАЛЛЕЛЬНО по одному каналу: последовательно тест
	// стоил бы двенадцать секунд в воротах вместо семи.
	k := medlennayaSluzhba(t, protokol.SrokBystroy+2*time.Second)

	var (
		gr                sync.WaitGroup
		oshDolgoy, oshBys error
	)
	gr.Add(2)
	go func() { defer gr.Done(); _, oshDolgoy = k.Zvat("connect", nil) }()
	go func() { defer gr.Done(); _, oshBys = k.Zvat("status", nil) }()
	gr.Wait()

	if oshDolgoy != nil {
		t.Fatalf("долгая команда не дождалась ответа: %v", oshDolgoy)
	}
	// Зеркальный случай, и он важнее. Задрать срок всем это интерфейс, висящий
	// минуту на мёртвой службе вместо честного «не отвечает».
	if oshBys == nil {
		t.Fatal("быстрая команда ждала дольше своего срока")
	}
}
