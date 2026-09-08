package set

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// Задача 6.3, checkExitIp. Собственные процессы службы по правилу петли идут
// МИМО туннеля, поэтому запрос «какой у меня адрес» из службы напрямую
// показал бы домашний адрес при любом состоянии туннеля. Через локальный
// прокси sing-box запрос идёт тем же путём, что и трафик приложений.

func TestAdresVyhodaIdyotCherezProksi(t *testing.T) {
	var videlProksi string
	proksi := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Прокси получает АБСОЛЮТНЫЙ адрес в строке запроса: так его и узнаём.
		videlProksi = r.RequestURI
		fmt.Fprint(w, " 203.0.113.7\n")
	}))
	defer proksi.Close()
	port, _ := strconv.Atoi(strings.TrimPrefix(proksi.URL, "http://127.0.0.1:"))
	adres, err := AdresVyhoda(context.Background(), "http://vyhod.example/", port)
	if err != nil {
		t.Fatal(err)
	}
	if adres != "203.0.113.7" {
		t.Fatalf("адрес %q", adres)
	}
	if !strings.HasPrefix(videlProksi, "http://vyhod.example/") {
		t.Fatalf("запрос пошёл мимо прокси: %q", videlProksi)
	}
}

func TestAdresVyhodaNapryamuyuBezProksi(t *testing.T) {
	cel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "198.51.100.9")
	}))
	defer cel.Close()
	adres, err := AdresVyhoda(context.Background(), cel.URL, 0)
	if err != nil {
		t.Fatal(err)
	}
	if adres != "198.51.100.9" {
		t.Fatalf("адрес %q", adres)
	}
}

func TestAdresVyhodaMusorEtoOshibka(t *testing.T) {
	// Страница «403 Forbidden» от чужого прокси, принятая за адрес, ушла бы на
	// экран как адрес выхода.
	cel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "<html>forbidden</html>")
	}))
	defer cel.Close()
	if _, err := AdresVyhoda(context.Background(), cel.URL, 0); err == nil {
		t.Fatal("не-адрес принят за адрес")
	}
}
