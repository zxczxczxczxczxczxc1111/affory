package main

import (
	"context"
	"errors"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/set"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/sostoyanie"
)

// Блок А аудита 1.8.0: замок без туннеля всегда значит идущее восстановление,
// и туннель, опущенный не человеком, возвращается сам.

func adapterStenda() set.Adapter {
	return set.Adapter{Indeks: 10, Imya: "tun0", Opisanie: "sing-tun Tunnel",
		Adresa: []netip.Addr{netip.MustParseAddr("172.19.0.1")}}
}

// H1б. Переподъём по команде (правила, защита) опустил туннель, а подъём не
// удался. Туннель опускал не человек, значит он обязан вернуться сам.
func TestProvalPerepodyomaUhoditVVosstanovlenie(t *testing.T) {
	s := sluzhbaPodnyatayaS(t, func(s *Sluzhba) { s.otstupy = []time.Duration{time.Millisecond} })
	var podyomov atomic.Int32
	s.podnyatTunnel = func(ctx context.Context) (set.Adapter, error) {
		if podyomov.Add(1) == 1 {
			return set.Adapter{}, errors.New("тест: адаптер не поднялся")
		}
		s.zapomnitKlash(52715, "sekret-stenda")
		return adapterStenda(), nil
	}
	if err := s.perepodklyuchit(context.Background()); err == nil {
		t.Fatal("переподъём с неподнявшимся адаптером ответил успехом")
	}
	dozhdatsya(t, "туннель вернулся после провала переподъёма", func() bool {
		return s.Status().Sostoyanie == protokol.SostPodnyat && podyomov.Load() >= 2
	})
}

// H1а. Ядро не приняло конфиг под новую сеть: старый туннель работает дальше,
// и наблюдатель обязан остаться при нём. Прежде он уходил сразу после запуска
// переподъёма, и туннель оставался без проб и без реакции на следующую смену.
func TestZabrakovannyyKandidatNeGasitNablyudatelya(t *testing.T) {
	s := sluzhbaSRezolverom(t, "192.168.0.1")
	s.mestnyyRezolver = func(...uint32) (netip.Addr, error) { return netip.MustParseAddr("10.0.0.1"), nil }
	s.proveritKonfig = func(string) error { return errors.New("тест: ядро не приняло конфиг") }

	if s.smotretSet(context.Background()) {
		t.Fatal("наблюдатель ушёл, хотя туннель остался прежним: дальше его никто не проверяет")
	}
	if s.poraSmotretSet() {
		t.Fatal("после забракованного кандидата повтор идёт сразу: проверка ядром на каждом тике")
	}
	if got := s.Status().Sostoyanie; got != protokol.SostPodnyat {
		t.Fatalf("состояние %s: забракованный кандидат тронул рабочий туннель", got)
	}
}

// H1а, вторая ветка. Кандидат принят, а подъём в новой сети не удался (сети
// ещё нет). Прежде туннель оставался лежать без восстановления.
func TestProvalPodyomaPodNovuyuSetUhoditVVosstanovlenie(t *testing.T) {
	s := sluzhbaSRezolverom(t, "192.168.0.1")
	s.mestnyyRezolver = func(...uint32) (netip.Addr, error) { return netip.MustParseAddr("10.0.0.1"), nil }
	s.proveritKonfig = func(string) error { return nil }
	s.otstupy = []time.Duration{time.Millisecond}
	var podyomov atomic.Int32
	s.podnyatTunnel = func(ctx context.Context) (set.Adapter, error) {
		if podyomov.Add(1) == 1 {
			return set.Adapter{}, errors.New("тест: в новой сети ещё нет интернета")
		}
		s.zapomnitKlash(52715, "sekret-stenda")
		return adapterStenda(), nil
	}
	if !s.smotretSet(context.Background()) {
		t.Fatal("смена сети не замечена")
	}
	dozhdatsya(t, "туннель вернулся после провала подъёма в новой сети", func() bool {
		return s.Status().Sostoyanie == protokol.SostPodnyat && podyomov.Load() >= 2
	})
}

// H1в. Автоподключение при старте на незапертой машине: Wi-Fi пришёл позже
// минуты. Прежде служба сдавалась после шести попыток, и VPN не поднимался
// до перезагрузки.
func TestAvtopodklyuchenieNeSdayotsyaPosleMinuty(t *testing.T) {
	s := podstavnaya(t, nil)
	var podnimali atomic.Int32
	s.podnyatTunnel = func(ctx context.Context) (set.Adapter, error) {
		if podnimali.Add(1) <= 8 {
			return set.Adapter{}, errors.New("тест: сети ещё нет")
		}
		s.zapomnitKlash(52715, "sekret-stenda")
		return adapterStenda(), nil
	}
	s.prochitat = func() (sostoyanie.SostoyanieFayla, error) {
		return sostoyanie.SostoyanieFayla{PodklyuchatPriStarte: true}, nil
	}
	s.zagruzitNastroyki()
	s.zhdat = func(context.Context, time.Duration) bool { return true }
	s.otstupy = []time.Duration{time.Millisecond}
	s.PodklyuchitPriStarte(context.Background())
	dozhdatsya(t, "подъём после долгого отсутствия сети", func() bool {
		return s.Status().Sostoyanie == protokol.SostPodnyat
	})
}

// M1. «Отключить» в первую минуту после загрузки обязано остановить и подъём
// при старте, а не только восстановление.
func TestOtklyuchitGasitPodyomPriStarte(t *testing.T) {
	s := podstavnaya(t, nil)
	var podnimali atomic.Int32
	s.podnyatTunnel = func(ctx context.Context) (set.Adapter, error) {
		podnimali.Add(1)
		return set.Adapter{}, errors.New("тест: сети ещё нет")
	}
	s.prochitat = func() (sostoyanie.SostoyanieFayla, error) {
		return sostoyanie.SostoyanieFayla{PodklyuchatPriStarte: true}, nil
	}
	s.zagruzitNastroyki()
	s.zhdat = func(ctx context.Context, _ time.Duration) bool {
		select {
		case <-ctx.Done():
			return false
		case <-time.After(20 * time.Millisecond):
			return true
		}
	}
	s.otstupy = []time.Duration{20 * time.Millisecond}
	gotovo := make(chan struct{})
	go func() {
		defer close(gotovo)
		s.PodklyuchitPriStarte(context.Background())
	}()
	dozhdatsya(t, "первая попытка при старте", func() bool { return podnimali.Load() >= 1 })
	s.Otklyuchit()
	bylo := podnimali.Load()
	time.Sleep(300 * time.Millisecond)
	if stalo := podnimali.Load(); stalo != bylo {
		t.Fatalf("после «Отключить» подъёмов стало %d, было %d: кнопка не работает", stalo, bylo)
	}
	if got := s.Status().Sostoyanie; got != protokol.SostVyklyuchen {
		t.Fatalf("состояние %s после «Отключить»", got)
	}
	select {
	case <-gotovo:
	case <-time.After(5 * time.Second):
		t.Fatal("подъём при старте не вышел после «Отключить»")
	}
}

// H2. После перезагрузки флаг защиты включён, а замок снят. Запись набора в
// окно между стартом ядра и пробой (обновление подписки на старте) прежде
// запирала машину до того, как туннель подтверждён.
func TestPeresborkaNeZapiraetDoPodtverzhdeniyaTunnelya(t *testing.T) {
	s := podstavnaya(t, nil)
	s.prochitat = func() (sostoyanie.SostoyanieFayla, error) {
		return sostoyanie.SostoyanieFayla{KillSwitch: true, PodklyuchatPriStarte: true}, nil
	}
	s.zagruzitNastroyki()
	var zaperli atomic.Int32
	s.vklyuchitVes = func(set.Razreshyonnoe, bool) error { zaperli.Add(1); return nil }
	voshli := make(chan struct{}, 1)
	otpusk := make(chan struct{})
	s.zamerit = func(ctx context.Context, adres, sekret, teg string) (time.Duration, error) {
		select {
		case voshli <- struct{}{}:
		default:
		}
		<-otpusk
		return 42 * time.Millisecond, nil
	}
	oshibka := make(chan error, 1)
	go func() { oshibka <- s.Connect(context.Background()) }()
	<-voshli
	_ = s.PeresobratRazresheniya()
	if n := zaperli.Load(); n != 0 {
		t.Errorf("машина заперта до подтверждения туннеля (%d раз): проба ещё идёт", n)
	}
	close(otpusk)
	if err := <-oshibka; err != nil {
		t.Fatalf("подъём не прошёл: %v", err)
	}
	dozhdatsya(t, "замок после подтверждения туннеля", func() bool { return zaperli.Load() >= 1 })
}

// M2. Пересборка, начатая до выключения защиты, прежде дожидалась замка и
// запирала машину заново, хотя защиту уже выключили.
func TestVyklyucheniyeZashchityNeZapiraetObratno(t *testing.T) {
	s := podstavnaya(t, nil)
	var zaperli atomic.Int32
	s.vklyuchitVes = func(set.Razreshyonnoe, bool) error { zaperli.Add(1); return nil }
	if err := s.Connect(context.Background()); err != nil {
		t.Fatalf("подъём не прошёл: %v", err)
	}
	if err := s.SetKillSwitch(true); err != nil {
		t.Fatalf("защита не включилась: %v", err)
	}
	voshli := make(chan struct{})
	blok := make(chan struct{})
	var odin atomic.Bool
	prezhniy := s.sobratAdresa
	s.sobratAdresa = func() (set.Adresa, error) {
		if odin.CompareAndSwap(false, true) {
			close(voshli)
			<-blok
		}
		return prezhniy()
	}
	gotovo := make(chan struct{})
	go func() {
		defer close(gotovo)
		_ = s.PeresobratRazresheniya()
	}()
	<-voshli
	if err := s.SetKillSwitch(false); err != nil {
		t.Fatalf("защита не выключилась: %v", err)
	}
	bylo := zaperli.Load()
	close(blok)
	<-gotovo
	if stalo := zaperli.Load(); stalo != bylo {
		t.Fatalf("после выключения защиты машину заперли снова (%d раз)", stalo-bylo)
	}
	s.mu.Lock()
	aktiven := s.zaslonAktiven
	s.mu.Unlock()
	if aktiven {
		t.Fatal("защита выключена, а замок числится стоящим")
	}
}

// L1. Два переподъёма одновременно (смена сети и команда окна) прежде брали
// поколение до чужого Disconnect и гасили друг друга: туннель оставался лежать.
func TestDvaPerepodyomaNeGasyatDrugDruga(t *testing.T) {
	var prishli atomic.Int32
	vse := make(chan struct{})
	s := sluzhbaPodnyatayaS(t, func(s *Sluzhba) {
		s.proveritKonfig = func(string) error {
			if prishli.Add(1) == 2 {
				close(vse)
			}
			select {
			case <-vse:
			case <-time.After(300 * time.Millisecond):
			}
			return nil
		}
	})
	oshibki := make(chan error, 2)
	for range 2 {
		go func() { oshibki <- s.perepodklyuchit(context.Background()) }()
	}
	for range 2 {
		if err := <-oshibki; err != nil {
			t.Errorf("переподъём: %v", err)
		}
	}
	if got := s.Status().Sostoyanie; got != protokol.SostPodnyat {
		t.Fatalf("после двух переподъёмов состояние %s: они погасили друг друга", got)
	}
}

// L2. Замок снял бы только флаг в памяти. Если на старте снять осиротевший замок
// не удалось, флаг ложный, а машина заперта, и «Отключить» её не распечатывало.
func TestOtklyuchitSnimaetZamokIzFaylaOtkata(t *testing.T) {
	s := podstavnaya(t, nil)
	var snyali atomic.Int32
	s.vyklyuchitVes = func() error { snyali.Add(1); return nil }
	s.estOtkat = func() bool { return true }
	s.Otklyuchit()
	if snyali.Load() != 1 {
		t.Fatal("файл отката говорит «заперто», а «Отключить» замок не сняло")
	}
}
