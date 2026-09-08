package main

import (
	"context"
	"net/netip"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/set"
)

// Находка 9. Адрес TUN брался как Adresa[0] без отбора IPv4.
//
// Windows вешает на интерфейс link-local fe80:: наравне с 172.19.0.1, а порядок
// выдачи GetAdaptersAddresses не гарантирован. Если первым приедет v6, правило
// станет localip=fe80::…, и весь IPv4 через туннель окажется запрещён политикой
// Block. В госте порядок оказался удачным, поэтому дефект и дожил.
func TestAdresTunBerotsyaIPv4APervyyVSpiske(t *testing.T) {
	s := podstavnaya(t, nil)
	tun := set.Adapter{
		Indeks: 10, Imya: "tun0",
		Adresa: []netip.Addr{
			netip.MustParseAddr("fe80::1"),
			netip.MustParseAddr("172.19.0.1"),
		},
	}

	r, err := s.spisokRazreshyonnogo(tun)
	if err != nil {
		t.Fatalf("список не собран: %v", err)
	}
	if !r.AdresTun.Is4() {
		t.Fatalf("адрес TUN %v это не IPv4: весь IPv4 будет запрещён политикой Block", r.AdresTun)
	}
	if r.AdresTun.String() != "172.19.0.1" {
		t.Fatalf("адрес TUN %v, ожидался 172.19.0.1", r.AdresTun)
	}
}

// Адаптер вообще без IPv4 это не «возьмём что дают», а отказ: правило по
// link-local молча запрещает весь трафик, и человек видит машину без сети без
// единой ошибки.
func TestAdapterBezIPv4EtoOtkaz(t *testing.T) {
	s := podstavnaya(t, nil)
	tun := set.Adapter{
		Indeks: 10, Imya: "tun0",
		Adresa: []netip.Addr{netip.MustParseAddr("fe80::1")},
	}

	if _, err := s.spisokRazreshyonnogo(tun); err == nil {
		t.Fatal("адаптер без IPv4 принят: правило ляжет по link-local и запретит всё")
	}
}

// Находка 8. Правило Affory-Allow-Tun не пересобиралось при переподъёме.
//
// PeresobratRazresheniya звалась только на изменение списка серверов. После
// автовосстановления или ручного disconnect/connect при включённом режиме
// правило продолжало разрешать localip СТАРОГО адаптера. Работало сегодня
// только потому, что sing-tun обычно выдаёт тот же 172.19.0.1.
func TestPodyomPriVklyuchennomRezhimePeresobiraetPravila(t *testing.T) {
	s := podstavnaya(t, nil)
	peresobrali := make(chan struct{}, 4)
	s.vklyuchitVes = func(set.Razreshyonnoe, bool) error {
		peresobrali <- struct{}{}
		return nil
	}
	s.mu.Lock()
	s.killSwitch = true
	s.mu.Unlock()

	if err := s.Connect(context.Background()); err != nil {
		t.Fatalf("подъём не удался: %v", err)
	}
	t.Cleanup(s.Otklyuchit)

	select {
	case <-peresobrali:
	default:
		t.Fatal("правила не пересобраны после подъёма: разрешён адрес прежнего адаптера")
	}
}

// КОНТРОЛЬ: без режима подъём правил НЕ трогает. Иначе обычное подключение
// начнёт лазить в брандмауэр без причины, а это самый дорогой путь в продукте.
func TestPodyomBezRezhimaPravilaNeTrogaet(t *testing.T) {
	s := podstavnaya(t, nil)
	trogali := false
	s.vklyuchitVes = func(set.Razreshyonnoe, bool) error { trogali = true; return nil }

	if err := s.Connect(context.Background()); err != nil {
		t.Fatalf("подъём не удался: %v", err)
	}
	t.Cleanup(s.Otklyuchit)

	if trogali {
		t.Fatal("обычный подъём полез в брандмауэр: режим выключен, трогать нечего")
	}
	if s.Status().Sostoyanie != protokol.SostPodnyat {
		t.Fatalf("состояние %s вместо podnyat", s.Status().Sostoyanie)
	}
}
