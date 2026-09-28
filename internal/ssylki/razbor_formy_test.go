package ssylki

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

// Формы ссылок из раздела «Ссылки и подписки» аудита 1.6.1: каждая из них
// либо роняла исправную ссылку, либо принимала ту, что не поднимется.

const uuidFormy = "11111111-2222-3333-4444-555555555555"

func ssIz(metodParol, adres string) string {
	return "ss://" + base64.StdEncoding.EncodeToString([]byte(metodParol)) + "@" + adres
}

// П1. vless поверх голого TCP под обычным TLS, с Vision или без, отвергался как
// «не наш транспорт», хотя ядро несёт его без условий сборки.
func TestVlessTlsTcpRazbiraetsya(t *testing.T) {
	s := "vless://" + uuidFormy + "@203.0.113.1:443?security=tls&type=tcp&flow=xtls-rprx-vision" +
		"&sni=a.example&fp=chrome&alpn=h2&pinSHA256=AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8%3D#V"
	srv, err := Razobrat(s)
	if err != nil {
		t.Fatal(err)
	}
	if srv.Transport != "tls-tcp" || srv.BezTLS || srv.Flow != "xtls-rprx-vision" ||
		srv.Sni != "a.example" || srv.Pin != "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=" {
		t.Fatalf("%+v", srv)
	}
	// Туда и обратно: выгрузка даёт ссылку, из которой выходит та же запись.
	nazad, err := Sobrat(srv)
	if err != nil {
		t.Fatal(err)
	}
	snova, err := Razobrat(nazad)
	if err != nil || snova != srv {
		t.Fatalf("выгрузка %q разобралась в %+v (%v)", nazad, snova, err)
	}
	// Без TLS голый TCP по-прежнему не наш: vless сам ничего не шифрует.
	if _, err := Razobrat("vless://" + uuidFormy + "@203.0.113.1:443?security=none&type=tcp#V"); !errors.Is(err, ErrTransportNePodderzhan) {
		t.Fatalf("vless поверх голого tcp без tls принят: %v", err)
	}
}

// П2. url.Parse отвергает «%» без двух шестнадцатеричных знаков, и ссылка с
// именем «Скидка 50%» падала целиком, хотя имя серверу не нужно вовсе.
func TestGolyyProtsentVImeniNeRonyaetSsylku(t *testing.T) {
	baza := map[string]string{
		"vless":  "vless://" + uuidFormy + "@203.0.113.1:443?security=reality&pbk=AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8&sid=ab&sni=a.example",
		"hy2":    "hy2://parol@203.0.113.1:443?sni=a.example",
		"trojan": "trojan://parol@203.0.113.1:443?sni=a.example",
		"anytls": "anytls://parol@203.0.113.1:443?sni=a.example",
		"tuic":   "tuic://" + uuidFormy + ":parol@203.0.113.1:443?sni=a.example",
		"ss":     ssIz("aes-256-gcm:parol", "203.0.113.1:8388"),
		"vmess":  vmessSsylka(t, `{"add":"203.0.113.1","port":443,"id":"`+uuidFormy+`","net":"tcp","tls":"tls"}`),
	}
	imena := map[string]string{
		"50%off":             "50%off",
		"NL%2050%":           "NL%2050%",
		"%D0%B4%D0%BE%D0%BC": "дом",
		"50%2541":            "50%41",
	}
	for shema, s := range baza {
		for hvost, imya := range imena {
			srv, err := Razobrat(s + "#" + hvost)
			if err != nil {
				t.Errorf("%s #%s: %v", shema, hvost, err)
				continue
			}
			if srv.Imya != imya {
				t.Errorf("%s #%s: имя %q, ждали %q", shema, hvost, srv.Imya, imya)
			}
		}
	}
}

// П8. Всё до «://» печаталось как схема. У строки без схемы там оказывался
// uuid, и он уезжал в отказ подписки и оттуда на экран.
func TestNeSkhemaNeUezzhaetVOtkaz(t *testing.T) {
	_, err := Razobrat(uuidFormy + "@203.0.113.1:443?next=vless://x")
	if err == nil {
		t.Fatal("строка без схемы разобралась")
	}
	if strings.Contains(err.Error(), uuidFormy) || strings.Contains(err.Error(), "203.0.113.1") {
		t.Fatalf("в отказ уехал кусок строки: %v", err)
	}
	if !strings.Contains(err.Error(), "неизвестная схема") {
		t.Fatalf("отказ %q не говорит, что схема неизвестна", err)
	}
	// Настоящая чужая схема называется по имени, иначе человек не поймёт, что
	// именно клиент не несёт.
	_, err = Razobrat("socks://dXNlcjpwYXNz@203.0.113.1:1080#S")
	if !errors.Is(err, ErrTransportNePodderzhan) || !strings.Contains(err.Error(), "socks") {
		t.Fatalf("socks: %v", err)
	}
}

// П3. Параметр, который меняет протокол, клиент молча выбрасывал: сервер
// добавлялся и не поднимался, а на экране была причина про ключ.
func TestNeponyatnyyProtokolOtvergaetsya(t *testing.T) {
	sluchai := []struct{ imya, ssylka, prichina, sekret string }{
		{"ss с плагином",
			ssIz("aes-256-gcm:parol", "203.0.113.1:8388/?plugin=obfs-local%3Bobfs%3Dhttp%3Bobfs-host%3Dskrytyy.example#X"),
			"obfs-local", "skrytyy.example"},
		{"vmess с маскировкой под HTTP",
			vmessSsylka(t, `{"add":"203.0.113.1","port":80,"id":"`+uuidFormy+`","net":"tcp","type":"http"}`),
			"http", ""},
		{"vmess поверх reality",
			vmessSsylka(t, `{"add":"203.0.113.1","port":443,"id":"`+uuidFormy+`","net":"tcp","tls":"reality"}`),
			"reality", ""},
		{"vless с encryption",
			"vless://" + uuidFormy + "@203.0.113.1:443?security=reality&encryption=mlkem768x25519plus.native.0rtt.SEKRETNYYKLYUCH" +
				"&pbk=AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8&sid=ab&sni=a.example#X",
			"encryption", "SEKRETNYYKLYUCH"},
	}
	for _, c := range sluchai {
		_, err := Razobrat(c.ssylka)
		if !errors.Is(err, ErrTransportNePodderzhan) {
			t.Errorf("%s: принято или отвергнуто не тем (%v)", c.imya, err)
			continue
		}
		if !strings.Contains(err.Error(), c.prichina) {
			t.Errorf("%s: отказ %q не называет %q", c.imya, err, c.prichina)
		}
		if c.sekret != "" && strings.Contains(err.Error(), c.sekret) {
			t.Errorf("%s: в отказ уехало значение параметра: %v", c.imya, err)
		}
	}

	// Контроль: пустые и нейтральные значения тех же параметров не мешают.
	for _, s := range []string{
		ssIz("aes-256-gcm:parol", "203.0.113.1:8388/?plugin=#X"),
		vmessSsylka(t, `{"add":"203.0.113.1","port":80,"id":"`+uuidFormy+`","net":"tcp","type":"none"}`),
		"vless://" + uuidFormy + "@203.0.113.1:443?security=reality&encryption=none" +
			"&pbk=AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8&sid=ab&sni=a.example#X",
	} {
		if _, err := Razobrat(s); err != nil {
			t.Errorf("нейтральный параметр отверг ссылку: %v", err)
		}
	}
}

// П5. Кириллический домен доезжал до ядра как есть, а DNS знает только
// ASCII-запись. Сервер добавлялся и не находился.
func TestKirillicheskiyDomenUezzhaetVPunycode(t *testing.T) {
	const zhdali = "xn--e1afmkfd.xn--p1ai"
	for _, s := range []string{
		"hy2://parol@пример.рф:443#X",
		"hy2://parol@%D0%BF%D1%80%D0%B8%D0%BC%D0%B5%D1%80.%D1%80%D1%84:443#X",
		ssIz("aes-256-gcm:parol", "пример.рф:8388#X"),
		vmessSsylka(t, `{"add":"пример.рф","port":443,"id":"`+uuidFormy+`","net":"tcp","tls":"tls"}`),
	} {
		srv, err := Razobrat(s)
		if err != nil {
			t.Errorf("%q: %v", s, err)
			continue
		}
		if srv.Host != zhdali {
			t.Errorf("%q: хост %q, ждали %q", s, srv.Host, zhdali)
		}
	}
	// ASCII-имя не трогается, даже если строгие правила IDNA его не любят.
	srv, err := Razobrat("hy2://parol@my_host.example:443#X")
	if err != nil || srv.Host != "my_host.example" {
		t.Fatalf("ASCII-имя изменено: %q, %v", srv.Host, err)
	}
}

// П6. Сам Hysteria 2 пишет порты прыжков прямо в адресе: «хост:443,20000-30000».
// url.Parse такой порт не принимает, и ссылка падала как битая.
func TestHy2SPortamiVAdrese(t *testing.T) {
	srv, err := Razobrat("hysteria2://parol@203.0.113.1:443,20000-30000/?sni=a.example#X")
	if err != nil {
		t.Fatal(err)
	}
	if srv.Port != 443 || srv.Porty != "443,20000-30000" || srv.Sni != "a.example" || srv.Imya != "X" {
		t.Fatalf("порт %d, прыжки %q, sni %q, имя %q", srv.Port, srv.Porty, srv.Sni, srv.Imya)
	}
	if srv.Id != IdHoppinga(srv.Host, srv.Port, srv.Transport) {
		t.Fatalf("Id не от IdHoppinga: %q", srv.Id)
	}

	v6, err := Razobrat("hy2://parol@[2001:db8::1]:20000-30000?sni=a.example#X")
	if err != nil {
		t.Fatal(err)
	}
	if v6.Host != "2001:db8::1" || v6.Port != 20000 || v6.Porty != "20000-30000" {
		t.Fatalf("IPv6: хост %q, порт %d, прыжки %q", v6.Host, v6.Port, v6.Porty)
	}

	for _, s := range []string{
		"hy2://parol@203.0.113.1:443,99999#X",
		"hy2://parol@203.0.113.1:3000-2000#X",
		"hy2://parol@203.0.113.1:443,,500#X",
	} {
		if _, err := Razobrat(s); !errors.Is(err, ErrSsylkaKrivaya) {
			t.Errorf("%q: негодный список портов принят (%v)", s, err)
		}
	}
}
