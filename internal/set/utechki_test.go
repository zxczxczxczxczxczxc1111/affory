package set

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// Задача 6.3, checkLeaks. Кнопка обязана сказать, чего она НЕ проверяет:
// браузерный DoH служба не видит вовсе, и зелёный результат при включённом
// DoH это не «утечек нет», это «мы туда не смотрим».

func vhodProverki(podnyat bool) VhodProverki {
	return VhodProverki{
		Podnyat:      podnyat,
		PortProksi:   1080,
		AdresServera: "192.0.2.10",
		Endpoint:     "http://vyhod.example/",
		SprositVyhod: func(ctx context.Context, endpoint string, port int) (string, error) {
			if port > 0 {
				return "192.0.2.10", nil
			}
			return "198.51.100.1", nil
		},
		IPv6Zaglushen: func() (bool, error) { return true, nil },
	}
}

func punkt(t *testing.T, r RezultatProverki, imya string) PunktProverki {
	t.Helper()
	for _, p := range r.Punkty {
		if p.Imya == imya {
			return p
		}
	}
	t.Fatalf("пункта %q нет: %+v", imya, r.Punkty)
	return PunktProverki{}
}

func TestProverkaNazyvaetChegoNeVidit(t *testing.T) {
	r := ProveritUtechki(context.Background(), vhodProverki(true))
	p := punkt(t, r, "DoH браузера")
	if p.Itog != ItogNeVidim {
		t.Fatalf("DoH выдан за проверенный: %+v", p)
	}
	if !strings.Contains(p.Tekst, "DoH") {
		t.Fatalf("пояснение не называет DoH: %q", p.Tekst)
	}
}

func TestProverkaVyhodaCherezTunnelZelyonaya(t *testing.T) {
	r := ProveritUtechki(context.Background(), vhodProverki(true))
	p := punkt(t, r, "адрес выхода")
	if p.Itog != ItogOk {
		t.Fatalf("выход через сервер не зелёный: %+v", p)
	}
	if !strings.Contains(p.Tekst, "192.0.2.10") {
		t.Fatalf("адрес выхода не назван: %q", p.Tekst)
	}
}

func TestProverkaLovitTrafikMimoTunnelya(t *testing.T) {
	v := vhodProverki(true)
	// Через прокси и напрямую один и тот же домашний адрес: туннель не несёт.
	v.SprositVyhod = func(ctx context.Context, endpoint string, port int) (string, error) { return "198.51.100.1", nil }
	r := ProveritUtechki(context.Background(), v)
	if p := punkt(t, r, "адрес выхода"); p.Itog != ItogUtechka {
		t.Fatalf("одинаковый адрес не признан утечкой: %+v", p)
	}
}

func TestProverkaBezTunnelyaEtoNeUtechka(t *testing.T) {
	// Выключенный туннель не утечка: человек его не включал. Пункт честно
	// говорит «туннель не поднят», а не рисует красное.
	r := ProveritUtechki(context.Background(), vhodProverki(false))
	if p := punkt(t, r, "адрес выхода"); p.Itog != ItogNeIzmereno {
		t.Fatalf("без туннеля: %+v", p)
	}
}

func TestProverkaNeIzmerenoPriOtkazeEndpointa(t *testing.T) {
	v := vhodProverki(true)
	v.SprositVyhod = func(ctx context.Context, endpoint string, port int) (string, error) {
		return "", errors.New("нет сети")
	}
	r := ProveritUtechki(context.Background(), v)
	if p := punkt(t, r, "адрес выхода"); p.Itog != ItogNeIzmereno {
		t.Fatalf("отказ эндпоинта выдан за результат: %+v", p)
	}
}

func TestProverkaIPv6BezPravilaEtoUtechka(t *testing.T) {
	v := vhodProverki(true)
	v.IPv6Zaglushen = func() (bool, error) { return false, nil }
	r := ProveritUtechki(context.Background(), v)
	if p := punkt(t, r, "IPv6"); p.Itog != ItogUtechka {
		t.Fatalf("IPv6 без правила не утечка: %+v", p)
	}
}

func TestProverkaBezProksiNeMeryaetVyhod(t *testing.T) {
	// Прокси не поднят (порт занят): через что мерить, нет. Честный ответ
	// «не измерено», а не запрос напрямую, который показал бы домашний адрес
	// и назвал бы это утечкой.
	v := vhodProverki(true)
	v.PortProksi = 0
	r := ProveritUtechki(context.Background(), v)
	if p := punkt(t, r, "адрес выхода"); p.Itog != ItogNeIzmereno || !strings.Contains(p.Tekst, "прокси") {
		t.Fatalf("без прокси: %+v", p)
	}
}
