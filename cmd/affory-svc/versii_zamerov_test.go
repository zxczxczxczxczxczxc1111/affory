package main

import (
	"context"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

func TestVersiyaZameraMenyaetsyaTolkoPriIzmeneniiParametrov(t *testing.T) {
	s := &Sluzhba{}
	srv := protokol.Server{Id: "a", Host: "example.org", Port: 443, Parol: "first", Imya: "old"}
	first, err := s.versiiServerov([]protokol.Server{srv})
	if err != nil {
		t.Fatal(err)
	}
	srv.Imya = "renamed"
	next, err := s.versiiServerov([]protokol.Server{srv})
	if err != nil {
		t.Fatal(err)
	}
	if first["a"] != next["a"] {
		t.Fatal("переименование сбросило замер")
	}
	srv.Parol = "rotated"
	next, err = s.versiiServerov([]protokol.Server{srv})
	if err != nil {
		t.Fatal(err)
	}
	if first["a"] == next["a"] {
		t.Fatal("ротация ключа не поменяла версию")
	}
	other, err := (&Sluzhba{}).versiiServerov([]protokol.Server{srv})
	if err != nil {
		t.Fatal(err)
	}
	if other["a"] == next["a"] {
		t.Fatal("версия допускает проверку ключа в другой службе")
	}
}

func TestZamerNePripisyvaetNovymKlyuchamOtklikStarogoYadra(t *testing.T) {
	s := &Sluzhba{}
	srv := protokol.Server{Id: "a", Parol: "old"}
	s.zapomnitServeryYadra([]protokol.Server{srv})
	srv.Parol = "new"
	z := s.zamerOdnogo(context.Background(), srv, "http://127.0.0.1:9", "test")
	if z.RealpingMs != nil || z.RealpingOtkaz != "параметры сервера изменились: нужно переподключение" {
		t.Fatal("замер использовал старые ключи")
	}
}
