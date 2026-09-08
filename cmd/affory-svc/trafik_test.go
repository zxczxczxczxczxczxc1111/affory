package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"testing"
)

func TestStaleReconnectCannotResurrectExplicitDisconnect(t *testing.T) {
	s := podstavnaya(t, nil)
	if err := s.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	expected := s.pokolenieP + 1
	s.mu.Unlock()
	s.Disconnect()
	s.Otklyuchit()
	if err := s.connect(context.Background(), &expected); !errors.Is(err, context.Canceled) {
		t.Fatalf("stale reconnect: %v", err)
	}
	if s.Status().Sostoyanie != protokol.SostVyklyuchen {
		t.Fatal("VPN resurrected after explicit off")
	}
}

func TestModernRulesReconnectActiveTunnelAndKeepIdleIdle(t *testing.T) {
	s := podstavnaya(t, nil)
	p := []byte(`{"trafik":{"po_umolchaniyu":"direct","servisy":[{"id":"youtube","marshrut":"vpn"}]}}`)
	r := s.Obrabotat(ctxAdmina(), protokol.Kadr{Id: 1, Imya: "setRules", Telo: p})
	if r.Oshib != nil || s.Status().Sostoyanie != protokol.SostVyklyuchen {
		t.Fatalf("idle rules: %+v", r)
	}
	if err := s.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	generation := s.pokolenieP
	s.mu.Unlock()
	r = s.Obrabotat(ctxAdmina(), protokol.Kadr{Id: 2, Imya: "setRules", Telo: p})
	if r.Oshib != nil || s.Status().Sostoyanie != protokol.SostPodnyat {
		t.Fatalf("active rules: %+v", r)
	}
	s.mu.Lock()
	after := s.pokolenieP
	s.mu.Unlock()
	if after <= generation {
		t.Fatal("rules were saved but core was never restarted")
	}
}

func TestProtectionAndDirectPolicyConflictIsRejectedBeforeSaving(t *testing.T) {
	s := podstavnaya(t, nil)
	if err := s.SetKillSwitch(true); err != nil {
		t.Fatal(err)
	}
	r := s.Obrabotat(ctxAdmina(), protokol.Kadr{Id: 1, Imya: "setRules", Telo: []byte(`{"trafik":{"po_umolchaniyu":"direct"}}`)})
	if r.Oshib == nil {
		t.Fatal("saved a route that firewall would block")
	}
	n, _ := s.nabor()
	if n.Pravila.Trafik != nil {
		t.Fatal("conflict changed saved rules")
	}
}

func TestLegacyRulesKeepTheirDirectIntent(t *testing.T) {
	p := PravilaNabora{Protsessy: []string{`C:\Steam\steam.exe`}, Domeny: []string{"example.org"}}
	r := trafikPravil(p)
	if r.PoUmolchaniyu != protokol.TrafikVPN || len(r.Prilozheniya) != 1 || len(r.Domeny) != 1 {
		t.Fatalf("migration: %+v", r)
	}
	if r.Prilozheniya[0].Marshrut != protokol.TrafikPryamo || r.Domeny[0].Marshrut != protokol.TrafikPryamo {
		t.Fatal("legacy exceptions changed route")
	}
}

func TestChangingTrafficDefaultDoesNotInvertExplicitRules(t *testing.T) {
	s := podstavnaya(t, nil)
	for _, route := range []string{"vpn", "direct"} {
		payload := []byte(`{"trafik":{"po_umolchaniyu":"` + route + `","prilozheniya":[],"domeny":[{"domen":" Example.ORG. ","marshrut":"vpn"}],"servisy":[{"id":"youtube","marshrut":"vpn"}]}}`)
		result := s.Obrabotat(ctxAdmina(), protokol.Kadr{Id: 1, Imya: "setRules", Telo: payload})
		if result.Oshib != nil {
			t.Fatal(result.Oshib)
		}
		n, err := s.nabor()
		if err != nil {
			t.Fatal(err)
		}
		got := trafikPravil(n.Pravila)
		if got.PoUmolchaniyu != protokol.MarshrutTrafika(route) || len(got.Domeny) != 1 || got.Domeny[0].Domen != "example.org" || got.Domeny[0].Marshrut != protokol.TrafikVPN {
			t.Fatalf("explicit rule inverted: %+v", got)
		}
	}
}

func TestTrafficValidationRejectsUnknownRoutesAndServicesAtomically(t *testing.T) {
	s := podstavnaya(t, nil)
	for _, traffic := range []string{
		`{"po_umolchaniyu":"mystery"}`,
		`{"po_umolchaniyu":"vpn","servisy":[{"id":"unknown","marshrut":"vpn"}]}`,
		`{"po_umolchaniyu":"vpn","domeny":[{"domen":"https://example.org","marshrut":"vpn"}]}`,
		`{"po_umolchaniyu":"direct","domeny":[{"domen":"example.org","marshrut":"invalid"}]}`,
	} {
		result := s.Obrabotat(ctxAdmina(), protokol.Kadr{Id: 1, Imya: "setRules", Telo: []byte(`{"trafik":` + traffic + `}`)})
		if result.Oshib == nil {
			t.Errorf("accepted invalid traffic %s", traffic)
		}
	}
	n, _ := s.nabor()
	if n.Pravila.Trafik != nil {
		t.Fatal("invalid traffic was saved")
	}
}

func TestModernApplicationRulePersistsDescendantScope(t *testing.T) {
	s := podstavnaya(t, nil)
	p := protokol.PravilaTrafika{PoUmolchaniyu: protokol.TrafikVPN,
		Prilozheniya: []protokol.PraviloPrilozheniya{{Put: faylProby(t), Imya: "Steam", Potomki: true, Marshrut: protokol.TrafikPryamo}}}
	b, _ := json.Marshal(map[string]any{"trafik": p})
	r := s.Obrabotat(ctxAdmina(), protokol.Kadr{Id: 1, Imya: "setRules", Telo: b})
	if r.Oshib != nil {
		t.Fatal(r.Oshib)
	}
	n, _ := s.nabor()
	if n.Pravila.Trafik == nil || len(n.Pravila.Trafik.Prilozheniya) != 1 || !n.Pravila.Trafik.Prilozheniya[0].Potomki {
		t.Fatal("descendant scope was lost")
	}
}
