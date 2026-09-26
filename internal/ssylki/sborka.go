package ssylki

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Sobrat собирает ссылку обратно из записи сервера: обратная сторона Razobrat.
//
// Нужна экспорту (26.09.2026): человек переносит свои серверы на другой ПК или
// на телефон одной строкой. Правило одно: Razobrat(Sobrat(s)) обязан вернуть
// ту же запись, и это держит тест на всём корпусе ссылок.
//
// Два поля не возвращаются намеренно. insecure не пишется: клиент его не
// выполняет, и отдать его дальше значило бы включить на чужом устройстве то,
// что здесь сознательно выключено. Отпечаток сертификата (pinSHA256 в hex)
// разбор не хранит вовсе, поэтому его нет и здесь: серверу с сертификатом от
// доверенного центра он не нужен, а наш собственный пин уезжает как
// pinPubKeySHA256.
func Sobrat(s protokol.Server) (string, error) {
	if s.Host == "" || s.Port < 1 || s.Port > 65535 {
		return "", fmt.Errorf("%w: у записи нет адреса", ErrSsylkaKrivaya)
	}
	adres := net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
	switch s.Transport {
	case "reality-tcp", "reality-grpc", "ws", "grpc", "httpupgrade":
		return sobratVless(s, adres), nil
	case "trojan", "trojan-ws":
		return sobratTrojan(s, adres), nil
	case "vmess", "vmess-ws":
		return sobratVmess(s)
	case "hy2":
		q := url.Values{}
		dobavit(q, "sni", s.Sni)
		dobavit(q, "obfs", s.Obfs)
		dobavit(q, "obfs-password", s.ObfsParol)
		dobavit(q, "mport", s.Porty)
		if s.PolosaVverh > 0 && s.PolosaVniz > 0 {
			q.Set("upmbps", strconv.Itoa(s.PolosaVverh))
			q.Set("downmbps", strconv.Itoa(s.PolosaVniz))
		}
		dobavit(q, "pinPubKeySHA256", s.Pin)
		return sobratURL("hy2", url.User(s.Parol), adres, q, s.Imya), nil
	case "anytls":
		q := url.Values{}
		dobavit(q, "sni", s.Sni)
		dobavit(q, "alpn", s.Alpn)
		dobavit(q, "fp", s.Fp)
		dobavit(q, "pinPubKeySHA256", s.Pin)
		return sobratURL("anytls", url.User(s.Parol), adres, q, s.Imya), nil
	case "tuic":
		q := url.Values{}
		dobavit(q, "sni", s.Sni)
		dobavit(q, "alpn", s.Alpn)
		dobavit(q, "fp", s.Fp)
		dobavit(q, "congestion_control", s.Peregruzka)
		dobavit(q, "udp_relay_mode", s.RezhimUDP)
		dobavit(q, "pinPubKeySHA256", s.Pin)
		return sobratURL("tuic", url.UserPassword(s.Uuid, s.Parol), adres, q, s.Imya), nil
	case "ss":
		// SIP002: метод и пароль в base64url, адрес открытым текстом.
		userinfo := base64.RawURLEncoding.EncodeToString([]byte(s.Metod + ":" + s.Parol))
		return "ss://" + userinfo + "@" + adres + imyaVSsylke(s.Imya), nil
	}
	return "", fmt.Errorf("%w: транспорт %q", ErrTransportNePodderzhan, s.Transport)
}

func sobratVless(s protokol.Server, adres string) string {
	q := url.Values{}
	q.Set("encryption", "none")
	switch {
	case s.Transport == "reality-tcp" || s.Transport == "reality-grpc":
		q.Set("security", "reality")
		dobavit(q, "pbk", s.PublicKey)
		dobavit(q, "sid", s.ShortId)
	case s.BezTLS:
		q.Set("security", "none")
	default:
		q.Set("security", "tls")
		dobavit(q, "pinPubKeySHA256", s.Pin)
	}
	switch s.Transport {
	case "reality-grpc", "grpc":
		q.Set("type", "grpc")
		dobavit(q, "serviceName", s.Put)
	case "reality-tcp":
		q.Set("type", "tcp")
		dobavit(q, "path", s.Put)
	default:
		q.Set("type", s.Transport)
		dobavit(q, "path", s.Put)
	}
	dobavit(q, "flow", s.Flow)
	dobavit(q, "sni", s.Sni)
	dobavit(q, "fp", s.Fp)
	dobavit(q, "alpn", s.Alpn)
	dobavit(q, "host", s.HostZagolovka)
	return sobratURL("vless", url.User(s.Uuid), adres, q, s.Imya)
}

func sobratTrojan(s protokol.Server, adres string) string {
	q := url.Values{}
	q.Set("security", "tls")
	if s.Transport == "trojan-ws" {
		q.Set("type", "ws")
		dobavit(q, "path", s.Put)
	} else {
		q.Set("type", "tcp")
	}
	dobavit(q, "sni", s.Sni)
	dobavit(q, "fp", s.Fp)
	dobavit(q, "alpn", s.Alpn)
	dobavit(q, "host", s.HostZagolovka)
	dobavit(q, "pinPubKeySHA256", s.Pin)
	return sobratURL("trojan", url.User(s.Parol), adres, q, s.Imya)
}

// sobratVmess пишет форму v2rayN: base64 от JSON, других у vmess нет.
func sobratVmess(s protokol.Server) (string, error) {
	v := map[string]any{
		"v": "2", "ps": s.Imya, "add": s.Host, "port": strconv.Itoa(s.Port),
		"id": s.Uuid, "aid": strconv.Itoa(s.AlterId), "scy": s.Shifr, "net": "tcp",
		"host": s.HostZagolovka, "path": "", "tls": "", "sni": s.Sni, "alpn": s.Alpn, "fp": s.Fp,
	}
	if s.Transport == "vmess-ws" {
		v["net"] = "ws"
		v["path"] = s.Put
	}
	if !s.BezTLS {
		v["tls"] = "tls"
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("тело vmess не собралось: %w", err)
	}
	return "vmess://" + base64.StdEncoding.EncodeToString(b), nil
}

func sobratURL(shema string, polzovatel *url.Userinfo, adres string, q url.Values, imya string) string {
	u := url.URL{Scheme: shema, User: polzovatel, Host: adres, RawQuery: q.Encode()}
	return u.String() + imyaVSsylke(imya)
}

// imyaVSsylke кодирует имя под PathUnescape, которым его снимает разбор.
func imyaVSsylke(imya string) string {
	if imya == "" {
		return ""
	}
	return "#" + url.PathEscape(imya)
}

// dobavit кладёт параметр, только если он есть: пустой sni= это не «нет
// имени», а имя из пустой строки, и чужой клиент поймёт его именно так.
func dobavit(q url.Values, k, v string) {
	if v != "" {
		q.Set(k, v)
	}
}
