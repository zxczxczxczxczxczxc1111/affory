package main

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/genkonfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/set"
)

// Порт и секрет clash_api рождаются в sobratTun и до задачи П2 уезжали ТОЛЬКО в
// файл состояния. Пробе, которая заменяет SOCKS-пробу, взять их оттуда нечем:
// файл пишется для следующего запуска, а не для текущего подъёма.
func TestSluzhbaPomnitDostupKKlash(t *testing.T) {
	s := podstavnaya(t, nil)
	s.zapomnitKlash(52715, "sekret")
	adres, sekret := s.dostupKKlash()
	if adres != "127.0.0.1:52715" {
		t.Fatalf("адрес clash_api %q, ожидался петлевой с портом 52715", adres)
	}
	if sekret != "sekret" {
		t.Fatalf("секрет %q, ожидался положенный", sekret)
	}
}

// Секрет живёт ровно столько, сколько поднят туннель: он новый на каждый старт
// ядра, и оставленный после опускания это секрет от ядра, которого больше нет.
func TestDisconnectZabyvaetDostupKKlash(t *testing.T) {
	s := podstavnaya(t, nil)
	s.zapomnitKlash(52715, "sekret")
	s.Disconnect()
	adres, sekret := s.dostupKKlash()
	if adres != "" || sekret != "" {
		t.Fatalf("после опускания остались адрес %q и секрет %q", adres, sekret)
	}
}

// Транспорт, который sing-box несёт сам: второго ядра нет вовсе, порт SOCKS
// никто не слушает, и SOCKS-проба на нём не могла пройти НИКОГДА. Восемь
// одинаковых отказов на чужой подписке были ровно этим, а не мёртвыми узлами.
func TestConnectPodnimaetTransportBezXray(t *testing.T) {
	s := podstavnaya(t, nil)
	svoy := serverProby()
	svoy.Transport = "reality-tcp"
	s.nabor = func() (Nabor, error) { return Nabor{Servery: []protokol.Server{svoy}}, nil }
	s.zamerit = func(ctx context.Context, adres, sekret, teg string) (time.Duration, error) {
		return 42 * time.Millisecond, nil
	}
	s.zhdatKlash = func(ctx context.Context, adres, sekret string) error { return nil }

	if err := s.Connect(context.Background()); err != nil {
		t.Fatalf("транспорт без Xray не поднялся: %v", err)
	}
	if got := s.Status().Sostoyanie; got != protokol.SostPodnyat {
		t.Fatalf("состояние %s, ожидалось podnyat", got)
	}
}

// Замер идёт по тому же пути, которым пойдёт трафик. Его отказ значит, что
// сервер не несёт ничего, и объявлять такое подключение поднятым нельзя: ровно
// это делала SOCKS-проба на чужом сервере, отвечая успехом.
// Ядро отвечает не мгновенно. Спросить его сразу после старта значит померить
// пустоту и объявить отказ на рабочем сервере: ровно это уже случалось со
// сверкой владельца порта, где проверка «сразу после запуска» не проходила ни
// разу. Ждать обязаны мы, а не человек.
func TestConnectNeMeryaetPokaKlashNeGotov(t *testing.T) {
	s := podstavnaya(t, nil)
	merili := false
	s.zhdatKlash = func(ctx context.Context, adres, sekret string) error {
		return errors.New("clash_api не отвечает за 5s")
	}
	s.zamerit = func(ctx context.Context, adres, sekret, teg string) (time.Duration, error) {
		merili = true
		return time.Millisecond, nil
	}
	if err := s.Connect(context.Background()); err == nil {
		t.Fatal("неготовый clash_api не помешал объявить подъём")
	}
	if merili {
		t.Fatal("замер спрошен у ядра, которое ещё не отвечает")
	}
	if got := s.Status().Sostoyanie; got == protokol.SostPodnyat {
		t.Fatalf("состояние %s при неготовом clash_api", got)
	}
}

func TestConnectOtkazyvaetKogdaZamerNeProshyol(t *testing.T) {
	s := podstavnaya(t, nil)
	staryy := zhdatPodyoma
	zhdatPodyoma = 900 * time.Millisecond
	defer func() { zhdatPodyoma = staryy }()
	s.zhdatKlash = func(ctx context.Context, adres, sekret string) error { return nil }
	s.zamerit = func(ctx context.Context, adres, sekret, teg string) (time.Duration, error) {
		return 0, errors.New("исходящий srv-nl не отвечает (код 504)")
	}
	if err := s.Connect(context.Background()); err == nil {
		t.Fatal("сервер, не несущий трафик, объявлен поднятым")
	}
	if got := s.Status().Sostoyanie; got == protokol.SostPodnyat {
		t.Fatalf("состояние %s, а замер не прошёл", got)
	}
}

// Туннель, переставший нести трафик, обязан заметить кто-то кроме человека.
// До П4 наблюдателя не было вовсе: константа PeriodProby существовала, а
// вызывающих у неё не было ни одного, и подъём проверялся ровно один раз.
func TestNablyudenieOpuskaetTunnelPerestavshiyNesti(t *testing.T) {
	s := podstavnaya(t, nil)
	var zvali atomic.Int32
	s.zamerit = func(ctx context.Context, adres, sekret, teg string) (time.Duration, error) {
		if zvali.Add(1) == 1 {
			return time.Millisecond, nil // подъём удался
		}
		return 0, errors.New("исходящий не отвечает (код 504)")
	}
	s.period = 20 * time.Millisecond

	if err := s.Connect(context.Background()); err != nil {
		t.Fatalf("подъём не прошёл: %v", err)
	}

	srok := time.Now().Add(3 * time.Second)
	for time.Now().Before(srok) {
		if s.Status().Sostoyanie == protokol.SostNeNeset {
			if got := s.Status().Oshib; got == nil || got.Kod != protokol.KodTunnelNeNeset {
				t.Fatalf("код ошибки %v, ожидался tunnel-not-carrying", got)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("туннель молчит, а служба продолжает считать его поднятым")
}

// Одиночный провал это норма жизни на мобильной сети, а не смерть туннеля.
// Опускать подключение по первому 504 значит рвать связь на ровном месте.
func TestOdinochnyyProvalZamesaNeOpuskaetTunnel(t *testing.T) {
	s := podstavnaya(t, nil)
	var zvali atomic.Int32
	s.zamerit = func(ctx context.Context, adres, sekret, teg string) (time.Duration, error) {
		// Через раз: подряд двух провалов не бывает никогда.
		if zvali.Add(1)%2 == 0 {
			return 0, errors.New("исходящий не отвечает (код 504)")
		}
		return time.Millisecond, nil
	}
	s.period = 20 * time.Millisecond

	if err := s.Connect(context.Background()); err != nil {
		t.Fatalf("подъём не прошёл: %v", err)
	}
	time.Sleep(600 * time.Millisecond)
	if got := s.Status().Sostoyanie; got != protokol.SostPodnyat {
		t.Fatalf("состояние %s, а провалы шли через раз", got)
	}
	if zvali.Load() < 4 {
		t.Fatalf("замеров %d, значит наблюдение не шло", zvali.Load())
	}
}

// Клиенты, которые считаются хорошими, при аварийном обрыве возвращаются САМИ.
// У Proton VPN это делают и Windows, и Linux, независимо от kill switch. Без
// этого «подключено» честно ровно до первой потери сети.
func TestPosleAvariiSluzhbaPodnimaetsyaSama(t *testing.T) {
	s := podstavnaya(t, nil)
	var podyomov atomic.Int32
	s.podnyatTunnel = func(ctx context.Context) (set.Adapter, error) {
		s.zapomnitKlash(52715, "sekret-stenda")
		podyomov.Add(1)
		return set.Adapter{Indeks: 10, Imya: "tun0"}, nil
	}
	var zamerov atomic.Int32
	s.zamerit = func(ctx context.Context, adres, sekret, teg string) (time.Duration, error) {
		// Первый замер каждого подъёма удачный, следующие два валят туннель.
		if zamerov.Add(1)%3 == 1 {
			return time.Millisecond, nil
		}
		return 0, errors.New("исходящий не отвечает (код 504)")
	}
	s.period = 20 * time.Millisecond
	s.otstupy = []time.Duration{10 * time.Millisecond}

	if err := s.Connect(context.Background()); err != nil {
		t.Fatalf("первый подъём не прошёл: %v", err)
	}

	srok := time.Now().Add(3 * time.Second)
	for time.Now().Before(srok) {
		if podyomov.Load() >= 2 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("подъёмов %d: после аварии служба не вернулась сама", podyomov.Load())
}

// Осознанное отключение человеком переподключать нельзя НИКОГДА, иначе кнопка
// «отключить» перестаёт работать. Ровно на этой границе у Proton проходит
// разделение между обычным kill switch и постоянным.
func TestRuchnoyDisconnectNePorozhdaetVosstanovleniya(t *testing.T) {
	s := podstavnaya(t, nil)
	var podyomov atomic.Int32
	s.podnyatTunnel = func(ctx context.Context) (set.Adapter, error) {
		s.zapomnitKlash(52715, "sekret-stenda")
		podyomov.Add(1)
		return set.Adapter{Indeks: 10, Imya: "tun0"}, nil
	}
	s.period = 20 * time.Millisecond
	s.otstupy = []time.Duration{10 * time.Millisecond}

	if err := s.Connect(context.Background()); err != nil {
		t.Fatalf("подъём не прошёл: %v", err)
	}
	s.Disconnect()
	time.Sleep(500 * time.Millisecond)
	if got := podyomov.Load(); got != 1 {
		t.Fatalf("подъёмов %d, а человек отключился сам", got)
	}
	if got := s.Status().Sostoyanie; got != protokol.SostVyklyuchen {
		t.Fatalf("состояние %s после ручного отключения", got)
	}
}

// Первая попытка вернуться почти всегда проваливается: сеть, из-за которой
// туннель упал, за секунду не чинится. Замерено 01.09.2026 на стенде: после
// гашения адаптера служба честно упала за 40.3 с, сделала одну попытку, та
// провалилась в otkaz, и восстановление ОСТАНОВИЛОСЬ само на своей же проверке
// состояния. Сеть вернули, туннель не вернулся.
func TestVosstanovlenieProdolzhaetsyaPosleNeudachnoyPopytki(t *testing.T) {
	s := podstavnaya(t, nil)
	var podyomov atomic.Int32
	s.podnyatTunnel = func(ctx context.Context) (set.Adapter, error) {
		s.zapomnitKlash(52715, "sekret-stenda")
		n := podyomov.Add(1)
		// Первый подъём удачный, второй (первая попытка возврата) валится, как
		// и бывает на ещё не вернувшейся сети, третий проходит.
		if n == 2 {
			return set.Adapter{}, errors.New("адаптер не появился")
		}
		return set.Adapter{Indeks: 10, Imya: "tun0"}, nil
	}
	var zamerov atomic.Int32
	s.zamerit = func(ctx context.Context, adres, sekret, teg string) (time.Duration, error) {
		if zamerov.Add(1) <= 1 {
			return time.Millisecond, nil
		}
		if podyomov.Load() >= 3 {
			return time.Millisecond, nil
		}
		return 0, errors.New("исходящий не отвечает (код 504)")
	}
	s.period = 20 * time.Millisecond
	s.otstupy = []time.Duration{10 * time.Millisecond}

	if err := s.Connect(context.Background()); err != nil {
		t.Fatalf("первый подъём не прошёл: %v", err)
	}

	srok := time.Now().Add(3 * time.Second)
	for time.Now().Before(srok) {
		if s.Status().Sostoyanie == protokol.SostPodnyat && podyomov.Load() >= 3 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("подъёмов %d, состояние %s: после провалившейся попытки восстановление сдалось",
		podyomov.Load(), s.Status().Sostoyanie)
}

// Замер спрашивает про ГРУППУ, а не про кандидата. Тег кандидата это снимок
// выбора в момент подъёма, и он устаревает молча: выбор внутри группы меняется
// без нашего участия. Успех по устаревшему тегу не говорит про наш путь ничего,
// а провал по нему опускает рабочий туннель.
func TestZamerSprashivaetTegGruppy(t *testing.T) {
	s := podstavnaya(t, nil)
	var sprosili string
	s.zamerit = func(ctx context.Context, adres, sekret, teg string) (time.Duration, error) {
		sprosili = teg
		return time.Millisecond, nil
	}
	if err := s.Connect(context.Background()); err != nil {
		t.Fatalf("подъём не прошёл: %v", err)
	}
	if sprosili != genkonfig.TegSelector {
		t.Fatalf("спрошен тег %q, ожидался %q", sprosili, genkonfig.TegSelector)
	}
}
