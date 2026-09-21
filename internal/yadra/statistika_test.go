package yadra

import (
	"context"
	"fmt"
	"net/http"
	"testing"
)

// Задача 6.1. Счётчики экрана берутся у clash_api из /connections. Задержки
// здесь больше нет: круг через туннель меряет служба своим запросом
// (set.Otklik), потому что число ядра это весь путь вместе с рукопожатием.
// Уровень журнала ядра при этом не трогается вовсе.

func TestStatistikaBerotSchetchiki(t *testing.T) {
	adres := podstavnoyKlash(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/connections" {
			http.Error(w, r.URL.Path, 404)
			return
		}
		fmt.Fprint(w, `{"downloadTotal":123456,"uploadTotal":789,"connections":[]}`)
	})
	s, err := Statistika(context.Background(), adres, "sekret")
	if err != nil {
		t.Fatal(err)
	}
	if s.Prinyato != 123456 || s.Otdano != 789 {
		t.Fatalf("счётчики принято=%d отдано=%d", s.Prinyato, s.Otdano)
	}
}

// Про исходящие снимок не спрашивает вовсе. Лишний запрос раз в секунду был
// нужен ровно ради задержки, и вместе с ней ушёл.
func TestStatistikaNeHoditZaIshodyashchimi(t *testing.T) {
	var puti []string
	adres := podstavnoyKlash(t, func(w http.ResponseWriter, r *http.Request) {
		puti = append(puti, r.URL.Path)
		fmt.Fprint(w, `{"downloadTotal":1,"uploadTotal":1}`)
	})
	if _, err := Statistika(context.Background(), adres, "sekret"); err != nil {
		t.Fatal(err)
	}
	if len(puti) != 1 || puti[0] != "/connections" {
		t.Fatalf("снимок сходил по %v", puti)
	}
}

func TestStatistikaSekretNePrinyatEtoOshibka(t *testing.T) {
	adres := podstavnoyKlash(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	if _, err := Statistika(context.Background(), adres, "sekret"); err == nil {
		t.Fatal("401 прошёл за успех")
	}
}

func TestStatistikaShlyotSekret(t *testing.T) {
	var zagolovok string
	adres := podstavnoyKlash(t, func(w http.ResponseWriter, r *http.Request) {
		zagolovok = r.Header.Get("Authorization")
		fmt.Fprint(w, `{"downloadTotal":1,"uploadTotal":1}`)
	})
	if _, err := Statistika(context.Background(), adres, "moy-sekret"); err != nil {
		t.Fatal(err)
	}
	if zagolovok != "Bearer moy-sekret" {
		t.Fatalf("заголовок %q", zagolovok)
	}
}
