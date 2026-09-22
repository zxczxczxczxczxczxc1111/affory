package ssylki

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Загрузчик через локальный вход ядра (A8). Проверяется не поле структуры, а
// то, куда уходит запрос: клиент с прокси, который на самом деле ходит мимо
// него, выглядит исправным и молча теряет весь смысл запасного пути.
func TestZagruzchikCherezProksiShlyotZaprosVProksi(t *testing.T) {
	var prishlo string
	proksi := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		prishlo = r.RequestURI
		w.Write([]byte("vless://11111111-2222-3333-4444-555555555555@203.0.113.7:443?type=ws&path=%2Fws#NL\n"))
	}))
	defer proksi.Close()

	z, err := NovyyZagruzchikCherez(strings.TrimPrefix(proksi.URL, "http://"))
	if err != nil {
		t.Fatalf("загрузчик через прокси не собрался: %v", err)
	}
	// Имя, которого нет в DNS: без прокси запрос сюда не дошёл бы никуда.
	r, err := z.Zagruzit(context.Background(), "http://panel.invalid/sub?token=tayna")
	if err != nil {
		t.Fatalf("загрузка через прокси: %v", err)
	}
	if len(r.Servery) != 1 {
		t.Fatalf("серверов %d, ждали один", len(r.Servery))
	}
	// Прокси видит ПОЛНЫЙ адрес: так и отличается запрос через прокси от
	// прямого, где в строку запроса уходит только путь.
	if !strings.HasPrefix(prishlo, "http://panel.invalid/sub") {
		t.Fatalf("прокси получил %q", prishlo)
	}
}

func TestZagruzchikCherezProksiNeOslablyaetTLS(t *testing.T) {
	z, err := NovyyZagruzchikCherez("127.0.0.1:10809")
	if err != nil {
		t.Fatalf("загрузчик: %v", err)
	}
	tr, ok := z.Klient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("транспорт %T", z.Klient.Transport)
	}
	// Запрос несёт пропуск к панели. Ослабить проверку сертификата ради
	// доступности значит отдать этот пропуск любому, кто встанет на пути.
	if tr.TLSClientConfig != nil && tr.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("проверка сертификата отключена на пути через туннель")
	}
	if z.Klient.Timeout <= 0 {
		t.Fatal("клиент без таймаута: на молчащем сокете он висит вечно")
	}
}

func TestZagruzchikBezProksiOstayotsyaPryamym(t *testing.T) {
	z, err := NovyyZagruzchikCherez("")
	if err != nil {
		t.Fatalf("загрузчик: %v", err)
	}
	if z.Klient.Transport != nil {
		t.Fatalf("у прямого загрузчика свой транспорт %T, а он должен остаться общим", z.Klient.Transport)
	}
}
