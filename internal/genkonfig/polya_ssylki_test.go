package genkonfig

import (
	"encoding/json"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Находка 17. Разбор ссылки вынимал fp, alpn и host, клал их в модель, а
// генератор о них не знал вовсе. Практическое последствие: fp=firefox в чужой
// ссылке игнорируется, отпечаток TLS уходит дефолтный, то есть НЕ тот, на
// который рассчитывал сервер, а REALITY отпечаток замечает.
func ishodyashchiyProby(t *testing.T, s protokol.Server) map[string]any {
	t.Helper()
	o, err := ishodyashchiy(Vhod{Server: s}, s, "srv-proba")
	if err != nil {
		t.Fatalf("исходящий не собран: %v", err)
	}
	b, _ := json.Marshal(o)
	var v map[string]any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

// Непроверенный отпечаток сводится к chrome, и это ЗАМЕР, а не осторожность.
// 02.09.2026 на стенде против своего сервера: firefox не понёс ни разу из трёх,
// chrome понёс два из трёх, при том что ссылка собрана из рабочего конфига
// Xray, где firefox несёт. Ядро такой конфиг принимает молча, поэтому отказ
// выглядит как «сервер не отвечает», и человек чинит не то.
func TestNeproverennyyOtpechatokSvoditsyaKChrome(t *testing.T) {
	s := protokol.Server{
		Id: "p", Transport: "reality-tcp", Host: "203.0.113.20", Port: 443,
		Uuid: "11111111-2222-3333-4444-555555555555",
		// 43 символа base64url: проверка ключа сильна в значениях.
		PublicKey: "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8", ShortId: "01ab",
		Sni: "www.example.com", Fp: "firefox",
	}
	v := ishodyashchiyProby(t, s)
	tls := v["tls"].(map[string]any)
	utls := tls["utls"].(map[string]any)
	if utls["fingerprint"] != "chrome" {
		t.Fatalf("отпечаток %v: непроверенный firefox уехал в ядро и не понесёт", utls["fingerprint"])
	}
}

// КОНТРОЛЬ: без fp в ссылке отпечаток обязан остаться chrome. Пустой отпечаток
// это отсутствие utls вовсе, то есть рукопожатие Go, которое REALITY отвергнет.
func TestBezOtpechatkaVSsylkeOstayotsyaChrome(t *testing.T) {
	s := protokol.Server{
		Id: "p", Transport: "reality-tcp", Host: "203.0.113.20", Port: 443,
		Uuid:      "11111111-2222-3333-4444-555555555555",
		PublicKey: "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8", ShortId: "01ab",
	}
	v := ishodyashchiyProby(t, s)
	utls := v["tls"].(map[string]any)["utls"].(map[string]any)
	if utls["fingerprint"] != "chrome" {
		t.Fatalf("отпечаток по умолчанию %v, ожидался chrome", utls["fingerprint"])
	}
}

func TestAlpnIzSsylkiDoezzhaetDoYadra(t *testing.T) {
	s := protokol.Server{
		Id: "p", Transport: "ws", Host: "203.0.113.20", Port: 443,
		Uuid: "11111111-2222-3333-4444-555555555555",
		Sni:  "a.example", Alpn: "h2,http/1.1",
	}
	v := ishodyashchiyProby(t, s)
	tls := v["tls"].(map[string]any)
	alpn, est := tls["alpn"].([]any)
	if !est {
		t.Fatalf("alpn до ядра не доехал: %v", tls)
	}
	if len(alpn) != 2 || alpn[0] != "h2" || alpn[1] != "http/1.1" {
		t.Fatalf("alpn разобран неверно: %v", alpn)
	}
}

// КОНТРОЛЬ: без alpn в ссылке поля быть не должно. Пустой список ALPN это не то
// же самое, что его отсутствие: сервер выберет по нему и не найдёт ничего.
func TestBezAlpnPolyaNet(t *testing.T) {
	s := protokol.Server{
		Id: "p", Transport: "ws", Host: "203.0.113.20", Port: 443,
		Uuid: "11111111-2222-3333-4444-555555555555", Sni: "a.example",
	}
	v := ishodyashchiyProby(t, s)
	if _, est := v["tls"].(map[string]any)["alpn"]; est {
		t.Fatal("пустой alpn всё равно уехал в конфиг")
	}
}

func TestZagolovokHostIzSsylkiDoezzhaetDoYadra(t *testing.T) {
	for _, transport := range []string{"ws", "httpupgrade"} {
		t.Run(transport, func(t *testing.T) {
			s := protokol.Server{
				Id: "p", Transport: transport, Host: "203.0.113.20", Port: 443,
				Uuid: "11111111-2222-3333-4444-555555555555", Sni: "a.example",
				PublicKey:     "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8",
				HostZagolovka: "front.example",
			}
			v := ishodyashchiyProby(t, s)
			tr, est := v["transport"].(map[string]any)
			if !est {
				t.Fatalf("нет транспорта вовсе: %v", v)
			}
			zag, est := tr["headers"].(map[string]any)
			if !est {
				t.Fatalf("заголовок Host до ядра не доехал: %v", tr)
			}
			if zag["Host"] != "front.example" {
				t.Fatalf("Host %v, в ссылке был front.example", zag["Host"])
			}
		})
	}
}

// Без fp в ссылке utls на обычном TLS не появляется.
//
// Включённый без спроса utls меняет рукопожатие там, где менять его никто не
// просил: транспорт без REALITY этого не требует, а отпечаток чужого браузера
// на обычном ws это ложь о себе без всякой пользы.
func TestBezOtpechatkaNaObychnomTlsNetUtls(t *testing.T) {
	s := protokol.Server{
		Id: "p", Transport: "ws", Host: "203.0.113.20", Port: 443,
		Uuid: "11111111-2222-3333-4444-555555555555", Sni: "a.example",
	}
	v := ishodyashchiyProby(t, s)
	if _, est := v["tls"].(map[string]any)["utls"]; est {
		t.Fatal("utls включён без спроса: рукопожатие подменено там, где не просили")
	}
}

// Зеркало: с ПРОВЕРЕННЫМ fp в ссылке utls на ws обязан появиться, иначе
// проверка выше зелёная и на генераторе, который отпечаток игнорирует вовсе.
func TestSOtpechatkomNaObychnomTlsUtlsEst(t *testing.T) {
	s := protokol.Server{
		Id: "p", Transport: "ws", Host: "203.0.113.20", Port: 443,
		Uuid: "11111111-2222-3333-4444-555555555555", Sni: "a.example", Fp: "chrome",
	}
	v := ishodyashchiyProby(t, s)
	utls, est := v["tls"].(map[string]any)["utls"].(map[string]any)
	if !est {
		t.Fatal("отпечаток из ссылки на ws не доехал")
	}
	if utls["fingerprint"] != "chrome" {
		t.Fatalf("отпечаток %v, в ссылке был chrome", utls["fingerprint"])
	}
}
