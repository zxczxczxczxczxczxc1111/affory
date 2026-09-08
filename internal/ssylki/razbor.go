// Пакет ssylki превращает то, что человек скопировал из чужой панели, в
// protokol.Server. Здесь нет ни одного обращения к сети и ни одной записи на
// диск: разбор обязан быть чистой функцией, иначе его нечем накрыть фикстурами.
package ssylki

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Два канала отказа, а не один, и это не аккуратизм.
//
// С одним `error` схемы trojan, vmess и httpupgrade выглядели бы «битой
// ссылкой», человек шёл бы искать опечатку в совершенно исправной строке, а
// обещанная карточка «транспорт не поддержан» не появилась бы никогда.
var (
	ErrSsylkaKrivaya         = errors.New("ссылка не разобрана")
	ErrTransportNePodderzhan = errors.New("транспорт не поддерживается")
	ErrUvedomleniePodpiski   = errors.New("подписка вместо сервера прислала сообщение")
)

// Список транспортов живёт В ОДНОМ месте, в genkonfig: там его читает
// генератор конфига, и второй экземпляр здесь разошёлся бы с ним молча. Разбор
// ссылки признаёт транспорт по её собственной форме (схема, type, security), а
// не по таблице; genkonfig.Izvestnyy остаётся единственным судьёй того, что
// ядро действительно умеет.
//
// Колонки «какое ядро» здесь больше нет вовсе: с 01.09.2026 ядро одно, и поле,
// одинаковое у всех, это поле, которое нечем отличить.

// Razobrat разбирает одну ссылку.
//
// Пустая строка это ErrSsylkaKrivaya, а не пустой сервер: сервер без адреса
// доехал бы до списка и там выглядел бы настоящим.
func Razobrat(s string) (protokol.Server, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return protokol.Server{}, fmt.Errorf("%w: пустая строка", ErrSsylkaKrivaya)
	}

	shema, _, est := strings.Cut(s, "://")
	if !est {
		return protokol.Server{}, fmt.Errorf("%w: нет схемы", ErrSsylkaKrivaya)
	}

	switch strings.ToLower(shema) {
	case "vless":
		return vless(s)
	case "hy2", "hysteria2":
		// Два написания одного и того же. Панели отдают оба, и знать одно
		// значит объявить половину чужих подписок чужими.
		return hysteria(s)
	case "ss":
		return shadowsocks(s)
	case "trojan":
		return trojan(s)
	case "vmess":
		return vmess(s)
	case "anytls":
		return anytls(s)
	case "tuic":
		return tuic(s)
	default:
		return protokol.Server{}, fmt.Errorf("%w: схема %s", ErrTransportNePodderzhan, shema)
	}
}

// razobratURL это общая часть: разбор, хост, порт, имя из фрагмента.
func razobratURL(s string) (*url.URL, string, int, string, error) {
	u, err := url.Parse(s)
	if err != nil {
		// Только причина, без самого адреса. url.Error печатает адрес ЦЕЛИКОМ,
		// то есть вместе с uuid и pbk, а этот текст доезжает до отказа
		// подписки, оттуда в кадр ответа и на экран. Барьер в cmd закрывал лишь
		// один путь из двух, поэтому он стоит здесь, у источника.
		var ue *url.Error
		if errors.As(err, &ue) && ue.Err != nil {
			return nil, "", 0, "", fmt.Errorf("%w: %v", ErrSsylkaKrivaya, ue.Err)
		}
		return nil, "", 0, "", ErrSsylkaKrivaya
	}
	host, port, err := hostPort(u.Host)
	if err != nil {
		return nil, "", 0, "", err
	}
	// Имя снимается PathUnescape, а НЕ QueryUnescape: фрагмент это не строка
	// запроса, и второй превратил бы «NL+2» в «NL 2». Сервер сменил бы имя сам
	// по себе, а если бы имя участвовало в идентификаторе, то и сервер.
	imya, err := url.PathUnescape(u.Fragment)
	if err != nil {
		imya = u.Fragment
	}
	if err := proveritZaglushku(host, imya); err != nil {
		return nil, "", 0, "", err
	}
	return u, host, port, imya, nil
}

// proveritZaglushku отделяет сообщение панели от настоящего сервера.
//
// Обе проверенные 01.09.2026 панели отвечают на ИСТЕКШУЮ подписку кодом 200 и
// синтаксически исправными ссылками, направленными на 0.0.0.0, а человеческий
// текст кладут в имя сервера: «Подписка закончилась», «Продлите подписку», и
// одна из них даже код оплаты.
//
// Без отдельного канала это даёт два одинаково скверных исхода. Либо в списке
// появляются четыре «сервера» с именем «Код оплаты», и человек в них тыкает.
// Либо разбор отвергает их за транспорт (у обеих панелей там security=none), и
// человек идёт искать опечатку в строке, с которой всё в порядке, вместо того
// чтобы заплатить.
//
// Поэтому текст обязан доехать до человека целиком: он и ЕСТЬ ответ панели.
func proveritZaglushku(host, imya string) error {
	ip := net.ParseIP(host)
	// Только неуказанный адрес и петля. Оба означают «сюда подключиться нельзя»
	// вне зависимости от чьих-либо намерений, и настоящим сервером подписки быть
	// не могут. Шире брать нельзя: приватные диапазоны это законный домашний узел.
	if ip == nil || !(ip.IsUnspecified() || ip.IsLoopback()) {
		return nil
	}
	if imya == "" {
		return fmt.Errorf("%w: адрес %s без текста", ErrUvedomleniePodpiski, host)
	}
	return fmt.Errorf("%w: %s", ErrUvedomleniePodpiski, imya)
}

// hostPort разбирает адрес, включая литерал IPv6 в квадратных скобках.
//
// Без разбора скобок «[2001:db8::1]:443» отдаёт хост вместе с частью адреса, и
// сервер молча оказывается недостижимым.
func hostPort(hp string) (string, int, error) {
	host, portStr, err := net.SplitHostPort(hp)
	if err != nil {
		return "", 0, fmt.Errorf("%w: адрес %q: %v", ErrSsylkaKrivaya, hp, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", 0, fmt.Errorf("%w: порт %q не число", ErrSsylkaKrivaya, portStr)
	}
	// Границы настоящие, а не «чужой порт». Любой порт в диапазоне законен, и
	// отличить чужой от своего разбор не может и не должен.
	if port < 1 || port > 65535 {
		return "", 0, fmt.Errorf("%w: порт %d вне 1..65535", ErrSsylkaKrivaya, port)
	}
	if host == "" {
		return "", 0, fmt.Errorf("%w: пустой хост", ErrSsylkaKrivaya)
	}
	return host, port, nil
}

// nebezopasnyy отвечает, стоял ли в ссылке флаг отключения проверки сертификата.
//
// Написаний два, и ловить одно значит пропустить половину панелей.
func nebezopasnyy(q url.Values) bool {
	for _, k := range []string{"insecure", "allowInsecure", "allow_insecure", "skip-cert-verify"} {
		switch strings.ToLower(q.Get(k)) {
		case "1", "true", "yes":
			return true
		}
	}
	return false
}

func vless(s string) (protokol.Server, error) {
	u, host, port, imya, err := razobratURL(s)
	if err != nil {
		return protokol.Server{}, err
	}
	// ParseQuery, а НЕ u.Query(): второй молча выбрасывает значение с битым
	// процентным кодированием, и «path=%zz» доезжает сюда пустым путём. Ссылка
	// тогда разбирается «успешно» и даёт сервер, который не подключится, а
	// причина на экране будет любой, кроме настоящей.
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return protokol.Server{}, fmt.Errorf("%w: параметры: %v", ErrSsylkaKrivaya, err)
	}

	// type=raw это новое имя type=tcp в Xray 26.x. Оба ведут к одному нашему
	// транспорту, и знать только одно значит не разобрать свою же подписку.
	tip := q.Get("type")
	if tip == "" {
		tip = "raw"
	}
	// security решает не меньше, чем type. Наш «reality-tcp» это ИМЕННО reality
	// поверх tcp: пометить любой tcp как reality значило бы построить серверу
	// reality-исходящий, и рукопожатие не состоялось бы никогда, а причина на
	// экране была бы про ключ. Живая подписка содержит и tls, и reality.
	bezopasnost := q.Get("security")
	// TLS есть только при tls или reality. `none`, `false` и отсутствие
	// параметра означают открытый транспорт, и это НЕ придирка: такие серверы
	// живут на портах 80 и 2200 в живых сборниках.
	bezTLS := bezopasnost != "tls" && bezopasnost != "reality"
	var transport string
	switch tip {
	// xhttp сюда НЕ возвращать не подумав: он снят 06.09.2026 вместе с чужим
	// форком ядра, и апстрим его не несёт. Ссылка с ним обязана отвергаться
	// здесь, а не превращаться в запись, которую ядро потом не примет: такая
	// запись уносит ВЕСЬ конфиг, а человеку показывает разговор про исходящий
	// номер ноль. Непонятая строка подписки видна на экране, и это честнее.
	case "raw", "tcp":
		if bezopasnost != "reality" {
			return protokol.Server{}, fmt.Errorf(
				"%w: vless поверх %q без reality", ErrTransportNePodderzhan, tip)
		}
		transport = "reality-tcp"
	case "ws", "grpc":
		transport = tip
	case "httpupgrade":
		// Тот же смысл, что у ws, но без веб-сокетов: одно рукопожатие
		// Upgrade и дальше голый поток. Ядро несёт его без отдельного тега.
		transport = "httpupgrade"
	default:
		return protokol.Server{}, fmt.Errorf("%w: type=%s", ErrTransportNePodderzhan, tip)
	}

	srv := protokol.Server{
		Imya:                    imya,
		Transport:               transport,
		Host:                    host,
		Port:                    port,
		BezTLS:                  bezTLS,
		Uuid:                    u.User.Username(),
		PublicKey:               q.Get("pbk"),
		ShortId:                 q.Get("sid"),
		Flow:                    q.Get("flow"),
		Sni:                     q.Get("sni"),
		Fp:                      q.Get("fp"),
		Alpn:                    q.Get("alpn"),
		HostZagolovka:           q.Get("host"),
		NebezopasnyyIgnorirovan: nebezopasnyy(q),
	}
	if srv.Uuid == "" {
		return protokol.Server{}, fmt.Errorf("%w: нет uuid", ErrSsylkaKrivaya)
	}

	// Путь нужен ws. Битое процентное кодирование ловится здесь, и это
	// ЕДИНСТВЕННОЕ, за что путь может быть отвергнут: валидное кодирование
	// (%2Fws) совершенно нормально и обязано разобраться.
	if p := q.Get("path"); p != "" {
		put, err := url.PathUnescape(p)
		if err != nil {
			return protokol.Server{}, fmt.Errorf("%w: путь %q: %v", ErrSsylkaKrivaya, p, err)
		}
		srv.Put = put
	}
	if transport == "grpc" {
		srv.Put = q.Get("serviceName")
	}

	// Пин это ЗАМЕНА доверию к цепочке, поэтому берётся ровно там, где цепочка
	// проверяется: при security=tls. До 05.09.2026 у vless его не брали вовсе,
	// и всякий самоподписанный сервер был недостижим молча: insecure мы
	// игнорируем намеренно, а пин, который и есть законный способ обойтись без
	// доверенного центра, выбрасывали. Виден отказ был только в журнале ядра,
	// строкой `x509: certificate signed by unknown authority`.
	//
	// У reality пин НЕ берётся, и это не экономия. Сертификат там подставной и
	// живёт одно рукопожатие, сверять его с записанным отпечатком нечему:
	// объявленный пин не совпал бы никогда, то есть рабочий сервер перестал бы
	// работать от лишнего параметра в ссылке.
	if bezopasnost == "tls" {
		if srv.Pin, err = pinIzZaprosa(q); err != nil {
			return protokol.Server{}, err
		}
	}

	// REALITY без публичного ключа не поднимется никогда: отказ здесь, а не
	// «сервер добавился и почему-то не работает».
	if bezopasnost == "reality" {
		if srv.PublicKey == "" {
			return protokol.Server{}, fmt.Errorf("%w: reality без pbk", ErrSsylkaKrivaya)
		}
		if err := proveritKlyuchReality(srv.PublicKey); err != nil {
			return protokol.Server{}, err
		}
	}

	srv.Id = Id(srv.Host, srv.Port, srv.Transport)
	return srv, nil
}

func hysteria(s string) (protokol.Server, error) {
	u, host, port, imya, err := razobratURL(s)
	if err != nil {
		return protokol.Server{}, err
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return protokol.Server{}, fmt.Errorf("%w: параметры: %v", ErrSsylkaKrivaya, err)
	}
	srv := protokol.Server{
		Imya:                    imya,
		Transport:               "hy2",
		Host:                    host,
		Port:                    port,
		Sni:                     q.Get("sni"),
		NebezopasnyyIgnorirovan: nebezopasnyy(q),
	}
	// Пароль у hy2 живёт в userinfo. Он может быть и в поле, и через двоеточие.
	if u.User != nil {
		if p, est := u.User.Password(); est {
			srv.Parol = p
		} else {
			srv.Parol = u.User.Username()
		}
	}
	if srv.Parol == "" {
		return protokol.Server{}, fmt.Errorf("%w: hy2 без пароля", ErrSsylkaKrivaya)
	}
	// Грамматика панелей Hysteria 2: obfs с паролем, mport, pinSHA256.
	// obfs без пароля отвергается здесь, потому что ядро отвергло бы ВЕСЬ
	// конфиг, а с ним и остальные серверы.
	srv.Obfs = q.Get("obfs")
	srv.ObfsParol = q.Get("obfs-password")
	if srv.Obfs != "" && srv.ObfsParol == "" {
		return protokol.Server{}, fmt.Errorf("%w: hy2 с obfs без obfs-password", ErrSsylkaKrivaya)
	}
	srv.Porty = q.Get("mport")
	// Полоса из ссылки. Пара или ничего: см. polosaIzZaprosa.
	srv.PolosaVverh, srv.PolosaVniz = polosaIzZaprosa(q)
	// Пин проверяется ЗДЕСЬ, а не в генераторе конфига и не ядром. Ядро
	// отвергает ВЕСЬ конфиг из-за одного негодного пина, а с ним и остальные
	// серверы: то же соображение, что и у obfs без пароля абзацем выше, только
	// для пина его никто не применил. Замерено 04.09.2026 живым прогоном, где
	// подписка из трёх серверов не подняла ни одного, и человек получил
	// tun-create-failed вместо «одна ссылка негодна».
	//
	// Ломается пин чаще всего процентным кодированием: base64 содержит «+», а
	// «+» в запросе URL это ПРОБЕЛ. Догадываться, что пробел это «+», нельзя:
	// пин это проверка подлинности сервера, и чинить её угадыванием значит
	// проверять не то, что думаешь.
	if srv.Pin, err = pinIzZaprosa(q); err != nil {
		return protokol.Server{}, err
	}
	srv.Id = Id(srv.Host, srv.Port, srv.Transport)
	return srv, nil
}

// anytls разбирает `anytls://пароль@хост:порт?sni=&alpn=`.
//
// Пароль это ВЕСЬ userinfo, и это единственное отличие от tuic, которое стоит
// целого разбора: двоеточие в пароле законно, а `u.User.Password()` уже разрезал
// строку по первому. Склейка обратно возвращает то, что написал сервер.
// Разрезанный пароль дал бы сервер, который добавился, попал в список и отвечает
// отказом проверки подлинности, то есть врёт про причину.
func anytls(s string) (protokol.Server, error) {
	u, host, port, imya, err := razobratURL(s)
	if err != nil {
		return protokol.Server{}, err
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return protokol.Server{}, fmt.Errorf("%w: параметры: %v", ErrSsylkaKrivaya, err)
	}
	srv := protokol.Server{
		Imya:                    imya,
		Transport:               "anytls",
		Host:                    host,
		Port:                    port,
		Parol:                   parolTselikom(u),
		Sni:                     q.Get("sni"),
		Alpn:                    q.Get("alpn"),
		Fp:                      q.Get("fp"),
		NebezopasnyyIgnorirovan: nebezopasnyy(q),
	}
	if srv.Parol == "" {
		return protokol.Server{}, fmt.Errorf("%w: anytls без пароля", ErrSsylkaKrivaya)
	}
	if srv.Pin, err = pinIzZaprosa(q); err != nil {
		return protokol.Server{}, err
	}
	srv.Id = Id(srv.Host, srv.Port, srv.Transport)
	return srv, nil
}

// tuic разбирает `tuic://uuid:пароль@хост:порт?congestion_control=&udp_relay_mode=`.
//
// Здесь userinfo это именно ПАРА, и оба её конца обязательны: ядро без любого из
// них отвергает исходящий, а вместе с ним и весь конфиг, то есть и остальные
// серверы подписки. Тот же довод, по которому здесь же отвергается hy2 без
// пароля и негодный пин.
func tuic(s string) (protokol.Server, error) {
	u, host, port, imya, err := razobratURL(s)
	if err != nil {
		return protokol.Server{}, err
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return protokol.Server{}, fmt.Errorf("%w: параметры: %v", ErrSsylkaKrivaya, err)
	}
	srv := protokol.Server{
		Imya:      imya,
		Transport: "tuic",
		Host:      host,
		Port:      port,
		Sni:       q.Get("sni"),
		Alpn:      q.Get("alpn"),
		Fp:        q.Get("fp"),
		// Управление перегрузкой и режим передачи UDP приезжают ИЗ ССЫЛКИ:
		// умолчание подставляет ядро, а не мы. Своё значение здесь было бы
		// сменой протокола под тем же именем, ровно как подстановка режима
		// xhttp, разобранная 05.09.2026.
		Peregruzka:              q.Get("congestion_control"),
		RezhimUDP:               q.Get("udp_relay_mode"),
		NebezopasnyyIgnorirovan: nebezopasnyy(q),
	}
	if u.User != nil {
		srv.Uuid = u.User.Username()
		if p, est := u.User.Password(); est {
			srv.Parol = p
		}
	}
	if srv.Uuid == "" || srv.Parol == "" {
		return protokol.Server{}, fmt.Errorf("%w: tuic без uuid или без пароля", ErrSsylkaKrivaya)
	}
	if srv.Pin, err = pinIzZaprosa(q); err != nil {
		return protokol.Server{}, err
	}
	srv.Id = Id(srv.Host, srv.Port, srv.Transport)
	return srv, nil
}

// parolTselikom собирает userinfo обратно в одну строку.
//
// `url.Userinfo` режет по первому двоеточию всегда, потому что для http там
// имя и пароль. Для протоколов, где userinfo это ОДИН секрет, разрез это порча
// ключа, и молчаливая.
func parolTselikom(u *url.URL) string {
	if u.User == nil {
		return ""
	}
	if p, est := u.User.Password(); est {
		return u.User.Username() + ":" + p
	}
	return u.User.Username()
}

// pinIzZaprosa разбирает pinSHA256 и приводит его к написанию, которое понимает
// ядро. Пустой параметр это пустой пин, а не отказ.
//
// Вынесено из hysteria 05.09.2026, когда пин понадобился второму и третьему
// протоколу. Причина отказа здесь та же, что была там: ядро отвергает ВЕСЬ
// конфиг из-за одного негодного пина, а с ним и рабочие серверы.
//
// Пинов в ссылке ДВА, и они не дубль друг друга (06.09.2026). `pinSHA256` по
// спецификации Hysteria 2 это hex отпечатка СЕРТИФИКАТА, его понимают чужие
// клиенты и не понимает наше ядро. Хеш публичного ключа наш сборщик кладёт
// отдельно, в `pinPubKeySHA256`. Разобрать отпечаток как base64 значит получить
// 48 байт вместо 32 и уронить всю ссылку.
func pinIzZaprosa(q url.Values) (string, error) {
	p := q.Get("pinPubKeySHA256")
	if p == "" {
		p = q.Get("pinSHA256")
	}
	if p == "" {
		return "", nil
	}
	// Отпечаток сертификата пропускаем молча: он адресован не нам, а ссылка от
	// его присутствия годной быть не перестаёт.
	if otpechatokSertifikata(p) {
		return "", nil
	}
	b, ok := dekodirovat(p)
	if !ok {
		return "", fmt.Errorf("%w: pinSHA256 не разбирается как base64", ErrSsylkaKrivaya)
	}
	if len(b) != sha256.Size {
		return "", fmt.Errorf("%w: pinSHA256 длиной %d байт, а sha256 это %d",
			ErrSsylkaKrivaya, len(b), sha256.Size)
	}
	// В ядро пин уезжает СТАНДАРТНЫМ написанием, каким бы ни пришёл: sing-box
	// другого не понимает, а панели пишут и url-safe, и без выравнивания.
	return base64.StdEncoding.EncodeToString(b), nil
}

// otpechatokSertifikata узнаёт hex отпечатка сертификата: ровно 64 знака, все
// шестнадцатеричные. Такая строка проходит и как base64 (алфавиты пересекаются),
// поэтому проверять её надо ДО раскодирования, иначе она молча станет 48
// байтами негодного пина.
func otpechatokSertifikata(s string) bool {
	if len(s) != sha256.Size*2 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}

// dekodirovat снимает base64 в любом из четырёх написаний.
//
// Панели расходятся во всём: обычный алфавит против url-safe, с выравниванием и
// без. Знать одно написание значит объявлять исправные ссылки битыми.
func dekodirovat(s string) ([]byte, bool) {
	for _, k := range []*base64.Encoding{
		base64.StdEncoding, base64.RawStdEncoding,
		base64.URLEncoding, base64.RawURLEncoding,
	} {
		if b, err := k.DecodeString(s); err == nil {
			return b, true
		}
	}
	return nil, false
}

// shadowsocks разбирает ВСЕ три формы.
//
// Разбор под одну форму на второй не падает, а тихо раскладывает мусор по
// Metod и Parol. Сервер попадает в список, не подключается, и на экране
// server-auth-failed: человека посылают проверять ключ, который разбор сам и
// испортил. Это худший вид ошибки, потому что он врёт про причину.
func shadowsocks(s string) (protokol.Server, error) {
	telo := strings.TrimPrefix(s, "ss://")
	telo, imyaSyroe, _ := strings.Cut(telo, "#")
	imya, err := url.PathUnescape(imyaSyroe)
	if err != nil {
		imya = imyaSyroe
	}
	// Плагин и прочие параметры SIP002 нам не нужны, но мешать разбору не должны.
	telo, _, _ = strings.Cut(telo, "?")
	telo = strings.TrimSuffix(telo, "/")

	var metodParol, adres string
	if userinfo, hostPortStr, est := strings.Cut(telo, "@"); est {
		// Legacy и SIP002: userinfo закодирован, адрес открытым текстом.
		adres = hostPortStr
		if b, ok := dekodirovat(userinfo); ok {
			metodParol = string(b)
		} else {
			metodParol = userinfo
		}
	} else {
		// Целиком закодированная: внутри «метод:пароль@хост:порт».
		b, ok := dekodirovat(telo)
		if !ok {
			return protokol.Server{}, fmt.Errorf("%w: ss не декодируется", ErrSsylkaKrivaya)
		}
		mp, hp, est := strings.Cut(string(b), "@")
		if !est {
			return protokol.Server{}, fmt.Errorf("%w: ss без адреса", ErrSsylkaKrivaya)
		}
		metodParol, adres = mp, hp
	}

	metod, parol, est := strings.Cut(metodParol, ":")
	if !est || metod == "" || parol == "" {
		return protokol.Server{}, fmt.Errorf("%w: ss без метода или пароля", ErrSsylkaKrivaya)
	}
	host, port, err := hostPort(adres)
	if err != nil {
		return protokol.Server{}, err
	}
	// ss через razobratURL не проходит, значит заглушку проверяем отдельно.
	if err := proveritZaglushku(host, imya); err != nil {
		return protokol.Server{}, err
	}

	srv := protokol.Server{
		Imya:      imya,
		Transport: "ss",
		Host:      host,
		Port:      port,
		Metod:     metod,
		Parol:     parol,
	}
	srv.Id = Id(srv.Host, srv.Port, srv.Transport)
	return srv, nil
}

// proveritKlyuchReality отвергает ключ, который ядро всё равно не примет.
//
// Найдено живым прогоном на стенде 01.09.2026: ссылка с pbk=aaaa прошла
// добавление, а ядро отказалось стартовать ЦЕЛИКОМ со словами
// «initialize outbound[1]: invalid public_key». Конфиг собирается из ВСЕХ
// серверов, поэтому один негодный ключ уносит и рабочие, а человек видит
// «TUN-адаптер не появился» и ищет причину в чём угодно, кроме своей ссылки.
//
// Проверяется ровно то, что проверяет ядро: 32 байта в base64url без набивки.
// Гадать про содержимое точки кривой мы не беремся, это работа ядра.
func proveritKlyuchReality(k string) error {
	syroe, err := base64.RawURLEncoding.DecodeString(k)
	if err != nil {
		return fmt.Errorf("%w: pbk не base64url", ErrSsylkaKrivaya)
	}
	if len(syroe) != 32 {
		return fmt.Errorf("%w: pbk длиной %d байт вместо 32", ErrSsylkaKrivaya, len(syroe))
	}
	return nil
}

// polosaIzZaprosa читает upmbps и downmbps, Мбит. Пара или ничего.
//
// У hysteria2 объявление полосы это ЕДИНСТВЕННЫЙ переключатель Brutal,
// отдельного флага нет. Отсюда три решения, и каждое куплено:
//
//	пара или ничего  — up_mbps без down_mbps даёт Brutal с неизвестной
//	                   половиной канала, а достроить вторую догадкой значит
//	                   объявить серверу выдуманное число;
//	ноль это не число — нулевая полоса тоже объявленная полоса, и она включила
//	                   бы Brutal с нулевой оценкой канала;
//	мусор молча мимо  — ссылку пишет чужая панель, и валить из-за неразборного
//	                   параметра ВЕСЬ сервер значит потерять рабочий узел
//	                   ради необязательной настройки.
func polosaIzZaprosa(q url.Values) (int, int) {
	chislo := func(imya string) int {
		s := q.Get(imya)
		if s == "" {
			return 0
		}
		n, err := strconv.Atoi(s)
		if err != nil || n <= 0 {
			return 0
		}
		return n
	}
	vverh, vniz := chislo("upmbps"), chislo("downmbps")
	if vverh == 0 || vniz == 0 {
		return 0, 0
	}
	return vverh, vniz
}
