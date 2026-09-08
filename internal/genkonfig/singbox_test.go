package genkonfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

func obraztsovyyVhod() Vhod {
	return Vhod{
		Server: protokol.Server{
			Id: "nl", Imya: "Нидерланды", Transport: "reality-tcp",
			Host: "192.0.2.225", Port: 443,
			Uuid: "11111111-2222-3333-4444-555555555555",
			// Ключи reality обязательны: ядро проверяет ключ формально.
			//
			// Образец был на xhttp до 06.09.2026, и от него зависели почти все
			// тесты пакета. Замена на reality-tcp это не вкусовщина: xhttp снят
			// вместе с чужим форком ядра, а образец обязан быть тем, что
			// продукт действительно несёт.
			PublicKey: "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8",
			ShortId:   "0123456789abcdef",
			Flow:      "xtls-rprx-vision",
		},
		Kandidaty: []netip.Addr{
			netip.MustParseAddr("192.0.2.225"),
			netip.MustParseAddr("203.0.113.7"),
		},
		Resolver: netip.MustParseAddr("10.7.0.1"),
		PutiProtsessov: []string{
			`C:\Program Files\Affory\sing-box.exe`,
			`C:\Program Files\Affory\affory-svc.exe`,
		},
		ClashApi: ClashApi{Adres: "127.0.0.1", Port: 9090, Sekret: "s3kr3t"},
	}
}

// A multi-value call can only be expanded when it is the sole argument, so the
// helper takes the input and does the generating itself.
func sobrat(t *testing.T, v Vhod) map[string]any {
	t.Helper()
	b, err := SingBox(v)
	if err != nil {
		t.Fatal(err)
	}
	var k map[string]any
	if err := json.Unmarshal(b, &k); err != nil {
		t.Fatal(err)
	}
	return k
}

func pravilaIz(t *testing.T, k map[string]any) []any {
	t.Helper()
	r, ok := k["route"].(map[string]any)
	if !ok {
		t.Fatal("нет блока route")
	}
	return spisok(r["rules"])
}

// indeksPravila ищет по признаку, а не по имени: у правил в sing-box имён нет,
// и попытка искать строкой по всему JSON нашла бы что угодно.
func indeksPravila(t *testing.T, k map[string]any, priznak func(map[string]any) bool) int {
	t.Helper()
	for i, v := range pravilaIz(t, k) {
		if m, ok := v.(map[string]any); ok && priznak(m) {
			return i
		}
	}
	return -1
}

func estHijack(m map[string]any) bool    { return m["action"] == "hijack-dns" }
func estChastnye(m map[string]any) bool  { _, ok := m["ip_is_private"]; return ok }
func estProtsessy(m map[string]any) bool { _, ok := m["process_path"]; return ok }

func TestHijackDnsVysheChastnyh(t *testing.T) {
	// The single most expensive line in this project. Above ip_is_private the
	// hijack never fires, because a home resolver lives at a private address,
	// and the entire dns block becomes an elaborate decoration.
	k := sobrat(t, obraztsovyyVhod())
	iH := indeksPravila(t, k, estHijack)
	iCh := indeksPravila(t, k, estChastnye)
	if iH == -1 || iCh == -1 {
		t.Fatalf("нет одного из правил: hijack=%d, chastnye=%d", iH, iCh)
	}
	if iH > iCh {
		t.Fatalf("hijack-dns на позиции %d ниже ip_is_private на %d", iH, iCh)
	}
}

func TestObhodyVysheHijack(t *testing.T) {
	// And the mirror image: our own processes must outrank it, or the service
	// resolving the server name gets sent into a tunnel that does not exist yet.
	k := sobrat(t, obraztsovyyVhod())
	iP := indeksPravila(t, k, estProtsessy)
	iH := indeksPravila(t, k, estHijack)
	if iP == -1 || iP > iH {
		t.Fatalf("правило процессов на %d, hijack на %d", iP, iH)
	}
}

func TestInvariant1DirectTolkoPoYavnym(t *testing.T) {
	k := sobrat(t, obraztsovyyVhod())
	r := k["route"].(map[string]any)
	// Селектор, а не ядро: с появлением кандидатов единственного выхода больше
	// нет. Проверка не ослабла, она сместилась: важно, что final НЕ direct и не
	// висячий, а на кого именно смотрит селектор, проверяет TestTegiKandidatov.
	if r["final"] != TegSelector {
		t.Fatalf("route.final = %v, а не %s: весь неопознанный трафик пошёл бы мимо туннеля", r["final"], TegSelector)
	}
}

func TestInvariant2PetlyaVseKandidaty(t *testing.T) {
	v := obraztsovyyVhod()
	k := sobrat(t, v)
	i := indeksPravila(t, k, func(m map[string]any) bool {
		s, ok := m["ip_cidr"]
		if !ok {
			return false
		}
		return strings.Contains(strings.Join(strok(s), ","), "192.0.2.225")
	})
	if i == -1 {
		t.Fatal("нет правила петли по кандидатам")
	}
	seti := strok(pravilaIz(t, k)[i].(map[string]any)["ip_cidr"])
	for _, kand := range v.Kandidaty {
		nashli := false
		for _, s := range seti {
			if strings.HasPrefix(s, kand.String()+"/") {
				nashli = true
			}
		}
		if !nashli {
			t.Fatalf("кандидат %v не попал в правило петли: переключение на него закрутит трафик", kand)
		}
	}
}

func TestInvariant3PoryadokPravil(t *testing.T) {
	k := sobrat(t, obraztsovyyVhod())
	p := pravilaIz(t, k)
	if len(p) == 0 || p[0].(map[string]any)["action"] != "sniff" {
		t.Fatal("первым правилом обязан быть sniff, иначе протокол неизвестен всем последующим")
	}
}

func TestInvariant4DvaPutiProtsessov(t *testing.T) {
	// In the "verified" config of revision 5 the second path was missing, and the
	// third loop would have come back on the TTL re-resolve. check does not see
	// this by construction.
	k := sobrat(t, obraztsovyyVhod())
	i := indeksPravila(t, k, estProtsessy)
	puti := strok(pravilaIz(t, k)[i].(map[string]any)["process_path"])
	if len(puti) < 2 {
		t.Fatalf("путей процессов %d, нужно два: ядро и служба", len(puti))
	}
}

func TestInvariant5ResolverMestnyy(t *testing.T) {
	v := obraztsovyyVhod()
	b, err := SingBox(v)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b, []byte(v.Resolver.String())) {
		t.Fatalf("резолвера %v нет в конфиге", v.Resolver)
	}
}

func TestInvariant6KillSwitchBezIsklyucheniy(t *testing.T) {
	v := obraztsovyyVhod()
	v.VesTrafik = true
	k := sobrat(t, v)
	if i := indeksPravila(t, k, estChastnye); i != -1 {
		t.Fatal("в режиме весь трафик правило ip_is_private обязано исчезнуть")
	}
	// Петлевые исключения при этом остаются: без них туннель съедает сам себя.
	if indeksPravila(t, k, estProtsessy) == -1 {
		t.Fatal("петлевое правило процессов снято вместе с удобными, это отказ другого класса")
	}
}

func TestInvariant7HotyaByOdnoPravilaProtsessa(t *testing.T) {
	// Measured on a live core: with no process rule clash_api returns an empty
	// processPath, with one it returns the real path. In "all traffic" mode the
	// convenience exceptions go away, and without this invariant the process
	// column in the log empties exactly where debugging is needed most.
	for _, ves := range []bool{false, true} {
		v := obraztsovyyVhod()
		v.VesTrafik = ves
		k := sobrat(t, v)
		if indeksPravila(t, k, estProtsessy) == -1 {
			t.Fatalf("нет правила по процессу при VesTrafik=%v", ves)
		}
	}
}

func TestVisyachiyTegLovitsyaSvoeyProverkoy(t *testing.T) {
	// sing-box check passes every one of these silently, so this is the only
	// thing standing between us and a config that routes into nowhere.
	k := map[string]any{
		"outbounds": []any{map[string]any{"type": "direct", "tag": TegPryamo}},
		"route":     map[string]any{"final": "yadro-kotorogo-net"},
	}
	if err := sveritTegi(k); err == nil {
		t.Fatal("висячий route.final не пойман")
	}
	kk := map[string]any{
		"outbounds": []any{map[string]any{"type": "direct", "tag": TegPryamo}},
		"dns": map[string]any{"servers": []any{
			map[string]any{"tag": TegTunnel, "detour": "net-takogo"},
		}},
	}
	if err := sveritTegi(kk); err == nil {
		t.Fatal("висячий detour у DNS-сервера не пойман")
	}
}

// ИНВАРИАНТ ПЕРЕВЁРНУТ ЗАДАЧЕЙ П5.
//
// Конфиг sing-box больше НЕ имеет права упоминать xhttp: апстрим этого
// транспорта не знает, замерено дословным «unknown transport type: xhttp» на
// сборке 1.14.0-rc.5 (06.09.2026, семь профилей из двадцати двух). Прежде его
// нёс форк sing-box-lx, ради него же и взятый; форк снят вместе с транспортом.
func TestXhttpBolsheNeUezzhaetVKonfig(t *testing.T) {
	v := obraztsovyyVhod()
	b, err := SingBox(v)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(b, []byte("xhttp")) {
		t.Fatal("в конфиге упомянут xhttp: апстримное ядро отвергнет его целиком")
	}
	if bytes.Contains(b, []byte(`"socks"`)) {
		t.Fatal("остался исходящий SOCKS: цепочка до Xray должна была исчезнуть")
	}
	if !bytes.Contains(b, []byte(`"reality"`)) {
		t.Fatal("исходящий без reality: ключи сервера потерялись")
	}
}

func TestPustoyVhodOtvergaetsya(t *testing.T) {
	sluchai := map[string]func(*Vhod){
		"без кандидатов": func(v *Vhod) { v.Kandidaty = nil },
		"один процесс":   func(v *Vhod) { v.PutiProtsessov = v.PutiProtsessov[:1] },
		"без резолвера":  func(v *Vhod) { v.Resolver = netip.Addr{} },
	}
	for imya, portit := range sluchai {
		v := obraztsovyyVhod()
		portit(&v)
		if _, err := SingBox(v); err == nil {
			t.Fatalf("%s: конфиг собрался, хотя не должен", imya)
		}
	}
}

func strok(v any) []string {
	switch s := v.(type) {
	case []string:
		return s
	case []any:
		out := make([]string, 0, len(s))
		for _, e := range s {
			if t, ok := e.(string); ok {
				out = append(out, t)
			}
		}
		return out
	}
	return nil
}

// --- Инвариант 8: пять профилей проходят sing-box check ---

func profili() map[string]Vhod {
	// Живой сервер один и он reality-tcp, поэтому остальные это синтетические
	// фикстуры с выдуманными адресами и ключами. Судья у них только check.
	baza := obraztsovyyVhod()
	nabor := map[string]protokol.Server{
		"reality-tcp": {Id: "rt", Transport: "reality-tcp", Host: "203.0.113.10", Port: 443,
			// 43 символа base64url, то есть ровно 32 байта. Первая версия фикстуры
			// была на четыре символа короче, и check её отверг: "invalid
			// public_key". Значит судья слаб в ссылках, но силён в значениях, и
			// на значения на него опираться можно.
			Uuid: baza.Server.Uuid, PublicKey: "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8",
			ShortId: "01ab", Sni: "www.example.com", Flow: "xtls-rprx-vision"},
		"ws":   {Id: "ws", Transport: "ws", Host: "203.0.113.11", Port: 443, Uuid: baza.Server.Uuid, Put: "/ws", Sni: "www.example.com"},
		"grpc": {Id: "gr", Transport: "grpc", Host: "203.0.113.12", Port: 443, Uuid: baza.Server.Uuid, Put: "gun", Sni: "www.example.com"},
		"hy2":  {Id: "h2", Transport: "hy2", Host: "203.0.113.13", Port: 443, Parol: "parol", Sni: "www.example.com"},
		// Наш будущий hy2-сервер: пин, обфускация, перебор портов. Судья
		// check ловит имена полей и форму «от:до» у server_ports.
		"hy2 с пином и obfs": {Id: "h2p", Transport: "hy2", Host: "203.0.113.16", Port: 443, Parol: "parol",
			Obfs: "salamander", ObfsParol: "sol", Porty: "20000-21000,443",
			Pin: "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8="},
		"ss": {Id: "ss", Transport: "ss", Host: "203.0.113.14", Port: 8388,
			Metod: "2022-blake3-aes-128-gcm", Parol: "MTIzNDU2Nzg5MDEyMzQ1Ng=="},
		// Добавлено 03.09.2026. Ядро несло эти три всё время, поддержки не
		// было только у нас. Судья check ловит имена полей: у trojan это
		// password, у vmess security и alter_id, у httpupgrade host отдельно
		// от headers.
		"httpupgrade": {Id: "hu", Transport: "httpupgrade", Host: "203.0.113.17", Port: 443,
			Uuid: baza.Server.Uuid, Put: "/hu", Sni: "www.example.com", HostZagolovka: "www.example.com"},
		"trojan": {Id: "tj", Transport: "trojan", Host: "203.0.113.18", Port: 443,
			Parol: "parol-servera", Sni: "www.example.com"},
		"trojan поверх ws": {Id: "tjw", Transport: "trojan-ws", Host: "203.0.113.19", Port: 443,
			Parol: "parol-servera", Put: "/tj", Sni: "www.example.com", HostZagolovka: "www.example.com"},
		"vmess": {Id: "vm", Transport: "vmess", Host: "203.0.113.20", Port: 443,
			Uuid: baza.Server.Uuid, Shifr: "auto", Sni: "www.example.com"},
		// alterId ненулевой намеренно: это единственный профиль, где поле
		// вообще попадает в конфиг, и форму его имени проверяет только check.
		"vmess поверх ws со старым alterId": {Id: "vmw", Transport: "vmess-ws", Host: "203.0.113.21", Port: 443,
			Uuid: baza.Server.Uuid, AlterId: 2, Put: "/vm", Sni: "www.example.com", HostZagolovka: "www.example.com"},
		// vmess без TLS: порты 80 и 8080 в живых сборниках это норма, и
		// генератор не должен включать TLS там, где ссылка его не просила.
		"vmess без tls": {Id: "vmp", Transport: "vmess", Host: "203.0.113.22", Port: 80,
			Uuid: baza.Server.Uuid, BezTLS: true},
		// Добавлено 05.09.2026. Ядро принимало оба исходящих и раньше
		// (проверено `sing-box check` с контролем на выдуманном типе), не было
		// только ветки у нас. Судья check ловит имена полей: у anytls это
		// password без uuid, у tuic пара uuid и password плюс congestion_control.
		"anytls": {Id: "at", Transport: "anytls", Host: "203.0.113.23", Port: 8443,
			Parol: "parol-anytls", Sni: "www.example.com"},
		"tuic": {Id: "tc", Transport: "tuic", Host: "203.0.113.24", Port: 443,
			Uuid: baza.Server.Uuid, Parol: "parol-tuic", Sni: "www.example.com",
			Alpn: "h3", Peregruzka: "bbr", RezhimUDP: "native"},
	}
	itog := map[string]Vhod{"с наборами": vhodSNaborami(), "с исключениями": vhodSProtsessami(), "с доменами": vhodSDomenami()}
	for imya, s := range nabor {
		v := baza
		v.Server = s
		// Адрес кандидата обязан быть в правиле петли. Раньше фикстуры этим не
		// озадачивались, потому что кандидат был один и его адрес лежал в
		// образце. Теперь за этим следит sveritKandidatov, и правильно следит.
		if a, err := netip.ParseAddr(s.Host); err == nil {
			v.Kandidaty = append(append([]netip.Addr{}, baza.Kandidaty...), a)
		}
		itog[imya] = v
	}

	// Два профиля СВЕРХ шести транспортов, ради которых задача 3.6 и написана.
	// Шесть это «каждый транспорт собирается», восемь это «несколько кандидатов
	// в одном конфиге не схлопываются».
	mnogo := baza
	mnogo.Servery = []protokol.Server{nabor["reality-tcp"], nabor["ws"], nabor["hy2"]}
	mnogo.Server = nabor["reality-tcp"]
	mnogo.Kandidaty = append(append([]netip.Addr{}, baza.Kandidaty...),
		netip.MustParseAddr("203.0.113.10"),
		netip.MustParseAddr("203.0.113.11"),
		netip.MustParseAddr("203.0.113.13"))
	itog["несколько кандидатов"] = mnogo

	// Два XHTTP-кандидата это отдельный профиль, а не частный случай: они обязаны
	// вести на РАЗНЫЕ адреса. Пока их нёс Xray, для этого каждому выдавался свой
	// порт SOCKS; со своим ядром исходящий прямой, и адрес у него свой по
	// построению.
	dvaKandidata := baza
	vtoroy := baza.Server
	vtoroy.Id, vtoroy.Host = "nl2", "203.0.113.7"
	dvaKandidata.Servery = []protokol.Server{baza.Server, vtoroy}
	itog["два кандидата"] = dvaKandidata

	// Девятый профиль: локальный прокси рядом с TUN. Судья тут не формальность.
	// Конфиг с mixed никогда не проходил через настоящее ядро, а именно оно
	// решает, законна ли пара «tun плюс mixed» и не спорят ли теги входящих.
	sProksi := baza
	sProksi.PortProksi = 10809
	itog["прокси рядом с tun"] = sProksi

	return itog
}

func TestInvariant8ProfiliProhodyatCheck(t *testing.T) {
	// Транспорты плюс три профиля: «несколько кандидатов», «два кандидата»
	// (волна 3) и «прокси рядом с tun» (01.09.2026). Шесть отвечали на вопрос
	// «собирается ли каждый транспорт», девять отвечают ещё и на «не
	// схлопываются ли кандидаты» и «законна ли пара входящих», а это разные
	// вопросы.
	//
	// Число проверяется ЯВНО. По «PASS на каждом имени» нельзя отличить
	// сделанное от несделанного: молча потерянный профиль тоже даёт зелёный.
	const skolkoZhdyom = 20
	if n := len(profili()); n != skolkoZhdyom {
		t.Fatalf("профилей %d, а ожидалось %d: профиль потерян или добавлен молча", n, skolkoZhdyom)
	}
	yadro := os.Getenv("AFFORY_SINGBOX")
	if yadro == "" {
		t.Skip("не задан AFFORY_SINGBOX: без живого ядра инвариант 8 непроверяем, " +
			"и притворяться, что он проверен, хуже, чем пропустить")
	}
	for imya, v := range profili() {
		b, err := SingBox(v)
		if err != nil {
			t.Fatalf("%s: генератор отказал: %v", imya, err)
		}
		put := filepath.Join(t.TempDir(), imya+".json")
		if err := os.WriteFile(put, b, 0o600); err != nil {
			t.Fatal(err)
		}
		vyhod, err := exec.Command(yadro, "check", "-c", put).CombinedOutput()
		if err != nil {
			t.Errorf("%s: check отверг конфиг: %v\n%s", imya, err, vyhod)
		}
	}
}

// Локальный прокси рядом с TUN.
//
// Приложения с жёстко заданным прокси в обход него не ходят НИКОГДА: для
// HTTP-клиента заданный прокси обязателен, а не желателен. Пока у нас был один
// TUN, включённый в Windows системный прокси на 127.0.0.1:10809 указывал в
// порт, который поднимала чужая программа, и с её уходом Claude Code переставал
// стартовать вовсе, хотя туннель нёс трафик прекрасно. Ровно поэтому v2rayN,
// nekoray и Karing держат mixed рядом с TUN, а не вместо него.
func TestProksiRyadomSTunnelem(t *testing.T) {
	v := obraztsovyyVhod()
	v.PortProksi = 10809

	k := sobrat(t, v)
	vhody, _ := k["inbounds"].([]any)
	if len(vhody) != 2 {
		t.Fatalf("входящих %d, а ожидались tun и прокси", len(vhody))
	}
	p, _ := vhody[1].(map[string]any)
	if p["type"] != "mixed" {
		t.Errorf("тип входящего %v, а не mixed: одним типом закрываются и HTTP, и SOCKS", p["type"])
	}
	// НАРУЖУ прокси не смотрит. Слушатель на 0.0.0.0 это открытый прокси для
	// всей подсети, то есть чужой трафик через наш сервер и наш адрес.
	if p["listen"] != "127.0.0.1" {
		t.Errorf("прокси слушает %v, а обязан только петлю", p["listen"])
	}
	if n, _ := p["listen_port"].(float64); int(n) != 10809 {
		t.Errorf("порт прокси %v, а задавали 10809", p["listen_port"])
	}
	// Системный прокси службой НЕ трогается: это настройка машины, её ставит
	// человек. Тихо переписать её значит сломать то, что настроено не нами.
	if _, est := p["set_system_proxy"]; est {
		t.Error("конфиг сам правит системный прокси: настройка машины не наша")
	}
}

// Ноль означает «без прокси», и это не мелочь: порт может быть занят чужой
// программой, и тогда ядро упадёт на бинде целиком, унося туннель. Отсутствие
// прокси обязано оставаться рабочим состоянием.
func TestBezProksiVhodTolkoOdin(t *testing.T) {
	v := obraztsovyyVhod()
	v.PortProksi = 0

	k := sobrat(t, v)
	vhody, _ := k["inbounds"].([]any)
	if len(vhody) != 1 {
		t.Fatalf("входящих %d, а ожидался один tun", len(vhody))
	}
	if vhody[0].(map[string]any)["type"] != "tun" {
		t.Errorf("единственный входящий не tun: %v", vhody[0])
	}
}

// Flow живёт только там, где он существует.
//
// `xtls-rprx-vision` работает поверх голого TCP под TLS или REALITY, и больше
// нигде: «XTLS only supports TLS and REALITY directly» в исходниках Xray.
// Генератор же писал его ЛЮБОМУ vless, включая ws, grpc, httpupgrade и xhttp,
// потому что ветки по транспорту в нём не было вовсе.
//
// Молчаливость дефекта тут полная: `sing-box check` такой конфиг принимает
// (проверено 05.09.2026), ядро стартует, а соединения нет. Ровно тот же дефект
// живёт и в v2rayN, то есть подсмотреть решение было не у кого.
//
// Ссылка при этом НЕ отвергается: сервер с flow над ws невозможен в принципе,
// значит это ошибка чужой панели, а не негодный сервер. Мы её игнорируем, как
// уже игнорируем insecure.
func TestFlowTolkoTamGdeOnSushchestvuet(t *testing.T) {
	baza := obraztsovyyVhod()
	nesut := map[string]bool{"reality-tcp": true}
	for _, transport := range []string{"reality-tcp", "ws", "grpc", "httpupgrade"} {
		t.Run(transport, func(t *testing.T) {
			s := protokol.Server{
				Id: "f", Transport: transport, Host: "203.0.113.30", Port: 443,
				Uuid: baza.Server.Uuid, Sni: "www.example.com",
				Flow: "xtls-rprx-vision",
				// REALITY нужен только своей ветке, но лишний ключ остальным не
				// мешает: они его не читают.
				PublicKey: "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8", ShortId: "01ab",
				Put: "/p",
			}
			v := baza
			v.Server = s
			v.Servery = []protokol.Server{s}
			v.Kandidaty = []netip.Addr{netip.MustParseAddr("203.0.113.30")}
			telo, err := SingBox(v)
			if err != nil {
				t.Fatal(err)
			}
			o := ishodyashchiyPoTegu(t, telo, TegKandidata("f"))
			_, est := o["flow"]
			if est != nesut[transport] {
				t.Fatalf("flow в конфиге %v, а должно быть %v: у %s его не существует",
					est, nesut[transport], transport)
			}
		})
	}
}

// Объявленная полоса включает Brutal у hysteria2, и это ЕДИНСТВЕННЫЙ способ его
// включить.
//
// Найдено 05.09.2026 разбором клиента. Документация sing-box: при пустых
// up_mbps и down_mbps ядро уходит на BBR вместо Hysteria CC. Наш вход
// `hy2-brutal` стоит с ignore_client_bandwidth false, то есть объявления ЖДЁТ, и
// без него замеренный выигрыш честного Brutal (плюс 20% полосы, минус 37%
// задержки) недостижим: вход работает обычным hy2 под именем, обещающим другое.
//
// Полоса это свойство КАНАЛА человека, а не сервера, поэтому она приезжает
// через Vhod, а не из ссылки. Ссылка с чужого сервера про домашний канал не
// знает ничего, и объявление её числа это ровно тот случай, который замер
// 05.09.2026 наказал восемью провалами и p95 3214 мс на полке 20 Мбит.
func TestPolosaKanalaVklyuchaetBrutalUHy2(t *testing.T) {
	hy2 := protokol.Server{Id: "h2", Transport: "hy2", Host: "203.0.113.13", Port: 443, Parol: "parol"}

	t.Run("объявленная полоса доезжает до ядра", func(t *testing.T) {
		v := obraztsovyyVhod()
		v.Server = hy2
		v.Servery = []protokol.Server{hy2}
		v.Kandidaty = []netip.Addr{netip.MustParseAddr("203.0.113.13")}
		v.PolosaVverh, v.PolosaVniz = 250, 440
		telo, err := SingBox(v)
		if err != nil {
			t.Fatal(err)
		}
		o := ishodyashchiyPoTegu(t, telo, TegKandidata("h2"))
		if o["up_mbps"] != float64(250) || o["down_mbps"] != float64(440) {
			t.Fatalf("полоса не дошла: up=%v down=%v", o["up_mbps"], o["down_mbps"])
		}
	})

	t.Run("нулевая полоса не пишется вовсе", func(t *testing.T) {
		// Ноль это НЕ «полоса неизвестна, пусть ядро решает»: объявленный ноль
		// включает Brutal с нулевой оценкой канала. Отсутствие полей это
		// единственный способ сказать ядру «считай сам», то есть BBR.
		v := obraztsovyyVhod()
		v.Server = hy2
		v.Servery = []protokol.Server{hy2}
		v.Kandidaty = []netip.Addr{netip.MustParseAddr("203.0.113.13")}
		telo, err := SingBox(v)
		if err != nil {
			t.Fatal(err)
		}
		o := ishodyashchiyPoTegu(t, telo, TegKandidata("h2"))
		if _, est := o["up_mbps"]; est {
			t.Fatalf("нулевая полоса попала в конфиг: %v", o["up_mbps"])
		}
		if _, est := o["down_mbps"]; est {
			t.Fatalf("нулевая полоса попала в конфиг: %v", o["down_mbps"])
		}
	})

	t.Run("половина полосы это не полоса", func(t *testing.T) {
		// Одно из двух чисел бессмысленно: Brutal управляет обоими
		// направлениями и объявление одного означало бы нулевое второе.
		v := obraztsovyyVhod()
		v.Server = hy2
		v.Servery = []protokol.Server{hy2}
		v.Kandidaty = []netip.Addr{netip.MustParseAddr("203.0.113.13")}
		v.PolosaVniz = 440
		if _, err := SingBox(v); !errors.Is(err, ErrPolosaNepolnaya) {
			t.Fatalf("односторонняя полоса принята: %v", err)
		}
	})

	t.Run("полоса не касается протоколов, которые её не понимают", func(t *testing.T) {
		v := obraztsovyyVhod()
		v.PolosaVverh, v.PolosaVniz = 250, 440
		telo, err := SingBox(v)
		if err != nil {
			t.Fatal(err)
		}
		o := ishodyashchiyPoTegu(t, telo, TegKandidata(v.Server.Id))
		if _, est := o["up_mbps"]; est {
			t.Fatal("полоса попала не в hy2: ядро отвергнет такой исходящий")
		}
	})
}

// Anytls и tuic доезжают до ядра в ЕГО именах полей.
//
// Проверяется именно перевод имён: в ссылке `congestion_control` и
// `udp_relay_mode`, у ядра они же, но у anytls пароль зовётся `password` и uuid
// не существует вовсе, а у tuic обязательны оба. Ошибка перевода не роняет
// конфиг, она даёт исходящий, который ядро примет и который не соединится.
func TestAnytlsITuicDoezzhayutDoYadra(t *testing.T) {
	t.Run("anytls", func(t *testing.T) {
		v := obraztsovyyVhod()
		v.Server = protokol.Server{Id: "at", Transport: "anytls", Host: "203.0.113.23", Port: 8443,
			Parol: "parol-anytls", Sni: "www.example.com",
			Pin: "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8="}
		v.Servery = []protokol.Server{v.Server}
		v.Kandidaty = []netip.Addr{netip.MustParseAddr("203.0.113.23")}
		telo, err := SingBox(v)
		if err != nil {
			t.Fatal(err)
		}
		o := ishodyashchiyPoTegu(t, telo, TegKandidata("at"))
		if o["type"] != "anytls" {
			t.Fatalf("тип %v", o["type"])
		}
		if o["password"] != "parol-anytls" {
			t.Fatalf("пароль не дошёл: %v", o["password"])
		}
		if _, est := o["uuid"]; est {
			t.Fatal("у anytls нет uuid, а генератор его написал")
		}
		tls, _ := o["tls"].(map[string]any)
		if tls["enabled"] != true || tls["server_name"] != "www.example.com" {
			t.Fatalf("tls: %v (anytls без TLS не существует)", tls)
		}
		pin, _ := tls["certificate_public_key_sha256"].([]any)
		if len(pin) != 1 {
			t.Fatalf("пин не дошёл до tls: %v", tls)
		}
	})

	t.Run("tuic", func(t *testing.T) {
		v := obraztsovyyVhod()
		v.Server = protokol.Server{Id: "tc", Transport: "tuic", Host: "203.0.113.24", Port: 443,
			Uuid: v.Server.Uuid, Parol: "parol-tuic", Sni: "www.example.com",
			Alpn: "h3", Peregruzka: "bbr", RezhimUDP: "native"}
		v.Servery = []protokol.Server{v.Server}
		v.Kandidaty = []netip.Addr{netip.MustParseAddr("203.0.113.24")}
		telo, err := SingBox(v)
		if err != nil {
			t.Fatal(err)
		}
		o := ishodyashchiyPoTegu(t, telo, TegKandidata("tc"))
		if o["type"] != "tuic" {
			t.Fatalf("тип %v", o["type"])
		}
		if o["uuid"] == "" || o["password"] != "parol-tuic" {
			t.Fatalf("uuid или пароль потеряны: %v / %v", o["uuid"], o["password"])
		}
		if o["congestion_control"] != "bbr" {
			t.Fatalf("congestion_control: %v", o["congestion_control"])
		}
		if o["udp_relay_mode"] != "native" {
			t.Fatalf("udp_relay_mode: %v", o["udp_relay_mode"])
		}
		tls, _ := o["tls"].(map[string]any)
		alpn, _ := tls["alpn"].([]any)
		if len(alpn) != 1 || alpn[0] != "h3" {
			t.Fatalf("alpn: %v (tuic без h3 не договорится)", tls["alpn"])
		}
	})

	t.Run("умолчания не выдумываются", func(t *testing.T) {
		// Ссылка без congestion_control это ссылка, где выбор оставлен ядру.
		// Своё значение здесь было бы сменой протокола под тем же именем.
		v := obraztsovyyVhod()
		v.Server = protokol.Server{Id: "tc", Transport: "tuic", Host: "203.0.113.24", Port: 443,
			Uuid: v.Server.Uuid, Parol: "parol-tuic", Sni: "www.example.com"}
		v.Servery = []protokol.Server{v.Server}
		v.Kandidaty = []netip.Addr{netip.MustParseAddr("203.0.113.24")}
		telo, err := SingBox(v)
		if err != nil {
			t.Fatal(err)
		}
		o := ishodyashchiyPoTegu(t, telo, TegKandidata("tc"))
		if _, est := o["congestion_control"]; est {
			t.Fatalf("congestion_control появился из ниоткуда: %v", o["congestion_control"])
		}
		if _, est := o["udp_relay_mode"]; est {
			t.Fatalf("udp_relay_mode появился из ниоткуда: %v", o["udp_relay_mode"])
		}
	})
}

func TestHy2VyhodNesyotObfsPortyIPin(t *testing.T) {
	// План «Стабильность и hy2» §4.2: the parsed fields must reach the core,
	// and in the core's own names. The pin goes to
	// tls.certificate_public_key_sha256, not to insecure.
	v := obraztsovyyVhod()
	v.Server = protokol.Server{Id: "h2", Transport: "hy2", Host: "203.0.113.13", Port: 443, Parol: "parol",
		Obfs: "salamander", ObfsParol: "sol", Porty: "20000-21000,443", Pin: "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8="}
	v.Servery = []protokol.Server{v.Server}
	v.Kandidaty = []netip.Addr{netip.MustParseAddr("203.0.113.13")}
	telo, err := SingBox(v)
	if err != nil {
		t.Fatal(err)
	}
	o := ishodyashchiyPoTegu(t, telo, TegKandidata("h2"))
	obfs, _ := o["obfs"].(map[string]any)
	if obfs["type"] != "salamander" || obfs["password"] != "sol" {
		t.Fatalf("obfs не дошёл до ядра: %v", o["obfs"])
	}
	porty, _ := o["server_ports"].([]any)
	// Одиночный порт тоже диапазон: «443» ядро отвергает как негодный
	// диапазон и роняет весь конфиг. Прежде здесь стояло ожидание «443», и
	// оно закрепляло ровно тот вид, который ядро не принимает.
	if len(porty) != 2 || porty[0] != "20000:21000" || porty[1] != "443:443" {
		t.Fatalf("server_ports: %v (ядро ждёт «от:до» даже для одного порта)", o["server_ports"])
	}
	if _, est := o["server_port"]; est {
		t.Fatal("server_port вместе с server_ports: ядро отвергает такой конфиг")
	}
	tls, _ := o["tls"].(map[string]any)
	pin, _ := tls["certificate_public_key_sha256"].([]any)
	if len(pin) != 1 || pin[0] != "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=" {
		t.Fatalf("пин не дошёл до tls: %v", tls)
	}
	if tls["insecure"] == true {
		t.Fatal("insecure включён: пин это проверка, а не её отключение")
	}
	// Без пина и obfs полей нет вовсе: пустой obfs ядро тоже отвергает.
	v.Server.Obfs, v.Server.ObfsParol, v.Server.Porty, v.Server.Pin = "", "", "", ""
	v.Servery = []protokol.Server{v.Server}
	telo, err = SingBox(v)
	if err != nil {
		t.Fatal(err)
	}
	o = ishodyashchiyPoTegu(t, telo, TegKandidata("h2"))
	if _, est := o["obfs"]; est {
		t.Fatal("пустой obfs попал в конфиг")
	}
	if _, est := o["server_ports"]; est {
		t.Fatal("пустой server_ports попал в конфиг")
	}
}

// TestPolosaSsylkiKakUmolchanie: полоса, объявленная ссылкой, работает без
// настройки, а настройка её перебивает.
//
// Ссылка входа `hy2-brutal` нашей же подписки несёт `upmbps=250&downmbps=440`,
// потому что у hysteria2 объявление полосы это единственный переключатель
// Brutal. Пока разбор эти параметры выбрасывал, вход на проде работал не тем,
// чем назван: ядро сваливалось в BBR, а на экране всё выглядело исправным.
//
// Порядок именно такой. Ссылку пишет держатель сервера и его число это оценка
// со стороны, а настройку ставит человек про СВОЙ домашний канал, и Brutal
// управляется именно им. Поэтому явно заданная настройка сильнее.
func TestPolosaSsylkiKakUmolchanie(t *testing.T) {
	sPolosoy := protokol.Server{Id: "h2", Transport: "hy2", Host: "203.0.113.13",
		Port: 443, Parol: "parol", PolosaVverh: 250, PolosaVniz: 440}

	t.Run("без настройки берётся полоса ссылки", func(t *testing.T) {
		v := obraztsovyyVhod()
		v.Server = sPolosoy
		v.Servery = []protokol.Server{sPolosoy}
		v.Kandidaty = []netip.Addr{netip.MustParseAddr("203.0.113.13")}
		telo, err := SingBox(v)
		if err != nil {
			t.Fatal(err)
		}
		o := ishodyashchiyPoTegu(t, telo, TegKandidata("h2"))
		if o["up_mbps"] != float64(250) || o["down_mbps"] != float64(440) {
			t.Fatalf("полоса ссылки не дошла: up=%v down=%v", o["up_mbps"], o["down_mbps"])
		}
	})

	t.Run("настройка перебивает ссылку", func(t *testing.T) {
		v := obraztsovyyVhod()
		v.Server = sPolosoy
		v.Servery = []protokol.Server{sPolosoy}
		v.Kandidaty = []netip.Addr{netip.MustParseAddr("203.0.113.13")}
		v.PolosaVverh, v.PolosaVniz = 40, 90
		telo, err := SingBox(v)
		if err != nil {
			t.Fatal(err)
		}
		o := ishodyashchiyPoTegu(t, telo, TegKandidata("h2"))
		if o["up_mbps"] != float64(40) || o["down_mbps"] != float64(90) {
			t.Fatalf("настройка не перебила ссылку: up=%v down=%v", o["up_mbps"], o["down_mbps"])
		}
	})

	t.Run("сервер без полосы остаётся без полосы", func(t *testing.T) {
		// Контроль: соседний сервер той же подписки, у которого параметров в
		// ссылке нет, не должен получить чужие числа.
		bez := protokol.Server{Id: "h3", Transport: "hy2", Host: "203.0.113.14",
			Port: 443, Parol: "parol"}
		v := obraztsovyyVhod()
		v.Server = sPolosoy
		v.Servery = []protokol.Server{sPolosoy, bez}
		v.Kandidaty = []netip.Addr{netip.MustParseAddr("203.0.113.13"),
			netip.MustParseAddr("203.0.113.14")}
		telo, err := SingBox(v)
		if err != nil {
			t.Fatal(err)
		}
		o := ishodyashchiyPoTegu(t, telo, TegKandidata("h3"))
		if _, est := o["up_mbps"]; est {
			t.Fatalf("чужая полоса протекла на соседний сервер: %v", o["up_mbps"])
		}
	})
}

// TestStekTunEtoUmolchanieYadra: стек TUN это `mixed`, а не `gvisor`.
//
// Замер 05.09.2026, гость против мишени стенда через тот же hy2, пять кругов
// вперемежку, канал один и тот же:
//
//	gvisor  35.2 Мбит (34.3, 35.2, 112.2, 37.4, 34.3), CPU клиента 10%
//	system  277.6 Мбит
//	mixed   340.0 Мбит (340.0, 388.5, 303.7, 378.6, 286.4), CPU 79%
//
// То есть зашитый `gvisor` стоил нам девяти десятых полосы, и не по нехватке
// процессора: он его почти не тратил. `mixed` это умолчание самого sing-box
// (системный TCP плюс gvisor для UDP), и отклонение от него было сделано без
// записанной причины ещё первым коммитом генератора.
//
// Тег сборки `with_gvisor` остаётся нужен: `mixed` тоже опирается на gvisor,
// но только для UDP.
func TestStekTunEtoUmolchanieYadra(t *testing.T) {
	telo, err := SingBox(obraztsovyyVhod())
	if err != nil {
		t.Fatal(err)
	}
	var k struct {
		Inbounds []map[string]any `json:"inbounds"`
	}
	if err := json.Unmarshal(telo, &k); err != nil {
		t.Fatal(err)
	}
	for _, v := range k.Inbounds {
		if v["type"] != "tun" {
			continue
		}
		if v["stack"] != "mixed" {
			t.Fatalf("стек TUN %v, а замер выбрал mixed", v["stack"])
		}
		return
	}
	t.Fatal("в конфиге нет входящего tun")
}

// Вторая половина той же правки 05.09.2026: разбор пин уже брал, а генератор
// клал его только hy2, anytls и tuic. Транспорты vless и trojan собирали блок
// TLS отдельными литералами, и пин до ядра не доезжал.
//
// Ядру он нужен под именем certificate_public_key_sha256, и без него
// самоподписанный сервер отвечает `x509: certificate signed by unknown
// authority`, а на экране ничего.
func TestPinDoezzhaetDoTLSVsehTransportov(t *testing.T) {
	const pin = "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8="
	transporty := []struct {
		transport string
		put       string
	}{
		{"ws", "/ws"},
		{"grpc", "gun"},
		{"httpupgrade", "/hu"},
		{"trojan", ""},
		{"trojan-ws", "/tw"},
	}
	for _, tr := range transporty {
		t.Run(tr.transport, func(t *testing.T) {
			v := obraztsovyyVhod()
			v.Server = protokol.Server{Id: "p", Transport: tr.transport, Host: "203.0.113.20", Port: 443,
				Uuid: "11111111-1111-1111-1111-111111111111", Parol: "parol", Put: tr.put, Pin: pin}
			v.Servery = []protokol.Server{v.Server}
			v.Kandidaty = []netip.Addr{netip.MustParseAddr("203.0.113.20")}
			telo, err := SingBox(v)
			if err != nil {
				t.Fatal(err)
			}
			o := ishodyashchiyPoTegu(t, telo, TegKandidata("p"))
			tls, _ := o["tls"].(map[string]any)
			if tls == nil {
				t.Fatal("блока tls нет вовсе")
			}
			p, _ := tls["certificate_public_key_sha256"].([]any)
			if len(p) != 1 || p[0] != pin {
				t.Fatalf("пин не дошёл до tls: %v", tls)
			}
			if tls["insecure"] == true {
				t.Fatal("insecure включён: пин это проверка, а не её отключение")
			}
		})
	}
}

// Обратная сторона: у reality пина в конфиге быть не должно даже если он
// как-то оказался в записи сервера. Сертификат там подставной, сверять нечего,
// и объявленный пин не совпал бы никогда.
func TestURealityPinaVKonfigeNet(t *testing.T) {
	v := obraztsovyyVhod()
	v.Server = protokol.Server{Id: "r", Transport: "reality-tcp", Host: "203.0.113.21", Port: 443,
		Uuid:      "11111111-1111-1111-1111-111111111111",
		PublicKey: v.Server.PublicKey, ShortId: v.Server.ShortId,
		Pin: "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8="}
	v.Servery = []protokol.Server{v.Server}
	v.Kandidaty = []netip.Addr{netip.MustParseAddr("203.0.113.21")}
	telo, err := SingBox(v)
	if err != nil {
		t.Fatal(err)
	}
	o := ishodyashchiyPoTegu(t, telo, TegKandidata("r"))
	tls, _ := o["tls"].(map[string]any)
	if _, est := tls["certificate_public_key_sha256"]; est {
		t.Fatalf("пин попал в reality: %v", tls)
	}
}
