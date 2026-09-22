package main

import (
	"context"
	"encoding/json"
	"net/netip"
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/set"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/ssylki"
)

func TestKatalogPereklyuchenieNeSmeshivaetPodpiski(t *testing.T) {
	for _, online := range []bool{false, true} {
		n := Nabor{Aktivnaya: "A", Podpiski: []ZapisPodpiski{{Id: "A"}, {Id: "B", Servery: []protokol.Server{{Id: "b", IzPodpiski: true}}}}, Servery: []protokol.Server{{Id: "a", IzPodpiski: true}, {Id: "manual"}, {Id: "obsolete", Uderzhan: true}}}
		n.PereklyuchitAktivnuyu("B", online)
		n.PereklyuchitAktivnuyu("A", online)
		if len(n.Servery) != 2 || len(n.zapisPodpiski("B").Servery) != 1 || n.zapisPodpiski("B").Servery[0].Id != "b" {
			t.Fatalf("смешались источники, online=%v", online)
		}
		if n.Servery[0].Id != "a" || !n.Servery[0].IzPodpiski || n.Servery[0].Uderzhan {
			t.Fatal("актуальный сервер не вернулся")
		}
	}
}

func TestKatalogUdalenieAktivnoyNeOstavlyaetStaryeServery(t *testing.T) {
	n := Nabor{Aktivnaya: "A", Podpiski: []ZapisPodpiski{{Id: "A"}, {Id: "B", Servery: []protokol.Server{{Id: "b", IzPodpiski: true}}}}, Servery: []protokol.Server{{Id: "a", IzPodpiski: true}, {Id: "manual"}}}
	n.UbratPodpisku("A")
	if n.Aktivnaya != "B" || len(n.Servery) != 2 || n.Servery[0].Id != "b" {
		t.Fatal("не подставлены серверы оставшейся подписки")
	}
	n.UbratPodpisku("B")
	if len(n.Servery) != 1 || n.Servery[0].Id != "manual" {
		t.Fatal("удаление подписки затронуло ручной сервер")
	}
}

func TestKatalogOtvetSetiNePopadaetVDruguyuPodpisku(t *testing.T) {
	s := podstavnaya(t, nil)
	n := Nabor{Aktivnaya: "A", Podpiski: []ZapisPodpiski{{Id: "A", Adres: "https://a.example/sub"}, {Id: "B", Adres: "https://b.example/sub", Servery: []protokol.Server{{Id: "b", IzPodpiski: true}}}}, Servery: []protokol.Server{{Id: "a", IzPodpiski: true}}}
	s.nabor = func() (Nabor, error) { return n, nil }
	s.sekretyPisat = func(v []byte) error { return json.Unmarshal(v, &n) }
	s.zagruzitPodpisku = func(context.Context, string) (ssylki.Razbor, error) {
		n.PereklyuchitAktivnuyu("B", false)
		return ssylki.Razbor{Servery: []protokol.Server{{Id: "new-a", IzPodpiski: true}}}, nil
	}
	if _, _, err := s.obnovitPodpisku(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n.Servery[0].Id != "b" || n.zapisPodpiski("A").Servery[0].Id != "new-a" {
		t.Fatal("ответ A переписал каталог B")
	}
}

func TestKatalogSpisokPodpisokNeOtdayotKlyuchi(t *testing.T) {
	s := podstavnaya(t, nil)
	n := Nabor{Aktivnaya: "A", Podpiski: []ZapisPodpiski{{Id: "A", Adres: "https://a.example/SECRET-URL"}, {Id: "B", Adres: "https://b.example/SECRET-URL", Servery: []protokol.Server{{Id: "b", Uuid: "SECRET-UUID", Parol: "SECRET-PASS", Imya: "B"}}}}, Servery: []protokol.Server{{Id: "a", Uuid: "SECRET-UUID", IzPodpiski: true}}}
	s.nabor = func() (Nabor, error) { return n, nil }
	r := s.listSubscriptions(protokol.Kadr{})
	if strings.Contains(string(r.Telo), "SECRET") {
		t.Fatal("секрет попал в список подписок")
	}
	if !strings.Contains(string(r.Telo), `"servery"`) {
		t.Fatal("нет строк для групп")
	}
}

func TestKatalogChitaetStaruyuIstoriyuBezSohraneniya(t *testing.T) {
	var n Nabor
	if err := json.Unmarshal([]byte(`{"servery":[{"id":"a","prezhnie_klyuchi":{"uuid":"OLD-SECRET"}},{"id":"old","uderzhan":true}]}`), &n); err != nil {
		t.Fatal(err)
	}
	n.Servery = aktualnyeServery(n.Servery)
	data, err := json.Marshal(n)
	if err != nil {
		t.Fatal(err)
	}
	if len(n.Servery) != 1 || strings.Contains(string(data), "OLD-SECRET") {
		t.Fatal("история пережила нормализацию")
	}
}

func TestKatalogNovyeKlyuchiTrebuyutPeresborki(t *testing.T) {
	s := &Sluzhba{portClash: 9090}
	srv := protokol.Server{Id: "same-id", Host: "203.0.113.1", Port: 443, Parol: "old-key", IzPodpiski: true}
	s.zapomnitServeryYadra([]protokol.Server{srv})
	s.nabor = func() (Nabor, error) { return Nabor{Servery: []protokol.Server{srv}}, nil }
	changed, err := s.serverTrebuetPodyoma(srv.Id)
	if err != nil || changed {
		t.Fatal("неизменённый профиль требует подъёма")
	}
	srv.Parol = "new-key"
	changed, err = s.serverTrebuetPodyoma(srv.Id)
	if err != nil || !changed {
		t.Fatal("новые ключи отправлены в старый тег ядра")
	}
}

func TestKatalogConnectPrimenyayetNovyeKlyuchi(t *testing.T) {
	s := podstavnaya(t, nil)
	srv := serverProby()
	hranilishcheProby(t, s, Nabor{Servery: []protokol.Server{srv}})
	start := s.podnyatTunnel
	podemy := 0
	s.podnyatTunnel = func(ctx context.Context) (set.Adapter, error) {
		podemy++
		n, err := s.nabor()
		if err != nil {
			return set.Adapter{}, err
		}
		s.zapomnitServeryYadra(n.Servery)
		return start(ctx)
	}
	if r := vypolnit(t, s, "connect", map[string]string{"server": srv.Id}); r.Oshib != nil {
		t.Fatal(r.Oshib)
	}
	if err := s.pravitNabor(func(n *Nabor) error { n.Servery[0].Uuid = "rotated-key"; return nil }); err != nil {
		t.Fatal(err)
	}
	if r := vypolnit(t, s, "connect", map[string]string{"server": srv.Id}); r.Oshib != nil {
		t.Fatal(r.Oshib)
	}
	if podemy != 2 {
		t.Fatalf("ядро не пересобрано: подъёмов %d", podemy)
	}
}

func TestKatalogAdresYadraOstaetsyaVRazresheniyahPosleUdalenia(t *testing.T) {
	s := podstavnaya(t, nil)
	hranilishcheProby(t, s, Nabor{})
	s.portClash = 9090
	s.zapomnitServeryYadra([]protokol.Server{{Id: "removed", Host: "203.0.113.9", Port: 443}})
	s.sobratAdresaSet = func(v []protokol.Server, _ string, _ ...string) ([]netip.Addr, error) {
		if len(v) != 1 || v[0].Host != "203.0.113.9" {
			t.Fatal("адрес ядра пропал из разрешений")
		}
		return []netip.Addr{netip.MustParseAddr(v[0].Host)}, nil
	}
	if _, err := s.adresaKandidatov(); err != nil {
		t.Fatal(err)
	}
	s.opustitYadro()
	if _, err := s.adresaKandidatov(); err != ErrNetServerov {
		t.Fatalf("адрес пережил остановку ядра: %v", err)
	}
}
