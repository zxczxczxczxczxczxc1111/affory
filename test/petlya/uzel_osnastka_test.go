package petlya

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"testing"
)

// Узел это сервер, поднятый рядом с тестом, плюс ссылка на него в том виде, в
// каком её пишет держатель сервера.
//
// Смысл конструкции в том, что клиентскую сторону собирает НАШ генератор из
// этой ссылки. Проверяется цепочка целиком: разбор ссылки, сборка исходящего,
// живое рукопожатие и байты до мишени. Ровно так устроен test/ у самого
// sing-box: сервер и клиент в одном прогоне, без сети наружу.
type Uzel struct {
	Transport string
	CaPEM     string
	Server    *Yadro

	sekret string
	pin    string
	// postroit собирает ссылку из секрета и пина. Функция, а не готовая строка,
	// потому что контролю нужна ссылка С ДРУГИМ секретом, а у vmess ссылка это
	// base64 от JSON: правкой строки там не обойтись.
	postroit func(sekret, pin string) string
}

// Ssylka отдаёт ссылку на узел.
func (u Uzel) Ssylka() string { return u.postroit(u.sekret, u.pin) }

// SIsporchennym отдаёт копию узла, у которой испорчено ровно одно: ключ
// (пароль, uuid или публичный ключ reality) либо пин.
//
// Нужен контролю. Зелёный тест транспорта без него не значит почти ничего:
// он одинаково зелен и когда подлинность проверяется, и когда её не проверяет
// никто. Проверка проверки стоит секунды и ловит ровно тот класс, из-за
// которого пин шести транспортов не доезжал до ядра целый день 05.09.2026.
func (u Uzel) SIsporchennym(t *testing.T, chto string) Uzel {
	t.Helper()
	switch chto {
	case "klyuch":
		u.sekret = drugoySekret(t, u.sekret)
	case "pin":
		if u.pin == "" {
			t.Fatal("у узла нет пина, портить нечего")
		}
		u.pin = podmenitPervyy(u.pin)
	default:
		t.Fatalf("портить нечего: %q", chto)
	}
	return u
}

// drugoySekret выдаёт секрет ТОГО ЖЕ ВИДА, но другой. Вид важен: подмена uuid
// на слово «не-тот» отвергается нашим же разбором, и проверенным окажется
// чтение ссылки вместо проверки подлинности сервера.
func drugoySekret(t *testing.T, byl string) string {
	t.Helper()
	switch {
	case pohozhNaUuid(byl):
		return novyyUuid(t)
	case pohozhNaKlyuchReality(byl):
		_, otkrytyy := paraReality(t)
		return otkrytyy
	default:
		return "ne-tot-sekret-" + byl
	}
}

func pohozhNaUuid(s string) bool {
	return len(s) == 36 && s[8] == '-' && s[13] == '-' && s[18] == '-' && s[23] == '-'
}

func pohozhNaKlyuchReality(s string) bool {
	b, err := base64.RawURLEncoding.DecodeString(s)
	return err == nil && len(b) == 32
}

// podmenitPervyy меняет первый символ так, чтобы строка осталась ЗАКОННЫМ
// base64 нужной длины: иначе отказ придёт от нашего же разбора, и проверено
// будет чтение ссылки вместо проверки подлинности сервера.
func podmenitPervyy(s string) string {
	if s == "" {
		return s
	}
	zamena := byte('A')
	if s[0] == 'A' {
		zamena = 'B'
	}
	return string(zamena) + s[1:]
}

// Имя узла одно на все семьи. Оно же в сертификате, оно же в ссылке.
//
// Именно ИМЯ, а не 127.0.0.1: разбор ссылок продукта отвергает петлю и
// неуказанный адрес намеренно, потому что панели отдают такие ссылки, когда
// подписка кончилась. Обходить эту проверку в тесте нельзя, она настоящая.
// Имя резолвится в петлю на стороне клиентского ядра (см. KonfigKlienta).
const imyaUzla = "petlya.example"

// Hysteria2: QUIC, пароль, объявление полосы. Несущий прода.
func Hysteria2(t *testing.T) Uzel {
	t.Helper()
	sert := NovyySertifikat(t, imyaUzla)
	port := SvobodnyyPortUDP(t)
	parol := "parol-petli-hy2"

	server := PodnyatServer(t, srvKonfig(map[string]any{
		"type": "hysteria2", "tag": "vhod",
		"listen": "127.0.0.1", "listen_port": port,
		"users": []any{map[string]any{"password": parol}},
		"tls":   tlsServera(sert),
	}))
	return Uzel{
		Transport: "hy2", CaPEM: sert.CaPEM, Server: server,
		sekret: parol, pin: sert.Pin,
		postroit: func(sekret, pin string) string {
			return fmt.Sprintf("hysteria2://%s@%s:%d?sni=%s&pinPubKeySHA256=%s#petlya-hy2",
				url.QueryEscape(sekret), imyaUzla, port, imyaUzla, url.QueryEscape(pin))
		},
	}
}

// Trojan: голый TCP под TLS.
func Trojan(t *testing.T) Uzel {
	return trojanovyy(t, "", "")
}

// TrojanWS: тот же trojan, но поверх веб-сокета.
func TrojanWS(t *testing.T) Uzel {
	return trojanovyy(t, "ws", "/petlya-trojan")
}

func trojanovyy(t *testing.T, tip, put string) Uzel {
	t.Helper()
	sert := NovyySertifikat(t, imyaUzla)
	port := SvobodnyyPort(t)
	parol := "parol-petli-trojan"

	vhod := map[string]any{
		"type": "trojan", "tag": "vhod",
		"listen": "127.0.0.1", "listen_port": port,
		"users": []any{map[string]any{"password": parol}},
		"tls":   tlsServera(sert),
	}
	transport := "trojan"
	if tip == "ws" {
		vhod["transport"] = map[string]any{"type": "ws", "path": put}
		transport = "trojan-ws"
	}
	server := PodnyatServer(t, srvKonfig(vhod))

	return Uzel{
		Transport: transport, CaPEM: sert.CaPEM, Server: server,
		sekret: parol, pin: sert.Pin,
		postroit: func(sekret, pin string) string {
			a := fmt.Sprintf("trojan://%s@%s:%d?sni=%s&pinPubKeySHA256=%s",
				url.QueryEscape(sekret), imyaUzla, port, imyaUzla, url.QueryEscape(pin))
			if tip == "ws" {
				a += "&type=ws&path=" + url.QueryEscape(put)
			}
			return a + "#petlya-trojan"
		},
	}
}

// Anytls: TLS плюс своя мультиплексия.
func Anytls(t *testing.T) Uzel {
	t.Helper()
	sert := NovyySertifikat(t, imyaUzla)
	port := SvobodnyyPort(t)
	parol := "parol-petli-anytls"

	server := PodnyatServer(t, srvKonfig(map[string]any{
		"type": "anytls", "tag": "vhod",
		"listen": "127.0.0.1", "listen_port": port,
		"users": []any{map[string]any{"password": parol}},
		"tls":   tlsServera(sert),
	}))
	return Uzel{
		Transport: "anytls", CaPEM: sert.CaPEM, Server: server,
		sekret: parol, pin: sert.Pin,
		postroit: func(sekret, pin string) string {
			return fmt.Sprintf("anytls://%s@%s:%d?sni=%s&pinPubKeySHA256=%s#petlya-anytls",
				url.QueryEscape(sekret), imyaUzla, port, imyaUzla, url.QueryEscape(pin))
		},
	}
}

// Tuic: QUIC, пара uuid плюс пароль.
func Tuic(t *testing.T) Uzel {
	t.Helper()
	sert := NovyySertifikat(t, imyaUzla)
	port := SvobodnyyPortUDP(t)
	uuid := novyyUuid(t)
	parol := "parol-petli-tuic"

	tls := tlsServera(sert)
	// alpn обязателен обеим сторонам: без него рукопожатие QUIC не сходится.
	tls["alpn"] = []string{"h3"}
	server := PodnyatServer(t, srvKonfig(map[string]any{
		"type": "tuic", "tag": "vhod",
		"listen": "127.0.0.1", "listen_port": port,
		"users":              []any{map[string]any{"uuid": uuid, "password": parol}},
		"congestion_control": "bbr",
		"tls":                tls,
	}))
	return Uzel{
		Transport: "tuic", CaPEM: sert.CaPEM, Server: server,
		sekret: uuid, pin: sert.Pin,
		postroit: func(sekret, pin string) string {
			return fmt.Sprintf(
				"tuic://%s:%s@%s:%d?sni=%s&alpn=h3&congestion_control=bbr&pinPubKeySHA256=%s#petlya-tuic",
				sekret, url.QueryEscape(parol), imyaUzla, port, imyaUzla, url.QueryEscape(pin))
		},
	}
}

// VlessWS, VlessHttpupgrade, VlessGrpc: один протокол, три обёртки.
func VlessWS(t *testing.T) Uzel { return vlessovyy(t, "ws", "/petlya-ws") }

func VlessHttpupgrade(t *testing.T) Uzel { return vlessovyy(t, "httpupgrade", "/petlya-hu") }

func VlessGrpc(t *testing.T) Uzel { return vlessovyy(t, "grpc", "petlya-grpc") }

func vlessovyy(t *testing.T, tip, put string) Uzel {
	t.Helper()
	sert := NovyySertifikat(t, imyaUzla)
	port := SvobodnyyPort(t)
	uuid := novyyUuid(t)

	transport := map[string]any{"type": tip}
	if tip == "grpc" {
		transport["service_name"] = put
	} else {
		transport["path"] = put
	}
	server := PodnyatServer(t, srvKonfig(map[string]any{
		"type": "vless", "tag": "vhod",
		"listen": "127.0.0.1", "listen_port": port,
		"users":     []any{map[string]any{"uuid": uuid}},
		"tls":       tlsServera(sert),
		"transport": transport,
	}))
	return Uzel{
		Transport: tip, CaPEM: sert.CaPEM, Server: server,
		sekret: uuid, pin: sert.Pin,
		postroit: func(sekret, pin string) string {
			a := fmt.Sprintf("vless://%s@%s:%d?security=tls&type=%s&sni=%s&pinPubKeySHA256=%s",
				sekret, imyaUzla, port, tip, imyaUzla, url.QueryEscape(pin))
			if tip == "grpc" {
				a += "&serviceName=" + url.QueryEscape(put)
			} else {
				a += "&path=" + url.QueryEscape(put)
			}
			return a + "#petlya-" + tip
		},
	}
}

// VlessReality: сертификата у него нет вовсе, подлинность держится на паре
// ключей x25519, а рукопожатие уводится на посторонний TLS-сервер.
//
// Маска поднимается СВОЯ, в той же петле: настоящий www.microsoft.com сделал
// бы тест зависимым от чужой сети, а чужая сеть уже дважды обвиняла продукт за
// то, чего он не делал.
func VlessReality(t *testing.T) Uzel {
	t.Helper()
	maska := NovayaMaska(t)
	port := SvobodnyyPort(t)
	uuid := novyyUuid(t)
	zakrytyy, otkrytyy := paraReality(t)
	korotkiy := "01ab"

	server := PodnyatServer(t, srvKonfig(map[string]any{
		"type": "vless", "tag": "vhod",
		"listen": "127.0.0.1", "listen_port": port,
		"users": []any{map[string]any{"uuid": uuid}},
		"tls": map[string]any{
			"enabled": true, "server_name": maska.Imya,
			"reality": map[string]any{
				"enabled":     true,
				"handshake":   map[string]any{"server": "127.0.0.1", "server_port": maska.Port},
				"private_key": zakrytyy,
				"short_id":    []string{korotkiy},
			},
		},
	}))
	return Uzel{
		Transport: "reality-tcp", Server: server,
		sekret: otkrytyy,
		postroit: func(sekret, pin string) string {
			return fmt.Sprintf(
				"vless://%s@%s:%d?security=reality&type=tcp&sni=%s&pbk=%s&sid=%s&fp=chrome#petlya-reality",
				uuid, imyaUzla, port, maska.Imya, sekret, korotkiy)
		},
	}
}

// Vmess и VmessWS идут БЕЗ TLS намеренно: пин в грамматике vmess не
// предусмотрен вовсе, а открытый vmess это живой случай на портах 80 и 8080.
func Vmess(t *testing.T) Uzel { return vmessovyy(t, "tcp", "") }

func VmessWS(t *testing.T) Uzel { return vmessovyy(t, "ws", "/petlya-vmess") }

func vmessovyy(t *testing.T, set, put string) Uzel {
	t.Helper()
	port := SvobodnyyPort(t)
	uuid := novyyUuid(t)

	vhod := map[string]any{
		"type": "vmess", "tag": "vhod",
		"listen": "127.0.0.1", "listen_port": port,
		"users": []any{map[string]any{"uuid": uuid, "alterId": 0}},
	}
	transport := "vmess"
	if set == "ws" {
		vhod["transport"] = map[string]any{"type": "ws", "path": put}
		transport = "vmess-ws"
	}
	server := PodnyatServer(t, srvKonfig(vhod))

	return Uzel{
		Transport: transport, Server: server,
		sekret: uuid,
		postroit: func(sekret, pin string) string {
			telo, err := json.Marshal(map[string]any{
				"v": "2", "ps": "petlya-" + transport, "add": imyaUzla,
				"port": port, "id": sekret, "aid": 0, "scy": "auto",
				"net": set, "path": put, "tls": "none",
			})
			if err != nil {
				panic("тело vmess не собралось: " + err.Error())
			}
			return "vmess://" + base64.StdEncoding.EncodeToString(telo)
		},
	}
}

// Shadowsocks: ни TLS, ни пина, вся подлинность в общем секрете.
func Shadowsocks(t *testing.T) Uzel {
	t.Helper()
	port := SvobodnyyPort(t)
	metod := "aes-128-gcm"
	// Ключ ss это base64 нужной длины, а не произвольная строка: ядро отвергает
	// короткий ключ отказом всего конфига.
	klyuch := base64.StdEncoding.EncodeToString(sluchaynye(t, 16))

	server := PodnyatServer(t, srvKonfig(map[string]any{
		"type": "shadowsocks", "tag": "vhod",
		"listen": "127.0.0.1", "listen_port": port,
		"method": metod, "password": klyuch,
	}))
	return Uzel{
		Transport: "ss", Server: server,
		sekret: klyuch,
		postroit: func(sekret, pin string) string {
			userinfo := base64.RawURLEncoding.EncodeToString([]byte(metod + ":" + sekret))
			return fmt.Sprintf("ss://%s@%s:%d#petlya-ss", userinfo, imyaUzla, port)
		},
	}
}

// srvKonfig это обвязка серверной роли: один вход и прямой выход.
func srvKonfig(vhod map[string]any) map[string]any {
	return map[string]any{
		"inbounds":  []any{vhod},
		"outbounds": []any{map[string]any{"type": "direct", "tag": "pryamo"}},
	}
}

func tlsServera(s Sertifikat) map[string]any {
	return map[string]any{
		"enabled": true, "server_name": s.Imya,
		"certificate_path": s.PutSert, "key_path": s.PutKlyuch,
	}
}

// PodnyatServer отличается от PodnyatYadro только ролью: у сервера нет входа
// mixed, ждать открытого TCP-порта нечего, признак подъёма один - слово ядра.
func PodnyatServer(t *testing.T, konfig map[string]any) *Yadro {
	t.Helper()
	return PodnyatYadro(t, konfig)
}

// SvobodnyyPortUDP нужен транспортам поверх QUIC: занятость TCP и UDP это
// разные вещи, и порт, свободный по TCP, у UDP может быть занят.
func SvobodnyyPortUDP(t *testing.T) int {
	t.Helper()
	c, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("свободный UDP-порт не нашёлся: %v", err)
	}
	defer c.Close()
	return c.LocalAddr().(*net.UDPAddr).Port
}

// paraReality отдаёт пару ключей x25519 в том написании, которое ждут обе
// стороны: base64 без выравнивания, как у `sing-box generate reality-keypair`.
func paraReality(t *testing.T) (zakrytyy, otkrytyy string) {
	t.Helper()
	k, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ключи reality не сгенерились: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(k.Bytes()),
		base64.RawURLEncoding.EncodeToString(k.PublicKey().Bytes())
}

func novyyUuid(t *testing.T) string {
	t.Helper()
	b := sluchaynye(t, 16)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func sluchaynye(t *testing.T, n int) []byte {
	t.Helper()
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("случайные байты не набрались: %v", err)
	}
	return b
}
