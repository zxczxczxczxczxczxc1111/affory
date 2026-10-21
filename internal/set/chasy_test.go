package set

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRashozhdenieIzDaty(t *testing.T) {
	tochnoe := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	data := tochnoe.Format(http.TimeFormat)

	// Часы машины на 7 минут позади: расхождение отрицательное.
	nachalo := tochnoe.Add(-7 * time.Minute)
	d, err := rashozhdenieIz(data, nachalo, nachalo.Add(200*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	if d > -6*time.Minute-58*time.Second || d < -7*time.Minute-2*time.Second {
		t.Fatalf("расхождение %s, ожидалось около -7 мин", d)
	}

	// Сутки вперёд.
	nachalo = tochnoe.Add(24 * time.Hour)
	d, _ = rashozhdenieIz(data, nachalo, nachalo)
	if d < 23*time.Hour+59*time.Minute {
		t.Fatalf("расхождение %s, ожидались сутки", d)
	}
}

func TestBezDatyOtkaz(t *testing.T) {
	for _, data := range []string{"", "вчера", "Mon, 01 Jan 0001 00:00:00 GMT"} {
		if _, err := rashozhdenieIz(data, time.Now(), time.Now()); !errors.Is(err, ErrNetDaty) {
			t.Errorf("Date %q: ошибка %v, ожидалась ErrNetDaty", data, err)
		}
	}
}

// Первая ответившая цель решает, молчащая не задерживает ответ.
func TestPervyyOtvetivshiyReshaet(t *testing.T) {
	molchit := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer molchit.Close()
	otvechaet := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer otvechaet.Close()

	ctx, otmena := context.WithTimeout(context.Background(), 5*time.Second)
	defer otmena()
	nachalo := time.Now()
	d, err := RashozhdenieChasov(ctx, []string{molchit.URL, otvechaet.URL})
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(nachalo) > 3*time.Second {
		t.Fatal("молчащая цель задержала ответ")
	}
	// Сервер теста живёт по тем же часам: расхождение в пределах округления Date.
	if d < -2*time.Second || d > 2*time.Second {
		t.Fatalf("расхождение со своими же часами %s", d)
	}
}

func TestVseMolchatOtkaz(t *testing.T) {
	bezDaty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// net/http ставит Date сам; пустое значение в карте его подавляет.
		w.Header()["Date"] = nil
		w.WriteHeader(http.StatusNoContent)
	}))
	defer bezDaty.Close()
	_, err := RashozhdenieChasov(context.Background(), []string{bezDaty.URL})
	if !errors.Is(err, ErrNetDaty) {
		t.Fatalf("ошибка %v, ожидалась ErrNetDaty", err)
	}
}
