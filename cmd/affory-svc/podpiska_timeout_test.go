package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/ssylki"
)

func TestRuchnoeObnovlenieOgranichenoDoTaymautaKanala(t *testing.T) {
	s := podstavnaya(t, nil)
	hranilishcheProby(t, s, Nabor{Aktivnaya: "a", Podpiski: []ZapisPodpiski{{Id: "a", Adres: "https://a.example/sub"}}, Servery: []protokol.Server{serverProby()}})
	s.zagruzitPodpisku = func(ctx context.Context, _ string) (ssylki.Razbor, error) {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 25*time.Second {
			t.Error("нет короткого срока обновления")
		}
		return ssylki.Razbor{}, errors.New("TLS handshake timeout")
	}
	r := s.refreshSubscription(context.Background(), protokol.Kadr{})
	if r.Oshib == nil {
		t.Fatal("ошибка обновления скрыта")
	}
	n, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}
	if len(n.Servery) != 1 || n.Podpiski[0].Otkaz == "" {
		t.Fatal("отказ потерял серверы или причину")
	}
	if srokRuchnogoObnovleniya >= protokol.SrokOtveta("refreshSubscription") {
		t.Fatal("канал прервёт обновление раньше службы")
	}
}

func TestUspeshnoeObnovlenieSbrositTolkoSvoyOtkaz(t *testing.T) {
	for _, id := range []string{"active", "reserve"} {
		t.Run(id, func(t *testing.T) {
			s := podstavnaya(t, nil)
			hranilishcheProby(t, s, Nabor{Aktivnaya: "active", Podpiski: []ZapisPodpiski{
				{Id: "active", Adres: "https://active.example/sub", Otkaz: "ошибка другого источника"},
				{Id: "reserve", Adres: "https://reserve.example/sub", Otkaz: "ошибка другого источника"},
			}, Servery: []protokol.Server{serverProby()}})
			s.zagruzitPodpisku = func(context.Context, string) (ssylki.Razbor, error) {
				return ssylki.Razbor{}, errors.New("TLS handshake timeout")
			}
			if _, _, err := s.obnovitPodpiskuPoId(context.Background(), id); err == nil {
				t.Fatal("ожидался сетевой отказ")
			}
			before, err := s.nabor()
			if err != nil {
				t.Fatal(err)
			}
			if before.zapisPodpiski(id).Otkaz == "" {
				t.Fatal("ошибка не сохранена")
			}
			s.zagruzitPodpisku = func(context.Context, string) (ssylki.Razbor, error) {
				srv := serverProby()
				srv.IzPodpiski = true
				return ssylki.Razbor{Servery: []protokol.Server{srv}}, nil
			}
			if _, _, err := s.obnovitPodpiskuPoId(context.Background(), id); err != nil {
				t.Fatal(err)
			}
			after, err := s.nabor()
			if err != nil {
				t.Fatal(err)
			}
			for _, p := range after.Podpiski {
				if p.Id == id && (p.Otkaz != "" || p.Obnovlena == nil) {
					t.Fatalf("успех оставил ошибку: %+v", p)
				}
				if p.Id != id && p.Otkaz != "ошибка другого источника" {
					t.Fatal("сброшена ошибка другой подписки")
				}
			}
		})
	}
}
