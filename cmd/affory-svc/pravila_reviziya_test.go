package main

import (
	"encoding/json"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"testing"
)

func TestStaryyChernovikNePerepishetNovyePravila(t *testing.T) {
	s := podstavnaya(t, nil)
	initial := s.listRules(protokol.Kadr{})
	var snapshot struct {
		Reviziya string `json:"reviziya_pravil"`
	}
	if err := json.Unmarshal(initial.Telo, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Reviziya == "" {
		t.Fatal("no revision in listRules")
	}
	newBody := []byte(`{"trafik":{"po_umolchaniyu":"direct","domeny":[{"domen":"new.example","marshrut":"vpn"}]}}`)
	if r := s.setRules(ctxAdmina(), protokol.Kadr{Telo: newBody}); r.Oshib != nil {
		t.Fatal(r.Oshib)
	}
	before, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}
	stale, err := json.Marshal(map[string]any{"reviziya_pravil": snapshot.Reviziya, "trafik": protokol.PravilaTrafika{PoUmolchaniyu: protokol.TrafikVPN}})
	if err != nil {
		t.Fatal(err)
	}
	if r := s.setRules(ctxAdmina(), protokol.Kadr{Telo: stale}); r.Oshib == nil || r.Oshib.Kod != protokol.KodPraviloNegodno {
		t.Fatal("stale draft accepted")
	}
	after, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}
	if otpechatokPravil(after.Pravila) != otpechatokPravil(before.Pravila) {
		t.Fatal("stale draft changed saved rules")
	}
	current, err := json.Marshal(map[string]any{"reviziya_pravil": reviziyaPravil(after.Pravila), "trafik": protokol.PravilaTrafika{PoUmolchaniyu: protokol.TrafikVPN}})
	if err != nil {
		t.Fatal(err)
	}
	if r := s.setRules(ctxAdmina(), protokol.Kadr{Telo: current}); r.Oshib != nil {
		t.Fatal(r.Oshib)
	}
}
