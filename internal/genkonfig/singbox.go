package genkonfig

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"strings"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Теги. Строки живут в одном месте, потому что ссылка на несуществующий тег это
// ровно та ошибка, которую sing-box check не находит вовсе (проверено пятью
// прогонами: висячий final, outbound, detour, server и rule_set проходят с
// кодом 0 и без единого слова).
const (
	TegPryamo  = "direct"
	TegTunnel  = "tunnel"
	TegMestnyy = "mestnyy"

	// Тег входящего прокси. Отдельная константа, потому что на теги смотрит
	// проверка висячих ссылок, а строка в двух местах разъезжается молча.
	TegProksiVhod = "proksi-in"
)

// SingBox собирает конфиг sing-box целиком.
func SingBox(v Vhod) ([]byte, error) {
	if err := v.proverit(); err != nil {
		return nil, err
	}
	if err := sveritKandidatov(v); err != nil {
		return nil, err
	}
	kandidaty := v.kandidaty()
	ishodyashchie := make([]any, 0, len(kandidaty)+3)
	tegi := make([]string, 0, len(kandidaty))
	vybrannyy := ""
	for _, s := range kandidaty {
		// Кандидат с транспортом, которого ядро не несёт, ПРОПУСКАЕТСЯ, а не
		// роняет сборку. Сборка идёт одним куском, и отказ здесь означал бы
		// клиент, который не поднимается вовсе из-за одной чужой записи в
		// списке. Так уже было 02.09.2026 с чужой подпиской, и наружу это
		// приезжало как «TUN-адаптер не появился».
		//
		// Выбранный человеком сервер под это правило НЕ попадает: его проверяет
		// proverit() выше и отказывает по имени транспорта. Молча увести трафик
		// на соседнюю запись значило бы показать одно, а сделать другое.
		if !Izvestnyy(s.Transport) {
			continue
		}
		teg := TegKandidata(s.Id)
		o, err := ishodyashchiy(v, s, teg)
		if err != nil {
			return nil, err
		}
		ishodyashchie = append(ishodyashchie, o)
		tegi = append(tegi, teg)
		if s.Id == v.Server.Id {
			vybrannyy = teg
		}
	}
	// Пустой список кандидатов это не «конфиг без серверов», а отказ. Селектор
	// без единого исходящего ядро принимает молча, туннель при этом не несёт
	// ничего, и сказать человеку было бы нечего.
	if len(tegi) == 0 {
		return nil, fmt.Errorf("%w: ни один сервер набора этому ядру не годится", ErrTransport)
	}
	ishodyashchie = append(ishodyashchie, gruppy(v, tegi, vybrannyy)...)
	// domain_resolver это не только «прямой трафик резолвится местным»: без
	// единой опции sing-box 1.14 считает прямой исходящий ПУСТЫМ и отвергает
	// detour на него («detour to an empty direct outbound makes no sense»),
	// то есть загрузка наборов через http_client молча не работала с первого
	// дня (замерено в госте 03.09.2026: набор ru ни разу не обновился).
	ishodyashchie = append(ishodyashchie, map[string]any{"type": "direct", "tag": TegPryamo, "domain_resolver": TegMestnyy})

	marshrut := map[string]any{
		"auto_detect_interface":   true,
		"default_domain_resolver": map[string]any{"server": TegMestnyy},
		// Селектор, а не ядро: с появлением нескольких кандидатов «ядра»
		// как единственного выхода больше нет.
		"final": trafikFinal(v),
		"rules": pravila(v),
	}
	if r := razdelNaborov(v); r != nil {
		marshrut["rule_set"] = r
	}
	eksperimentalnoe := map[string]any{
		"clash_api": map[string]any{
			"external_controller": fmt.Sprintf("%s:%d", v.ClashApi.Adres, v.ClashApi.Port),
			"secret":              v.ClashApi.Sekret,
		},
	}
	if kesh, est := keshFayl(v); est {
		eksperimentalnoe["cache_file"] = kesh
	}

	k := map[string]any{
		"log": map[string]any{"level": "warn", "timestamp": true},
		"dns": map[string]any{
			// Резолвер туннеля намеренно публичный: запрос к нему уходит внутрь
			// туннеля и потому не выдаёт нас домашнему провайдеру. Местный нужен
			// только для имён, которых в интернете нет.
			"servers": []any{
				map[string]any{"type": "udp", "tag": TegTunnel, "server": "1.1.1.1", "detour": TegSelector},
				map[string]any{"type": "udp", "tag": TegMestnyy, "server": v.Resolver.String()},
			},
			"rules":    dnsPravila(v),
			"final":    trafikDNSFinal(v),
			"strategy": "ipv4_only",
		},
		"inbounds":     vhodyashchie(v),
		"outbounds":    ishodyashchie,
		"route":        marshrut,
		"experimental": eksperimentalnoe,
	}

	if err := sveritTegi(k); err != nil {
		return nil, err
	}
	return json.MarshalIndent(k, "", "  ")
}

// vhodyashchie: TUN всегда, локальный прокси по требованию.
//
// mixed, а не отдельные http и socks: один слушатель закрывает оба протокола,
// а два порта это два способа промахнуться настройкой. Слушает СТРОГО петлю:
// на 0.0.0.0 это открытый прокси для всей подсети, то есть чужой трафик под
// нашим адресом.
func vhodyashchie(v Vhod) []any {
	vh := []any{
		map[string]any{
			"type": "tun", "tag": "tun-in",
			"address":    []string{v.adresTun()},
			"auto_route": true, "strict_route": true,
			// mixed это умолчание самого sing-box: системный TCP плюс gvisor
			// для UDP. Здесь до 05.09.2026 стоял gvisor, без записанной
			// причины, и замер показал цену: 35.2 Мбит против 340.0 на том же
			// канале, пять кругов вперемежку. Причём не из-за процессора,
			// gvisor тратил 10% против 79% у mixed, то есть просто не тянул.
			"stack": "mixed", "dns_mode": "hijack",
		},
	}
	if v.PortProksi > 0 {
		vh = append(vh, map[string]any{
			"type": "mixed", "tag": TegProksiVhod,
			"listen": "127.0.0.1", "listen_port": v.PortProksi,
		})
	}
	return vh
}

func (v Vhod) adresTun() string {
	if v.AdresTun != "" {
		return v.AdresTun
	}
	return "172.19.0.1/30"
}

// pravila это весь смысл пакета. Порядок менялся дважды, и оба раза check
// пропускал ошибку молча:
//
//	редакция 1: hijack-dns выше обходов  -> DNS службы уезжал в несуществующий
//	                                        туннель, холодный старт мёртв;
//	редакция 5: ВСЕ обходы выше hijack   -> ip_is_private съедал запросы к
//	                                        домашнему резолверу, DNS мимо
//	                                        туннеля всегда и открытым текстом.
//
// Верный порядок: наши процессы и адреса кандидатов выше, hijack-dns следом,
// ip_is_private ниже всех.
func pravila(v Vhod) []any {
	p := []any{
		map[string]any{"action": "sniff"},
		// Только для TUN-входа. Через mixed-вход sing-box тоже узнаёт процесс,
		// и без ограничения наш же affory-svc.exe, постучавшийся в прокси,
		// чтобы выйти ЧЕРЕЗ туннель (checkExitIp), уезжал напрямую. Найдено
		// живьём 03.09.2026: проверка утечек обвинила туннель, а виновато
		// было это правило.
		map[string]any{"inbound": []string{"tun-in"}, "process_path": v.PutiProtsessov, "outbound": TegPryamo},
		map[string]any{"ip_cidr": setiKandidatov(v.Kandidaty), "outbound": TegPryamo},
		map[string]any{"ip_cidr": []string{"224.0.0.0/4", "255.255.255.255/32"}, "outbound": TegPryamo},
		map[string]any{
			"type": "logical", "mode": "or",
			"rules": []any{
				map[string]any{"protocol": "dns"},
				map[string]any{"port": 53},
			},
			"action": "hijack-dns",
		},
	}
	// В режиме "весь трафик" отменяются УДОБНЫЕ исключения, а не петлевые.
	if v.Trafik != nil {
		// Explicit proxy traffic requests the VPN even in selective mode.
		if v.PortProksi > 0 {
			p = append(p, map[string]any{"inbound": []string{TegProksiVhod}, "outbound": TegSelector})
		}
		p = append(p, trafikPravila(v, false)...)
		if !v.VesTrafik {
			p = append(p, map[string]any{"ip_is_private": true, "outbound": TegPryamo})
		}
		if pn, est := praviloNaborov(v); est {
			p = append(p, pn)
		}
		return p
	}
	// Петлевые выше по списку и остаются всегда: без них туннель съедает сам
	// себя, и это отказ другого класса, чем неработающий принтер.
	if !v.VesTrafik {
		p = append(p, map[string]any{"ip_is_private": true, "outbound": TegPryamo})
	}
	// Исключения по процессам и домены из наборов: тоже удобные исключения,
	// оба помощника сами молчат в режиме «весь трафик».
	if pp, est := praviloProtsessov(v); est {
		p = append(p, pp)
	}
	if pd, est := praviloDomenov(v); est {
		p = append(p, pd)
	}
	if pn, est := praviloNaborov(v); est {
		p = append(p, pn)
	}
	return p
}

// praviloDomenov: домен и его поддомены мимо туннеля. domain_suffix, а не
// domain: человек пишет «example.org» и ждёт, что cdn.example.org пойдёт туда
// же. Правило совпадает по имени из sniff (первое правило списка), поэтому
// стоит после него и ниже hijack-dns, как всякое удобное исключение.
func praviloDomenov(v Vhod) (map[string]any, bool) {
	if len(v.Domeny) == 0 || v.VesTrafik {
		return nil, false
	}
	return map[string]any{"domain_suffix": v.Domeny, "outbound": TegPryamo}, true
}

// praviloProtsessov это исключения человека, ОТДЕЛЬНО от правила по нашим
// процессам выше hijack-dns: то держит петлю и стоит всегда, это удобство и
// стоит ниже. process_path, а не process_name: имя ловит любой steam.exe на
// машине, включая чужой, а подмена одноимённого процесса это ровно тот случай,
// ради которого правило и ставится.
func praviloProtsessov(v Vhod) (map[string]any, bool) {
	if len(v.Protsessy) == 0 || v.VesTrafik {
		return nil, false
	}
	return map[string]any{"process_path": v.Protsessy, "outbound": TegPryamo}, true
}

func setiKandidatov(a []netip.Addr) []string {
	s := make([]string, 0, len(a))
	for _, k := range a {
		bit := 32
		if k.Is6() {
			bit = 128
		}
		s = append(s, fmt.Sprintf("%s/%d", k.String(), bit))
	}
	return s
}

// ishodyashchiy строит исходящий под транспорт КОНКРЕТНОГО сервера.
//
// Тег передаётся снаружи, а не берётся из константы: один тег на всех означал
// бы, что два кандидата схлопнутся в селекторе в один, причём молча.
func ishodyashchiy(v Vhod, s protokol.Server, teg string) (map[string]any, error) {
	sni := s.Sni
	if sni == "" {
		sni = s.Host
	}
	// ALPN и заголовок Host приезжают ИЗ ССЫЛКИ: разбор их вынимал с самого
	// начала, а сюда они не доезжали.
	//
	// С отпечатком сложнее, и это ЗАМЕР, а не осторожность. 02.09.2026 на стенде
	// против своего сервера: firefox не понёс ни разу из трёх, chrome понёс два
	// раза из трёх. Ссылка при этом собрана из РАБОЧЕГО конфига Xray, где
	// firefox несёт, то есть сервер отпечатка не требует, а ломается своя сборка
	// ядра. `sing-box check` такой конфиг принимает молча, поэтому отказ
	// выглядит как «сервер не отвечает».
	//
	// Отсюда правило: берём отпечаток из ссылки только из ПРОВЕРЕННЫХ, остальные
	// сводим к chrome. Список расширяется замером, а не догадкой.
	otpechatok := otpechatokIliChrome(s.Fp)

	switch s.Transport {
	case "reality-tcp":
		o := vless(s, teg)
		o["tls"] = map[string]any{
			"enabled": true, "server_name": sni,
			"reality": map[string]any{
				"enabled": true, "public_key": s.PublicKey, "short_id": s.ShortId,
			},
			"utls": map[string]any{"enabled": true, "fingerprint": otpechatok},
		}
		dobavitAlpn(o, s)
		return o, nil

	case "ws":
		o := vless(s, teg)
		// Открытый ws это не экзотика: в живом сборнике таких шесть из 145, на
		// портах 80 и 2200. См. protokol.Server.BezTLS.
		if !s.BezTLS {
			o["tls"] = tlsSPinom(s, sni)
			dobavitUtls(o, s)
		}
		dobavitAlpn(o, s)
		tr := map[string]any{"type": "ws", "path": putIli(s.Put, "/")}
		dobavitHost(tr, s)
		o["transport"] = tr
		return o, nil

	case "httpupgrade":
		// Тот же смысл, что ws, но без веб-сокетов: одно рукопожатие Upgrade,
		// дальше голый поток. Ядро несёт его без отдельного тега сборки.
		o := vless(s, teg)
		if !s.BezTLS {
			o["tls"] = tlsSPinom(s, sni)
			dobavitUtls(o, s)
		}
		dobavitAlpn(o, s)
		tr := map[string]any{"type": "httpupgrade", "path": putIli(s.Put, "/")}
		// У httpupgrade ядро ждёт host отдельным полем, а не только в headers.
		if s.HostZagolovka != "" {
			tr["host"] = s.HostZagolovka
		}
		dobavitHost(tr, s)
		o["transport"] = tr
		return o, nil

	case "trojan", "trojan-ws":
		// TLS у trojan есть всегда: протокол только им и прикрыт. Поэтому
		// здесь нет ветки BezTLS, в отличие от ws и grpc у vless.
		o := map[string]any{
			"type": "trojan", "tag": teg,
			"server": s.Host, "server_port": s.Port, "password": s.Parol,
			"tls": tlsSPinom(s, sni),
		}
		dobavitUtls(o, s)
		dobavitAlpn(o, s)
		if s.Transport == "trojan-ws" {
			tr := map[string]any{"type": "ws", "path": putIli(s.Put, "/")}
			dobavitHost(tr, s)
			o["transport"] = tr
		}
		return o, nil

	case "vmess", "vmess-ws":
		// security ставится ЗДЕСЬ, а не разбором: ссылка вправе его не
		// называть, а ядру нужно значение. auto это то, что подставляют все
		// клиенты, и оно же единственное безопасное умолчание.
		o := map[string]any{
			"type": "vmess", "tag": teg,
			"server": s.Host, "server_port": s.Port, "uuid": s.Uuid,
			"security": putIli(s.Shifr, "auto"),
		}
		// alter_id пишется только ненулевым: ноль это современный режим, и
		// явный ноль в конфиге ядро понимает так же, но читателя путает.
		if s.AlterId > 0 {
			o["alter_id"] = s.AlterId
		}
		if !s.BezTLS {
			o["tls"] = tlsSPinom(s, sni)
			dobavitUtls(o, s)
			dobavitAlpn(o, s)
		}
		if s.Transport == "vmess-ws" {
			tr := map[string]any{"type": "ws", "path": putIli(s.Put, "/")}
			dobavitHost(tr, s)
			o["transport"] = tr
		}
		return o, nil

	case "grpc":
		o := vless(s, teg)
		if !s.BezTLS {
			o["tls"] = tlsSPinom(s, sni)
			dobavitUtls(o, s)
		}
		dobavitAlpn(o, s)
		o["transport"] = map[string]any{"type": "grpc", "service_name": putIli(s.Put, "gun")}
		return o, nil

	case "hy2":
		o := map[string]any{
			"type": "hysteria2", "tag": teg,
			"server": s.Host, "password": s.Parol,
			// Пин публичного ключа: проверка ПОВЕРХ цепочки, не вместо неё.
			"tls": tlsSPinom(s, sni),
		}
		if s.Porty != "" {
			// server_ports и server_port взаимно исключают друг друга у ядра.
			// Ссылка пишет «от-до», ядро ждёт «от:до».
			o["server_ports"] = portyHy2(s.Porty)
		} else {
			o["server_port"] = s.Port
		}
		if s.Obfs != "" {
			o["obfs"] = map[string]any{"type": s.Obfs, "password": s.ObfsParol}
		}
		// Объявление полосы это переключатель Brutal, а не ограничитель:
		// отдельного флага у протокола нет. Пустые поля означают BBR, поэтому
		// неизмеренная полоса НЕ пишется, а не пишется нулём.
		//
		// Источников два, и настройка сильнее ссылки. Ссылку пишет держатель
		// сервера, и его число это оценка со стороны; настройку ставит человек
		// про свой домашний канал, а Brutal управляется именно им.
		if vverh, vniz := polosaDlya(v, s); vverh > 0 && vniz > 0 {
			o["up_mbps"] = vverh
			o["down_mbps"] = vniz
		}
		dobavitAlpn(o, s)
		return o, nil

	case "anytls":
		// TLS есть всегда: имя протокола это и есть его обёртка, ветки BezTLS
		// здесь нет по той же причине, что у trojan. Uuid не пишется вовсе:
		// у anytls его не существует, и пустое поле ядро отвергло бы.
		o := map[string]any{
			"type": "anytls", "tag": teg,
			"server": s.Host, "server_port": s.Port, "password": s.Parol,
			"tls": tlsSPinom(s, sni),
		}
		dobavitUtls(o, s)
		dobavitAlpn(o, s)
		return o, nil

	case "tuic":
		o := map[string]any{
			"type": "tuic", "tag": teg,
			"server": s.Host, "server_port": s.Port,
			"uuid": s.Uuid, "password": s.Parol,
			"tls": tlsSPinom(s, sni),
		}
		// Оба поля пишутся ТОЛЬКО когда их назвала ссылка. Умолчание у ядра своё
		// (`cubic` и `native`), и подставлять вместо него своё значит менять
		// протокол под тем же именем. Пустая строка при этом не безобидна: ядро
		// отвергает неизвестное значение и роняет весь конфиг.
		if s.Peregruzka != "" {
			o["congestion_control"] = s.Peregruzka
		}
		if s.RezhimUDP != "" {
			o["udp_relay_mode"] = s.RezhimUDP
		}
		dobavitUtls(o, s)
		dobavitAlpn(o, s)
		return o, nil

	case "ss":
		return map[string]any{
			"type": "shadowsocks", "tag": teg,
			"server": s.Host, "server_port": s.Port,
			"method": s.Metod, "password": s.Parol,
		}, nil
	}
	return nil, fmt.Errorf("%w: %s", ErrTransport, s.Transport)
}

// portyHy2 переводит mport ссылки («20000-21000,443») в server_ports ядра
// («20000:21000», «443:443»). Пробелы вокруг запятых панели тоже ставят.
//
// Одиночный порт обязан стать диапазоном из самого себя. Числом ядро его не
// принимает: «bad port range: 443», и падает ВЕСЬ конфиг, а вместе с ним и
// исправные серверы рядом. Найдено 03.09.2026 прогоном инварианта 8 настоящим
// ядром: обычный go test без ядра эти профили пропускает и печатает ok.
func portyHy2(mport string) []string {
	var itog []string
	for _, ch := range strings.Split(mport, ",") {
		ch = strings.TrimSpace(ch)
		if ch == "" {
			continue
		}
		ch = strings.Replace(ch, "-", ":", 1)
		if !strings.Contains(ch, ":") {
			ch += ":" + ch
		}
		itog = append(itog, ch)
	}
	return itog
}

func vless(s protokol.Server, teg string) map[string]any {
	o := map[string]any{
		"type": "vless", "tag": teg,
		"server": s.Host, "server_port": s.Port, "uuid": s.Uuid,
	}
	// Flow только там, где он существует. `xtls-rprx-vision` работает поверх
	// ГОЛОГО TCP под TLS или REALITY, и больше нигде: «XTLS only supports TLS
	// and REALITY directly» в исходниках Xray. Любой транспорт поверх (ws, grpc,
	// httpupgrade, xhttp) его исключает.
	//
	// Чужая ссылка с такой парой не отвергается: сервер с flow над ws невозможен
	// в принципе, значит это ошибка панели, а не негодный сервер. Игнорируем,
	// как игнорируем insecure. Отвергать было бы хуже: сервер рабочий.
	//
	// Проверить это ядром нельзя: `sing-box check` пару принимает молча, конфиг
	// стартует, трафика нет.
	if s.Flow != "" && flowVozmozhen(s.Transport) {
		o["flow"] = s.Flow
	}
	return o
}

// flowVozmozhen отвечает, бывает ли у транспорта XTLS-flow.
//
// Список от РАЗРЕШЁННОГО, а не от запрещённого: новый транспорт по умолчанию
// оказывается без flow, и это верная сторона для ошибки. Обратный список молча
// раздал бы flow всему, что добавят потом.
func flowVozmozhen(transport string) bool { return transport == "reality-tcp" }

// Отпечатки, проверенные живым подъёмом. Пополнять ТОЛЬКО прогоном на стенде:
// негодный отпечаток не отвергается ядром, а тихо не несёт трафик.
var proverennyeOtpechatki = map[string]bool{"chrome": true}

func otpechatokIliChrome(fp string) string {
	if proverennyeOtpechatki[fp] {
		return fp
	}
	return "chrome"
}

// dobavitAlpn кладёт список ТОЛЬКО когда он задан. Пустой список ALPN это не
// то же самое, что его отсутствие: сервер будет выбирать по нему и не найдёт
// ничего.
func dobavitAlpn(o map[string]any, s protokol.Server) {
	if s.Alpn == "" {
		return
	}
	spisok := make([]string, 0, 2)
	for _, ch := range strings.Split(s.Alpn, ",") {
		if ch = strings.TrimSpace(ch); ch != "" {
			spisok = append(spisok, ch)
		}
	}
	if len(spisok) == 0 {
		return
	}
	tls, est := o["tls"].(map[string]any)
	if !est {
		return
	}
	tls["alpn"] = spisok
}

// dobavitHost нужен фронту: без заголовка Host он вернёт чужую страницу вместо
// туннеля, и выглядеть это будет как молчащий сервер.
func dobavitHost(tr map[string]any, s protokol.Server) {
	if s.HostZagolovka == "" {
		return
	}
	tr["headers"] = map[string]any{"Host": s.HostZagolovka}
}

// dobavitUtls только по явной просьбе ссылки. Для транспортов без REALITY
// отпечаток не обязателен, а включённый без спроса utls меняет рукопожатие там,
// где его никто не просил менять.
// tlsSPinom это блок TLS для ВСЕХ протоколов, кроме reality. Пин добавляется
// только когда он есть, и он ПОВЕРХ цепочки, а не вместо неё: insecure остаётся
// выключенным всегда.
//
// Вынесено 05.09.2026, когда третий протокол подряд начал собирать один и тот же
// блок руками. Три копии разошлись бы на первом же изменении, причём молча:
// забытый пин это не отказ ядра, а потерянная проверка подлинности. В тот же
// день это и случилось: ws, grpc, httpupgrade, xhttp, trojan и vmess собирали
// блок своими литералами, и пин до ядра не доезжал ни у одного из шести.
//
// reality сюда НЕ переведён намеренно. Сертификат там подставной и живёт одно
// рукопожатие, сверять его с записанным отпечатком нечему: объявленный пин не
// совпал бы никогда, то есть лишний параметр в ссылке ломал бы рабочий сервер.
func tlsSPinom(s protokol.Server, sni string) map[string]any {
	tls := map[string]any{"enabled": true, "server_name": sni}
	if s.Pin != "" {
		tls["certificate_public_key_sha256"] = []string{s.Pin}
	}
	return tls
}

func dobavitUtls(o map[string]any, s protokol.Server) {
	if s.Fp == "" {
		return
	}
	tls, est := o["tls"].(map[string]any)
	if !est {
		return
	}
	tls["utls"] = map[string]any{"enabled": true, "fingerprint": otpechatokIliChrome(s.Fp)}
}

func putIli(v, poumolchaniyu string) string {
	if v == "" {
		return poumolchaniyu
	}
	return v
}

// dnsPravila: имена, которых в интернете нет, местным резолвером всегда; а
// домены, идущие МИМО туннеля (исключения человека и наборы), тоже местным,
// иначе российский сайт получает ответ из Нидерландов и голландский узел CDN,
// и прямой путь оказывается медленнее, чем без VPN. Вторая пара правил это
// удобные исключения: в режиме «весь трафик» их нет, как и самих обходов.
func dnsPravila(v Vhod) []any {
	p := []any{
		map[string]any{"domain_suffix": []string{".local", ".lan", ".home.arpa"}, "server": TegMestnyy},
		map[string]any{"domain_regex": []string{"^[^.]+$"}, "server": TegMestnyy},
	}
	if v.VesTrafik {
		return p
	}
	if v.Trafik != nil {
		p = append(p, trafikPravila(v, true)...)
	} else if len(v.Domeny) > 0 {
		p = append(p, map[string]any{"domain_suffix": v.Domeny, "server": TegMestnyy})
	}
	if len(v.Nabory) > 0 {
		tegi := make([]string, 0, len(v.Nabory))
		for _, n := range v.Nabory {
			tegi = append(tegi, n.Teg)
		}
		p = append(p, map[string]any{"rule_set": tegi, "server": TegMestnyy})
	}
	return p
}

// polosaDlya выбирает, чью полосу объявлять серверу hysteria2.
//
// Настройка человека сильнее ссылки, и это не вкусовщина. Brutal шлёт с
// объявленной скоростью, не глядя на потери, поэтому число обязано описывать
// канал ТОГО, кто шлёт, то есть домашний канал человека. Ссылка знает лишь то,
// что вписал держатель сервера.
//
// Пара проверяется у каждого источника отдельно: смешивать «вверх из настройки,
// вниз из ссылки» нельзя, получилось бы объявление, которого не делал никто.
func polosaDlya(v Vhod, s protokol.Server) (int, int) {
	if v.PolosaVverh > 0 && v.PolosaVniz > 0 {
		return v.PolosaVverh, v.PolosaVniz
	}
	if s.PolosaVverh > 0 && s.PolosaVniz > 0 {
		return s.PolosaVverh, s.PolosaVniz
	}
	return 0, 0
}
