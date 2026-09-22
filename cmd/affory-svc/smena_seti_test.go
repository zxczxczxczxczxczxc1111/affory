package main

import (
	"context"
	"errors"
	"net/netip"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Смена сети под поднятым туннелем (A4, 22.09.2026).
//
// Воспроизведено в госте: местный резолвер записан в конфиг при подъёме, ядро
// конфиг на ходу не перечитывает, и в чужой сети весь российский набор
// перестаёт разрешаться - по 12 секунд на имя. Туннель при этом жив, HTTP-проба
// зелёная, и заметить поломку может только тот, кто спрашивает про сеть прямо.

func sluzhbaSRezolverom(t *testing.T, adres string) *Sluzhba {
	t.Helper()
	s := sluzhbaPodnyataya(t)
	s.mu.Lock()
	s.rezolverKonfiga = netip.MustParseAddr(adres)
	s.posledniyPerezapuskSeti = time.Time{}
	s.mu.Unlock()
	return s
}

func TestSmenaRezolveraVidnaNablyudatelyu(t *testing.T) {
	s := sluzhbaSRezolverom(t, "192.168.0.1")
	s.mestnyyRezolver = func() (netip.Addr, error) { return netip.MustParseAddr("10.0.0.1"), nil }

	stalo, smenilsya := s.rezolverSmenilsya()
	if !smenilsya {
		t.Fatal("смена сети не замечена: конфиг останется с резолвером прежней сети")
	}
	if stalo.String() != "10.0.0.1" {
		t.Errorf("новый резолвер %s", stalo)
	}
}

func TestTotZheRezolverNeSchitaetsyaSmenoySeti(t *testing.T) {
	// Иначе продукт рвал бы соединения на каждом тике наблюдателя.
	s := sluzhbaSRezolverom(t, "192.168.0.1")
	s.mestnyyRezolver = func() (netip.Addr, error) { return netip.MustParseAddr("192.168.0.1"), nil }
	if _, smenilsya := s.rezolverSmenilsya(); smenilsya {
		t.Fatal("тот же адрес принят за смену сети")
	}
}

func TestPropavshayaSetNeSchitaetsyaSmenoySeti(t *testing.T) {
	// Сеть выдернули: резолвера нет НИКАКОГО. Переподъём в этот момент это
	// попытка подняться в пустоту; туннель разбирает такое пробой живости и
	// восстановлением, а не пересборкой конфига.
	s := sluzhbaSRezolverom(t, "192.168.0.1")
	s.mestnyyRezolver = func() (netip.Addr, error) { return netip.Addr{}, errors.New("адаптеров нет") }
	if _, smenilsya := s.rezolverSmenilsya(); smenilsya {
		t.Fatal("пропажа сети принята за смену сети")
	}
}

func TestBezKonfigaSmenaSetiNeRassmatrivaetsya(t *testing.T) {
	// Туннель опущен: ядра нет, сверять не с чем, пересобирать нечего.
	s := podstavnaya(t, nil)
	s.mestnyyRezolver = func() (netip.Addr, error) { return netip.MustParseAddr("10.0.0.1"), nil }
	if _, smenilsya := s.rezolverSmenilsya(); smenilsya {
		t.Fatal("смена сети замечена при опущенном туннеле")
	}
}

func TestSborkaKonfigaZapominaetRezolver(t *testing.T) {
	// Через sobratTun напрямую, а не через подъём: в фикстуре подъём заглушен
	// целиком (s.podnyatTunnel), и боевая сборка в нём не случается вовсе.
	// Запоминается ФАКТ - то, что уехало в конфиг, - иначе сверять потом не с чем.
	s := podstavnayaSPolnymNaborom(t)
	s.mestnyyRezolver = func() (netip.Addr, error) { return netip.MustParseAddr("192.168.7.1"), nil }

	if _, _, _, err := s.sobratTun(nil, false); err != nil {
		t.Fatalf("конфиг не собрался: %v", err)
	}
	s.mu.Lock()
	zapomnen := s.rezolverKonfiga
	s.mu.Unlock()
	if zapomnen.String() != "192.168.7.1" {
		t.Fatalf("после сборки резолвер конфига %s: смену сети заметить нечем", zapomnen)
	}
}

func TestSuhayaSborkaNeTrogaetRezolverKonfiga(t *testing.T) {
	// Сухая сборка проверяет кандидата на живом туннеле. Запиши она свой
	// резолвер - и смена сети, случившаяся в этот момент, была бы забыта молча.
	s := podstavnayaSPolnymNaborom(t)
	s.mestnyyRezolver = func() (netip.Addr, error) { return netip.MustParseAddr("192.168.7.1"), nil }
	if _, _, _, err := s.sobratTun(nil, false); err != nil {
		t.Fatalf("конфиг не собрался: %v", err)
	}
	s.mestnyyRezolver = func() (netip.Addr, error) { return netip.MustParseAddr("10.9.9.9"), nil }
	if _, _, _, err := s.sobratTun(nil, true); err != nil {
		t.Fatalf("сухая сборка не прошла: %v", err)
	}
	s.mu.Lock()
	zapomnen := s.rezolverKonfiga
	s.mu.Unlock()
	if zapomnen.String() != "192.168.7.1" {
		t.Fatalf("сухая сборка переписала резолвер конфига на %s", zapomnen)
	}
}

func TestOpuskanieZabyvaetRezolverKonfiga(t *testing.T) {
	// Иначе наблюдатель следующего подъёма примет за смену сети то, что
	// сменилось, пока туннель лежал.
	s := sluzhbaPodnyataya(t)
	s.Disconnect()
	s.mu.Lock()
	ostalsya := s.rezolverKonfiga
	s.mu.Unlock()
	if ostalsya.IsValid() {
		t.Fatalf("после опускания резолвер конфига остался %s", ostalsya)
	}
}

func TestSmenaSetiPerepodnimaetTunnel(t *testing.T) {
	s := sluzhbaSRezolverom(t, "192.168.0.1")
	s.mestnyyRezolver = func() (netip.Addr, error) { return netip.MustParseAddr("10.0.0.1"), nil }
	s.proveritKonfig = func(string) error { return nil }

	s.mu.Lock()
	pokolenieBylo := s.pokolenieP
	s.mu.Unlock()

	if !s.smotretSet(context.Background()) {
		t.Fatal("наблюдатель не увидел смены сети")
	}
	// Переподъём идёт своей горутиной на фоновом контексте: ждём, пока туннель
	// поднимется ЗАНОВО. Судим по поколению подъёма, а не по состоянию: оно
	// было podnyat и до, и после, и на нём тест зелен всегда.
	srok := time.Now().Add(10 * time.Second)
	for time.Now().Before(srok) {
		s.mu.Lock()
		stalo, sost := s.pokolenieP, s.sost
		s.mu.Unlock()
		if stalo > pokolenieBylo && sost == protokol.SostPodnyat {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	s.mu.Lock()
	stalo := s.pokolenieP
	s.mu.Unlock()
	t.Fatalf("туннель не переподнялся: поколение было %d, стало %d, состояние %s",
		pokolenieBylo, stalo, s.Status().Sostoyanie)
}

func TestVtoroyPerezapuskSetiZhdyotPauzy(t *testing.T) {
	// Сеть дребезжит: переключение Wi-Fi на провод и пробуждение дают несколько
	// изменений подряд. Без паузы продукт рвал бы соединения на каждое.
	s := sluzhbaSRezolverom(t, "192.168.0.1")
	s.mestnyyRezolver = func() (netip.Addr, error) { return netip.MustParseAddr("10.0.0.1"), nil }
	s.mu.Lock()
	s.posledniyPerezapuskSeti = s.seychas()
	s.mu.Unlock()

	if s.smotretSet(context.Background()) {
		t.Fatal("второй переподъём пошёл сразу: дребезг сети рвёт соединения")
	}
	// А после паузы - идёт.
	s.mu.Lock()
	s.posledniyPerezapuskSeti = s.seychas().Add(-2 * s.perezapuskSetiNeChashche)
	s.mu.Unlock()
	s.proveritKonfig = func(string) error { return nil }
	if !s.smotretSet(context.Background()) {
		t.Fatal("после паузы смена сети снова не замечена")
	}
}

func TestNablyudatelSamSprashivaetProSet(t *testing.T) {
	// Без этого теста вся проверка смены сети мертва: логика работает, а из
	// наблюдателя её никто не зовёт, и на живой машине не происходит ничего.
	s := sluzhbaSRezolverom(t, "192.168.0.1")
	s.proveritKonfig = func(string) error { return nil }
	s.mu.Lock()
	pokolenieBylo := s.pokolenieP
	s.mu.Unlock()
	// Тик наблюдателя укорачивается, чтобы не ждать штатных секунд.
	s.periodNesushchego = time.Millisecond
	s.period = time.Millisecond
	// Сеть «переезжает» уже под работающим наблюдателем.
	s.mestnyyRezolver = func() (netip.Addr, error) { return netip.MustParseAddr("10.0.0.1"), nil }

	srok := time.Now().Add(10 * time.Second)
	for time.Now().Before(srok) {
		s.mu.Lock()
		stalo, sost := s.pokolenieP, s.sost
		s.mu.Unlock()
		if stalo > pokolenieBylo && sost == protokol.SostPodnyat {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("наблюдатель не спросил про сеть: смена сети замечена не будет никогда")
}
