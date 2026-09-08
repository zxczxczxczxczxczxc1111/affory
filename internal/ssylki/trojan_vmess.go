package ssylki

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// trojan и vmess, добавлено 03.09.2026.
//
// До этого обе схемы разбирались ровно затем, чтобы быть помеченными «не
// поддержан». Ядро при этом умело их всё время: у protocol/trojan и
// protocol/vmess в sing-box нет ни одного условия сборки, и в реестр они
// попадают безусловно. То есть поддержки не было только у нас.

// trojan разбирает trojan://пароль@хост:порт?параметры#имя.
//
// TLS у trojan есть всегда: протокол только им и прикрыт, и «security=none»
// в его ссылках не встречается. Поэтому BezTLS здесь не выставляется никогда,
// в отличие от vless, где открытый транспорт это живой случай.
func trojan(s string) (protokol.Server, error) {
	u, host, port, imya, err := razobratURL(s)
	if err != nil {
		return protokol.Server{}, err
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return protokol.Server{}, fmt.Errorf("%w: параметры: %v", ErrSsylkaKrivaya, err)
	}
	// Пароль лежит там, где у vless uuid. Пустой пароль это не «сервер без
	// пароля», это оборванная ссылка: приняв её, мы положили бы в список
	// запись, которая не подключится, а на экране было бы про отказ сервера.
	parol := u.User.Username()
	if parol == "" {
		return protokol.Server{}, fmt.Errorf("%w: trojan без пароля", ErrSsylkaKrivaya)
	}
	// Пароль в ссылке закодирован процентами: панели кладут туда что угодно.
	if p, err := url.QueryUnescape(parol); err == nil {
		parol = p
	}

	srv := protokol.Server{
		Imya:                    imya,
		Host:                    host,
		Port:                    port,
		Parol:                   parol,
		Sni:                     q.Get("sni"),
		Fp:                      q.Get("fp"),
		Alpn:                    q.Get("alpn"),
		HostZagolovka:           q.Get("host"),
		NebezopasnyyIgnorirovan: nebezopasnyy(q),
	}

	switch tip := q.Get("type"); tip {
	case "", "tcp", "raw", "none":
		srv.Transport = "trojan"
	case "ws":
		srv.Transport = "trojan-ws"
		put, err := url.PathUnescape(q.Get("path"))
		if err != nil {
			return protokol.Server{}, fmt.Errorf("%w: путь %q: %v", ErrSsylkaKrivaya, q.Get("path"), err)
		}
		srv.Put = put
	default:
		return protokol.Server{}, fmt.Errorf("%w: trojan поверх type=%s", ErrTransportNePodderzhan, tip)
	}
	// trojan это ВСЕГДА TLS, без вариантов, поэтому пин берётся безусловно.
	// До 05.09.2026 его тут не брали, и самоподписанный сервер был недостижим
	// молча: insecure мы игнорируем намеренно, а пин, законную замену доверию к
	// цепочке, выбрасывали. Тот же разбор, что у hy2, anytls и tuic.
	if srv.Pin, err = pinIzZaprosa(q); err != nil {
		return protokol.Server{}, err
	}
	srv.Id = Id(srv.Host, srv.Port, srv.Transport)
	return srv, nil
}

// chisloIz читает поле, которое панели пишут то числом, то строкой.
//
// Разбор под одну форму объявляет исправную ссылку битой, а это худший вид
// отказа: человека посылают искать опечатку в строке, где её нет. json.Number
// тут не годится, потому что строку он не примет, а свой тип с UnmarshalJSON
// не годится потому, что ворота мёртвого экспорта его не видят: json зовёт
// такой метод через отражение, а не по имени.
func chisloIz(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(t))
		return n
	}
	return 0
}

// stroka приводит поле к строке, каким бы видом его ни прислали.
func stroka(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.Itoa(int(t))
	case bool:
		if t {
			return "1"
		}
		return "0"
	}
	return ""
}

// vmess разбирает vmess://<base64 от JSON>.
//
// Формат придуман панелью v2rayN и с тех пор не менялся, но и не описан: поля
// приходят в двух видах каждое. Читаем оба.
func vmess(s string) (protokol.Server, error) {
	telo := strings.TrimPrefix(s, "vmess://")
	// Имя иногда дописывают фрагментом поверх base64, хотя оно есть и внутри.
	telo, hvost, _ := strings.Cut(telo, "#")
	b, ok := dekodirovat(strings.TrimSpace(telo))
	if !ok {
		return protokol.Server{}, fmt.Errorf("%w: тело vmess не base64", ErrSsylkaKrivaya)
	}
	var v struct {
		Ps       string `json:"ps"`
		Add      string `json:"add"`
		Port     any    `json:"port"`
		Id       string `json:"id"`
		Aid      any    `json:"aid"`
		Scy      string `json:"scy"`
		Net      string `json:"net"`
		Host     string `json:"host"`
		Path     string `json:"path"`
		Tls      string `json:"tls"`
		Sni      string `json:"sni"`
		Alpn     string `json:"alpn"`
		Fp       string `json:"fp"`
		Insecure any    `json:"allowInsecure"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return protokol.Server{}, fmt.Errorf("%w: тело vmess не JSON: %v", ErrSsylkaKrivaya, err)
	}
	if v.Add == "" || v.Id == "" {
		return protokol.Server{}, fmt.Errorf("%w: в теле vmess нет адреса или uuid", ErrSsylkaKrivaya)
	}
	port := chisloIz(v.Port)
	if port <= 0 || port > 65535 {
		return protokol.Server{}, fmt.Errorf("%w: порт %q", ErrSsylkaKrivaya, stroka(v.Port))
	}
	imya := v.Ps
	if imya == "" {
		if h, err := url.PathUnescape(hvost); err == nil {
			imya = h
		}
	}

	srv := protokol.Server{
		Imya:          imya,
		Host:          v.Add,
		Port:          port,
		Uuid:          v.Id,
		AlterId:       chisloIz(v.Aid),
		Shifr:         v.Scy,
		Sni:           v.Sni,
		Alpn:          v.Alpn,
		Fp:            v.Fp,
		HostZagolovka: v.Host,
		// tls тут строка, а не флаг: «tls», «none» или пусто. Открытый vmess
		// это живой случай, порты 80 и 8080 в сборниках.
		BezTLS:                  v.Tls != "tls" && v.Tls != "reality",
		NebezopasnyyIgnorirovan: stroka(v.Insecure) == "1" || strings.EqualFold(stroka(v.Insecure), "true"),
	}

	switch v.Net {
	case "", "tcp", "raw":
		srv.Transport = "vmess"
	case "ws":
		srv.Transport = "vmess-ws"
		put, err := url.PathUnescape(v.Path)
		if err != nil {
			return protokol.Server{}, fmt.Errorf("%w: путь %q: %v", ErrSsylkaKrivaya, v.Path, err)
		}
		srv.Put = put
	default:
		return protokol.Server{}, fmt.Errorf("%w: vmess поверх net=%s", ErrTransportNePodderzhan, v.Net)
	}
	srv.Id = Id(srv.Host, srv.Port, srv.Transport)
	return srv, nil
}
