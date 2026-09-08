package yadra

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// Задача 6.1. Своих проб задержки не нужно: clash_api отдаёт историю
// urltest у исходящего и суммарные счётчики у /connections. Уровень журнала
// ядра при этом не трогается вовсе.

func TestStatistikaBerotPosledniyZamerISchetchiki(t *testing.T) {
	adres := podstavnoyKlash(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/proxies/"):
			if r.URL.Path != "/proxies/srv%20nl" && r.URL.Path != "/proxies/srv nl" {
				http.Error(w, "не тот тег: "+r.URL.Path, 404)
				return
			}
			// История это ПОСЛЕДНИЕ замеры в порядке времени, берётся хвост.
			fmt.Fprint(w, `{"type":"VLESS","name":"srv nl","history":[{"time":"t1","delay":300},{"time":"t2","delay":87}]}`)
		case r.URL.Path == "/connections":
			fmt.Fprint(w, `{"downloadTotal":123456,"uploadTotal":789,"connections":[]}`)
		default:
			http.Error(w, r.URL.Path, 404)
		}
	})
	s, err := Statistika(context.Background(), adres, "sekret", "srv nl")
	if err != nil {
		t.Fatal(err)
	}
	if s.Zaderzhka != 87*time.Millisecond || !s.EstZaderzhka {
		t.Fatalf("задержка %v (есть=%v), ожидалось 87 мс", s.Zaderzhka, s.EstZaderzhka)
	}
	if s.Prinyato != 123456 || s.Otdano != 789 {
		t.Fatalf("счётчики принято=%d отдано=%d", s.Prinyato, s.Otdano)
	}
}

func TestStatistikaBezIstoriiEtoNeNol(t *testing.T) {
	// Пустая история у только что поднятого ядра. Ноль вместо «не измерено»
	// нарушил бы договор запасного пути: экран не рисует ноль за неизмеренное.
	adres := podstavnoyKlash(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/proxies/") {
			fmt.Fprint(w, `{"type":"VLESS","name":"srv","history":[]}`)
			return
		}
		fmt.Fprint(w, `{"downloadTotal":0,"uploadTotal":0,"connections":[]}`)
	})
	s, err := Statistika(context.Background(), adres, "sekret", "srv")
	if err != nil {
		t.Fatal(err)
	}
	if s.EstZaderzhka {
		t.Fatalf("пустая история выдана за замер: %+v", s)
	}
}

func TestStatistikaSekretNePrinyatEtoOshibka(t *testing.T) {
	adres := podstavnoyKlash(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	if _, err := Statistika(context.Background(), adres, "sekret", "srv"); err == nil {
		t.Fatal("401 прошёл за успех")
	}
}

func TestStatistikaShlyotSekret(t *testing.T) {
	var zagolovok string
	adres := podstavnoyKlash(t, func(w http.ResponseWriter, r *http.Request) {
		zagolovok = r.Header.Get("Authorization")
		if strings.HasPrefix(r.URL.Path, "/proxies/") {
			fmt.Fprint(w, `{"history":[{"delay":5}]}`)
			return
		}
		fmt.Fprint(w, `{"downloadTotal":1,"uploadTotal":1}`)
	})
	if _, err := Statistika(context.Background(), adres, "moy-sekret", "srv"); err != nil {
		t.Fatal(err)
	}
	if zagolovok != "Bearer moy-sekret" {
		t.Fatalf("заголовок %q", zagolovok)
	}
}
