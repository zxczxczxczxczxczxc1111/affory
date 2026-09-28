package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/reklama"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/set"
)

// Каталог наборов один на весь прогон пакета (TestMain), поэтому файлы
// рекламы убираются до теста и после него.
func chistayaReklama(t *testing.T) {
	t.Helper()
	ubrat := func() {
		fayly, err := filepath.Glob(filepath.Join(katalogNaborov(), "reklama*"))
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range fayly {
			if err := os.Remove(f); err != nil && !errors.Is(err, os.ErrNotExist) {
				t.Errorf("%s не убран: %v", filepath.Base(f), err)
			}
		}
	}
	ubrat()
	t.Cleanup(ubrat)
}

// Те же обязательные домены, что проверяет reklama.Razobrat.
var obyazatelnyeReklamy = []string{
	"an.yandex.ru", "mc.yandex.ru", "ad.mail.ru", "top-fwz1.mail.ru",
	"googleads.g.doubleclick.net", "pagead2.googlesyndication.com",
}

// spisokLight собирает годный по форме список light из pravil правил.
func spisokLight(pravil int) []byte { return spisokUrovnya("Multi LIGHT", pravil) }

func spisokUrovnya(metka string, pravil int) []byte {
	var b strings.Builder
	b.WriteString("[Adblock Plus]\n! Title: HaGeZi's " + metka + " - test\n")
	b.WriteString("! Last modified: 28 Sep 2026 08:46 UTC\n! Version: 2026.0928.0846.00\n")
	fmt.Fprintf(&b, "! Number of entries: %d\n", pravil)
	for _, d := range obyazatelnyeReklamy {
		b.WriteString("||" + d + "^\n")
	}
	for i := len(obyazatelnyeReklamy); i < pravil; i++ {
		fmt.Fprintf(&b, "||dobivka%d.example^\n", i)
	}
	return []byte(b.String())
}

// sobratPodstavno изображает convert: набор зависит от длины текста, то есть
// у разных списков разный sha256.
func sobratPodstavno(_ context.Context, tekst, vyhod string) error {
	b, err := os.ReadFile(tekst)
	if err != nil {
		return err
	}
	return os.WriteFile(vyhod, []byte(fmt.Sprintf("SRS\x02 %d", len(b))), 0o644)
}

func vklyuchitReklamu(t *testing.T, s *Sluzhba, telo string) {
	t.Helper()
	if r := setRulesTelo(t, s, telo); r.Oshib != nil {
		t.Fatal(r.Oshib)
	}
}

func shaReklamy(t *testing.T) string {
	t.Helper()
	sha, err := sha256Fayla(putReklamy())
	if err != nil {
		t.Fatal(err)
	}
	return sha
}

func shaVstroennogo() string {
	_, m := reklama.Vstroennyy()
	return m.Sha256
}

// chasyReklamy: время стоит, пока тест его не сдвинет.
type chasyReklamy struct {
	mu    sync.Mutex
	teper time.Time
}

func (c *chasyReklamy) seychas() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.teper
}

func (c *chasyReklamy) sdvinut(d time.Duration) {
	c.mu.Lock()
	c.teper = c.teper.Add(d)
	c.mu.Unlock()
}

// shagiReklamy ведёт расписание по шагам: каждое ожидание отдаёт тесту свою
// паузу и ждёт от него ответа, толчок это или срок.
type shagiReklamy struct {
	pauzy  chan time.Duration
	otvety chan bool
	vyshlo chan struct{}
}

func zapustitShagi(t *testing.T, s *Sluzhba) *shagiReklamy {
	t.Helper()
	sh := &shagiReklamy{pauzy: make(chan time.Duration), otvety: make(chan bool), vyshlo: make(chan struct{})}
	s.zhdatReklamu = func(ctx context.Context, d time.Duration) (bool, bool) {
		select {
		case sh.pauzy <- d:
		case <-ctx.Done():
			return false, false
		}
		select {
		case tolchok := <-sh.otvety:
			return tolchok, true
		case <-ctx.Done():
			return false, false
		}
	}
	ctx, otmena := context.WithCancel(context.Background())
	go func() {
		defer close(sh.vyshlo)
		s.raspisanieReklamy(ctx)
	}()
	t.Cleanup(func() {
		otmena()
		select {
		case <-sh.vyshlo:
		case <-time.After(10 * time.Second):
			t.Error("расписание рекламы не вышло за 10 секунд")
		}
	})
	return sh
}

func (sh *shagiReklamy) pauza(t *testing.T) time.Duration {
	t.Helper()
	select {
	case d := <-sh.pauzy:
		return d
	case <-time.After(10 * time.Second):
		t.Fatal("расписание не дошло до ожидания за 10 секунд")
		return 0
	}
}

func (sh *shagiReklamy) dalshe(tolchok bool) { sh.otvety <- tolchok }

type zaprosySpiska struct {
	mu     sync.Mutex
	adresa []string
}

func (z *zaprosySpiska) zapomnit(adres string) {
	z.mu.Lock()
	z.adresa = append(z.adresa, adres)
	z.mu.Unlock()
}

func (z *zaprosySpiska) vse() []string {
	z.mu.Lock()
	defer z.mu.Unlock()
	return slices.Clone(z.adresa)
}

func podstavnayaReklamy(t *testing.T) (*Sluzhba, *chasyReklamy, *zaprosySpiska) {
	t.Helper()
	chistayaReklama(t)
	s := podstavnaya(t, nil)
	ch := &chasyReklamy{teper: time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)}
	s.seychas = ch.seychas
	z := &zaprosySpiska{}
	s.skachatSpisok = func(_ context.Context, adres string) ([]byte, error) {
		z.zapomnit(adres)
		return nil, errors.New("network disabled in service fixture")
	}
	return s, ch, z
}

func sostoyanieReklamy(t *testing.T, s *Sluzhba) protokol.ReklamaSostoyanie {
	t.Helper()
	st := s.Status().Reklama
	if st == nil {
		t.Fatal("в статусе нет состояния рекламы")
	}
	return *st
}

// Выключенная блокировка не ходит в сеть ни по сроку, ни по толчку: ни та,
// которую не заводили, ни выключенная после включения.
func TestReklamaVyklyuchenaNeHoditVSet(t *testing.T) {
	t.Run("не заводили", func(t *testing.T) { proveritVyklyuchennuyu(t, "") })
	t.Run("выключили", func(t *testing.T) {
		proveritVyklyuchennuyu(t, `{"reklama":{"vkl":false,"uroven":"multi"}}`)
	})
}

func proveritVyklyuchennuyu(t *testing.T, pravila string) {
	s, _, z := podstavnayaReklamy(t)
	if pravila != "" {
		vklyuchitReklamu(t, s, pravila)
		if r := pravilaReklamy(t, s); r == nil || r.Vkl {
			t.Fatalf("выключенная настройка не сохранилась объектом: %+v", r)
		}
	}
	sh := zapustitShagi(t, s)
	if d := sh.pauza(t); d != pervyyZahodReklamy {
		t.Fatalf("первый заход через %v, ждали %v", d, pervyyZahodReklamy)
	}
	sh.dalshe(false)
	if d := sh.pauza(t); d != reklama.Period {
		t.Fatalf("выключенная ждёт %v, ждали %v", d, reklama.Period)
	}
	sh.dalshe(true)
	sh.pauza(t)
	if n := len(z.vse()); n != 0 {
		t.Fatalf("выключенная сходила в сеть %d раз", n)
	}
	if _, err := os.Stat(putReklamy()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("выключенная выложила файл: %v", err)
	}
}

// Включённая без файла сразу получает встроенный список, потом идёт в сеть.
// Отказ сети оставляет встроенный, называет голую причину и ждёт час; толчок
// человека отступа не ждёт.
func TestReklamaVstroennyyIOtkazSeti(t *testing.T) {
	s, _, z := podstavnayaReklamy(t)
	vklyuchitReklamu(t, s, `{"reklama":{"vkl":true}}`)
	sh := zapustitShagi(t, s)
	sh.pauza(t)
	sh.dalshe(false)
	if d := sh.pauza(t); d != otstupReklamy {
		t.Fatalf("после отказа сети ждёт %v, ждали %v", d, otstupReklamy)
	}
	if got := shaReklamy(t); got != shaVstroennogo() {
		t.Fatal("на месте файла не встроенный список")
	}
	if got := z.vse(); len(got) != 1 || got[0] != reklama.Bazovyy.Adres() {
		t.Fatalf("запросы списка: %v", got)
	}
	st := sostoyanieReklamy(t, s)
	if !st.Vstroennyy || st.Pravil == 0 {
		t.Fatalf("статус не описывает встроенный список: %+v", st)
	}
	if !strings.Contains(st.Otkaz, "network disabled") || strings.HasPrefix(st.Otkaz, "список рекламы") {
		t.Fatalf("причина отказа %q", st.Otkaz)
	}

	sh.dalshe(true)
	sh.pauza(t)
	if n := len(z.vse()); n != 2 {
		t.Fatalf("толчок после отказа: запросов %d, ждали 2", n)
	}
	if got := shaReklamy(t); got != shaVstroennogo() {
		t.Fatal("отказ сети тронул лежащий файл")
	}
}

// Смена уровня применяется заменой файла, и толчок из setRules ведёт в сеть
// сразу, хотя light проверен только что.
func TestReklamaSmenaUrovnyaTolchkomSrazu(t *testing.T) {
	s, _, z := podstavnayaReklamy(t)
	s.sobratSpisok = sobratPodstavno
	s.skachatSpisok = func(_ context.Context, adres string) ([]byte, error) {
		z.zapomnit(adres)
		if adres == reklama.Bazovyy.Adres() {
			return spisokLight(30000), nil
		}
		return spisokUrovnya("Multi NORMAL", 120000), nil
	}
	var podyomov atomic.Int32
	prezhniy := s.podnyatTunnel
	s.podnyatTunnel = func(ctx context.Context) (set.Adapter, error) {
		podyomov.Add(1)
		return prezhniy(ctx)
	}
	vklyuchitReklamu(t, s, `{"reklama":{"vkl":true}}`)
	sh := zapustitShagi(t, s)
	sh.pauza(t)
	sh.dalshe(false)
	if d := sh.pauza(t); d != reklama.Period {
		t.Fatalf("после удачи ждёт %v, ждали %v", d, reklama.Period)
	}
	vklyuchitReklamu(t, s, `{"reklama":{"vkl":true,"uroven":"multi"}}`)
	sh.dalshe(true)
	sh.pauza(t)
	got := z.vse()
	if len(got) != 2 || got[1] == reklama.Bazovyy.Adres() {
		t.Fatalf("после смены уровня запросы %v: multi не запрошен", got)
	}
	if m := s.metaReklamy(); m == nil || m.Uroven != "multi" || m.Pravil != 120000 {
		t.Fatalf("мета после смены уровня: %+v", m)
	}
	// И6: за весь прогон расписания туннель не поднимался ни разу.
	if n := podyomov.Load(); n != 0 {
		t.Fatalf("расписание рекламы подняло туннель %d раз", n)
	}
}

// Вывод ядра при отказе сборки бывает на десятки килобайт, а причина едет в
// каждый status.
func TestReklamaOtkazObrezaetsya(t *testing.T) {
	s := podstavnaya(t, nil)
	s.zapomnitOtkazReklamy("ядро не собрало список: exit status 1: " + strings.Repeat("ошибка ", 10000))
	if n := len([]rune(sostoyanieReklamy(t, s).Otkaz)); n > predelPrichinyReklamy+1 {
		t.Fatalf("причина длиной %d символов", n)
	}
}

// Падение числа правил на 40% это отказ: файл прежний, подозрение в мете,
// следующий заход через 20 часов, а не через час.
func TestReklamaPadenieZhdyotPodtverzhdeniya(t *testing.T) {
	s, ch, z := podstavnayaReklamy(t)
	s.sobratSpisok = sobratPodstavno
	s.skachatSpisok = func(_ context.Context, adres string) ([]byte, error) {
		z.zapomnit(adres)
		if len(z.vse()) == 1 {
			return spisokLight(40000), nil
		}
		return spisokLight(24000), nil
	}
	vklyuchitReklamu(t, s, `{"reklama":{"vkl":true}}`)
	sh := zapustitShagi(t, s)
	sh.pauza(t)
	sh.dalshe(false)
	sh.pauza(t)
	prezhniy := shaReklamy(t)
	ch.sdvinut(reklama.Period)
	sh.dalshe(false)
	if d := sh.pauza(t); d < 20*time.Hour {
		t.Fatalf("после падения ждёт %v, ждали не меньше 20 ч", d)
	}
	if shaReklamy(t) != prezhniy {
		t.Fatal("урезанный список подменил прежний")
	}
	m := s.metaReklamy()
	if m == nil || m.Padenie == nil || m.Padenie.Pravil != 24000 || m.Pravil != 40000 {
		t.Fatalf("мета после падения: %+v", m)
	}
	if st := sostoyanieReklamy(t, s); st.Otkaz == "" {
		t.Fatal("отказ по падению не назван")
	}
}

// Битый текст не доходит до ядра, файл прежний.
func TestReklamaBityyTekstNeTrogaetFayl(t *testing.T) {
	s, _, _ := podstavnayaReklamy(t)
	if err := s.obespechitFaylReklamy(); err != nil {
		t.Fatal(err)
	}
	var sobiral atomic.Bool
	s.sobratSpisok = func(context.Context, string, string) error { sobiral.Store(true); return nil }
	s.skachatSpisok = func(context.Context, string) ([]byte, error) {
		return []byte("<html>captive portal</html>"), nil
	}
	if _, err := s.obnovitReklamu(context.Background(), reklama.Bazovyy); err == nil {
		t.Fatal("битый текст принят")
	}
	if sobiral.Load() {
		t.Fatal("битый текст ушёл в ядро")
	}
	if shaReklamy(t) != shaVstroennogo() {
		t.Fatal("битый текст тронул файл")
	}
}

// Отказ сборки не оставляет недособранных файлов и не трогает лежащий.
func TestReklamaOtkazSborkiBezHvostov(t *testing.T) {
	s, _, _ := podstavnayaReklamy(t)
	if err := s.obespechitFaylReklamy(); err != nil {
		t.Fatal(err)
	}
	s.skachatSpisok = func(context.Context, string) ([]byte, error) { return spisokLight(30000), nil }
	s.sobratSpisok = func(_ context.Context, _, vyhod string) error {
		if err := os.WriteFile(vyhod, []byte("SRS\x02 недо"), 0o644); err != nil {
			return err
		}
		return errors.New("ядро не собрало набор")
	}
	if _, err := s.obnovitReklamu(context.Background(), reklama.Bazovyy); err == nil {
		t.Fatal("отказ сборки не дошёл")
	}
	hvosty, err := filepath.Glob(filepath.Join(katalogNaborov(), "reklama*chast"))
	if err != nil {
		t.Fatal(err)
	}
	if len(hvosty) != 0 {
		t.Fatalf("остались хвосты: %v", hvosty)
	}
	if shaReklamy(t) != shaVstroennogo() {
		t.Fatal("отказ сборки тронул файл")
	}
}

// И5: в конфиг идёт только файл, сошедшийся с метой. Испорченный заменяется
// встроенным побайтно, сошедшийся не трогается.
func TestReklamaIsporchennyyFaylZamenyaetsya(t *testing.T) {
	chistayaReklama(t)
	s := podstavnaya(t, nil)
	vstroennyy, _ := reklama.Vstroennyy()
	if err := s.obespechitFaylReklamy(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(putReklamy(), []byte("SRS\x02 испорчен"), 0o644); err != nil {
		t.Fatal(err)
	}
	var zhurnal bytes.Buffer
	prezhniy := log.Writer()
	log.SetOutput(&zhurnal)
	err := s.obespechitFaylReklamy()
	log.SetOutput(prezhniy)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(putReklamy())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b, vstroennyy) {
		t.Fatal("испорченный файл остался на месте")
	}
	if !strings.Contains(zhurnal.String(), "не сошёлся с метой") {
		t.Fatalf("починка не названа в журнале: %q", zhurnal.String())
	}

	svoy := []byte("SRS\x02 свой")
	if err := os.WriteFile(putReklamy(), svoy, 0o644); err != nil {
		t.Fatal(err)
	}
	m := reklama.Meta{Uroven: reklama.Bazovyy, Pravil: 30000, Sha256: shaReklamy(t)}
	if err := zapisatMetuReklamy(m); err != nil {
		t.Fatal(err)
	}
	if err := s.obespechitFaylReklamy(); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(putReklamy()); err != nil || !bytes.Equal(b, svoy) {
		t.Fatalf("сошедшийся файл заменён: %v", err)
	}
}

// И6: обновление списка меняет файл и ни разу не переподнимает ядро.
func TestReklamaObnovlenieNePerepodnimaet(t *testing.T) {
	s, _, _ := podstavnayaReklamy(t)
	vklyuchitReklamu(t, s, `{"reklama":{"vkl":true}}`)
	if err := s.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	bylo := pokolenie(s)
	s.sobratSpisok = sobratPodstavno
	s.skachatSpisok = func(context.Context, string) ([]byte, error) { return spisokLight(30000), nil }
	if _, err := s.obnovitReklamu(context.Background(), reklama.Bazovyy); err != nil {
		t.Fatal(err)
	}
	if pokolenie(s) != bylo {
		t.Fatal("обновление списка переподняло ядро")
	}
	if m := s.metaReklamy(); m == nil || m.Pravil != 30000 || m.Vstroennyy || m.Proveren == nil {
		t.Fatalf("мета после обновления: %+v", m)
	}
}

// Кэш DNS Windows сбрасывается, когда меняется то, что в конфиге. Уровень
// меняет только файл, и сбрасывать нечего.
func TestReklamaSbrosKeshaDNS(t *testing.T) {
	s, _, _ := podstavnayaReklamy(t)
	var sbrosov atomic.Int32
	s.sbrositKeshDNS = func() error { sbrosov.Add(1); return nil }
	for _, shag := range []struct {
		telo  string
		zhdom int32
	}{
		{`{"reklama":{"vkl":true}}`, 1},
		{`{"reklama":{"vkl":true}}`, 0},
		{`{"reklama":{"vkl":true,"uroven":"multi"}}`, 0},
		{`{"reklama":{"vkl":true,"uroven":"multi","razresheno":["mc.yandex.ru"]}}`, 1},
		{`{"reklama":{"vkl":false,"uroven":"multi","razresheno":["mc.yandex.ru"]}}`, 1},
	} {
		sbrosov.Store(0)
		vklyuchitReklamu(t, s, shag.telo)
		if got := sbrosov.Load(); got != shag.zhdom {
			t.Errorf("%s: сбросов %d, ждали %d", shag.telo, got, shag.zhdom)
		}
	}
}

// Отказ обновления живёт, пока нужен тот же уровень: правка исключений его не
// снимает, смена уровня снимает.
func TestReklamaSmenaUrovnyaSnimaetOtkaz(t *testing.T) {
	s, _, _ := podstavnayaReklamy(t)
	vklyuchitReklamu(t, s, `{"reklama":{"vkl":true,"uroven":"multi"}}`)
	s.zapomnitOtkazReklamy("список не скачан: multi.txt")
	otkaz := func() string {
		if r := s.Status().Reklama; r != nil {
			return r.Otkaz
		}
		return ""
	}
	vklyuchitReklamu(t, s, `{"reklama":{"vkl":true,"uroven":"multi","razresheno":["mc.yandex.ru"]}}`)
	if otkaz() == "" {
		t.Fatal("правка исключений сняла отказ того же уровня")
	}
	vklyuchitReklamu(t, s, `{"reklama":{"vkl":true,"uroven":"light","razresheno":["mc.yandex.ru"]}}`)
	if got := otkaz(); got != "" {
		t.Fatalf("после смены уровня остался отказ прежнего: %q", got)
	}
}

// Свои имена службы не блокируются: константы кода, хост подписки и имена
// серверов. Адреса в списке имён не бывают.
func TestSvoiHostyReklamy(t *testing.T) {
	s := podstavnaya(t, nil)
	n := Nabor{
		Servery: []protokol.Server{
			{Id: "a", Host: "VPN.Example.NET.", Sni: "sni.example.org"},
			{Id: "b", Host: "203.0.113.9"},
		},
		Podpiski:  []ZapisPodpiski{{Id: "p", Adres: "https://panel.example.com/sub/sekret"}},
		Aktivnaya: "p",
	}
	imena := s.svoiHostyReklamy(n)
	for _, d := range []string{"vpn.example.net", "sni.example.org", "panel.example.com",
		"raw.githubusercontent.com", imyaProbyImeni} {
		if !slices.Contains(imena, d) {
			t.Errorf("нет %s в %v", d, imena)
		}
	}
	for _, d := range imena {
		if strings.Contains(d, "203.0.113.9") || strings.Contains(d, "sekret") {
			t.Errorf("в своих именах %q", d)
		}
	}
	if !slices.IsSorted(imena) || len(slices.Compact(slices.Clone(imena))) != len(imena) {
		t.Errorf("имена не упорядочены или повторяются: %v", imena)
	}
}

// Расписание рекламы живёт у того же владельца, что подписка: Zavershit его
// дожидается, а не только отменяет.
func TestRaspisanieReklamyUmiraetSoSluzhboy(t *testing.T) {
	s, _, _ := raspisanieProby(t, nil, 1)
	s.zhdat = func(ctx context.Context, _ time.Duration) bool { <-ctx.Done(); return false }
	var vyshlo atomic.Bool
	voshli := make(chan struct{})
	odin := sync.Once{}
	s.zhdatReklamu = func(ctx context.Context, _ time.Duration) (bool, bool) {
		odin.Do(func() { close(voshli) })
		<-ctx.Done()
		time.Sleep(300 * time.Millisecond)
		vyshlo.Store(true)
		return false, false
	}
	s.ZapustitRaspisanie()
	select {
	case <-voshli:
	case <-time.After(10 * time.Second):
		t.Fatal("расписание рекламы не дошло до ожидания за 10 секунд")
	}
	gotovo := make(chan struct{})
	go func() { s.Zavershit(); close(gotovo) }()
	select {
	case <-gotovo:
	case <-time.After(10 * time.Second):
		t.Fatal("Zavershit не дождался расписания рекламы")
	}
	if !vyshlo.Load() {
		t.Fatal("Zavershit вернулся раньше, чем расписание рекламы вышло")
	}
}

func reklamaVKonfige(s *Sluzhba) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reklamaVKonfige
}

// Ядро не приняло файл набора: туннель поднимается без блока, файл уходит в
// сторону для разбора, статус не врёт, что блок действует.
func TestReklamaOtvergnutayaYadromPodnimaetsyaBezBloka(t *testing.T) {
	chistayaReklama(t)
	s := podstavnaya(t, nil)
	vklyuchitReklamu(t, s, `{"reklama":{"vkl":true}}`)
	put := filepath.Join(t.TempDir(), "sing-box.json")

	if _, _, err := s.sobratTunProverennyy(put, false); err != nil {
		t.Fatal(err)
	}
	if !reklamaVKonfige(s) {
		t.Fatal("без судьи блок не попал в конфиг: дальше проверять нечего")
	}

	zvali := 0
	s.proveritKonfig = func(p string) error {
		zvali++
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if bytes.Contains(b, []byte("reklama.srs")) {
			return errors.New("initialize router: parse rule-set[1]: unexpected EOF")
		}
		return nil
	}
	// Сухая проверка ничего не откладывает: файл судит боевая сборка.
	if _, _, err := s.sobratTunProverennyy(put, true); err != nil {
		t.Fatalf("сухая проверка без блока: %v", err)
	}
	if _, err := os.Stat(putReklamy()); err != nil {
		t.Fatalf("сухая проверка тронула файл: %v", err)
	}

	zvali = 0
	if _, _, err := s.sobratTunProverennyy(put, false); err != nil {
		t.Fatalf("подъём отказал из-за файла рекламы: %v", err)
	}
	if zvali != 2 {
		t.Fatalf("ядро спросили %d раз, ждали 2: с блоком и без", zvali)
	}
	if b, err := os.ReadFile(put); err != nil || bytes.Contains(b, []byte("reklama.srs")) {
		t.Fatalf("в поднятом конфиге остался блок: %v", err)
	}
	if _, err := os.Stat(putReklamy() + ".otvergnut"); err != nil {
		t.Fatalf("отвергнутый файл не отложен: %v", err)
	}
	if reklamaVKonfige(s) {
		t.Fatal("служба считает блок действующим")
	}
	st := sostoyanieReklamy(t, s)
	if st.Deystvuet || st.Otkaz == "" {
		t.Fatalf("статус после отказа ядра: %+v", st)
	}
}

// Повтор без блока ровно один: отказ ядра, к рекламе не относящийся, валит
// подъём как прежде, а файл списка остаётся на месте.
func TestReklamaChuzhoyOtkazYadraValitPodyom(t *testing.T) {
	chistayaReklama(t)
	s := podstavnaya(t, nil)
	vklyuchitReklamu(t, s, `{"reklama":{"vkl":true}}`)
	zvali := 0
	s.proveritKonfig = func(string) error {
		zvali++
		return errors.New("initialize inbound[0]: listen tcp: bind: address already in use")
	}
	if _, _, err := s.sobratTunProverennyy(filepath.Join(t.TempDir(), "sing-box.json"), false); err == nil {
		t.Fatal("подъём прошёл при отказе ядра, не связанном с рекламой")
	}
	if zvali != 2 {
		t.Fatalf("ядро спросили %d раз, ждали 2: с блоком и один раз без", zvali)
	}
	if _, err := os.Stat(putReklamy()); err != nil {
		t.Fatalf("файл списка тронут: %v", err)
	}
	if _, err := os.Stat(putReklamy() + ".otvergnut"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("файл отложен, хотя виновато не оно: %v", err)
	}
}

// Сторож перезапускает ядро с тем же конфигом: перед этим битый файл набора
// заменяется встроенным, иначе ядро не поднимется вовсе.
func TestReklamaPeredPerezapuskomYadra(t *testing.T) {
	chistayaReklama(t)
	s := podstavnaya(t, nil)
	vstroennyy, _ := reklama.Vstroennyy()
	if err := s.obespechitFaylReklamy(); err != nil {
		t.Fatal(err)
	}
	isportit := func() {
		t.Helper()
		if err := os.WriteFile(putReklamy(), []byte("SRS\x02 испорчен"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	celyy := func() bool {
		b, err := os.ReadFile(putReklamy())
		return err == nil && bytes.Equal(b, vstroennyy)
	}

	isportit()
	s.peredPerezapuskomYadra(protokol.SostVosstanavl)
	if celyy() {
		t.Fatal("блока в конфиге нет, а файл тронут")
	}

	s.mu.Lock()
	s.reklamaVKonfige = true
	s.mu.Unlock()
	s.peredPerezapuskomYadra(protokol.SostPodnyat)
	if celyy() {
		t.Fatal("файл тронут на событии, которое не перезапуск")
	}
	s.peredPerezapuskomYadra(protokol.SostVosstanavl)
	if !celyy() {
		t.Fatal("перед перезапуском ядра битый файл не заменён")
	}
}
