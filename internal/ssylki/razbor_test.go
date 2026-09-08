package ssylki_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/ssylki"
)

// Фикстуры лежат файлами, а не строками в коде, по одной причине: ссылка это
// то, что человек копирует из чужой панели целиком. Строка в коде неизбежно
// оказывается той единственной формой, которую мы сами и придумали.
func vzyat(t *testing.T, imya string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", imya))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(b))
}

func TestRazborPoFiksturam(t *testing.T) {
	// Table-driven over testdata/, one file per shape. A parser that only ever
	// sees the one link we happen to own is a parser that breaks on the second
	// subscription, quietly, in a field nobody looks at.
	sluchai := []struct {
		fayl      string
		transport string
		oshib     error
	}{
		// xhttp снят 06.09.2026 вместе с чужим форком ядра: апстрим его не несёт,
		// а принятая ссылка стала бы записью, которую ядро потом не примет.
		{fayl: "vless-xhttp.txt", oshib: ssylki.ErrTransportNePodderzhan},
		{fayl: "vless-reality-raw.txt", transport: "reality-tcp"},
		{fayl: "vless-ws.txt", transport: "ws"},
		{fayl: "vless-grpc.txt", transport: "grpc"},
		{fayl: "vless-ipv6-literal.txt", transport: "reality-tcp"},
		{fayl: "vless-path-procenty.txt", transport: "ws"},
		{fayl: "hy2.txt", transport: "hy2"},
		{fayl: "hysteria2.txt", transport: "hy2"},
		{fayl: "ss-legacy.txt", transport: "ss"},
		{fayl: "ss-sip002.txt", transport: "ss"},
		{fayl: "ss-celikom.txt", transport: "ss"},

		{fayl: "bitiy-port-nol.txt", oshib: ssylki.ErrSsylkaKrivaya},
		{fayl: "bitiy-port-bolshoy.txt", oshib: ssylki.ErrSsylkaKrivaya},
		{fayl: "bitiy-port-bukvy.txt", oshib: ssylki.ErrSsylkaKrivaya},
		{fayl: "bitiy-bez-porta.txt", oshib: ssylki.ErrSsylkaKrivaya},
		{fayl: "bitiy-bez-pbk.txt", oshib: ssylki.ErrSsylkaKrivaya},
		{fayl: "bitiy-procenty.txt", oshib: ssylki.ErrSsylkaKrivaya},
		{fayl: "bitiy-pusto.txt", oshib: ssylki.ErrSsylkaKrivaya},

		// Три бывшие «чужие» фикстуры: с 03.09.2026 это наши транспорты.
		// Ядро умело их и раньше, поддержки не было только у нас.
		{fayl: "nash-trojan.txt", transport: "trojan"},
		{fayl: "nash-vmess.txt", transport: "vmess"},
		{fayl: "nash-httpupgrade.txt", transport: "httpupgrade"},

		// Ещё две бывшие «чужие», с 05.09.2026. Та же история, что и у трёх
		// выше, и найдена она тем же способом: `sing-box check` нашей сборкой
		// принимает оба исходящих, то есть ядро несло их всё время. Anytls при
		// этом первый по задержке в обоих условиях замеров, и терять его из-за
		// отсутствующей ветки разбора было чистым убытком.
		{fayl: "nash-anytls.txt", transport: "anytls"},
		{fayl: "nash-tuic.txt", transport: "tuic"},

		// Разбирается, но не наша: отдельный код отказа, иначе карточка
		// «транспорт не поддержан» не появится никогда.
		{fayl: "chuzhoy-socks.txt", oshib: ssylki.ErrTransportNePodderzhan},
	}

	for _, sl := range sluchai {
		t.Run(sl.fayl, func(t *testing.T) {
			srv, err := ssylki.Razobrat(vzyat(t, sl.fayl))
			if sl.oshib != nil {
				if !errors.Is(err, sl.oshib) {
					t.Fatalf("ошибка %v, ожидалась %v", err, sl.oshib)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if srv.Transport != sl.transport {
				t.Fatalf("транспорт %q, ожидался %q", srv.Transport, sl.transport)
			}
			if srv.Id == "" {
				t.Fatal("пустой идентификатор: сервер нечем выбрать")
			}
		})
	}
}

func TestValidnoeProcentnoeKodirovanieRazbiraetsya(t *testing.T) {
	// path=%2Fws%2Fpath is an ordinary link. Редакция 1 плана числила процентное
	// кодирование признаком БИТОЙ ссылки, и тест по нему закрепил бы отказ на
	// исправной подписке: первая же чужая развалилась бы молча.
	srv, err := ssylki.Razobrat(vzyat(t, "vless-path-procenty.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if srv.Put != "/ws/path" {
		t.Fatalf("путь %q, ожидался /ws/path", srv.Put)
	}
	// Имя снимается PathUnescape, а не QueryUnescape: второй превратил бы
	// «NL+2» в «NL 2», и сервер сменил бы имя сам по себе.
	if srv.Imya != "NL+2" {
		t.Fatalf("имя %q, ожидалось NL+2", srv.Imya)
	}
}

func TestLiteralIPv6RazbiraetsyaSoSkobkami(t *testing.T) {
	srv, err := ssylki.Razobrat(vzyat(t, "vless-ipv6-literal.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if srv.Host != "2001:db8::1" {
		t.Fatalf("хост %q: скобки не сняты либо порт уехал в хост", srv.Host)
	}
	if srv.Port != 443 {
		t.Fatalf("порт %d", srv.Port)
	}
}

func TestSsVoVsehTryohFormah(t *testing.T) {
	// A parser written for one ss form does not fail on the second: it quietly
	// spreads garbage across Metod and Parol. The server then lands in the list,
	// refuses to connect, and the screen says server-auth-failed, so the человек
	// is sent to check a key the parser itself mangled.
	for _, f := range []string{"ss-legacy.txt", "ss-sip002.txt", "ss-celikom.txt"} {
		t.Run(f, func(t *testing.T) {
			srv, err := ssylki.Razobrat(vzyat(t, f))
			if err != nil {
				t.Fatal(err)
			}
			if srv.Metod != "aes-256-gcm" {
				t.Fatalf("метод %q", srv.Metod)
			}
			if srv.Parol != "parol" {
				t.Fatalf("пароль разобран как %q", srv.Parol)
			}
			if srv.Host != "203.0.113.6" || srv.Port != 8388 {
				t.Fatalf("адрес %s:%d", srv.Host, srv.Port)
			}
		})
	}
}

// sParametrom дописывает параметр в ЗАПРОС, а не в конец строки: у фикстур есть
// фрагмент с именем, и дописанное после «#» уезжает в имя, а не в параметры.
// Первая редакция теста этим и промахнулась, обвинив исправный код.
func sParametrom(t *testing.T, fayl, param string) string {
	t.Helper()
	s := vzyat(t, fayl)
	i := strings.Index(s, "#")
	if i < 0 {
		return s + "&" + param
	}
	return s[:i] + "&" + param + s[i:]
}

func TestInsecureIgnoriruetsyaSPometkoy(t *testing.T) {
	// insecure=1 turns off the only check that stops somebody impersonating the
	// server. Passing it through would make every subscription a MITM primitive,
	// and invisibly so. The link still parses: refusing it outright would make a
	// perfectly usable server unusable over a flag we can simply ignore.
	srv, err := ssylki.Razobrat(sParametrom(t, "vless-reality-raw.txt", "insecure=1"))
	if err != nil {
		t.Fatal(err)
	}
	if !srv.NebezopasnyyIgnorirovan {
		t.Fatal("флаг insecure проглочен молча: в списке не будет пометки")
	}
	// Второе написание того же самого. Ловить только одно значит пропустить
	// половину панелей.
	srv2, err := ssylki.Razobrat(sParametrom(t, "hy2.txt", "allowInsecure=1"))
	if err != nil {
		t.Fatal(err)
	}
	if !srv2.NebezopasnyyIgnorirovan {
		t.Fatal("allowInsecure не опознан")
	}
}

func TestIdNeMenyaetsyaPriRotatsiiKlyuchey(t *testing.T) {
	// The id decides which server you connect to tomorrow. Deriving it from the
	// whole link means a key rotation renames a live server, and the client
	// reports selected-server-gone about a server that never went anywhere.
	bylo := vzyat(t, "vless-reality-raw.txt")
	stalo := strings.NewReplacer(
		"11111111-2222-3333-4444-555555555555", "99999999-8888-7777-6666-555555555555",
		"a213aa21b288980e", "ffffffffffffffff",
	).Replace(bylo)

	a, err := ssylki.Razobrat(bylo)
	if err != nil {
		t.Fatal(err)
	}
	b, err := ssylki.Razobrat(stalo)
	if err != nil {
		t.Fatal(err)
	}
	if a.Uuid == b.Uuid {
		t.Fatal("фикстура не изменилась, тест бессмыслен")
	}
	if a.Id != b.Id {
		t.Fatal("ротация ключей сменила идентификатор: живой сервер объявят исчезнувшим")
	}

	// Имя тоже не участвует: переименование в панели не должно менять сервер.
	c, err := ssylki.Razobrat(strings.Replace(bylo, "#NL-raw", "#Drugoe-imya", 1))
	if err != nil {
		t.Fatal(err)
	}
	if a.Id != c.Id {
		t.Fatal("переименование сменило идентификатор")
	}
}

func TestIdRazlichaetAdresPortITransport(t *testing.T) {
	// The mirror case. An id that ignores too much collapses different servers
	// into one, and then «переключиться» becomes a no-op nobody can see.
	baza, err := ssylki.Razobrat(vzyat(t, "vless-reality-raw.txt"))
	if err != nil {
		t.Fatal(err)
	}
	drugoyPort, err := ssylki.Razobrat(vzyat(t, "realno-ws-9443.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if baza.Id == drugoyPort.Id {
		t.Fatal("разные порт и транспорт дали один идентификатор")
	}
}

func TestChuzhoySkhemyOtlichayutsyaOtBityh(t *testing.T) {
	// Один канал ошибок сделал бы чужую схему «битой ссылкой», и человек пошёл
	// бы искать опечатку в совершенно исправной строке, а обещанная карточка
	// «транспорт не поддержан» не появилась бы никогда.
	//
	// Образец с 05.09.2026 это socks: tuic вслед за trojan, vmess и
	// httpupgrade переехал в поддержанные, и оставить проверку без единого
	// образца значило бы оставить её зелёной и пустой.
	for _, f := range []string{"chuzhoy-socks.txt"} {
		_, err := ssylki.Razobrat(vzyat(t, f))
		if errors.Is(err, ssylki.ErrSsylkaKrivaya) {
			t.Fatalf("%s объявлена битой, хотя она просто не наша", f)
		}
		if !errors.Is(err, ssylki.ErrTransportNePodderzhan) {
			t.Fatalf("%s: ошибка %v", f, err)
		}
	}
}

// Ниже формы, снятые с ЖИВОЙ чужой подписки 01.09.2026. Ключи и адреса в
// фикстурах поддельные, сохранены только формы. Каждая из них ловила настоящую
// дыру: собственные фикстуры их не покрывали ни одной.
func TestFormyZhivoyPodpiski(t *testing.T) {
	t.Run("type=tcp это старое имя raw", func(t *testing.T) {
		// Восемь из одиннадцати ссылок живой подписки пишут именно tcp, а не raw.
		srv, err := ssylki.Razobrat(vzyat(t, "realno-tcp-reality.txt"))
		if err != nil {
			t.Fatal(err)
		}
		if srv.Transport != "reality-tcp" {
			t.Fatalf("транспорт %q", srv.Transport)
		}
		if srv.Fp != "chrome" {
			t.Fatalf("отпечаток %q: без него рукопожатие будет не тем, о котором договаривались", srv.Fp)
		}
	})

	t.Run("имя с эмодзи и кириллицей", func(t *testing.T) {
		// Панели кладут во фрагмент флаги, пробелы и вертикальные черты в
		// процентном кодировании. Имя, приехавшее сырым, попадёт в интерфейс.
		srv, err := ssylki.Razobrat(vzyat(t, "realno-grpc-tls.txt"))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(srv.Imya, "%") {
			t.Fatalf("имя не раскодировано: %q", srv.Imya)
		}
		if !strings.Contains(srv.Imya, "Рос") {
			t.Fatalf("кириллица в имени потеряна: %q", srv.Imya)
		}
	})

	t.Run("нестандартный порт и заголовок host", func(t *testing.T) {
		srv, err := ssylki.Razobrat(vzyat(t, "realno-ws-9443.txt"))
		if err != nil {
			t.Fatal(err)
		}
		if srv.Port != 9443 {
			t.Fatalf("порт %d", srv.Port)
		}
		if srv.HostZagolovka != "cdn.example.org" {
			t.Fatalf("host %q: заголовок нужен, иначе фронт вернёт чужую страницу", srv.HostZagolovka)
		}
	})

	t.Run("tls это НЕ reality", func(t *testing.T) {
		// Живой баг: разбор помечал любой tcp как reality-tcp, не глядя на
		// security. Сервер попадал бы в список как REALITY, генератор строил бы
		// ему reality-исходящий, и рукопожатие не состоялось бы никогда.
		for _, f := range []string{"realno-tcp-tls.txt", "realno-grpc-tls.txt"} {
			srv, err := ssylki.Razobrat(vzyat(t, f))
			if f == "realno-tcp-tls.txt" {
				if !errors.Is(err, ssylki.ErrTransportNePodderzhan) {
					t.Fatalf("%s: vless поверх обычного tls принят как наш транспорт (%v, %q)",
						f, err, srv.Transport)
				}
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
			if srv.PublicKey != "" {
				t.Fatalf("%s: у tls-сервера появился ключ reality", f)
			}
			if srv.Alpn != "h2" {
				t.Fatalf("alpn %q потерян", srv.Alpn)
			}
		}
	})
}

func TestIstekshayaPodpiskaEtoUvedomlenie(t *testing.T) {
	// Both panels checked on 01.09.2026 answer an expired subscription with
	// HTTP 200 and syntactically valid links pointing at 0.0.0.0, where the
	// MESSAGE lives in the server name. One of them even puts the payment code
	// there.
	//
	// Without a separate channel this gives two equally bad outcomes: either the
	// list shows four «servers» called «💳 Код оплаты», or the parser refuses
	// them as «транспорт не поддержан» and the человек goes looking for a typo
	// that does not exist instead of renewing the subscription.
	sluchai := map[string]string{
		"uvedomlenie-0000-443.txt":   "Подписка закончилась",
		"uvedomlenie-0000-port1.txt": "Подписка истекла",
		"uvedomlenie-loopback.txt":   "Zaglushka",
	}
	for f, chast := range sluchai {
		t.Run(f, func(t *testing.T) {
			_, err := ssylki.Razobrat(vzyat(t, f))
			if !errors.Is(err, ssylki.ErrUvedomleniePodpiski) {
				t.Fatalf("ошибка %v, ожидалось уведомление подписки", err)
			}
			// Текст обязан доехать до человека: он и есть сообщение.
			if !strings.Contains(err.Error(), chast) {
				t.Fatalf("текст уведомления потерян: %v", err)
			}
		})
	}
}

func TestNastoyashchiyServerNeSchitaetsyaUvedomleniem(t *testing.T) {
	// The mirror case, without which the check above could be a blanket refusal.
	for _, f := range []string{"realno-ws-9443.txt", "realno-tcp-reality.txt", "hy2.txt"} {
		if _, err := ssylki.Razobrat(vzyat(t, f)); errors.Is(err, ssylki.ErrUvedomleniePodpiski) {
			t.Fatalf("%s принят за уведомление", f)
		}
	}
}

func TestTekstOtkazaNeSoderzhitKlyuchey(t *testing.T) {
	// url.Parse кладёт в err.Error() ВЕСЬ адрес, а в адресе VLESS сидит uuid.
	// Обёртка через %v уносила его в отказ подписки, оттуда в кадр ответа, а
	// оттуда на экран любому, кто запустил CLI.
	const uuid = "11111111-2222-3333-4444-555555555555"
	const klyuch = "aBcDeFgHiJkLmNoPqRsTuVwXyZ0123456789abcdEFG"
	ssylka := "vless://" + uuid + "@example.com:поррт?security=reality&pbk=" + klyuch + "#NL"

	_, err := ssylki.Razobrat(ssylka)
	if err == nil {
		t.Fatal("битая ссылка разобралась")
	}
	tekst := err.Error()
	if strings.Contains(tekst, uuid) {
		t.Fatalf("uuid уехал в текст отказа: %q", tekst)
	}
	if strings.Contains(tekst, klyuch) {
		t.Fatalf("ключ уехал в текст отказа: %q", tekst)
	}
	if strings.Contains(tekst, "example.com") {
		t.Fatalf("адрес сервера уехал в текст отказа: %q", tekst)
	}
}

func TestNegodnyyKlyuchRealityOtvergaetsya(t *testing.T) {
	// Найдено живым прогоном на стенде 01.09.2026. servers add принял ссылку с
	// pbk=aaaa, после чего ядро отказалось стартовать ЦЕЛИКОМ:
	//
	//   FATAL create service: initialize outbound[1]: invalid public_key
	//
	// Конфиг ядра собирается из ВСЕХ серверов, поэтому один негодный ключ
	// уносит и рабочие. Отказывать надо при добавлении, где человек ещё видит,
	// какую ссылку он вставил.
	bityye := []struct{ imya, pbk string }{
		{"слишком короткий", "aaaa"},
		{"не base64url", "!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!"},
		{"31 байт вместо 32", "ovnZk8p1o3JvVzTgu0OFbQmXV0dNIHVDLoZUOe5uS"},
	}
	for _, b := range bityye {
		t.Run(b.imya, func(t *testing.T) {
			s := "vless://11111111-2222-3333-4444-555555555555@203.0.113.20:443?security=reality&pbk=" +
				b.pbk + "&sid=ab&sni=a.example#X"
			if _, err := ssylki.Razobrat(s); err == nil {
				t.Fatal("ссылка с негодным ключом принята: ядро не стартует вовсе")
			}
		})
	}

	// КОНТРОЛЬ: годный ключ обязан проходить, иначе проверка отвергает всё
	// подряд и доказывает лишь собственную строгость.
	horoshaya := "vless://11111111-2222-3333-4444-555555555555@203.0.113.20:443?security=reality" +
		"&pbk=ovnZk8p1o3JvVzTgu0OFbQmXV0dNIHVDLoZUOe5uS0Y&sid=ab&sni=a.example#X"
	if _, err := ssylki.Razobrat(horoshaya); err != nil {
		t.Fatalf("годный ключ отвергнут: %v", err)
	}
}

func TestHy2ParametryObfsPortovIPina(t *testing.T) {
	// План «Стабильность и hy2» §4.1. Our own hy2 server will carry a pinned
	// key (insecure stays ignored, the pin is the only way to trust a
	// self-issued or IP certificate), and the link grammar of Hysteria 2
	// panels adds obfs, obfs-password and mport. Dropping them silently gives
	// a server that parses, lists, and never connects.
	srv, err := ssylki.Razobrat(sParametrom(t, "hy2.txt",
		"obfs=salamander&obfs-password=sol&mport=20000-21000,443&pinSHA256=AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8%3D"))
	if err != nil {
		t.Fatal(err)
	}
	if srv.Obfs != "salamander" || srv.ObfsParol != "sol" {
		t.Fatalf("obfs потерян: %q / %q", srv.Obfs, srv.ObfsParol)
	}
	if srv.Porty != "20000-21000,443" {
		t.Fatalf("mport потерян: %q", srv.Porty)
	}
	if srv.Pin != "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=" {
		t.Fatalf("pinSHA256 потерян или не раскодирован: %q", srv.Pin)
	}
	// Без параметров поля пустые, а не выдуманные.
	chistyy, err := ssylki.Razobrat(vzyat(t, "hy2.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if chistyy.Obfs != "" || chistyy.ObfsParol != "" || chistyy.Porty != "" || chistyy.Pin != "" {
		t.Fatalf("поля появились из ниоткуда: %+v", chistyy)
	}
	// obfs без пароля это битая ссылка: ядро такой конфиг отвергнет целиком,
	// а вместе с ним и все остальные серверы.
	if _, err := ssylki.Razobrat(sParametrom(t, "hy2.txt", "obfs=salamander")); !errors.Is(err, ssylki.ErrSsylkaKrivaya) {
		t.Fatalf("obfs без пароля принят: %v", err)
	}
}

// Anytls и tuic несут СВОИ поля, а не общий огрызок.
//
// Оба протокола добавлены 05.09.2026, и оба ложатся в существующие поля лишь
// частично. У anytls пароль лежит в userinfo целиком, без двоеточия: разбор «по
// образцу hy2» здесь бы сработал, а «по образцу tuic» отрезал бы половину
// пароля по первому двоеточию. У tuic наоборот, userinfo это ПАРА uuid и
// пароля, и оба обязательны ядру.
//
// Управление перегрузкой и режим передачи UDP это не украшения: замер
// 05.09.2026 показал, что три варианта congestion_control дают 259-271 Мбит,
// то есть выбор чужой панели надо уважать, а не подставлять своё умолчание.
func TestAnytlsITuicNesutSvoiPolya(t *testing.T) {
	t.Run("anytls: пароль целиком", func(t *testing.T) {
		srv, err := ssylki.Razobrat(vzyat(t, "nash-anytls.txt"))
		if err != nil {
			t.Fatal(err)
		}
		if srv.Parol != "parol-anytls" {
			t.Fatalf("пароль %q, ожидался parol-anytls", srv.Parol)
		}
		if srv.Sni != "www.example.com" {
			t.Fatalf("sni потерян: %q", srv.Sni)
		}
		if srv.Uuid != "" {
			t.Fatalf("у anytls нет uuid, а разбор его выдумал: %q", srv.Uuid)
		}
	})

	t.Run("anytls: пароль с двоеточием не режется", func(t *testing.T) {
		// Пароли с двоеточием законны, и именно на них ломается разбор,
		// написанный по образцу tuic. Ошибка была бы молчаливой: сервер
		// добавился, ключ обрезан, на экране «server-auth-failed».
		srv, err := ssylki.Razobrat("anytls://parol:s:dvoetochiem@203.0.113.22:8443#A")
		if err != nil {
			t.Fatal(err)
		}
		if srv.Parol != "parol:s:dvoetochiem" {
			t.Fatalf("пароль обрезан: %q", srv.Parol)
		}
	})

	t.Run("anytls без пароля отвергается", func(t *testing.T) {
		if _, err := ssylki.Razobrat("anytls://@203.0.113.22:8443#A"); !errors.Is(err, ssylki.ErrSsylkaKrivaya) {
			t.Fatalf("anytls без пароля принят: %v", err)
		}
	})

	t.Run("tuic: uuid, пароль и оба режима", func(t *testing.T) {
		srv, err := ssylki.Razobrat(vzyat(t, "nash-tuic.txt"))
		if err != nil {
			t.Fatal(err)
		}
		if srv.Uuid != "11111111-2222-3333-4444-555555555555" {
			t.Fatalf("uuid %q", srv.Uuid)
		}
		if srv.Parol != "parol" {
			t.Fatalf("пароль %q", srv.Parol)
		}
		if srv.Peregruzka != "bbr" {
			t.Fatalf("congestion_control потерян: %q", srv.Peregruzka)
		}
		if srv.RezhimUDP != "native" {
			t.Fatalf("udp_relay_mode потерян: %q", srv.RezhimUDP)
		}
		if srv.Alpn != "h3" {
			t.Fatalf("alpn потерян: %q", srv.Alpn)
		}
	})

	t.Run("tuic без uuid или без пароля отвергается", func(t *testing.T) {
		for _, s := range []string{
			"tuic://:parol@203.0.113.8:443#T",
			"tuic://11111111-2222-3333-4444-555555555555@203.0.113.8:443#T",
		} {
			if _, err := ssylki.Razobrat(s); !errors.Is(err, ssylki.ErrSsylkaKrivaya) {
				t.Fatalf("%s принят: %v", s, err)
			}
		}
	})
}

// Негодный пин ОТВЕРГАЕТСЯ ссылкой, а не уезжает в конфиг ядра.
//
// Ядро отвергает ВЕСЬ конфиг из-за одного негодного пина, а с ним и остальные
// серверы. Замерено 04.09.2026 живым прогоном: подписка из трёх серверов не
// подняла ни одного, человек получил tun-create-failed, и причина лежала в
// одном hy2 из трёх. Ровно то соображение, по которому здесь же отвергается
// obfs без пароля, только для пина его никто не применил.
//
// Ломается пин ссылкой без процентного кодирования: base64 содержит «+», а «+»
// в запросе URL это ПРОБЕЛ. Наш же сборщик ссылок так и писал.
func TestHy2SNegodnymPinomOtvergaetsya(t *testing.T) {
	// Тот же пин, что в соседнем тесте, но первый знак «+» и НЕ закодирован:
	// url.ParseQuery отдаст пробел, и base64 развалится на 31-м байте.
	_, err := ssylki.Razobrat(sParametrom(t, "hy2.txt",
		"pinSHA256=+AECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8="))
	if !errors.Is(err, ssylki.ErrSsylkaKrivaya) {
		t.Fatalf("ссылка с испорченным пином разобралась: %v", err)
	}
}

// Пин не той длины тоже негоден: pinSHA256 это ровно 32 байта, и всё прочее
// значит, что панель прислала не тот хеш. Молча пропустить значит поставить
// проверку, которая не сойдётся никогда, а выглядит включённой.
func TestHy2SPinomNeToyDlinyOtvergaetsya(t *testing.T) {
	_, err := ssylki.Razobrat(sParametrom(t, "hy2.txt", "pinSHA256=AAECAwQF"))
	if !errors.Is(err, ssylki.ErrSsylkaKrivaya) {
		t.Fatalf("пин из шести байтов принят: %v", err)
	}
}

// Обратная половина: правильно закодированный пин с «+» обязан ПРОЙТИ и
// доехать до поля без изменений. Без неё починку закрывает «отвергать всё».
func TestHy2SZakodirovannymPlyusomProhodit(t *testing.T) {
	srv, err := ssylki.Razobrat(sParametrom(t, "hy2.txt",
		"pinSHA256=%2BAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8%3D"))
	if err != nil {
		t.Fatalf("годный пин отвергнут: %v", err)
	}
	if srv.Pin != "+AECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=" {
		t.Fatalf("пин доехал как %q", srv.Pin)
	}
}

// Пин в url-safe написании доезжает до поля СТАНДАРТНЫМ.
//
// Панели пишут base64 как придётся, а sing-box понимает только стандартный
// алфавит: пин, отданный ему как есть, разберётся в другие 32 байта, и
// проверка подлинности не сойдётся никогда. Отказа при этом не будет, будет
// молчаливое «сервер не отвечает».
func TestHy2PinIzUrlSafePriezhaetStandartnym(t *testing.T) {
	const url = "4OHi4-Tl5ufo6err7O3u7_Dx8vP09fb3-Pn6-_z9_v8="
	const std = "4OHi4+Tl5ufo6err7O3u7/Dx8vP09fb3+Pn6+/z9/v8="
	srv, err := ssylki.Razobrat(sParametrom(t, "hy2.txt", "pinSHA256="+url))
	if err != nil {
		t.Fatalf("url-safe пин отвергнут: %v", err)
	}
	if srv.Pin != std {
		t.Fatalf("пин доехал как %q, а ядро понимает только %q", srv.Pin, std)
	}
}

// Ссылка hysteria2 несёт ДВА пина, и ядру годится только один.
//
// Замер 06.09.2026 чужим клиентом: `pinSHA256` по спецификации Hysteria 2 это
// hex отпечатка СЕРТИФИКАТА, и чужие клиенты понимают только его. Наше ядро
// отпечатка сертификата не знает вовсе, ему нужен хеш публичного КЛЮЧА, и наш
// сборщик кладёт его отдельным параметром `pinPubKeySHA256`.
func TestHy2PinPublichnogoKlyuchaDoezzhaetDoYadra(t *testing.T) {
	const pin = "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8="
	srv, err := ssylki.Razobrat(sParametrom(t, "hy2.txt",
		"pinPubKeySHA256=AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8%3D"))
	if err != nil {
		t.Fatalf("пин публичного ключа отвергнут: %v", err)
	}
	if srv.Pin != pin {
		t.Fatalf("пин доехал как %q, а ждали %q", srv.Pin, pin)
	}
}

// Отпечаток сертификата (64 знака hex) НЕ уезжает в ядро и НЕ ломает ссылку.
//
// До 06.09.2026 такой параметр разбирался как base64, давал 48 байт вместо 32 и
// ронял ВСЮ ссылку в ErrSsylkaKrivaya. То есть собственная подписка после
// починки генератора перестала бы читаться собственным клиентом.
func TestHy2OtpechatokSertifikataNeUezzhaetVYadro(t *testing.T) {
	const hex = "9d4f2b1c8e7a60d35f4813ca62b9e0d7148a3f5c9b02e6d18a47c35f9e2b6104"
	srv, err := ssylki.Razobrat(sParametrom(t, "hy2.txt", "pinSHA256="+hex))
	if err != nil {
		t.Fatalf("ссылка с отпечатком сертификата отвергнута: %v", err)
	}
	if srv.Pin != "" {
		t.Fatalf("отпечаток сертификата уехал в ядро как пин: %q", srv.Pin)
	}
}

// Ссылка нашего сборщика несёт оба параметра сразу. Ядру достаётся тот, что оно
// понимает, и ссылка при этом остаётся годной.
func TestHy2ObaPinaVOdnoySsylke(t *testing.T) {
	const pin = "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8="
	srv, err := ssylki.Razobrat(sParametrom(t, "hy2.txt",
		"pinSHA256=9d4f2b1c8e7a60d35f4813ca62b9e0d7148a3f5c9b02e6d18a47c35f9e2b6104"+
			"&pinPubKeySHA256=AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8%3D"))
	if err != nil {
		t.Fatalf("ссылка с двумя пинами отвергнута: %v", err)
	}
	if srv.Pin != pin {
		t.Fatalf("в ядро уехал %q, а понимает оно только %q", srv.Pin, pin)
	}
	if !srv.SPinom && srv.Pin == "" {
		t.Fatal("пин потерян целиком")
	}
}

// TestHy2PolosaIzSsylki: полоса, объявленная в самой ссылке, доезжает до сервера.
//
// Это не украшение, а причина, по которой вход `hy2-brutal` на проде работает
// не тем, чем назван. Наш же сборщик подписки (`vpn/skripty/sobrat-ssylki.py`)
// дописывает в ссылку этого входа `&upmbps=250&downmbps=440`, потому что у
// hysteria2 объявление полосы это ЕДИНСТВЕННЫЙ переключатель Brutal,
// отдельного флага нет. Разбор клиента параметры выбрасывал, ядро не получало
// ни `up_mbps`, ни `down_mbps` и сваливалось в BBR. Замеренный на стенде
// выигрыш Brutal (плюс 20% полосы, минус 37% задержки) при этом был
// недостижим, а на экране всё выглядело исправным.
//
// Чужие подписки пишут те же два параметра той же грамматикой, проверено на
// живом наборе кандидатов.
func TestHy2PolosaIzSsylki(t *testing.T) {
	srv, err := ssylki.Razobrat(sParametrom(t, "hy2.txt", "upmbps=250&downmbps=440"))
	if err != nil {
		t.Fatal(err)
	}
	if srv.PolosaVverh != 250 || srv.PolosaVniz != 440 {
		t.Fatalf("полоса из ссылки потеряна: вверх %d, вниз %d",
			srv.PolosaVverh, srv.PolosaVniz)
	}
	// Контроль: без параметров полосы нет, а не ноль, выданный за объявление.
	// Нулевая полоса это ТОЖЕ объявленная полоса, и она включила бы Brutal с
	// нулевой оценкой канала.
	chistyy, err := ssylki.Razobrat(vzyat(t, "hy2.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if chistyy.PolosaVverh != 0 || chistyy.PolosaVniz != 0 {
		t.Fatalf("полоса появилась из ниоткуда: %+v", chistyy)
	}
}

// TestHy2PolosaNeParoyIgnoriruetsya: одно число без второго это не объявление.
//
// Ядру нужны оба поля: `up_mbps` без `down_mbps` даёт Brutal с неизвестной
// половиной канала. Чужая подписка вполне может прислать одно, и молча
// достроить второе догадкой значит объявить серверу выдуманную полосу.
func TestHy2PolosaNeParoyIgnoriruetsya(t *testing.T) {
	for _, hvost := range []string{"upmbps=250", "downmbps=440", "upmbps=0&downmbps=440",
		"upmbps=abc&downmbps=440", "upmbps=-5&downmbps=440"} {
		srv, err := ssylki.Razobrat(sParametrom(t, "hy2.txt", hvost))
		if err != nil {
			t.Fatalf("%s: ссылка отвергнута целиком, а негодна только полоса: %v", hvost, err)
		}
		if srv.PolosaVverh != 0 || srv.PolosaVniz != 0 {
			t.Fatalf("%s: половинчатая полоса принята: вверх %d, вниз %d",
				hvost, srv.PolosaVverh, srv.PolosaVniz)
		}
	}
}

// 05.09.2026. Пин разбирался только у hysteria2, anytls и tuic, а у vless и
// trojan выбрасывался молча. Цена не теоретическая: собственный стенд десяти
// протоколов кладёт pinSHA256 во ВСЕ ссылки и insecure не ставит нигде, и
// ровно поэтому ws, grpc, httpupgrade и trojan через конфиг Affory
// отдавали ноль байт с `x509: certificate signed by unknown authority` в
// журнале ядра. На экране при этом не было ничего.
//
// То же самое у всякого, кто поднял себе vless или trojan с самоподписанным
// сертификатом: подключиться нечем, потому что insecure мы игнорируем
// НАМЕРЕННО, а пин, который и есть замена доверия к цепочке, теряли.
func TestPinDoezzhaetUVlessITrojan(t *testing.T) {
	const pin = "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8="
	sluchai := []struct {
		imya   string
		ssylka string
	}{
		{"vless-ws", "vless://11111111-1111-1111-1111-111111111111@ex.test:9003" +
			"?encryption=none&security=tls&sni=ex.test&type=ws&path=%2Fws&pinSHA256=" + pin + "#ws"},
		{"vless-grpc", "vless://11111111-1111-1111-1111-111111111111@ex.test:9004" +
			"?encryption=none&security=tls&sni=ex.test&type=grpc&serviceName=g&pinSHA256=" + pin + "#grpc"},
		{"vless-httpupgrade", "vless://11111111-1111-1111-1111-111111111111@ex.test:9005" +
			"?encryption=none&security=tls&sni=ex.test&type=httpupgrade&path=%2Fhu&pinSHA256=" + pin + "#hu"},
		{"trojan", "trojan://parol@ex.test:9006?security=tls&sni=ex.test&type=tcp&pinSHA256=" + pin + "#tr"},
	}
	for _, s := range sluchai {
		t.Run(s.imya, func(t *testing.T) {
			srv, err := ssylki.Razobrat(s.ssylka)
			if err != nil {
				t.Fatalf("ссылка не разобралась: %v", err)
			}
			if srv.Pin != pin {
				t.Fatalf("пин потерян или искажён: %q, ожидался %q", srv.Pin, pin)
			}
		})
	}
}

// Негодный пин обязан валить ссылку, а не уезжать в ядро. Ядро отвергает ВЕСЬ
// конфиг из-за одного кривого пина, и вместе с ним умирают рабочие серверы.
func TestKrivoyPinUVlessOtvergaetSsylku(t *testing.T) {
	_, err := ssylki.Razobrat("vless://11111111-1111-1111-1111-111111111111@ex.test:9003" +
		"?encryption=none&security=tls&sni=ex.test&type=ws&path=%2Fws&pinSHA256=AAECAwQF#ws")
	if err == nil {
		t.Fatal("пин не той длины принят: ядро отвергнет весь конфиг, а виноватым будет выглядеть сервер")
	}
}

// Обратная сторона той же правки: у reality пин брать НЕЛЬЗЯ.
//
// Сертификат там подставной и живёт одно рукопожатие, сверять его с записанным
// отпечатком нечему. Объявленный пин не совпал бы никогда, то есть лишний
// параметр в ссылке ломал бы рабочий сервер. Панели пишут в ссылки много
// лишнего, и это ровно тот случай.
func TestURealityPinNeBeryotsya(t *testing.T) {
	srv, err := ssylki.Razobrat(sParametrom(t, "vless-reality-raw.txt",
		"pinSHA256=AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8="))
	if err != nil {
		t.Fatal(err)
	}
	if srv.Pin != "" {
		t.Fatalf("пин у reality взят (%q): сверять его не с чем, и сервер перестанет подниматься", srv.Pin)
	}
}
