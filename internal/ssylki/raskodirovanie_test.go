package ssylki

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Каждое поле ссылки раскодируется РОВНО один раз (26.09.2026).
//
// Стандарт ссылок Xray (XTLS/Xray-core, обсуждение 716) кодирует каждое поле
// одним encodeURIComponent, v2rayN и v2rayNG снимают его одним проходом.
// Второй проход незаметен на обычных значениях и портит ровно те, где после
// первого остаётся годная процентная последовательность: путь «/a%41» у нас
// становился «/a» плюс «A». Проверяется буквальный процент, закодированный
// один раз: `%2541` обязан дать `%41`, а не `A`.

func TestPutVlessRaskodiruetsyaOdinRaz(t *testing.T) {
	for _, tip := range []string{"ws", "httpupgrade"} {
		s, err := Razobrat("vless://11111111-2222-3333-4444-555555555555@203.0.113.1:443?security=tls&type=" + tip + "&path=%2Fa%2541%20b")
		if err != nil {
			t.Fatalf("%s: %v", tip, err)
		}
		if s.Put != "/a%41 b" {
			t.Errorf("%s: путь %q, ждали %q", tip, s.Put, "/a%41 b")
		}
	}
}

func TestPutTrojanWsRaskodiruetsyaOdinRaz(t *testing.T) {
	s, err := Razobrat("trojan://p@203.0.113.1:443?type=ws&path=%2Fa%2541")
	if err != nil {
		t.Fatal(err)
	}
	if s.Put != "/a%41" {
		t.Fatalf("путь %q, ждали %q", s.Put, "/a%41")
	}
}

// Путь vmess лежит в JSON, а JSON не кодируется процентами: v2rayN и v2rayNG
// берут его как есть и так же пишут.
func TestPutVmessIzJsonKakEst(t *testing.T) {
	telo := `{"v":"2","ps":"x","add":"203.0.113.1","port":"443","id":"11111111-2222-3333-4444-555555555555","net":"ws","path":"/a%41","tls":"tls"}`
	s, err := Razobrat("vmess://" + base64.StdEncoding.EncodeToString([]byte(telo)))
	if err != nil {
		t.Fatal(err)
	}
	if s.Put != "/a%41" {
		t.Fatalf("путь %q, ждали %q", s.Put, "/a%41")
	}
}

func TestImyaRaskodiruetsyaOdinRaz(t *testing.T) {
	for ssylka, imya := range map[string]string{
		"hy2://p@203.0.113.1:443#50%2541":                "50%41",
		"hy2://p@203.0.113.1:443#100%25":                 "100%",
		"hy2://p@203.0.113.1:443#NL+2":                   "NL+2",
		"hy2://p@203.0.113.1:443#%D0%B4%D0%BE%D0%BC%201": "дом 1",
	} {
		s, err := Razobrat(ssylka)
		if err != nil {
			t.Fatalf("%s: %v", ssylka, err)
		}
		if s.Imya != imya {
			t.Errorf("%s: имя %q, ждали %q", ssylka, s.Imya, imya)
		}
	}
}

// SIP002: у шифров 2022 userinfo не в base64, а открытым «метод:пароль» с
// процентным кодированием. Пароль там base64 и несёт «+», «/», «=».
func TestSsOtkrytyyUserinfoRaskodiruetsyaOdinRaz(t *testing.T) {
	for _, ssylka := range []string{
		"ss://2022-blake3-aes-128-gcm:kX9%2Bq%2FZt4%3D@203.0.113.1:8388#ss",
		"ss://2022-blake3-aes-128-gcm%3AkX9%2Bq%2FZt4%3D@203.0.113.1:8388#ss",
	} {
		s, err := Razobrat(ssylka)
		if err != nil {
			t.Fatalf("%s: %v", ssylka, err)
		}
		if s.Metod != "2022-blake3-aes-128-gcm" || s.Parol != "kX9+q/Zt4=" {
			t.Errorf("%s: метод %q, пароль %q", ssylka, s.Metod, s.Parol)
		}
	}
	// Буквальный плюс в открытом userinfo остаётся плюсом.
	s, err := Razobrat("ss://aes-256-gcm:pa+ss@203.0.113.1:8388")
	if err != nil {
		t.Fatal(err)
	}
	if s.Parol != "pa+ss" {
		t.Fatalf("пароль %q", s.Parol)
	}
}

// Выгрузка кодирует каждое поле как encodeURIComponent: всё, кроме
// A-Z a-z 0-9 - _ . ~, в процентах, пробел как %20. Голый «+» в пароле или
// имени чужой клиент вправе прочитать пробелом, «+» вместо пробела в пути
// v2rayNG читает плюсом.
func TestSborkaKodiruetKakEncodeURIComponent(t *testing.T) {
	for _, sl := range []struct {
		srv    protokol.Server
		nuzhno []string
	}{
		{protokol.Server{Transport: "trojan", Host: "203.0.113.1", Port: 443, Parol: "kX9+q/Zt4=", Imya: "NL+2 дом"},
			[]string{"trojan://kX9%2Bq%2FZt4%3D@", "#NL%2B2%20%D0%B4%D0%BE%D0%BC"}},
		{protokol.Server{Transport: "hy2", Host: "203.0.113.1", Port: 443, Parol: "p+q=:r", ObfsParol: "o+b", Obfs: "salamander"},
			[]string{"hy2://p%2Bq%3D%3Ar@", "obfs-password=o%2Bb"}},
		{protokol.Server{Transport: "tuic", Host: "203.0.113.1", Port: 443, Uuid: "11111111-2222-3333-4444-555555555555", Parol: "p+q"},
			[]string{"tuic://11111111-2222-3333-4444-555555555555:p%2Bq@"}},
		{protokol.Server{Transport: "ws", Host: "203.0.113.1", Port: 443, Uuid: "11111111-2222-3333-4444-555555555555", Put: "/a b+c"},
			[]string{"path=%2Fa%20b%2Bc"}},
	} {
		ss, err := Sobrat(sl.srv)
		if err != nil {
			t.Fatal(err)
		}
		for _, n := range sl.nuzhno {
			if !strings.Contains(ss, n) {
				t.Errorf("в %s нет %s", ss, n)
			}
		}
	}
}
