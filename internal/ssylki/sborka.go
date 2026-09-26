package ssylki

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"

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
		return sobratURL("hy2", komponent(s.Parol), adres, q, s.Imya), nil
	case "anytls":
		q := url.Values{}
		dobavit(q, "sni", s.Sni)
		dobavit(q, "alpn", s.Alpn)
		dobavit(q, "fp", s.Fp)
		dobavit(q, "pinPubKeySHA256", s.Pin)
		return sobratURL("anytls", komponent(s.Parol), adres, q, s.Imya), nil
	case "tuic":
		q := url.Values{}
		dobavit(q, "sni", s.Sni)
		dobavit(q, "alpn", s.Alpn)
		dobavit(q, "fp", s.Fp)
		dobavit(q, "congestion_control", s.Peregruzka)
		dobavit(q, "udp_relay_mode", s.RezhimUDP)
		dobavit(q, "pinPubKeySHA256", s.Pin)
		return sobratURL("tuic", komponent(s.Uuid)+":"+komponent(s.Parol), adres, q, s.Imya), nil
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
	return sobratURL("vless", komponent(s.Uuid), adres, q, s.Imya)
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
	return sobratURL("trojan", komponent(s.Parol), adres, q, s.Imya)
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

// sobratURL собирает строку руками, а не через url.URL (26.09.2026).
//
// Стандарт ссылок Xray кодирует каждое поле одним encodeURIComponent. url.URL
// так не умеет: в userinfo он оставляет «+», «=» и «:» как есть, в запросе
// пишет пробел плюсом. Голый «+» в пароле чужой клиент вправе прочитать
// пробелом, а плюс вместо пробела в пути v2rayNG читает плюсом.
func sobratURL(shema, userinfo, adres string, q url.Values, imya string) string {
	s := shema + "://" + userinfo + "@" + adres
	if len(q) > 0 {
		s += "?" + zapros(q)
	}
	return s + imyaVSsylke(imya)
}

// komponent кодирует как encodeURIComponent: всё, кроме A-Za-z0-9-_.~, в
// процентах, пробел как %20. QueryEscape отличается от него только пробелом.
func komponent(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}

// zapros это q.Encode с кодированием komponent. Ключи по алфавиту, как у
// Encode, чтобы одна запись всегда давала одну строку.
func zapros(q url.Values) string {
	klyuchi := make([]string, 0, len(q))
	for k := range q {
		klyuchi = append(klyuchi, k)
	}
	sort.Strings(klyuchi)
	var b strings.Builder
	for _, k := range klyuchi {
		for _, v := range q[k] {
			if b.Len() > 0 {
				b.WriteByte('&')
			}
			b.WriteString(komponent(k) + "=" + komponent(v))
		}
	}
	return b.String()
}

func imyaVSsylke(imya string) string {
	if imya == "" {
		return ""
	}
	return "#" + komponent(imya)
}

// dobavit кладёт параметр, только если он есть: пустой sni= это не «нет
// имени», а имя из пустой строки, и чужой клиент поймёт его именно так.
func dobavit(q url.Values, k, v string) {
	if v != "" {
		q.Set(k, v)
	}
}
