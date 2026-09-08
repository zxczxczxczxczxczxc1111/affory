package ssylki_test

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/ssylki"
)

// Поддельные ссылки: настоящих ключей в тестах нет и не будет.
const (
	ssylkaOdna = "vless://11111111-2222-3333-4444-555555555555@203.0.113.10:443" +
		"?encryption=none&type=tcp&security=reality&sni=www.microsoft.com&fp=chrome" +
		"&pbk=AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA&sid=bb01#Odin"
	ssylkaDve = "vless://11111111-2222-3333-4444-555555555555@203.0.113.11:9443" +
		"?encryption=none&type=ws&security=tls&sni=www.microsoft.com&fp=chrome" +
		"&path=%2Fdva&host=www.microsoft.com#Dva"
	ssylkaUvedomlenie = "vless://00000000-0000-0000-0000-000000000000@0.0.0.0:443" +
		"?type=tcp&security=none#Podpiska%20zakonchilas"
)

func telo(stroki ...string) string { return strings.Join(stroki, "\n") + "\n" }

func TestTeloVoVsehFormah(t *testing.T) {
	// Panels disagree about everything: plain text, standard base64, url-safe
	// base64 without padding, base64 wrapped at 76 columns, BOM, CRLF, and bodies
	// separated by a bare \r. A parser that knows one of them turns a perfectly
	// good subscription into subscription-malformed.
	//
	// The bare-\r case is the one that actually bites: splitting on \n alone
	// leaves the WHOLE body as a single line, so every server disappears at once
	// and the subscription looks empty rather than broken. Trailing \r\n is
	// milder only because TrimSpace on each line already eats the \r, which a
	// mutation run established on 01.09.2026 after this very test failed to
	// notice its removal.
	pryamo := telo(ssylkaOdna, ssylkaDve)
	obychnyy := base64.StdEncoding.EncodeToString([]byte(pryamo))

	// Перенос строки ВНУТРИ base64 это не экзотика: так его печатают панели,
	// собранные вокруг почтовых библиотек.
	var slomannyy strings.Builder
	for i := 0; i < len(obychnyy); i += 76 {
		k := i + 76
		if k > len(obychnyy) {
			k = len(obychnyy)
		}
		slomannyy.WriteString(obychnyy[i:k] + "\r\n")
	}

	sluchai := map[string][]byte{
		"простой текст":       []byte(pryamo),
		"base64 обычный":      []byte(obychnyy),
		"base64 url-safe":     []byte(base64.RawURLEncoding.EncodeToString([]byte(pryamo))),
		"base64 с переносами": []byte(slomannyy.String()),
		"с BOM":               append([]byte{0xEF, 0xBB, 0xBF}, pryamo...),
		"с CRLF":              []byte(strings.ReplaceAll(pryamo, "\n", "\r\n")),
		"с голым CR":          []byte(strings.ReplaceAll(pryamo, "\n", "\r")),
		"base64 из тела с CRLF": []byte(base64.StdEncoding.EncodeToString(
			[]byte(strings.ReplaceAll(pryamo, "\n", "\r\n")))),
	}
	for imya, t2 := range sluchai {
		t.Run(imya, func(t *testing.T) {
			r, err := ssylki.RazobratSpisok(t2)
			if err != nil {
				t.Fatal(err)
			}
			if len(r.Servery) != 2 {
				t.Fatalf("серверов %d, ожидалось 2 (отказы: %v)", len(r.Servery), r.Otkazy)
			}
			for _, s := range r.Servery {
				// Хвост CRLF уезжает в ПОСЛЕДНЕЕ поле ссылки, а им бывает и имя,
				// и sid. Проверяем все, иначе поймаем только сегодняшний порядок.
				if strings.ContainsAny(s.Imya+s.ShortId+s.Sni+s.PublicKey, "\r\n") {
					t.Fatalf("перенос уехал хвостом в поле сервера %q", s.Imya)
				}
				if !s.IzPodpiski {
					t.Fatalf("сервер %q не помечен как пришедший из подписки", s.Imya)
				}
			}
		})
	}
}

func TestPustoySpisokEtoOtkaz(t *testing.T) {
	// Accepting an empty list as valid wipes every server, including the spare
	// added by hand. Under kill-switch that is a locked machine with nowhere to go.
	for _, t2 := range [][]byte{{}, []byte("\n\n  \n"), []byte("   ")} {
		if _, err := ssylki.RazobratSpisok(t2); !errors.Is(err, ssylki.ErrPodpiskaPusta) {
			t.Fatalf("пустое тело %q принято как валидный список: %v", t2, err)
		}
	}
}

func TestChastichnoBityySpisok(t *testing.T) {
	// Nine good servers must not be lost because the tenth line has a typo, and
	// the tenth must not vanish silently either: it comes back with its line number.
	stroki := make([]string, 0, 10)
	for i := 0; i < 9; i++ {
		stroki = append(stroki, strings.Replace(ssylkaOdna, "203.0.113.10", fmt.Sprintf("203.0.113.%d", 20+i), 1))
	}
	stroki = append(stroki, "мусор")
	r, err := ssylki.RazobratSpisok([]byte(telo(stroki...)))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Servery) != 9 {
		t.Fatalf("серверов %d, ожидалось 9", len(r.Servery))
	}
	if len(r.Otkazy) != 1 || r.Otkazy[0].Stroka != 10 {
		t.Fatalf("отказы %v, ожидался один на строке 10", r.Otkazy)
	}
}

func TestNomerStrokiSchitaetPustye(t *testing.T) {
	// Line numbers are for a человек looking at the subscription in an editor.
	// Counting only non-empty lines points at the wrong one, and nothing about
	// the answer looks wrong.
	r, err := ssylki.RazobratSpisok([]byte(ssylkaOdna + "\n\n\nмусор\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Otkazy) != 1 || r.Otkazy[0].Stroka != 4 {
		t.Fatalf("отказы %v, ожидался один на строке 4", r.Otkazy)
	}
}

func TestIstekshayaPodpiskaEtoOtdelnyyIshod(t *testing.T) {
	// Checked on two panels 01.09.2026: an expired subscription arrives as HTTP
	// 200 with syntactically valid links. Reporting it as malformed sends the
	// человек hunting a typo instead of paying, and reporting it as empty hides
	// the payment code the panel put in the name.
	r, err := ssylki.RazobratSpisok([]byte(telo(ssylkaUvedomlenie, ssylkaUvedomlenie)))
	if !errors.Is(err, ssylki.ErrPodpiskaIstekla) {
		t.Fatalf("ожидалась истекшая подписка, получено: %v", err)
	}
	if len(r.Uvedomleniya) != 2 {
		t.Fatalf("уведомлений %d, ожидалось 2", len(r.Uvedomleniya))
	}
	if !strings.Contains(r.Uvedomleniya[0].Tekst, "Podpiska zakonchilas") {
		t.Fatalf("текст панели потерян: %q", r.Uvedomleniya[0].Tekst)
	}
	if r.Uvedomleniya[1].Stroka != 2 {
		t.Fatalf("номер строки уведомления %d, ожидался 2", r.Uvedomleniya[1].Stroka)
	}
	// Уведомления НЕ отказы: смешать их значит утопить текст среди номеров строк.
	if len(r.Otkazy) != 0 {
		t.Fatalf("уведомление попало в отказы: %v", r.Otkazy)
	}
}

func TestZhivayaPodpiskaSUvedomleniemNeSchitaetsyaIstekshey(t *testing.T) {
	// A panel that keeps an advert line among working servers must not take the
	// whole subscription down with it.
	r, err := ssylki.RazobratSpisok([]byte(telo(ssylkaOdna, ssylkaUvedomlenie)))
	if err != nil {
		t.Fatalf("подписка с рабочим сервером отвергнута: %v", err)
	}
	if len(r.Servery) != 1 || len(r.Uvedomleniya) != 1 {
		t.Fatalf("серверов %d, уведомлений %d, ожидалось 1 и 1", len(r.Servery), len(r.Uvedomleniya))
	}
}

func TestVseStrokiBityeEtoTozheOtkaz(t *testing.T) {
	// A list where every line failed leaves zero servers, which has exactly the
	// same consequence as an empty body. Returning nil here would wipe the list.
	r, err := ssylki.RazobratSpisok([]byte(telo("мусор", "и ещё мусор")))
	if !errors.Is(err, ssylki.ErrPodpiskaPusta) {
		t.Fatalf("список без единого сервера принят: %v", err)
	}
	// Причины обязаны дожить до вызывающего даже при отказе, иначе человеку
	// нечего показать, кроме слова «пусто».
	if len(r.Otkazy) != 2 {
		t.Fatalf("отказы потеряны при общем отказе: %v", r.Otkazy)
	}
}

// --- загрузка по сети ---

func otdayot(t *testing.T, telo []byte) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(telo)
	}))
	t.Cleanup(s.Close)
	return s
}

func TestZagruzkaRazbiraetOtvet(t *testing.T) {
	s := otdayot(t, []byte(telo(ssylkaOdna, ssylkaDve)))
	z := ssylki.NovyyZagruzchik()
	r, err := z.Zagruzit(context.Background(), s.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Servery) != 2 {
		t.Fatalf("серверов %d, ожидалось 2", len(r.Servery))
	}
}

func TestUrlNePopadaetVTekstOshibki(t *testing.T) {
	// net/http wraps the full URL into *url.Error, and this project already has
	// the habit of handing err.Error() outward verbatim. One unreachable
	// subscription would then print the token into the log, into the protocol
	// frame, and onto the screen of wave 4.
	const metka = "SEKRETNYY-TOKEN"
	z := ssylki.NovyyZagruzchik()
	_, err := z.Zagruzit(context.Background(), "https://127.0.0.1:1/sub/"+metka)
	if err == nil {
		t.Fatal("ожидалась ошибка")
	}
	if strings.Contains(err.Error(), metka) {
		t.Fatalf("URL подписки уехал в текст ошибки: %s", err)
	}
	if !errors.Is(err, ssylki.ErrPodpiskaNedostupna) {
		t.Fatalf("недоступность не опознана: %v", err)
	}
}

func TestKontrolPoiskaMetki(t *testing.T) {
	// The control for the test above, and it has to be a real one. Checking that
	// strings.Contains works proves nothing: «нет совпадений» would look the same
	// if the search were pointed at the wrong string, or if net/http had stopped
	// putting the URL into the error at all and the cleanup were dead code.
	//
	// So the control makes the SAME request with the SAME client and asserts that
	// the untouched error DOES carry the token. If this ever fails, the test above
	// is measuring nothing.
	const metka = "SEKRETNYY-TOKEN"
	const adres = "https://127.0.0.1:1/sub/" + metka

	zapros, err := http.NewRequest(http.MethodGet, adres, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = (&http.Client{Timeout: 5 * time.Second}).Do(zapros)
	if err == nil {
		t.Fatal("ожидалась ошибка соединения")
	}
	if !strings.Contains(err.Error(), metka) {
		t.Fatalf("контроль не нашёл метку в СЫРОЙ ошибке net/http: проверка выше "+
			"ничего не меряет. Ошибка: %v", err)
	}
}

func TestUrlNeTechyotIzKodaOtveta(t *testing.T) {
	// The 404 path builds its own message and is a separate leak from the
	// transport one: *url.Error is not involved here at all.
	const metka = "SEKRETNYY-TOKEN"
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "нет такой подписки", http.StatusNotFound)
	}))
	t.Cleanup(s.Close)
	z := ssylki.NovyyZagruzchik()
	_, err := z.Zagruzit(context.Background(), s.URL+"/sub/"+metka)
	if err == nil {
		t.Fatal("ожидалась ошибка на 404")
	}
	if strings.Contains(err.Error(), metka) {
		t.Fatalf("URL подписки уехал в текст ошибки кода ответа: %s", err)
	}
}

func TestPotolokEtoOtkazANeUsechenie(t *testing.T) {
	// io.LimitReader is the first thing the hand reaches for, and it truncates
	// silently: the result is a valid list of fewer servers with no error at all.
	bolshoe := make([]byte, 0, 3<<20)
	for len(bolshoe) < 3<<20 {
		bolshoe = append(bolshoe, []byte(ssylkaOdna+"\n")...)
	}
	s := otdayot(t, bolshoe)
	z := ssylki.NovyyZagruzchik()
	if _, err := z.Zagruzit(context.Background(), s.URL); !errors.Is(err, ssylki.ErrPodpiskaVelika) {
		t.Fatalf("тело больше потолка усечено молча: %v", err)
	}
}

func TestPonizhenieDoHttpZapreshcheno(t *testing.T) {
	// A redirect from https to http on a request carrying the subscription token
	// hands the token to anyone on the path. Following it is a leak, not a
	// compatibility feature.
	prostoy := otdayot(t, []byte(telo(ssylkaOdna)))
	tls := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, prostoy.URL, http.StatusFound)
	}))
	t.Cleanup(tls.Close)

	z := ssylki.NovyyZagruzchik()
	z.Klient = tls.Client()
	if _, err := z.Zagruzit(context.Background(), tls.URL); !errors.Is(err, ssylki.ErrPonizhenieTLS) {
		t.Fatalf("понижение до http не остановлено: %v", err)
	}
}

func TestPerenapravlenieVnutriHttpsRabotaet(t *testing.T) {
	// The mirror case: without it the rule above could be a blanket ban on
	// redirects, and panels do move their paths around.
	var adres string
	tls := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/novyy" {
			_, _ = w.Write([]byte(telo(ssylkaOdna)))
			return
		}
		http.Redirect(w, r, adres+"/novyy", http.StatusFound)
	}))
	t.Cleanup(tls.Close)
	adres = tls.URL

	z := ssylki.NovyyZagruzchik()
	z.Klient = tls.Client()
	r, err := z.Zagruzit(context.Background(), tls.URL+"/staryy")
	if err != nil {
		t.Fatalf("перенаправление внутри https отвергнуто: %v", err)
	}
	if len(r.Servery) != 1 {
		t.Fatalf("серверов %d, ожидался 1", len(r.Servery))
	}
}

func TestPovtoryPriPervoyZagruzke(t *testing.T) {
	// The service starts Automatic, which is earlier than the network comes up.
	// Without retries a freshly installed client would sit up to twelve hours on
	// a list it failed to fetch, and the человек would see a VPN with no servers.
	var popytok int
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		popytok++
		if popytok < 3 {
			http.Error(w, "сеть ещё не поднялась", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(telo(ssylkaOdna)))
	}))
	t.Cleanup(s.Close)

	var pauzy []time.Duration
	z := ssylki.NovyyZagruzchik()
	z.Spat = func(d time.Duration) { pauzy = append(pauzy, d) }

	r, err := z.ZagruzitSPovtorami(context.Background(), s.URL, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Servery) != 1 {
		t.Fatalf("серверов %d, ожидался 1", len(r.Servery))
	}
	if popytok != 3 {
		t.Fatalf("попыток %d, ожидалось 3", popytok)
	}
	if len(pauzy) != 2 {
		t.Fatalf("пауз %d, ожидалось 2", len(pauzy))
	}
	if pauzy[1] <= pauzy[0] {
		t.Fatalf("пауза не нарастает: %v", pauzy)
	}
}

func TestPovtorovNetNaSoderzhatelnomOtkaze(t *testing.T) {
	// Retrying an expired subscription five times changes nothing except how long
	// the человек waits for the answer the panel already gave.
	var popytok int
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		popytok++
		_, _ = w.Write([]byte(telo(ssylkaUvedomlenie)))
	}))
	t.Cleanup(s.Close)

	z := ssylki.NovyyZagruzchik()
	z.Spat = func(time.Duration) {}
	if _, err := z.ZagruzitSPovtorami(context.Background(), s.URL, 5); !errors.Is(err, ssylki.ErrPodpiskaIstekla) {
		t.Fatalf("ожидалась истекшая подписка: %v", err)
	}
	if popytok != 1 {
		t.Fatalf("попыток %d, ожидалась 1: повторять содержательный отказ незачем", popytok)
	}
}

func TestOtmenaKonteksta(t *testing.T) {
	ctx, otmena := context.WithCancel(context.Background())
	otmena()
	z := ssylki.NovyyZagruzchik()
	z.Spat = func(time.Duration) {}
	if _, err := z.ZagruzitSPovtorami(ctx, "https://127.0.0.1:1/sub", 5); err == nil {
		t.Fatal("отменённый контекст не остановил загрузку")
	}
}

// --- слияние поколений ---

func serverS(uuid, sid string) protokol.Server {
	return protokol.Server{
		Id: "aaa", Imya: "Один", Transport: "reality-tcp", Host: "203.0.113.10", Port: 443,
		Uuid: uuid, ShortId: sid, IzPodpiski: true,
	}
}

func TestRotatsiyaHranitPrezhneePokolenie(t *testing.T) {
	// One mistaken publication currently erases the working credentials with no
	// way back, and this project has exactly one server.
	slit := ssylki.Slit([]protokol.Server{serverS("uuid-odin", "sid-odin")},
		[]protokol.Server{serverS("uuid-dva", "sid-dva")})
	if slit[0].Uuid != "uuid-dva" {
		t.Fatal("новые ключи не применены")
	}
	if slit[0].PrezhnieKlyuchi == nil || slit[0].PrezhnieKlyuchi.Uuid != "uuid-odin" {
		t.Fatal("прежние ключи потеряны: откатиться будет некуда")
	}
}

func TestIstoriyaNeUglublyaetsya(t *testing.T) {
	// Одно поколение назад значит ОДНО. Иначе файл состояния копит все ключи,
	// которые у нас когда-либо были, а откатываются всё равно на предыдущее.
	a := ssylki.Slit([]protokol.Server{serverS("u1", "s1")}, []protokol.Server{serverS("u2", "s2")})
	b := ssylki.Slit(a, []protokol.Server{serverS("u3", "s3")})
	if b[0].PrezhnieKlyuchi.Uuid != "u2" {
		t.Fatalf("прежние ключи %q, ожидалось u2", b[0].PrezhnieKlyuchi.Uuid)
	}
}

func TestBezRotatsiiIstoriyaNeTeryaetsya(t *testing.T) {
	// A refresh that changed nothing must not quietly drop the rollback point.
	a := ssylki.Slit([]protokol.Server{serverS("u1", "s1")}, []protokol.Server{serverS("u2", "s2")})
	b := ssylki.Slit(a, []protokol.Server{serverS("u2", "s2")})
	if b[0].PrezhnieKlyuchi == nil || b[0].PrezhnieKlyuchi.Uuid != "u1" {
		t.Fatal("обновление без ротации стёрло точку отката")
	}
}

func TestRuchnyeServeryPerezhivayutObnovlenie(t *testing.T) {
	// The hand-added spare is the thing a человек falls back on when the
	// subscription itself is the problem. A refresh must not eat it.
	ruchnoy := protokol.Server{Id: "ruch", Imya: "Запасной", Host: "203.0.113.99", Port: 443}
	slit := ssylki.Slit([]protokol.Server{serverS("u1", "s1"), ruchnoy},
		[]protokol.Server{serverS("u1", "s1")})
	if len(slit) != 2 {
		t.Fatalf("серверов %d, ожидалось 2: ручной сервер съеден обновлением", len(slit))
	}
	var nashli bool
	for _, s := range slit {
		if s.Id == "ruch" {
			nashli = true
		}
	}
	if !nashli {
		t.Fatal("ручной сервер потерян")
	}
}

func TestUshedshiyIzPodpiskiServerIschezaet(t *testing.T) {
	// The mirror of the rule above: a server the panel removed must go, otherwise
	// the list only ever grows and selected-server-gone never fires.
	slit := ssylki.Slit([]protokol.Server{serverS("u1", "s1")}, []protokol.Server{
		{Id: "bbb", Imya: "Два", Host: "203.0.113.11", Port: 443, IzPodpiski: true},
	})
	if len(slit) != 1 || slit[0].Id != "bbb" {
		t.Fatalf("ушедший из подписки сервер остался: %v", slit)
	}
}

func TestPodpiskaSDublyamiSkhlopyvaetsya(t *testing.T) {
	// Живая подписка 01.09.2026 отдала ОДИННАДЦАТЬ строк, из которых две вели на
	// один и тот же узел с одним и тем же идентификатором. Модель такого не
	// предусматривала, а последствие тяжёлое: sveritKandidatov из задачи 3.6
	// отвергает список с повторяющимся Id, то есть конфиг ядра не собирается и
	// подъём не происходит вовсе. Человек вставляет рабочую подписку и получает
	// клиент, который не подключается ни к чему.
	odna := "vless://11111111-2222-3333-4444-555555555555@uzel.example:9443" +
		"?type=tcp&security=reality&pbk=AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA&fp=chrome#Один"
	telo := base64.StdEncoding.EncodeToString([]byte(odna + "\n" + odna + "\n"))

	r, err := ssylki.RazobratSpisok([]byte(telo))
	if err != nil {
		t.Fatalf("разбор не удался: %v", err)
	}
	if len(r.Servery) != 1 {
		t.Fatalf("серверов %d, а строка вела на один и тот же узел", len(r.Servery))
	}
}

func TestOtkazyPodpiskiNeNesutKlyuchey(t *testing.T) {
	// Путь подписки был вторым, и барьер на нём отсутствовал. Строка с битым
	// портом уносила uuid и pbk в Otkazy[].Prichina, оттуда в кадр ответа, а
	// оттуда на экран любому, кто запустил CLI.
	const uuid = "11111111-2222-3333-4444-555555555555"
	const klyuch = "aBcDeFgHiJkLmNoPqRsTuVwXyZ0123456789abcdEFG"
	bityaya := "vless://" + uuid + "@example.com:поррт?security=reality&pbk=" + klyuch + "#NL"
	horoshaya := "vless://" + uuid + "@203.0.113.20:443?security=reality&pbk=" + klyuch +
		"&sid=ab&fp=chrome&sni=a.example#OK"

	r, err := ssylki.RazobratSpisok([]byte(horoshaya + "\n" + bityaya + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Otkazy) != 1 {
		t.Fatalf("ожидался ровно один отказ, получено %d", len(r.Otkazy))
	}
	prichina := r.Otkazy[0].Prichina
	for _, tayna := range []string{uuid, klyuch, "example.com"} {
		if strings.Contains(prichina, tayna) {
			t.Fatalf("секрет %q уехал в отказ подписки: %q", tayna, prichina)
		}
	}
	if prichina == "" {
		t.Fatal("причина вычищена целиком: человек не поймёт, что чинить")
	}
}

// Одна негодная строка не имеет права утопить подписку.
//
// Замер 04.09.2026, живой прогон раунда 3: подписка из трёх серверов не подняла
// НИ ОДНОГО, потому что у одного hy2 был испорчен пин, а ядро отвергает конфиг
// целиком. Человек получил tun-create-failed и ни намёка на то, что негодна
// одна ссылка из трёх.
//
// Судится не сам пин (это дело разбора ссылки), а обещание подписки: годные
// строки доезжают, негодная становится ОТКАЗОМ СТРОКИ с номером.
func TestPodpiskaSNegodnoyStrokoyNeTeryaetOstalnye(t *testing.T) {
	// Средняя строка: «+» в пине не закодирован, url.ParseQuery отдаст пробел.
	telo := []byte(strings.Join([]string{
		"vless://11111111-2222-3333-4444-555555555555@203.0.113.9:8443" +
			"?encryption=none&type=tcp&security=reality&sni=www.example.com&fp=chrome" +
			"&pbk=AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8&sid=01ab#Германия",
		"hy2://parol@203.0.113.13:443?sni=a.example" +
			"&pinSHA256=+AECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=#Битый",
		"vless://11111111-2222-3333-4444-555555555555@203.0.113.10:8443" +
			"?encryption=none&type=tcp&security=reality&sni=www.example.com&fp=chrome" +
			"&pbk=AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8&sid=01ab#Нидерланды",
	}, "\n"))

	r, err := ssylki.RazobratSpisok(telo)
	if err != nil {
		t.Fatalf("подписка с одной негодной строкой отвергнута целиком: %v", err)
	}
	if len(r.Servery) != 2 {
		t.Fatalf("серверов %d, а годных строк две: негодная утащила соседей", len(r.Servery))
	}
	if len(r.Otkazy) != 1 || r.Otkazy[0].Stroka != 2 {
		t.Fatalf("отказы %v, ожидался ровно один на второй строке", r.Otkazy)
	}
}
