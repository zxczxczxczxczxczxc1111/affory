package main

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/genkonfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/sboi"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/set"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/yadra"
)

// Причина отказа пробы живого ядра из его журнала (02.10.2026). Истёкший
// сертификат сервера давал 503 у Clash API, служба называла это «сервер не
// принял ключ» и вела обновлять подписку.

// 503 в том виде, в каком его отдаёт yadra.Zaderzhka.
func otkaz503() error {
	return fmt.Errorf("%w (код ответа %d)", yadra.ErrServerOtvergKlyuchi, 503)
}

// zhalobaZhivogoYadra ведёт себя как настоящее хранилище: отдаёт жалобу на
// группу живого ядра, если она не старше posle.
func zhalobaZhivogoYadra(t *testing.T, prichina sboi.PrichinaYadra, kogda time.Time) func(konfig, teg string, posle time.Time) (yadra.ZhalobaYadra, bool) {
	t.Helper()
	return func(konfig, teg string, posle time.Time) (yadra.ZhalobaYadra, bool) {
		if konfig != putKonfigaTun() || teg != tegDlyaZamera() || kogda.Before(posle) {
			return yadra.ZhalobaYadra{}, false
		}
		return yadra.ZhalobaYadra{Prichina: prichina, Vremya: kogda}, true
	}
}

// Путь целиком, от 503 подставного Clash API до кода на проводе и в состоянии.
func TestPodyomNazyvaetPrichinuYadraVmestoOtkazaKlyucha(t *testing.T) {
	korotkiyPodyom(t)
	s, _ := sZhivoyProboy(t, otvergaetProbu)
	// Ядро жалуется, пока идёт подъём: строка трафика приходит после старта.
	s.zhalobaVyhoda = func(konfig, teg string, posle time.Time) (yadra.ZhalobaYadra, bool) {
		return zhalobaZhivogoYadra(t, sboi.SrokSertifikata, time.Now())(konfig, teg, posle)
	}

	o := s.Obrabotat(ctxAdmina(), protokol.Kadr{Tip: "cmd", Id: 1, Imya: "connect"})
	if o.Oshib == nil {
		t.Fatal("подъём объявлен удачным при отвергнутой пробе")
	}
	if o.Oshib.Kod != protokol.KodZashchitaServera {
		t.Errorf("код отказа %q, ждали server-tls-failed", o.Oshib.Kod)
	}
	if !strings.Contains(o.Oshib.Tekst, sboi.SrokSertifikata.Tekst()) {
		t.Errorf("в тексте отказа нет причины ядра: %q", o.Oshib.Tekst)
	}
	if st := s.Status(); st.Oshib == nil || st.Oshib.Kod != protokol.KodZashchitaServera ||
		!strings.Contains(st.Oshib.Tekst, sboi.SrokSertifikata.Tekst()) {
		t.Errorf("состояние говорит другое, чем ответ команды: %+v", st.Oshib)
	}
}

func TestPereklyucheniePoPrichineYadra(t *testing.T) {
	s, port := sZhivoyProboy(t, otvergaetProbu)
	s.zapomnitKlash(port, "sekret-stenda")
	s.zhalobaVyhoda = func(konfig, teg string, posle time.Time) (yadra.ZhalobaYadra, bool) {
		return zhalobaZhivogoYadra(t, sboi.ChuzhoeImya, time.Now())(konfig, teg, posle)
	}

	o := s.Obrabotat(ctxAdmina(), protokol.Kadr{Tip: "cmd", Id: 1, Imya: "setServer", Telo: []byte(`{"id":"nl"}`)})
	if o.Oshib == nil || o.Oshib.Kod != protokol.KodZashchitaServera {
		t.Fatalf("отказ переключения %+v, ждали server-tls-failed", o.Oshib)
	}
	if !strings.Contains(o.Oshib.Tekst, sboi.ChuzhoeImya.Tekst()) {
		t.Errorf("в тексте отказа нет причины ядра: %q", o.Oshib.Tekst)
	}
}

// Группа vybor сервер не называет. Жалоба, пришедшая ДО переключения, говорит
// о прежнем сервере, и выдать её за причину отказа нового значит соврать.
func TestZhalobaDoPereklyucheniyaNeProNovyyServer(t *testing.T) {
	s, port := sZhivoyProboy(t, otvergaetProbu)
	s.zapomnitKlash(port, "sekret-stenda")
	ranshe := time.Now()
	time.Sleep(5 * time.Millisecond)
	s.zhalobaVyhoda = zhalobaZhivogoYadra(t, sboi.SrokSertifikata, ranshe)

	o := s.Obrabotat(ctxAdmina(), protokol.Kadr{Tip: "cmd", Id: 1, Imya: "setServer", Telo: []byte(`{"id":"nl"}`)})
	if o.Oshib == nil || o.Oshib.Kod != protokol.KodServerAuthFailed {
		t.Fatalf("отказ переключения %+v: жалоба прежнего сервера приписана новому", o.Oshib)
	}
}

// Разрыв посреди работы: 02.10.2026 сертификат истёк, когда VPN уже был
// поднят. Своей причины у сети нет, и человек видел голое «VPN перестал нести
// трафик».
func TestRazryvNazyvaetPrichinuYadra(t *testing.T) {
	s := podstavnaya(t, nil)
	bezSetevyhProb(s)
	// Адаптер туннеля жив и держит маршрут: у сети своей причины нет, как и
	// было 02.10.2026.
	s.adaptery = func() ([]set.Adapter, error) {
		s.mu.Lock()
		indeks := s.tun.Indeks
		s.mu.Unlock()
		return []set.Adapter{{Indeks: indeks, Imya: "tun0",
			Adresa: []netip.Addr{netip.MustParseAddr("172.19.0.1")}, Umolchanie: true}}, nil
	}
	var zvali atomic.Int32
	s.zamerit = func(ctx context.Context, adres, sekret, teg string) (time.Duration, error) {
		if zvali.Add(1) == 1 {
			return time.Millisecond, nil // подъём удался
		}
		return 0, otkaz503()
	}
	s.zhalobaVyhoda = func(konfig, teg string, posle time.Time) (yadra.ZhalobaYadra, bool) {
		return zhalobaZhivogoYadra(t, sboi.SrokSertifikata, time.Now())(konfig, teg, posle)
	}
	s.period = 20 * time.Millisecond

	vzglyanut, _ := sobytiyaStatusa(s)
	if err := s.Connect(context.Background()); err != nil {
		t.Fatalf("подъём не прошёл: %v", err)
	}
	got := pervayaPrichinaVosstanovleniya(t, vzglyanut)
	if got.Kod != protokol.KodTunnelNeNeset || !strings.Contains(got.Tekst, sboi.SrokSertifikata.Tekst()) {
		t.Fatalf("причина разрыва %+v, ждали tunnel-not-carrying с причиной ядра", got)
	}
}

func TestBezZhalobyOtkazOstayotsyaPrezhnim(t *testing.T) {
	s := podstavnaya(t, nil)
	s.zamerit = func(context.Context, string, string, string) (time.Duration, error) { return 0, otkaz503() }
	s.zhalobaVyhoda = func(string, string, time.Time) (yadra.ZhalobaYadra, bool) { return yadra.ZhalobaYadra{}, false }
	_, err := s.zameritYadro(context.Background(), "127.0.0.1:1", "x", tegDlyaZamera(), time.Now())
	if !errors.Is(err, yadra.ErrServerOtvergKlyuchi) || estOtkazZashchity(err) {
		t.Fatalf("без жалобы ядра отказ подменён: %v", err)
	}
}

// Таймаут пробы это не отказ звонка: жалобы не спрашиваются вовсе, иначе
// старая беда соседнего сервера назвалась бы причиной молчания.
func TestTaymautProbyNeSprashivaetZhalob(t *testing.T) {
	s := podstavnaya(t, nil)
	s.zamerit = func(context.Context, string, string, string) (time.Duration, error) {
		return 0, fmt.Errorf("%w: не отвечает дольше 5s", yadra.ErrProbaNeUspela)
	}
	s.zhalobaVyhoda = func(string, string, time.Time) (yadra.ZhalobaYadra, bool) {
		t.Error("жалоба спрошена на таймауте пробы")
		return yadra.ZhalobaYadra{}, false
	}
	if _, err := s.zameritYadro(context.Background(), "127.0.0.1:1", "x", tegDlyaZamera(), time.Now()); !errors.Is(err, yadra.ErrProbaNeUspela) {
		t.Fatalf("таймаут пробы подменён: %v", err)
	}
}

func TestKodyOtkazaZashchity(t *testing.T) {
	oz := &otkazZashchity{prichina: sboi.NeizvestnyyPodpisant}
	chuzhoe := &protokol.Oshibka{Kod: protokol.KodFirewallFailed}
	sluchai := []struct {
		imya, kod, zhdyom string
	}{
		{"подъём", kodPodklyucheniya(fmt.Errorf("VPN не понёс трафик: %w", oz), chuzhoe), protokol.KodZashchitaServera},
		{"состояние подъёма", kodNepodnyavshegosya(oz), protokol.KodZashchitaServera},
		{"переключение с откатом", kodPereklyucheniya(fmt.Errorf("%w: %w", ErrNovyyVyborNeNesyot, oz)), protokol.KodZashchitaServera},
		// Возврат не прошёл: подключения больше нет, и это важнее причины.
		{"переключение без отката", kodPereklyucheniya(fmt.Errorf("%w: проба %v, возврат %v", ErrOtkatNeUdalsya, oz, errors.New("x"))), protokol.KodTunnelNeNeset},
	}
	for _, sl := range sluchai {
		if sl.kod != sl.zhdyom {
			t.Errorf("%s: код %q, ждали %q", sl.imya, sl.kod, sl.zhdyom)
		}
	}
	if errors.Is(oz, yadra.ErrServerOtvergKlyuchi) {
		t.Error("отказ защиты всё ещё читается как отказ ключа")
	}
	if got := sboi.DlyaCheloveka(fmt.Errorf("VPN не понёс трафик: %w", oz)); !strings.Contains(got, sboi.NeizvestnyyPodpisant.Tekst()) {
		t.Errorf("человеку уходит %q, причина потеряна", got)
	}
}

// Доспрос у ядра замера (приёмка 1.9.3, 02.10.2026). Живое ядро молчит: через
// новый сервер не прошло ни одного соединения человека. Ядро замера звонит
// серверу само и жалуется на его исходящий srv-<id>.
const konfigZameraTesta = "sing-box.zamer.json"

type dosprosZamera struct {
	mu        sync.Mutex
	tegi      []string
	ostanovok atomic.Int32
	podnyato  atomic.Int32
}

func (d *dosprosZamera) sprosheno() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.tegi...)
}

func yadroZameraNazyvaet(t *testing.T, s *Sluzhba, prichina sboi.PrichinaYadra, server time.Duration) *dosprosZamera {
	t.Helper()
	d := &dosprosZamera{}
	s.yadroZamera = func(context.Context) (vremennoeYadro, error) {
		d.podnyato.Add(1)
		return vremennoeYadro{
			zamer:       genkonfig.VhodZamera{Port: 10810, Parol: "sekret"},
			isklyucheny: map[string]bool{},
			konfig:      konfigZameraTesta,
			ostanovit:   func() { d.ostanovok.Add(1) },
		}, nil
	}
	s.zamerPinga = func(context.Context, string, *url.URL) (time.Duration, error) {
		if server > 0 {
			return server, nil
		}
		return 0, errors.New("соединение оборвано")
	}
	s.zhalobaVyhoda = func(konfig, teg string, posle time.Time) (yadra.ZhalobaYadra, bool) {
		if konfig != konfigZameraTesta {
			return yadra.ZhalobaYadra{}, false
		}
		d.mu.Lock()
		d.tegi = append(d.tegi, teg)
		d.mu.Unlock()
		return yadra.ZhalobaYadra{Prichina: prichina, Vremya: time.Now()}, true
	}
	return d
}

func TestPereklyucheniePrichinaOtYadraZamera(t *testing.T) {
	s, port := sZhivoyProboy(t, otvergaetProbu)
	s.zapomnitKlash(port, "sekret-stenda")
	d := yadroZameraNazyvaet(t, s, sboi.SrokSertifikata, 0)

	o := s.Obrabotat(ctxAdmina(), protokol.Kadr{Tip: "cmd", Id: 1, Imya: "setServer", Telo: []byte(`{"id":"nl"}`)})
	if o.Oshib == nil || o.Oshib.Kod != protokol.KodZashchitaServera {
		t.Fatalf("отказ переключения %+v, ждали server-tls-failed", o.Oshib)
	}
	if !strings.Contains(o.Oshib.Tekst, sboi.SrokSertifikata.Tekst()) {
		t.Errorf("в тексте отказа нет причины: %q", o.Oshib.Tekst)
	}
	if got := d.sprosheno(); len(got) != 1 || got[0] != genkonfig.TegKandidata("nl") {
		t.Errorf("жалобы ядра замера спрошены про %v, ждали один %s", got, genkonfig.TegKandidata("nl"))
	}
	if n := d.ostanovok.Load(); n != 1 {
		t.Errorf("ядро замера остановлено %d раз, ждали 1", n)
	}
}

func TestPodyomVRuchnomPrichinaOtYadraZamera(t *testing.T) {
	korotkiyPodyom(t)
	s, _ := sZhivoyProboy(t, otvergaetProbu)
	if err := s.zapisatNabor(Nabor{
		Servery: []protokol.Server{serverProby()}, Vybran: "nl", Rezhim: protokol.RezhimRuchnoy,
	}); err != nil {
		t.Fatalf("набор не записан: %v", err)
	}
	d := yadroZameraNazyvaet(t, s, sboi.NeizvestnyyPodpisant, 0)

	o := s.Obrabotat(ctxAdmina(), protokol.Kadr{Tip: "cmd", Id: 1, Imya: "connect"})
	if o.Oshib == nil || o.Oshib.Kod != protokol.KodZashchitaServera {
		t.Fatalf("отказ подъёма %+v, ждали server-tls-failed", o.Oshib)
	}
	if st := s.Status(); st.Oshib == nil || !strings.Contains(st.Oshib.Tekst, sboi.NeizvestnyyPodpisant.Tekst()) {
		t.Errorf("в состоянии нет причины: %+v", st.Oshib)
	}
	// Один доспрос на весь подъём, а не на каждую пробу.
	if n := d.podnyato.Load(); n != 1 {
		t.Errorf("ядро замера поднято %d раз, ждали 1", n)
	}
}

// В «авто» 503 группы не называет сервер, и доспрос одного из них приписал бы
// его беду всем.
func TestPodyomVAvtoNeSprashivaetYadroZamera(t *testing.T) {
	korotkiyPodyom(t)
	s, _ := sZhivoyProboy(t, otvergaetProbu)
	if err := s.zapisatNabor(Nabor{
		Servery: []protokol.Server{serverProby()}, Rezhim: protokol.RezhimAvto,
	}); err != nil {
		t.Fatalf("набор не записан: %v", err)
	}
	d := yadroZameraNazyvaet(t, s, sboi.SrokSertifikata, 0)

	o := s.Obrabotat(ctxAdmina(), protokol.Kadr{Tip: "cmd", Id: 1, Imya: "connect"})
	if o.Oshib == nil || o.Oshib.Kod != protokol.KodServerAuthFailed {
		t.Fatalf("отказ подъёма %+v, ждали прежний server-auth-failed", o.Oshib)
	}
	if n := d.podnyato.Load(); n != 0 {
		t.Errorf("в авто поднято ядро замера: %d раз", n)
	}
}

func TestUtochnenieBezPrichiny(t *testing.T) {
	s := podstavnaya(t, nil)
	// Сервер через ядро замера ответил: 503 живого ядра был не про защищённое
	// соединение, и старая жалоба тут ничего не доказывает.
	d := yadroZameraNazyvaet(t, s, sboi.SrokSertifikata, 30*time.Millisecond)
	if err := s.utochnitPrichinu(context.Background(), otkaz503(), "nl"); !errors.Is(err, yadra.ErrServerOtvergKlyuchi) || estOtkazZashchity(err) {
		t.Fatalf("при ответившем сервере отказ подменён: %v", err)
	}
	if got := d.sprosheno(); len(got) != 0 {
		t.Errorf("жалобы спрошены при ответившем сервере: %v", got)
	}
	// Таймаут пробы не доспрашивается вовсе.
	taymaut := fmt.Errorf("%w: не отвечает дольше 5s", yadra.ErrProbaNeUspela)
	if err := s.utochnitPrichinu(context.Background(), taymaut, "nl"); err != taymaut {
		t.Fatalf("таймаут подменён: %v", err)
	}
	if n := d.podnyato.Load(); n != 1 {
		t.Errorf("ядро замера поднято %d раз, ждали 1", n)
	}
}

func TestZhalobyNePrezheSmenyVybora(t *testing.T) {
	s := podstavnaya(t, nil)
	nedavno := time.Now().Add(-10 * time.Second)
	s.otmetitSmenuVybora(nedavno)
	if got := s.zhalobyNePrezhe(); !got.Equal(nedavno) {
		t.Fatalf("граница %v, а выбор сменился в %v", got, nedavno)
	}
	s.otmetitSmenuVybora(time.Now().Add(-time.Hour))
	if got := time.Since(s.zhalobyNePrezhe()); got < srokZhalobyYadra-time.Second || got > srokZhalobyYadra+time.Second {
		t.Fatalf("без недавней смены выбора граница отстоит на %v, ждали %v", got, srokZhalobyYadra)
	}
}
