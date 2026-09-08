package main

import (
	"context"
	"fmt"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/set"
)

// set.ErrBrandmauerVyklyuchen существует с самого начала, но errors.Is с ней не
// вызывается нигде: человек читает «не удалось поставить защиту» и идёт
// повторять команду, которая при выключенном профиле не сработает никогда.
//
// Экран для этого случая написан: «режим весь трафик недоступен: брандмауэр
// выключен», действие nichego. Показать его было нечем.
func TestVyklyuchennyyBrandmauerNazyvayetsyaSvoimKodom(t *testing.T) {
	s, _, _ := sKillSwitch(t)
	s.vklyuchitVes = func(set.Razreshyonnoe, bool) error {
		return fmt.Errorf("%w: профиль Domain", set.ErrBrandmauerVyklyuchen)
	}
	if err := s.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}

	o := s.Obrabotat(ctxAdmina(), protokol.Kadr{
		Tip: "cmd", Id: 1, Imya: "setKillSwitch", Telo: []byte(`{"vkl":true}`),
	})
	if o.Oshib == nil {
		t.Fatal("режим включился при выключенном брандмауэре")
	}
	if kod := o.Oshib.Kod; kod != protokol.KodFirewallDisabled {
		t.Errorf("код отказа %q, ожидали firewall-disabled", kod)
	}
	// Состояние обязано говорить то же самое: баннер Главной живёт дольше
	// ответа команды.
	if st := s.Status(); st.Oshib == nil || st.Oshib.Kod != protokol.KodFirewallDisabled {
		t.Errorf("код в состоянии %v, ожидали firewall-disabled", st.Oshib)
	}
}

// Второй путь той же ошибки: пересборка правил после записи набора. Она
// оборачивается в errPravilaOtstali, и до этой задачи причина в обёртке
// терялась вместе с признаком.
func TestVyklyuchennyyBrandmauerVidenIPriZapisiNabora(t *testing.T) {
	s, _, _ := sKillSwitch(t)
	if err := s.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.SetKillSwitch(true); err != nil {
		t.Fatal(err)
	}
	// Профиль выключили уже ПОСЛЕ включения режима: правила пересобрать нечем.
	s.vklyuchitVes = func(set.Razreshyonnoe, bool) error {
		return fmt.Errorf("%w: профиль Domain", set.ErrBrandmauerVyklyuchen)
	}

	o := s.Obrabotat(ctxAdmina(), protokol.Kadr{
		Tip: "cmd", Id: 2, Imya: "addServer",
		Telo: []byte(`{"ssylka":"hy2://11111111-2222-3333-4444-555555555555@198.51.100.7:443?sni=www.example.com#DE"}`),
	})
	if o.Oshib == nil {
		t.Fatal("запись набора прошла без единого слова об отставших правилах")
	}
	if kod := o.Oshib.Kod; kod != protokol.KodFirewallDisabled {
		t.Errorf("код отказа %q, ожидали firewall-disabled", kod)
	}
}

// Обратная сторона обоих: отказ netsh по ЛЮБОЙ другой причине остаётся
// firewall-failed. Без этого судьи починка назвала бы выключенным брандмауэром
// каждый сбой правила.
func TestObychnyyOtkazNetshOstayotsyaFirewallFailed(t *testing.T) {
	s, _, _ := sKillSwitch(t)
	s.vklyuchitVes = func(set.Razreshyonnoe, bool) error {
		return fmt.Errorf("правило %s не заведено: netsh вернул 1", set.PravAllowTun)
	}
	if err := s.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	o := s.Obrabotat(ctxAdmina(), protokol.Kadr{
		Tip: "cmd", Id: 1, Imya: "setKillSwitch", Telo: []byte(`{"vkl":true}`),
	})
	if o.Oshib == nil || o.Oshib.Kod != protokol.KodFirewallFailed {
		t.Fatalf("код %v, ожидался firewall-failed", o.Oshib)
	}
}
