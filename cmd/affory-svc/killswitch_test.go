package main

import (
	"context"
	"errors"
	"net/netip"
	"slices"
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/set"
)

func sKillSwitch(t *testing.T) (*Sluzhba, *[]set.Razreshyonnoe, *int) {
	t.Helper()
	s := podstavnaya(t, nil)
	vklyucheno := []set.Razreshyonnoe{}
	vyklyucheno := 0
	s.vklyuchitVes = func(r set.Razreshyonnoe, namerenno bool) error {
		if !namerenno {
			t.Error("команда человека записана как ненамеренная: следующий старт распечатает машину сам")
		}
		vklyucheno = append(vklyucheno, r)
		return nil
	}
	s.vyklyuchitVes = func() error { vyklyucheno++; return nil }
	s.podnyatTunnel = func(ctx context.Context) (set.Adapter, error) {
		// Доступ к clash_api запоминается ТУТ ЖЕ, как в настоящем sobratTun:
		// он записывается до старта ядра, и подъёма без него не бывает.
		// Заглушка, которая его пропускала, изображала невозможное: состояние
		// podnyat при мёртвом порту управления. На критерии «ядро живо это
		// непустой адрес clash_api» она и покраснела.
		s.zapomnitKlash(52715, "sekret-stenda")
		return set.Adapter{
			Indeks: 10, Imya: "tun0", Opisanie: "sing-tun Tunnel",
			Adresa: []netip.Addr{netip.MustParseAddr("172.19.0.1")},
		}, nil
	}
	return s, &vklyucheno, &vyklyucheno
}

func TestKillSwitchIdleOnlySavesPreference(t *testing.T) {
	// Saving a switch must not lock a machine with no tunnel to carry traffic.
	s, vklyucheno, _ := sKillSwitch(t)
	if err := s.SetKillSwitch(true); err != nil {
		t.Fatal(err)
	}
	if len(*vklyucheno) != 0 {
		t.Fatal("до брандмауэра дошло, хотя туннеля нет")
	}
}

func TestKillSwitchSobiraetSpisokIzSistemy(t *testing.T) {
	s, vklyucheno, vyklyucheno := sKillSwitch(t)
	if err := s.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.SetKillSwitch(true); err != nil {
		t.Fatal(err)
	}
	if len(*vklyucheno) != 1 {
		t.Fatalf("вызовов включения %d", len(*vklyucheno))
	}
	r := (*vklyucheno)[0]
	if r.AdresTun.String() != "172.19.0.1" {
		t.Fatalf("адрес TUN %v, а правило привязывается именно к нему", r.AdresTun)
	}
	if len(r.Kandidaty) == 0 {
		t.Fatal("список кандидатов пуст: запертая машина отрежет сама себя от сервера")
	}
	// Процессов два: ядро и служба (задача П5, ядро одно). Пропустить ядро
	// значило бы, что в запертом режиме оно не дозвонится до своего сервера.
	if len(r.Protsessy) != 2 {
		t.Fatalf("процессов %d, ожидалось два: ядро и служба (%v)", len(r.Protsessy), r.Protsessy)
	}
	if !s.Status().KillSwitch {
		t.Fatal("статус не показывает включённый режим")
	}

	if err := s.SetKillSwitch(false); err != nil {
		t.Fatal(err)
	}
	if *vyklyucheno != 1 {
		t.Fatalf("вызовов выключения %d", *vyklyucheno)
	}
	if s.Status().KillSwitch {
		t.Fatal("статус показывает режим после выключения")
	}
}

func TestKillSwitchCherezDispetcher(t *testing.T) {
	// The command left the "later" table in wave 2, so the dispatcher must answer
	// it for real. A command that still says not-implemented while the code exists
	// is a command the interface will believe.
	if _, est := pozzhe["setKillSwitch"]; est {
		t.Fatal("setKillSwitch всё ещё числится заглушкой")
	}
	s, _, _ := sKillSwitch(t)
	if err := s.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	o := s.Obrabotat(ctxAdmina(), protokol.Kadr{
		Tip: "cmd", Id: 1, Imya: "setKillSwitch", Telo: []byte(`{"vkl":true}`),
	})
	if o.Oshib != nil {
		t.Fatalf("команда отказала: %v", o.Oshib)
	}
	if !s.Status().KillSwitch {
		t.Fatal("режим не включился через диспетчер")
	}
}

func TestOtkazBrandmaueraEtoFirewallFailed(t *testing.T) {
	s, _, _ := sKillSwitch(t)
	s.vklyuchitVes = func(set.Razreshyonnoe, bool) error { return errors.New("netsh отказал") }
	if err := s.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	o := s.Obrabotat(ctxAdmina(), protokol.Kadr{
		Tip: "cmd", Id: 1, Imya: "setKillSwitch", Telo: []byte(`{"vkl":true}`),
	})
	if o.Oshib == nil || o.Oshib.Kod != protokol.KodFirewallFailed {
		t.Fatalf("код %v, ожидался %s", o.Oshib, protokol.KodFirewallFailed)
	}
	if s.Status().KillSwitch {
		t.Fatal("режим числится включённым после отказа брандмауэра")
	}
}

func TestZapertayaMashinaVidnaVStatuse(t *testing.T) {
	// After a reboot on an intentionally locked machine the firewall keeps the
	// lock, but the service starts with killSwitch=false. The UI would then show
	// the mode as off and offer no way out of a machine that has no internet.
	// A status that contradicts the firewall is worse than no status at all.
	s, _, _ := sKillSwitch(t)
	s.PomnitZapertuyu(true)
	if !s.Status().KillSwitch {
		t.Fatal("статус говорит, что режим выключен, а машина заперта брандмауэром")
	}
	// И выйти из него обязано быть можно, иначе признать блокировку бесполезно.
	if err := s.SetKillSwitch(false); err != nil {
		t.Fatalf("из запертого состояния нет выхода: %v", err)
	}
	if s.Status().KillSwitch {
		t.Fatal("режим числится включённым после выключения")
	}
}

func TestSpiskiProtsessovNeRashodyatsya(t *testing.T) {
	// Two consumers, one truth. The tunnel config exempts processes from the
	// hijack so they can reach the server; the firewall exempts the same
	// processes so they can reach it under lockdown. A path present in one list
	// and missing from the other is a hole that only opens on the transports
	// where that binary is the one dialling out.
	s, _, _ := sKillSwitch(t)
	if err := s.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	vTunnele, err := s.putiProtsessov()
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.spisokRazreshyonnogo(set.Adapter{
		Indeks: 10, Imya: "tun0",
		Adresa: []netip.Addr{netip.MustParseAddr("172.19.0.1")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(vTunnele, r.Protsessy) {
		t.Fatalf("списки разошлись:\n  туннель: %v\n  брандмауэр: %v", vTunnele, r.Protsessy)
	}
	// Ядро ОДНО (задача П5), путей поэтому два: ядро и служба. Пока ядер было
	// два, отсутствие любого из них давало дыру ровно на тех транспортах, где
	// наружу ходит именно он, и незаметную на остальных.
	if len(vTunnele) != 2 {
		t.Fatalf("путей %d, ожидалось два: ядро и служба (%v)", len(vTunnele), vTunnele)
	}
	for _, p := range vTunnele {
		if strings.Contains(strings.ToLower(p), "xray") {
			t.Fatalf("путь к Xray остался в списке: %s", p)
		}
	}
}

// Пересборка, которой НЕ БЫЛО, обязана отличаться от удавшейся.
//
// Прежде обе возвращали nil, то есть успех, и вызывающий не мог узнать, что
// список ушёл на диск, а брандмауэр остался под прежние адреса. Второй заслон
// внутри записи набора при этом спрашивал другой критерий, чем первый.
func TestPeresborkaKotoroyNeByloNeNazyvaetsyaUdachnoy(t *testing.T) {
	s, _, _ := sKillSwitch(t)
	if err := s.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.SetKillSwitch(true); err != nil {
		t.Fatal(err)
	}
	// Адаптер без адреса: правило режима привязывается именно к нему, и
	// пересобирать нечем.
	s.mu.Lock()
	s.tun = set.Adapter{}
	s.mu.Unlock()

	bylo, err := s.peresobratRazresheniya()
	if bylo {
		t.Error("пересборка объявлена состоявшейся, хотя её не было")
	}
	if err == nil {
		t.Error("пересборки не было, а ответ nil значит успех: правила остались под прежний список")
	}

	// Зеркало: при ВЫКЛЮЧЕННОМ режиме пересобирать нечего, и это не отказ.
	// Без него проверка выше зелена на реализации, которая жалуется всегда.
	if err := s.SetKillSwitch(false); err != nil {
		t.Fatal(err)
	}
	if bylo, err := s.peresobratRazresheniya(); bylo || err != nil {
		t.Fatalf("выключенный режим: было=%v ошибка=%v, ждали честное «не потребовалось»", bylo, err)
	}
}

// adresaZaNaborom заставляет сбор адресов идти ЗА набором.
//
// Общая фикстура отвечает ОДНИМ постоянным адресом мимо набора, и на ней
// «правило сузили» и «правило не тронули» неотличимы: список кандидатов один и
// тот же до удаления и после. Здесь адреса берутся из самих серверов, а пустой
// набор отвечает тем же ErrNetServerov, что и настоящий adresaKandidatov, иначе
// проверка пустого списка судила бы поведение, которого в продукте нет.
func adresaZaNaborom(s *Sluzhba) {
	s.sobratAdresa = func() ([]netip.Addr, error) {
		n, err := s.nabor()
		if err != nil {
			return nil, err
		}
		if len(n.Servery) == 0 {
			return nil, ErrNetServerov
		}
		var a []netip.Addr
		for _, srv := range n.Servery {
			adr, err := netip.ParseAddr(srv.Host)
			if err != nil {
				return nil, err
			}
			a = append(a, adr)
		}
		return a, nil
	}
}

// ПРОВАЛ 3.7 живого прогона 03.09.2026: сервер, удалённый при включённом режиме
// и упавшем туннеле, оставался разрешённым в брандмауэре. Причина не в netsh:
// пересборка правил выходила молчаливым «не потребовалось», как только адрес
// clash_api оказывался пустым, потому что собрать НАБОР правил без адреса TUN
// действительно нечем. Список серверов от туннеля не зависит, и оставлять в
// разрешающих адрес удалённого сервера значит держать дыру ровно в том режиме,
// ради которого его включают.
func TestUdalenieServeraPriMyortvomYadreSuzhaetPravila(t *testing.T) {
	s, _, _ := sKillSwitch(t)
	adresaZaNaborom(s)
	var suzheno [][]netip.Addr
	s.suzitServery = func(k []netip.Addr) error {
		suzheno = append(suzheno, append([]netip.Addr{}, k...))
		return nil
	}
	if o := vypolnit(t, s, "addServer", map[string]string{"ssylka": ssylkaProby}); o.Oshib != nil {
		t.Fatalf("второй сервер не добавился: %v", o.Oshib)
	}
	n, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}
	var lishniy, adresLishnego string
	for _, srv := range n.Servery {
		if srv.Id != "nl" {
			lishniy, adresLishnego = srv.Id, srv.Host
		}
	}
	if lishniy == "" {
		t.Fatal("второго сервера в наборе нет: удалять нечего")
	}

	if err := s.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.SetKillSwitch(true); err != nil {
		t.Fatal(err)
	}
	// Туннель падает. Машина при этом остаётся ЗАПЕРТОЙ: ради этого режим и
	// включают, и если бы опускание его снимало, проверять дальше было бы нечего.
	s.Disconnect()
	if !s.Status().KillSwitch {
		t.Fatal("опускание туннеля сняло режим: предмет проверки исчез")
	}
	if adres, _ := s.dostupKKlash(); adres != "" {
		t.Fatalf("ядро числится живым (%s): проверяется не тот случай", adres)
	}

	suzheno = nil
	if o := vypolnit(t, s, "removeServer", map[string]string{"id": lishniy}); o.Oshib != nil {
		t.Fatalf("удаление отклонено: %v", o.Oshib)
	}
	if len(suzheno) != 1 {
		t.Fatalf("сужений правила %d, ждали одно: разрешающие остались под прежний список", len(suzheno))
	}
	for _, a := range suzheno[0] {
		if a.String() == adresLishnego {
			t.Errorf("адрес удалённого сервера %s остался разрешённым", adresLishnego)
		}
	}
	if len(suzheno[0]) == 0 {
		t.Error("список сузили до пустого: оставшийся сервер тоже отрезан")
	}
}

// Зеркало: при ЖИВОМ ядре правило пересобирается набором целиком, и узкий путь
// туда лезть не должен. Без этого проверка выше зелена на реализации, которая
// сужает правило всегда и дважды.
func TestPriZhivomYadreSuzheniyaNeProishodit(t *testing.T) {
	s, vklyucheno, _ := sKillSwitch(t)
	suzheno := 0
	s.suzitServery = func([]netip.Addr) error { suzheno++; return nil }
	if err := s.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.SetKillSwitch(true); err != nil {
		t.Fatal(err)
	}
	bylo := len(*vklyucheno)
	if o := vypolnit(t, s, "addServer", map[string]string{"ssylka": ssylkaProby}); o.Oshib != nil {
		t.Fatalf("сервер не добавился: %v", o.Oshib)
	}
	if suzheno != 0 {
		t.Errorf("узкий путь сработал при живом ядре %d раз: набор правил останется недособранным", suzheno)
	}
	if len(*vklyucheno) != bylo+1 {
		t.Errorf("набор правил не пересобран: включений %d, было %d", len(*vklyucheno), bylo)
	}
}

// Удаление ПОСЛЕДНЕГО сервера при запертой машине не снимает правило.
//
// Снятое правило отрезало бы адрес подписки, который стоит в том же списке, и
// человек остался бы заперт без серверов и без способа их получить. Ответ тот
// же, что у живого пути: список сохранён, правила отстали.
func TestUdaleniePoslednegoServeraNeSnimaetPravilo(t *testing.T) {
	s, _, _ := sKillSwitch(t)
	adresaZaNaborom(s)
	suzheno := 0
	s.suzitServery = func([]netip.Addr) error { suzheno++; return nil }
	if err := s.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.SetKillSwitch(true); err != nil {
		t.Fatal(err)
	}
	s.Disconnect()

	o := vypolnit(t, s, "removeServer", map[string]string{"id": "nl"})
	if o.Oshib == nil {
		t.Fatal("удаление последнего сервера прошло молча: человек не узнает, что правила отстали")
	}
	if suzheno != 0 {
		t.Errorf("правило переписано %d раз на пустом списке: снято заодно разрешение подписки", suzheno)
	}
	// Список при этом всё же сохранён: отказ пересборки его не откатывает.
	n, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}
	if len(n.Servery) != 0 {
		t.Errorf("серверов осталось %d: отказ правил откатил запись списка", len(n.Servery))
	}
}
