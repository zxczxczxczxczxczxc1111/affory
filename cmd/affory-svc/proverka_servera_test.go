package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/set"
)

// Б аудита 1.8.0 (H3, M10). Пока сервер мёртв, автоматические попытки
// поднимали TUN и на 15-30 секунд уводили весь трафик в туннель, который не
// несёт. Теперь сервер сначала проверяет ядро без TUN.

func sluzhbaSProverkoy(t *testing.T, otvet error) (*Sluzhba, *atomic.Int32, *atomic.Int32) {
	t.Helper()
	s := podstavnaya(t, nil)
	var proverok, podnimali atomic.Int32
	s.proveritServer = func(context.Context) error { proverok.Add(1); return otvet }
	prezhniy := s.podnyatTunnel
	s.podnyatTunnel = func(ctx context.Context) (set.Adapter, error) {
		podnimali.Add(1)
		return prezhniy(ctx)
	}
	return s, &proverok, &podnimali
}

func TestAvtoPodyomNePodnimaetTunPriMolchashchemServere(t *testing.T) {
	s, proverok, podnimali := sluzhbaSProverkoy(t, errors.New("тест: сервер молчит"))
	if err := s.connect(context.Background(), nil, true); err == nil {
		t.Fatal("автоматический подъём при молчащем сервере ответил успехом")
	}
	if proverok.Load() != 1 {
		t.Fatalf("проверок сервера %d, ожидалась одна", proverok.Load())
	}
	if n := podnimali.Load(); n != 0 {
		t.Fatalf("TUN поднимался %d раз при молчащем сервере: весь трафик ушёл в мёртвый туннель", n)
	}
	st := s.Status()
	if st.Sostoyanie != protokol.SostOtkaz || st.Oshib == nil || st.Oshib.Kod != protokol.KodAllServersDown {
		t.Fatalf("состояние %s, ошибка %+v: восстановление ждёт otkaz с причиной", st.Sostoyanie, st.Oshib)
	}
}

func TestRuchnoyPodyomBezProverkiServera(t *testing.T) {
	// Человек нажал «Подключить» и ждёт попытки, а не догадки о ней.
	s, proverok, _ := sluzhbaSProverkoy(t, errors.New("тест: сервер молчит"))
	if err := s.Connect(context.Background()); err != nil {
		t.Fatalf("ручной подъём: %v", err)
	}
	if proverok.Load() != 0 {
		t.Fatal("ручной подъём проверял сервер до TUN")
	}
}

func TestZapertayaMashinaBezProverkiServera(t *testing.T) {
	// Под замком сети мимо туннеля нет и так: отнимать нечего, проверять незачем.
	s, proverok, podnimali := sluzhbaSProverkoy(t, errors.New("тест: сервер молчит"))
	s.PomnitZapertuyu(true)
	if err := s.connect(context.Background(), nil, true); err != nil {
		t.Fatalf("подъём запертой машины: %v", err)
	}
	if proverok.Load() != 0 || podnimali.Load() != 1 {
		t.Fatalf("проверок %d, подъёмов %d на запертой машине", proverok.Load(), podnimali.Load())
	}
}

func TestNesostoyavshayasyaProverkaNeMeshaetPodyomu(t *testing.T) {
	// Ядро не стартовало для проверки: это не приговор серверу. Подъём идёт
	// обычным путём и сам назовёт настоящую причину, если она есть.
	s, _, podnimali := sluzhbaSProverkoy(t, fmt.Errorf("%w: тест: ядро не запустилось", errProverkaNeSostoyalas))
	if err := s.connect(context.Background(), nil, true); err != nil {
		t.Fatalf("подъём после несостоявшейся проверки: %v", err)
	}
	if podnimali.Load() != 1 {
		t.Fatalf("подъёмов %d", podnimali.Load())
	}
}

func TestVosstanovlenieNaMyortvomServereNeTrogaetSet(t *testing.T) {
	s, proverok, podnimali := sluzhbaSProverkoy(t, errors.New("тест: сервер лёг"))
	s.otstupy = []time.Duration{time.Millisecond}
	s.postavit(protokol.SostNeNeset, nil)
	s.zapustitVosstanovlenie()
	dozhdatsya(t, "несколько попыток восстановления", func() bool { return proverok.Load() >= 3 })
	if n := podnimali.Load(); n != 0 {
		t.Fatalf("восстановление поднимало TUN %d раз на мёртвом сервере", n)
	}
}

func TestKonfigProverkiServeraBezTunIBezSledov(t *testing.T) {
	s := podstavnaya(t, nil)
	s.mu.Lock()
	s.portProksiNash = 4242
	s.mu.Unlock()
	telo, _, _, _, _, err := s.sobratTunPolno(nil, false, false, true, false)
	if err != nil {
		t.Fatal(err)
	}
	var k struct {
		Inbounds     []any          `json:"inbounds"`
		Experimental map[string]any `json:"experimental"`
	}
	if err := json.Unmarshal(telo, &k); err != nil {
		t.Fatal(err)
	}
	if len(k.Inbounds) != 0 {
		t.Fatalf("в конфиге проверки входы %v", k.Inbounds)
	}
	if k.Experimental["cache_file"] != nil {
		t.Fatal("конфиг проверки пишет кэш боевого ядра")
	}
	s.mu.Lock()
	port, rezolver := s.portProksiNash, s.rezolverKonfiga
	s.mu.Unlock()
	if port != 4242 || rezolver.IsValid() {
		t.Fatalf("сборка проверки запомнила порт %d и резолвер %s: это состояние живого туннеля", port, rezolver)
	}
}

func TestZameritLyuboyNuzhenOdinZhivoy(t *testing.T) {
	s := podstavnaya(t, nil)
	s.zamerit = func(_ context.Context, _, _, teg string) (time.Duration, error) {
		if teg == "srv-c" {
			return time.Millisecond, nil
		}
		return 0, errors.New("тест: " + teg + " молчит")
	}
	if err := s.zameritLyuboy(context.Background(), "127.0.0.1:1", "s", []string{"srv-a", "srv-b", "srv-c"}); err != nil {
		t.Fatalf("один живой в группе, а проверка отказала: %v", err)
	}
	if err := s.zameritLyuboy(context.Background(), "127.0.0.1:1", "s", []string{"srv-a", "srv-b"}); err == nil {
		t.Fatal("все молчат, а проверка прошла")
	}
	if err := s.zameritLyuboy(context.Background(), "127.0.0.1:1", "s", nil); !errors.Is(err, errProverkaNeSostoyalas) {
		t.Fatalf("мерить нечего, а ответ %v", err)
	}
}

// M10. После десятка неудач при живой сети пауза вырастает до пяти минут, и
// прежде её не сокращало ничто: сервер вернулся, а человек под замком ждёт.
func TestProbuzhdeniePreryvaetPauzuVosstanovleniya(t *testing.T) {
	s, podnimali := sluzhbaSMyortvymServerom(t)
	s.zapustitVosstanovlenie()
	dozhdatsya(t, "длинная пауза", func() bool { return podnimali.Load() >= neudachDoRedkih })
	time.Sleep(50 * time.Millisecond)
	s.ProbaPosleSna()
	dozhdatsya(t, "попытка после пробуждения", func() bool { return podnimali.Load() > neudachDoRedkih })
}

func TestSmenaSetiPreryvaetPauzuVosstanovleniya(t *testing.T) {
	s, podnimali := sluzhbaSMyortvymServerom(t)
	var rezolver atomic.Value
	rezolver.Store(netip.MustParseAddr("192.168.0.1"))
	s.mestnyyRezolver = func(...uint32) (netip.Addr, error) { return rezolver.Load().(netip.Addr), nil }
	s.periodSetiVPauze = 5 * time.Millisecond
	s.zapustitVosstanovlenie()
	dozhdatsya(t, "длинная пауза", func() bool { return podnimali.Load() >= neudachDoRedkih })
	time.Sleep(50 * time.Millisecond)
	if n := podnimali.Load(); n != neudachDoRedkih {
		t.Fatalf("попыток %d до смены сети: пауза не держится", n)
	}
	rezolver.Store(netip.MustParseAddr("10.0.0.1"))
	dozhdatsya(t, "попытка после смены сети", func() bool { return podnimali.Load() > neudachDoRedkih })
}
