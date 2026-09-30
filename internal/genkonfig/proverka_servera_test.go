package genkonfig

import (
	"net/netip"
	"slices"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Б1 аудита 1.8.0: сервер проверяется самим ядром до подъёма TUN. Конфиг
// проверки тот же, что боевой, без входов и без кэша выбора.

func TestKonfigProverkiBezVhodovIKesha(t *testing.T) {
	v := vhodSNaborami()
	v.BezTun = true
	k := sobrat(t, v)
	if vhody, _ := k["inbounds"].([]any); len(vhody) != 0 {
		t.Fatalf("в конфиге проверки входы %v: TUN поднимется и отнимет сеть", vhody)
	}
	if e, _ := k["experimental"].(map[string]any); e["cache_file"] != nil {
		t.Fatal("в конфиге проверки кэш: он делит файл с боевым ядром и навязывает старый выбор")
	}
}

func TestBoevoyKonfigSTunom(t *testing.T) {
	k := sobrat(t, obraztsovyyVhod())
	vhody, _ := k["inbounds"].([]any)
	if len(vhody) == 0 || vhody[0].(map[string]any)["type"] != "tun" {
		t.Fatalf("боевой конфиг без TUN: %v", vhody)
	}
}

func vtorayaNidrlandam() protokol.Server {
	srv := obraztsovyyVhod().Server
	srv.Id, srv.Host = "de", "203.0.113.7"
	return srv
}

func TestTegiProverkiRuchnoy(t *testing.T) {
	v := obraztsovyyVhod()
	v.Servery = []protokol.Server{v.Server, vtorayaNidrlandam()}
	telo, err := SingBox(v)
	if err != nil {
		t.Fatal(err)
	}
	tegi, err := TegiProverki(telo)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(tegi, []string{TegKandidata("nl")}) {
		t.Fatalf("в ручном режиме меряется %v, а трафик пойдёт через выбранный nl", tegi)
	}
}

func TestTegiProverkiAvto(t *testing.T) {
	v := obraztsovyyVhod()
	v.Servery = []protokol.Server{v.Server, vtorayaNidrlandam()}
	v.Kandidaty = append(v.Kandidaty, netip.MustParseAddr("203.0.113.7"))
	v.Rezhim = protokol.RezhimAvto
	v.VneAvto = []string{"de"}
	telo, err := SingBox(v)
	if err != nil {
		t.Fatal(err)
	}
	tegi, err := TegiProverki(telo)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(tegi, []string{TegKandidata("nl")}) {
		t.Fatalf("в авто меряется %v, а группа состоит из nl: убранный из авто сервер трафик не понесёт", tegi)
	}
}
