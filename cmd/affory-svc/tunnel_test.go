package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/set"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/yadra"
)

// ИНВАРИАНТ ПЕРЕВЁРНУТ ЗАДАЧЕЙ П3, и это не оплошность.
//
// Прежде туннель поднимался только ПОСЛЕ успешной пробы, и порядок защищал от
// ne-neset: машина не должна заворачивать трафик в ядро, которое никуда не
// дозвонилось. Защита была настоящей, но пробовать до подъёма умеет лишь путь
// мимо туннеля, а он меряет не то, чем трафик пойдёт: на чужом сервере такая
// проба отвечала успехом там, где не проходило ничего.
//
// Теперь замер идёт ПО живому туннелю, а от ne-neset защищает откат: провал
// замера туннель опускает. Тест проверяет обе половины нового инварианта.
func TestZamerIdyotPoPodnyatomuTunneluIEgoProvalTunnelOpuskaet(t *testing.T) {
	var tunnelByl, zamerBylPosle atomic.Bool
	s := podstavnaya(t, nil)
	s.podnyatTunnel = func(ctx context.Context) (set.Adapter, error) {
		s.zapomnitKlash(52715, "sekret-stenda")
		tunnelByl.Store(true)
		return set.Adapter{Indeks: 10, Imya: "tun0"}, nil
	}
	s.zamerit = func(ctx context.Context, adres, sekret, teg string) (time.Duration, error) {
		zamerBylPosle.Store(tunnelByl.Load())
		if adres == "" {
			t.Error("замер спрошен без адреса clash_api: подъём не запомнил доступ")
		}
		return 0, errors.New("исходящий не отвечает (код 504)")
	}
	staryy := zhdatPodyoma
	zhdatPodyoma = 900 * time.Millisecond
	defer func() { zhdatPodyoma = staryy }()

	if err := s.Connect(context.Background()); err == nil {
		t.Fatal("туннель, не понёсший трафик, признан поднятым")
	}
	if !zamerBylPosle.Load() {
		t.Fatal("замер спрошен раньше, чем поднялся туннель: меряется не тот путь")
	}
	if got := s.Status().Sostoyanie; got != protokol.SostNeNeset {
		t.Fatalf("состояние %s, ожидалось ne-neset", got)
	}
}

func TestOtkazTunnelyaEtoTunCreateFailed(t *testing.T) {
	s := podstavnaya(t, nil)
	s.podnyatTunnel = func(ctx context.Context) (set.Adapter, error) {
		return set.Adapter{}, errors.New("адаптер не появился")
	}

	err := s.Connect(context.Background())
	if err == nil {
		t.Fatal("connect объявил успех при неподнявшемся туннеле")
	}
	st := s.Status()
	if st.Sostoyanie != protokol.SostOtkaz {
		t.Fatalf("состояние %s, ожидался otkaz", st.Sostoyanie)
	}
	if st.Oshib == nil || st.Oshib.Kod != protokol.KodTunCreateFailed {
		t.Fatalf("код ошибки %v, ожидался %s", st.Oshib, protokol.KodTunCreateFailed)
	}
}

func TestDisconnectZabyvaetAdapter(t *testing.T) {
	// A stale interface index is worse than an empty one: firewall rules would
	// exclude an adapter that no longer exists, or worse, whoever took the freed
	// index next.
	s := podstavnaya(t, nil)
	if err := s.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s.tun.Indeks == 0 {
		t.Fatal("адаптер не запомнен после подъёма")
	}
	s.Disconnect()
	if s.tun.Indeks != 0 || s.tun.Imya != "" {
		t.Fatalf("после опускания адаптер остался: %+v", s.tun)
	}
}

func TestPraviloIPv6ZhivyotStolkoZheSkolkoTunnel(t *testing.T) {
	// Revision 5 hung this on the kill-switch, which is off by default: in the
	// mode the client actually lives in, IPv6 was blocked by nothing at all.
	var postavleno, snyato atomic.Bool
	s := podstavnaya(t, nil)
	s.glushitIPv6 = func() error { postavleno.Store(true); return nil }
	s.vernutIPv6 = func() error { snyato.Store(true); return nil }

	if err := s.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !postavleno.Load() {
		t.Fatal("правило IPv6 не поставлено при подъёме туннеля")
	}
	s.Disconnect()
	if !snyato.Load() {
		t.Fatal("правило IPv6 не снято при опускании")
	}
}

func TestOtkazPravilaIPv6RonyaetPodklyuchenie(t *testing.T) {
	// A tunnel up with IPv6 unblocked is a tunnel that leaks, and leaking
	// quietly is worse than not connecting loudly.
	s := podstavnaya(t, nil)
	s.glushitIPv6 = func() error { return errors.New("netsh отказал") }

	if err := s.Connect(context.Background()); err == nil {
		t.Fatal("подключение объявлено успешным при незаглушенном IPv6")
	}
	if st := s.Status(); st.Oshib == nil || st.Oshib.Kod != protokol.KodFirewallFailed {
		t.Fatalf("код ошибки %v, ожидался %s", st.Oshib, protokol.KodFirewallFailed)
	}
}

// Пропавший драйвер и занятое имя адаптера дают ОДИН таймаут ожидания, и до
// этой задачи оба уезжали как tun-create-failed. Человек с невставшим драйвером
// читал «не удалось создать адаптер» и шёл повторять, хотя повторять там нечего.
func TestPropavshiyDrayverNeNazyvaetsyaNesozdannymAdapterom(t *testing.T) {
	s := podstavnaya(t, nil)
	s.podnyatTunnel = func(ctx context.Context) (set.Adapter, error) {
		return set.Adapter{}, fmt.Errorf("%w: create service: wintun: %w",
			yadra.ErrDrayverNeVstal, set.ErrAdapterNePoyavilsya)
	}

	if err := s.Connect(context.Background()); err == nil {
		t.Fatal("connect объявил успех при неподнявшемся туннеле")
	}
	st := s.Status()
	if st.Oshib == nil || st.Oshib.Kod != protokol.KodWintunMissing {
		t.Fatalf("код ошибки %v, ожидался %s", st.Oshib, protokol.KodWintunMissing)
	}
}

// Признак ставится ТОЛЬКО по жалобе ядра. Пустая жалоба означает «виновник не
// установлен», а не «драйвера нет»: молчаливое обвинение здесь стоило бы
// человеку установки службы заново при исправном драйвере.
func TestOtkazOzhidaniyaAdapteraNazyvaetDrayverTolkoPoZhalobe(t *testing.T) {
	taymaut := fmt.Errorf("ожидание: %w", set.ErrAdapterNePoyavilsya)

	bezZhaloby := otkazOzhidaniyaAdaptera(taymaut, "")
	if errors.Is(bezZhaloby, yadra.ErrDrayverNeVstal) {
		t.Error("драйвер обвинён без единой жалобы ядра: это догадка, а не факт")
	}
	if !errors.Is(bezZhaloby, set.ErrAdapterNePoyavilsya) {
		t.Errorf("причина таймаута потеряна: %v", bezZhaloby)
	}

	sZhaloboy := otkazOzhidaniyaAdaptera(taymaut, "create service: wintun: Access is denied.")
	if !errors.Is(sZhaloboy, yadra.ErrDrayverNeVstal) {
		t.Errorf("жалоба ядра на драйвер не доехала до причины: %v", sZhaloboy)
	}
	// Текст жалобы обязан доехать до экрана: §9.1 обещает «плюс причина от
	// системы», а причина от системы это ровно то, что сказало ядро.
	if !strings.Contains(sZhaloboy.Error(), "Access is denied") {
		t.Errorf("жалоба ядра потеряна в тексте: %v", sZhaloboy)
	}
	if otkazOzhidaniyaAdaptera(nil, "жалоба была") != nil {
		t.Error("удачное ожидание превращено в отказ")
	}
}
