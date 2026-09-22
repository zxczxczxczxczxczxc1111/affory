package main

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/ssylki"
)

func TestParallelRefreshSohranyaetNoveyshiyOtvet(t *testing.T) {
	s := podstavnaya(t, nil)
	hranilishcheProby(t, s, Nabor{Aktivnaya: "a", Podpiski: []ZapisPodpiski{{Id: "a", Adres: "https://a.example/sub"}}})
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	s.zagruzitPodpisku = func(ctx context.Context, _ string) (ssylki.Razbor, error) {
		srv := serverProby()
		srv.Imya = "new"
		if calls.Add(1) == 1 {
			close(started)
			select {
			case <-release:
			case <-ctx.Done():
				return ssylki.Razbor{}, ctx.Err()
			}
			srv.Imya = "old"
		}
		return ssylki.Razbor{Servery: []protokol.Server{srv}}, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, _, err := s.obnovitPodpiskuPoId(ctx, "a"); done <- err }()
	<-started
	if _, _, err := s.obnovitPodpiskuPoId(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; !errors.Is(err, errObnovlenieZameneno) {
		t.Fatalf("late response: %v", err)
	}
	n, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}
	if len(n.Servery) != 1 || n.Servery[0].Imya != "new" {
		t.Fatal("parallel old response overwrote new data")
	}
}

func TestOstanovkaSluzhbyOtmenyaetObnovlenie(t *testing.T) {
	s := podstavnaya(t, nil)
	hranilishcheProby(t, s, Nabor{Aktivnaya: "a", Podpiski: []ZapisPodpiski{{Id: "a", Adres: "https://a.example/sub"}}})
	s.zagruzitPodpisku = func(context.Context, string) (ssylki.Razbor, error) {
		s.Zavershit()
		return ssylki.Razbor{Servery: []protokol.Server{serverProby()}}, nil
	}
	if _, _, err := s.obnovitPodpiskuPoId(context.Background(), "a"); !errors.Is(err, context.Canceled) {
		t.Fatalf("shutdown: %v", err)
	}
	n, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}
	if len(n.Servery) != 0 || n.Podpiski[0].Obnovlena != nil {
		t.Fatal("shutdown allowed a late commit")
	}
}

func TestPozdniyOtvetPodpiskiNeMenyaetNovoePokolenie(t *testing.T) {
	for _, oldFailure := range []bool{false, true} {
		t.Run(map[bool]string{false: "late-success", true: "late-error"}[oldFailure], func(t *testing.T) {
			s := podstavnaya(t, nil)
			hranilishcheProby(t, s, Nabor{Aktivnaya: "a", Podpiski: []ZapisPodpiski{{Id: "a", Adres: "https://a.example/sub"}}})
			calls := 0
			s.zagruzitPodpisku = func(ctx context.Context, _ string) (ssylki.Razbor, error) {
				calls++
				srv := serverProby()
				srv.IzPodpiski = true
				if calls == 1 {
					if _, _, err := s.obnovitPodpiskuPoId(ctx, "a"); err != nil {
						t.Fatal(err)
					}
					if oldFailure {
						return ssylki.Razbor{}, errors.New("old TLS timeout")
					}
					srv.Imya = "old"
				} else {
					srv.Imya = "new"
				}
				return ssylki.Razbor{Servery: []protokol.Server{srv}}, nil
			}
			r, _, err := s.obnovitPodpiskuPoId(context.Background(), "a")
			if !errors.Is(err, errObnovlenieZameneno) {
				t.Fatalf("late request: %v", err)
			}
			if frame := otkazPodpiski(protokol.Kadr{}, r, err); frame.Oshib != nil {
				t.Fatal("stale result became a visible failure")
			}
			n, err := s.nabor()
			if err != nil {
				t.Fatal(err)
			}
			if len(n.Servery) != 1 || n.Servery[0].Imya != "new" || n.Podpiski[0].Otkaz != "" {
				t.Fatal("late result changed current subscription")
			}
			if len(s.obnovleniyaPodpisok) != 0 {
				t.Fatal("finished generations retained")
			}
		})
	}
}

func TestOtmenyonnoeObnovlenieNePrimenyayetOtvet(t *testing.T) {
	s := podstavnaya(t, nil)
	hranilishcheProby(t, s, Nabor{Aktivnaya: "a", Podpiski: []ZapisPodpiski{{Id: "a", Adres: "https://a.example/sub"}}, Servery: []protokol.Server{serverProby()}})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.zagruzitPodpisku = func(context.Context, string) (ssylki.Razbor, error) {
		cancel()
		return ssylki.Razbor{Servery: []protokol.Server{vtoroyServer()}}, nil
	}
	if _, _, err := s.obnovitPodpiskuPoId(ctx, "a"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled response: %v", err)
	}
	n, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}
	if len(n.Servery) != 1 || n.Servery[0].Id != "nl" || n.Podpiski[0].Obnovlena != nil {
		t.Fatal("cancelled result saved")
	}
}

func TestUdalyonnayaIPovtornoDobavlennayaPodpiskaNePrinimayetStaryyOtvet(t *testing.T) {
	s := podstavnaya(t, nil)
	hranilishcheProby(t, s, Nabor{Aktivnaya: "a", Podpiski: []ZapisPodpiski{{Id: "a", Adres: "https://a.example/sub"}}})
	s.zagruzitPodpisku = func(context.Context, string) (ssylki.Razbor, error) {
		if r := s.removeSubscription(protokol.Kadr{Telo: []byte(`{"id":"a"}`)}); r.Oshib != nil {
			t.Fatal(r.Oshib)
		}
		if err := s.pravitNabor(func(n *Nabor) error {
			n.Aktivnaya = "a"
			n.Podpiski = []ZapisPodpiski{{Id: "a", Adres: "https://a.example/sub"}}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		return ssylki.Razbor{Servery: []protokol.Server{serverProby()}}, nil
	}
	if _, _, err := s.obnovitPodpiskuPoId(context.Background(), "a"); !errors.Is(err, errObnovlenieZameneno) {
		t.Fatalf("old incarnation: %v", err)
	}
	n, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}
	if len(n.Servery) != 0 || n.Podpiski[0].Obnovlena != nil {
		t.Fatal("old incarnation populated new subscription")
	}
}
