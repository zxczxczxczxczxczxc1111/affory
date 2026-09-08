package petlya

import (
	"testing"
	"time"
)

// Семьи транспортов. Одна строка на семью, и каждая поднимает НАСТОЯЩИЙ сервер
// этого протокола рядом с тестом.
//
// Проверяется цепочка продукта целиком: ссылка -> наш разбор -> наш генератор
// -> живое рукопожатие -> байты до мишени. Ровно так устроен test/ у самого
// sing-box, и по той же причине: транспорт, проверенный на макете, не проверен.
var semeystva = []struct {
	Imya    string
	Podnyat func(*testing.T) Uzel
	SPinom  bool
}{
	{"hysteria2", Hysteria2, true},
	{"trojan", Trojan, true},
	{"trojan-ws", TrojanWS, true},
	{"anytls", Anytls, true},
	{"tuic", Tuic, true},
	{"vless-ws", VlessWS, true},
	{"vless-httpupgrade", VlessHttpupgrade, true},
	{"vless-grpc", VlessGrpc, true},
	{"vless-reality", VlessReality, false},
	{"vmess", Vmess, false},
	{"vmess-ws", VmessWS, false},
	{"shadowsocks", Shadowsocks, false},
}

func TestTransportyNesutDoMisheni(t *testing.T) {
	for _, s := range semeystva {
		t.Run(s.Imya, func(t *testing.T) {
			t.Parallel()
			mishen := NovayaMishen(t, "mishen-"+s.Imya)
			klient := PodnyatKlienta(t, s.Podnyat(t))

			if imya := klient.SprositCherezProksi(t, mishen.Adres); imya != mishen.Imya {
				t.Errorf("через %s пришло %q, а мишень зовут %q", s.Imya, imya, mishen.Imya)
			}
		})
	}
}

// Контроль. Зелёный транспорт без него значит только «байты дошли», но не
// «сервер проверен»: подмена пина или пароля обязана рвать соединение.
func TestChuzhoyPinNeProhodit(t *testing.T) {
	for _, s := range semeystva {
		if !s.SPinom {
			continue
		}
		t.Run(s.Imya, func(t *testing.T) {
			t.Parallel()
			proveritOtkaz(t, s.Podnyat(t).SIsporchennym(t, "pin"))
		})
	}
}

func TestChuzhoyKlyuchNeProhodit(t *testing.T) {
	for _, s := range semeystva {
		t.Run(s.Imya, func(t *testing.T) {
			t.Parallel()
			proveritOtkaz(t, s.Podnyat(t).SIsporchennym(t, "klyuch"))
		})
	}
}

func proveritOtkaz(t *testing.T, u Uzel) {
	t.Helper()
	mishen := NovayaMishen(t, "mishen-otkaza")
	klient := PodnyatKlienta(t, u)

	if imya, err := klient.SprositTiho(mishen.Adres, 8*time.Second); err == nil {
		t.Fatalf("испорченный узел принят: пришло %q", imya)
	}
}

// Сторож живёт в отдельном тесте и БЕЗ t.Parallel: он смотрит на всю машину
// целиком, и соседний тест, поднимающий своё ядро, для него неотличим от
// продукта, забывшего погасить своё.
func TestPetlyaNeMenyaetMashinu(t *testing.T) {
	Storozhit(t)
	mishen := NovayaMishen(t, "mishen-storozha")
	klient := PodnyatKlienta(t, Trojan(t))

	if imya := klient.SprositCherezProksi(t, mishen.Adres); imya != mishen.Imya {
		t.Errorf("пришло %q, а мишень зовут %q", imya, mishen.Imya)
	}
}
