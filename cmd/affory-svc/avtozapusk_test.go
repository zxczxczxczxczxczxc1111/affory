package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/netip"
	"testing"

	"golang.org/x/sys/windows/registry"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/set"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/sostoyanie"
)

// The Run key under HKCU\Software\Affory-test stands in for HKLM's. Same
// value shape, same code path, no admin needed, and it is deleted after.
func testovyyKlyuchRun(t *testing.T) {
	t.Helper()
	const put = `Software\Affory-test\Run`
	k, _, err := registry.CreateKey(registry.CURRENT_USER, put, registry.ALL_ACCESS)
	if err != nil {
		t.Fatalf("тестовый ключ не создан: %v", err)
	}
	k.Close()
	staryy := klyuchAvtozapuska
	klyuchAvtozapuska = func() (registry.Key, error) {
		return registry.OpenKey(registry.CURRENT_USER, put, registry.ALL_ACCESS)
	}
	t.Cleanup(func() {
		klyuchAvtozapuska = staryy
		_ = registry.DeleteKey(registry.CURRENT_USER, put)
		_ = registry.DeleteKey(registry.CURRENT_USER, `Software\Affory-test`)
	})
}

func TestAvtozapuskPishetsyaIChitaetsyaIzReestra(t *testing.T) {
	testovyyKlyuchRun(t)
	if avtozapuskVklyuchen() {
		t.Fatal("пустой ключ, а автозапуск уже включён: читаем не то")
	}
	if err := vklyuchitAvtozapusk(`C:\Program Files\Affory\affory-ui.exe`); err != nil {
		t.Fatal(err)
	}
	if !avtozapuskVklyuchen() {
		t.Fatal("после включения реестр говорит «выключен»")
	}
	// The value is the quoted path: a path with a space and no quotes starts
	// C:\Program.exe on the next logon, and Windows will not say a word.
	k, _ := klyuchAvtozapuska()
	v, _, err := k.GetStringValue(imyaZnacheniyaRun)
	k.Close()
	if err != nil {
		t.Fatal(err)
	}
	// Quoted path, then the tray flag: at logon the program comes up in the
	// tray and the window stays closed (spec, "Автозапуск"). Without the
	// flag every logon would throw the window in the human's face.
	if v != `"C:\Program Files\Affory\affory-ui.exe" --trey` {
		t.Fatalf("значение без кавычек, без --trey или не то: %q", v)
	}
	if err := vyklyuchitAvtozapusk(); err != nil {
		t.Fatal(err)
	}
	if avtozapuskVklyuchen() {
		t.Fatal("после выключения реестр говорит «включён»")
	}
	// Switching off twice is not an error: the human presses the toggle, not
	// a state machine, and the second press must not turn into a red screen.
	if err := vyklyuchitAvtozapusk(); err != nil {
		t.Fatalf("повторное выключение упало: %v", err)
	}
}

func TestSetAutostartCherezDispetcherIStatus(t *testing.T) {
	if _, est := pozzhe["setAutostart"]; est {
		t.Fatal("setAutostart всё ещё числится заглушкой")
	}
	testovyyKlyuchRun(t)
	s := podstavnaya(t, nil)

	o := s.Obrabotat(ctxAdmina(), protokol.Kadr{
		Tip: "cmd", Id: 1, Imya: "setAutostart", Telo: []byte(`{"vkl":true}`),
	})
	if o.Oshib != nil {
		t.Fatalf("включение отвергнуто: %s %s", o.Oshib.Kod, o.Oshib.Tekst)
	}
	if !s.Status().Avtozapusk {
		t.Fatal("статус не показывает включённый автозапуск")
	}
	o = s.Obrabotat(ctxAdmina(), protokol.Kadr{
		Tip: "cmd", Id: 2, Imya: "setAutostart", Telo: []byte(`{"vkl":false}`),
	})
	if o.Oshib != nil {
		t.Fatalf("выключение отвергнуто: %s %s", o.Oshib.Kod, o.Oshib.Tekst)
	}
	if s.Status().Avtozapusk {
		t.Fatal("статус показывает автозапуск после выключения")
	}
}

// Task 4.8: the second flag. It lives on disk (the service is what connects
// at boot, and it must know the answer before any window exists), survives
// the full-file reset that Disconnect does, and is read back at construction.
func TestPodklyuchatPriStarteZhivyotVFayle(t *testing.T) {
	s := podstavnaya(t, nil)
	var poslednyaya sostoyanie.SostoyanieFayla
	s.zapisat = func(f sostoyanie.SostoyanieFayla) error { poslednyaya = f; return nil }
	s.podnyatTunnel = func(ctx context.Context) (set.Adapter, error) {
		return set.Adapter{Indeks: 10, Imya: "tun0", Adresa: []netip.Addr{netip.MustParseAddr("172.19.0.1")}}, nil
	}

	if s.Status().PodklyuchatPriStarte {
		t.Fatal("умолчание должно быть «выключено» (§9.2)")
	}
	o := s.Obrabotat(ctxAdmina(), protokol.Kadr{Tip: "cmd", Id: 1, Imya: "setConnectOnStart", Telo: json.RawMessage(`{"vkl":true}`)})
	if o.Oshib != nil {
		t.Fatalf("setConnectOnStart отказала: %s %s", o.Oshib.Kod, o.Oshib.Tekst)
	}
	if !s.Status().PodklyuchatPriStarte {
		t.Fatal("после команды статус не показывает флаг")
	}
	if !poslednyaya.PodklyuchatPriStarte {
		t.Fatal("флаг не записан в файл состояния: после перезагрузки службе нечем его узнать")
	}

	// Disconnect rewrites the file from scratch; the flag is a setting, not
	// tunnel state, and must ride through like the subscription timestamp.
	if err := s.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.Disconnect()
	if !poslednyaya.PodklyuchatPriStarte {
		t.Fatal("Disconnect стёр флаг вместе с состоянием туннеля")
	}

	// Next service start: the flag comes from disk.
	s2 := podstavnaya(t, nil)
	s2.prochitat = func() (sostoyanie.SostoyanieFayla, error) { return poslednyaya, nil }
	s2.zagruzitNastroyki()
	if !s2.Status().PodklyuchatPriStarte {
		t.Fatal("после перезапуска флаг не прочитан с диска")
	}
}

func TestPodklyuchaetPriStarteTolkoPoFlagu(t *testing.T) {
	for _, vkl := range []bool{false, true} {
		s := podstavnaya(t, nil)
		podnimali := 0
		s.podnyatTunnel = func(ctx context.Context) (set.Adapter, error) {
			podnimali++
			return set.Adapter{Indeks: 10, Imya: "tun0", Adresa: []netip.Addr{netip.MustParseAddr("172.19.0.1")}}, nil
		}
		s.prochitat = func() (sostoyanie.SostoyanieFayla, error) {
			return sostoyanie.SostoyanieFayla{PodklyuchatPriStarte: vkl}, nil
		}
		s.zagruzitNastroyki()
		s.PodklyuchitPriStarte(context.Background())
		if vkl && podnimali != 1 {
			t.Fatalf("флаг включён, а подъёмов при старте %d", podnimali)
		}
		if !vkl && podnimali != 0 {
			t.Fatalf("флаг выключен, а туннель поднялся сам: это самый неприятный сюрприз у VPN")
		}
	}
}

// Ф1 от 05.09.2026. Жалоба владельца: убитый через «Снять задачу» клиент
// оставляет ПК без интернета. Корень не в замке, а в том, что вернуть службу
// некому, и восстановление SCM это только половина починки.
//
// Вторая половина здесь. Служба вернулась, замок помнит (PomnitZapertuyu), а
// туннель не поднимает: флаг «подключать при старте» выключен. Замерено в
// госте: машина осталась заперта с живой службой и без сети.
//
// Правило: замок ПОДРАЗУМЕВАЕТ туннель. Запертая машина без туннеля это машина
// без сети и без выхода, и подъём тут не сюрприз, а единственный способ вернуть
// человеку связь. Обратное правило (соседний тест) остаётся в силе для машины
// НЕЗАПЕРТОЙ: там самовольный подъём и правда самый неприятный сюрприз у VPN.
func TestZapertayaMashinaPodnimaetTunnelDazheBezFlaga(t *testing.T) {
	s := podstavnaya(t, nil)
	podnimali := 0
	s.podnyatTunnel = func(ctx context.Context) (set.Adapter, error) {
		podnimali++
		return set.Adapter{Indeks: 10, Imya: "tun0", Adresa: []netip.Addr{netip.MustParseAddr("172.19.0.1")}}, nil
	}
	s.prochitat = func() (sostoyanie.SostoyanieFayla, error) {
		return sostoyanie.SostoyanieFayla{PodklyuchatPriStarte: false}, nil
	}
	s.zagruzitNastroyki()
	// Ровно то, что делает main.go, увидев запертую машину в файле отката.
	s.PomnitZapertuyu(true)
	s.PodklyuchitPriStarte(context.Background())
	if podnimali != 1 {
		t.Fatalf("машина заперта, флага нет, подъёмов %d: человек остался без сети и без выхода", podnimali)
	}
}

// klyuchRunTolkoChtenie отдаёт ключ, который ЧИТАЕТСЯ, но не пишется.
//
// Настоящий отказ реестра, а не подставная функция: SetStringValue упирается в
// права на сам ключ ровно так же, как упёрлась бы на машине с запертым Run.
// Отдельной точки подмены для этого заводить не пришлось, klyuchAvtozapuska уже
// есть.
func klyuchRunTolkoChtenie(t *testing.T) {
	t.Helper()
	const put = `Software\Affory-test-ro\Run`
	k, _, err := registry.CreateKey(registry.CURRENT_USER, put, registry.ALL_ACCESS)
	if err != nil {
		t.Fatalf("тестовый ключ не создан: %v", err)
	}
	k.Close()
	staryy := klyuchAvtozapuska
	klyuchAvtozapuska = func() (registry.Key, error) {
		return registry.OpenKey(registry.CURRENT_USER, put, registry.QUERY_VALUE)
	}
	t.Cleanup(func() {
		klyuchAvtozapuska = staryy
		_ = registry.DeleteKey(registry.CURRENT_USER, put)
		_ = registry.DeleteKey(registry.CURRENT_USER, `Software\Affory-test-ro`)
	})
}

// Б4. Служба это LocalSystem, права вызывающего к записи HKLM отношения не
// имеют. Ответ «нужен администратор» отправляет человека нажимать кнопку
// «Повторить от администратора», которая ничего не исправит: повышение прав у
// окна не меняет прав службы, а пишет ключ именно служба.
func TestSetAutostartNeSsylaetsyaNaAdminaPriOshibkeReestra(t *testing.T) {
	klyuchRunTolkoChtenie(t)
	s := podstavnaya(t, nil)

	o := s.Obrabotat(ctxAdmina(), protokol.Kadr{
		Tip: "cmd", Id: 1, Imya: "setAutostart", Telo: []byte(`{"vkl":true}`)})
	if o.Oshib == nil {
		t.Fatal("запись в незаписываемый ключ обязана отказать")
	}
	if o.Oshib.Kod == protokol.KodTrebuetsyaAdmin {
		t.Errorf("код %q отправляет человека за правами, которых службе не нужно", o.Oshib.Kod)
	}
}

// Б4. Отказ записи файла состояния обязан доехать до человека.
//
// SetConnectOnStart проглатывала его в pravitSost (_ = s.zapisat(f)), команда
// отвечала успехом, экран рисовал галочку, а после перезагрузки туннель не
// поднимался: флага в файле не было.
func TestSetConnectOnStartNeVryotPriNeudachnoyZapisi(t *testing.T) {
	s := podstavnaya(t, nil)
	s.zapisat = func(sostoyanie.SostoyanieFayla) error { return errors.New("диск не пишется") }

	o := s.Obrabotat(ctxAdmina(), protokol.Kadr{
		Tip: "cmd", Id: 1, Imya: "setConnectOnStart", Telo: []byte(`{"vkl":true}`)})
	if o.Oshib == nil {
		t.Fatal("отказ записи файла состояния обязан доехать до вызывающего")
	}
}
