package set

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Задача 6.4. Снимок состояния сервера лежит РЯДОМ с подпиской как
// <адрес подписки>.sostoyanie.json: клиент выводит адрес, новых секретов нет.
// Забирается напрямую, мимо туннеля: нужен он ровно тогда, когда туннель лежит.

func TestAdresSnimkaVyvoditsyaIzPodpiski(t *testing.T) {
	a, err := AdresSnimka("https://primer.example/0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	if a != "https://primer.example/0123456789abcdef0123456789abcdef.sostoyanie.json" {
		t.Fatalf("адрес снимка %q", a)
	}
	for _, plohoy := range []string{"", "https://primer.example/", "https://primer.example/x?a=1", "https://primer.example/x#y", "не адрес"} {
		if _, err := AdresSnimka(plohoy); err == nil {
			t.Errorf("подписка %q дала адрес снимка, а не ошибку", plohoy)
		}
	}
}

func TestZagruzitSnimokRazbiraetPolya(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/p.sostoyanie.json" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(`{"vremya":1756900000,"trevogi":["сертификат маски истекает"],"dney_do_konca_sertifikata_maski":5,"dney_s_obnovleniya_xray":40,"xray_versiya":"25.1.1","hy2_aktiven":true,"dney_do_konca_sertifikata_hy2":6}`))
	}))
	defer srv.Close()
	s, err := ZagruzitSnimok(context.Background(), srv.URL+"/p.sostoyanie.json")
	if err != nil {
		t.Fatal(err)
	}
	if s.Vremya != 1756900000 || len(s.Trevogi) != 1 || s.DneySertifikata == nil || *s.DneySertifikata != 5 || s.XrayVersiya != "25.1.1" || s.DneySertifikataHy2 == nil || *s.DneySertifikataHy2 != 6 {
		t.Fatalf("снимок разобран неверно: %+v", s)
	}
	_, err = ZagruzitSnimok(context.Background(), srv.URL+"/net.sostoyanie.json")
	if !errors.Is(err, ErrSnimkaNet) {
		t.Fatalf("404 должен быть ErrSnimkaNet, а не %v", err)
	}
}

func TestZagruzitSnimokOtvergaetMusor(t *testing.T) {
	// Страница-заглушка чужого прокси со статусом 200 это не снимок.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html>403</html>"))
	}))
	defer srv.Close()
	if _, err := ZagruzitSnimok(context.Background(), srv.URL+"/p.sostoyanie.json"); err == nil {
		t.Fatal("HTML принят за снимок")
	}
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"trevogi":[]}`))
	}))
	defer srv2.Close()
	if _, err := ZagruzitSnimok(context.Background(), srv2.URL+"/p.sostoyanie.json"); err == nil {
		t.Fatal("снимок без vremya принят: возраст судить не по чему")
	}
}
