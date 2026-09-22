package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/yadra"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProverkaSoedineniyChitaetAPIYadra(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/connections" || r.Header.Get("Authorization") != "Bearer test-secret" {
			t.Error("incorrect core request")
			http.Error(w, "unauthorized", 401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"connections":[{"id":"one","metadata":{"host":"api.example.org","destinationIP":"203.0.113.9","destinationPort":"443","processPath":"C:\\App.exe"},"chains":["srv-test","vybor"],"rule":"domain_suffix=example.org","start":"2026-09-22T10:00:00Z"}]}`))
	}))
	defer api.Close()
	s := podstavnaya(t, nil)
	s.portClash = api.Listener.Addr().(*net.TCPAddr).Port
	s.sekretClash = "test-secret"
	s.soedineniyaYadra = yadra.Soedineniya
	o := s.Obrabotat(context.Background(), protokol.Kadr{Imya: "listConnections", Telo: json.RawMessage(`{"domen":"example.org"}`)})
	if o.Oshib != nil {
		t.Fatal(o.Oshib)
	}
	var got snimokSoedineniy
	if err := json.Unmarshal(o.Telo, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Soedineniya) != 1 || got.Soedineniya[0].Vyhod != "srv-test" || got.Soedineniya[0].Protsess != `C:\App.exe` {
		t.Fatalf("API data lost: %+v", got)
	}
	if protokol.TeloMozhnoLogirovat("listConnections") {
		t.Fatal("connection history can leak into frame log")
	}
}

func TestProverkaSoedineniyFiltruetPoGranitseDomenaIPuti(t *testing.T) {
	s, _, _ := sluzhbaSZhurnalomSoedineniy(t)
	s.soedineniyaYadra = func(context.Context, string, string) ([]yadra.Soedinenie, error) {
		return []yadra.Soedinenie{
			{Id: "parent", Host: "example.org", Protsess: `C:\App.exe`},
			{Id: "child", Host: "API.EXAMPLE.ORG.", Protsess: `C:\APP.EXE`},
			{Id: "lookalike", Host: "notexample.org", Protsess: `C:\App.exe`},
			{Id: "other", Host: "example.org", Protsess: `C:\Other.exe`},
		}, nil
	}
	o := s.Obrabotat(context.Background(), protokol.Kadr{Imya: "listConnections", Telo: json.RawMessage(`{"domen":"Example.org.","put":"C:\\app.exe"}`)})
	if o.Oshib != nil {
		t.Fatal(o.Oshib)
	}
	var got snimokSoedineniy
	if err := json.Unmarshal(o.Telo, &got); err != nil {
		t.Fatal(err)
	}
	if !got.Yadro || len(got.Soedineniya) != 2 || got.Soedineniya[1].Id != "child" {
		t.Fatalf("wrong snapshot: %+v", got)
	}
	if s.Status().Zhurnal {
		t.Fatal("inspection enabled history")
	}
}

func TestProverkaSoedineniyRazlichaetPustotuOshibkuISmenuYadra(t *testing.T) {
	for _, kind := range []string{"offline", "empty", "error", "restart", "limit"} {
		t.Run(kind, func(t *testing.T) {
			s, _, _ := sluzhbaSZhurnalomSoedineniy(t)
			if kind == "offline" {
				s.portClash = 0
			}
			s.soedineniyaYadra = func(ctx context.Context, _ string, _ string) ([]yadra.Soedinenie, error) {
				if _, ok := ctx.Deadline(); !ok {
					t.Error("missing timeout")
				}
				if kind == "offline" {
					t.Fatal("offline snapshot queried core")
				}
				if kind == "error" {
					return nil, errors.New("test error")
				}
				if kind == "restart" {
					s.mu.Lock()
					s.sekretClash = "new"
					s.mu.Unlock()
				}
				if kind == "limit" {
					return make([]yadra.Soedinenie, 201), nil
				}
				return nil, nil
			}
			o := s.Obrabotat(context.Background(), protokol.Kadr{Imya: "listConnections", Telo: json.RawMessage(`{}`)})
			if kind == "error" || kind == "restart" {
				if o.Oshib == nil {
					t.Fatal("failure presented as empty")
				}
				return
			}
			if o.Oshib != nil {
				t.Fatal(o.Oshib)
			}
			var got snimokSoedineniy
			if err := json.Unmarshal(o.Telo, &got); err != nil {
				t.Fatal(err)
			}
			if got.Yadro != (kind != "offline") || got.Soedineniya == nil {
				t.Fatalf("wrong snapshot: %+v", got)
			}
			if kind == "limit" && (!got.Ogranichen || len(got.Soedineniya) != 200) {
				t.Fatal("snapshot not bounded")
			}
		})
	}
}
