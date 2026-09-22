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
