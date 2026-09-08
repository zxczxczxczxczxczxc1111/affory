package yadra

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Подставной clash_api. Отдаёт адрес в виде «хост:порт», потому что именно так
// он приезжает из конфига (external_controller), а не в виде URL.
func podstavnoyKlash(t *testing.T, ruka http.HandlerFunc) string {
	t.Helper()
	s := httptest.NewServer(ruka)
	t.Cleanup(s.Close)
	return strings.TrimPrefix(s.URL, "http://")
}

func TestZaderzhkaOtdayotChislo(t *testing.T) {
	adres := podstavnoyKlash(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"delay":296}`)
	})
	d, err := Zaderzhka(context.Background(), adres, "sekret", "srv-1")
	if err != nil {
		t.Fatalf("задержка не получена: %v", err)
	}
	if d != 296*time.Millisecond {
		t.Fatalf("задержка %v, ожидалась 296ms", d)
	}
}

// Тег и секрет обязаны доехать до сервера. Спросить не тот исходящий значит
// померить чужой путь и объявить своё подключение рабочим по чужому ответу:
// ровно та ошибка, ради которой этот механизм и заводится.
func TestZaderzhkaSprashivaetImennoSvoyTegISekret(t *testing.T) {
	var put, avt string
	adres := podstavnoyKlash(t, func(w http.ResponseWriter, r *http.Request) {
		put = r.URL.Path
		avt = r.Header.Get("Authorization")
		fmt.Fprint(w, `{"delay":10}`)
	})
	if _, err := Zaderzhka(context.Background(), adres, "sekret", "srv-a765753ab41f"); err != nil {
		t.Fatalf("задержка не получена: %v", err)
	}
	if put != "/proxies/srv-a765753ab41f/delay" {
		t.Fatalf("спрошен путь %q, а не путь выбранного исходящего", put)
	}
	if avt != "Bearer sekret" {
		t.Fatalf("заголовок доступа %q, ожидался Bearer с секретом", avt)
	}
}

// Тег уезжает в путь, а в теге может оказаться что угодно из подписки. Без
// экранирования пробел или слэш в имени дают запрос по другому адресу.
func TestZaderzhkaEkraniruetTeg(t *testing.T) {
	var put string
	adres := podstavnoyKlash(t, func(w http.ResponseWriter, r *http.Request) {
		put = r.URL.EscapedPath()
		fmt.Fprint(w, `{"delay":10}`)
	})
	if _, err := Zaderzhka(context.Background(), adres, "s", "srv a/b"); err != nil {
		t.Fatalf("задержка не получена: %v", err)
	}
	if put != "/proxies/srv%20a%2Fb/delay" {
		t.Fatalf("путь %q, тег не экранирован", put)
	}
}

// 504 это беда сервера: человек чинит её сменой сервера.
func TestZaderzhka504ZnachitIshodyashchiyNeOtvechaet(t *testing.T) {
	adres := podstavnoyKlash(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusGatewayTimeout)
		fmt.Fprint(w, `{"message":"timeout"}`)
	})
	_, err := Zaderzhka(context.Background(), adres, "s", "srv-1")
	if err == nil {
		t.Fatal("504 принят за успех")
	}
	if !strings.Contains(err.Error(), "не отвечает") {
		t.Fatalf("текст %q не говорит, что не отвечает ИСХОДЯЩИЙ", err)
	}
}

// 401 это НАШ дефект: секрет собран не тот. Путать его с 504 нельзя, иначе
// человека отправят менять сервер вместо того, чтобы чинить нас.
func TestZaderzhka401OtlichaetsyaOt504(t *testing.T) {
	// Владельцем порта тут выступает сам тест, поэтому без подмены имени ядра
	// вердикт будет «перехват», а проверяется здесь другая развилка.
	staroe := ImyaYadra
	ImyaYadra = filepath.Base(os.Args[0])
	defer func() { ImyaYadra = staroe }()

	adres := podstavnoyKlash(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	_, err := Zaderzhka(context.Background(), adres, "ne-tot", "srv-1")
	if err == nil {
		t.Fatal("401 принят за успех")
	}
	if !strings.Contains(err.Error(), "секрет") {
		t.Fatalf("текст %q не называет причиной секрет", err)
	}
}

func TestZaderzhkaMusorVTele(t *testing.T) {
	adres := podstavnoyKlash(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `не json вовсе`)
	})
	if _, err := Zaderzhka(context.Background(), adres, "s", "srv-1"); err == nil {
		t.Fatal("мусор в теле принят за задержку")
	}
}

// Молчание с кодом 200 это самый неприятный ответ: соединение есть, числа нет.
func TestZaderzhkaNolBezOshibkiEtoNeUspeh(t *testing.T) {
	adres := podstavnoyKlash(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{}`)
	})
	if _, err := Zaderzhka(context.Background(), adres, "s", "srv-1"); err == nil {
		t.Fatal("ответ без поля delay принят за успех")
	}
}

func TestZaderzhkaSrokIstyokSchitaetsyaOshibkoy(t *testing.T) {
	derzhi := make(chan struct{})
	adres := podstavnoyKlash(t, func(w http.ResponseWriter, r *http.Request) {
		<-derzhi
	})
	// Регистрируется ПОСЛЕ сервера: уборка идёт в обратном порядке, и с обратной
	// регистрацией Close ждал бы обработчик, который ждёт этот канал.
	t.Cleanup(func() { close(derzhi) })
	ctx, otm := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer otm()
	if _, err := Zaderzhka(ctx, adres, "s", "srv-1"); err == nil {
		t.Fatal("молчание сервера принято за успех")
	}
}

// Ядро поднимается не мгновенно, и спрашивать его сразу после старта значит
// измерить пустоту. Это уже случалось со сверкой владельца порта.
func TestZhdatKlashDozhidaetsyaGotovnosti(t *testing.T) {
	var mu sync.Mutex
	poprobovali := 0
	adres := podstavnoyKlash(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		poprobovali++
		hvatit := poprobovali >= 3
		mu.Unlock()
		if !hvatit {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		fmt.Fprint(w, `{"version":"1.14.0"}`)
	})
	if err := zhdatKlashS(context.Background(), adres, "s", time.Second, 10*time.Millisecond); err != nil {
		t.Fatalf("готовность не дождалась: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if poprobovali < 3 {
		t.Fatalf("попыток %d, значит ожидания не было", poprobovali)
	}
}

func TestZhdatKlashSdayotsyaPoSroku(t *testing.T) {
	adres := podstavnoyKlash(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	nach := time.Now()
	err := zhdatKlashS(context.Background(), adres, "s", 200*time.Millisecond, 10*time.Millisecond)
	if err == nil {
		t.Fatal("вечно мёртвый clash_api признан готовым")
	}
	if proshlo := time.Since(nach); proshlo > time.Second {
		t.Fatalf("ждали %v, срок 200ms не соблюдён", proshlo)
	}
}

// 401 от clash_api имеет ДВЕ разные причины, и до 01.09.2026 обе назывались
// одной: «секрет собран не тот», то есть наш дефект. Но 401 отвечает только
// живой HTTP-сервер, а на выданном системой порту им может оказаться ЧУЖОЙ
// клиент: на этой машине жили nekoray, Throne, Amnezia и Sota. Тогда человек
// идёт искать опечатку в нашем коде, а порт держит чужая программа.
//
// Механизм сверки владельца порта существовал с волны 1 и после ухода на одно
// ядро не вызывался НИОТКУДА, при том что комментарий в тесте утверждал
// обратное. Здесь он снова включён в дело.
func TestChuzhoyVladelecPortaNazyvaetsyaPerehvatom(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer s.Close()

	// Слушателя держит сам тест, то есть чужой для нас процесс: имя его бинаря
	// заведомо не sing-box.exe.
	_, err := Zaderzhka(context.Background(), strings.TrimPrefix(s.URL, "http://"), "sekret", "teg")
	if err == nil {
		t.Fatal("чужой владелец порта принят за свой")
	}
	if !strings.Contains(err.Error(), protokol.KodForeignProxyHijack) {
		t.Fatalf("отказ без кода перехвата: %v", err)
	}
	if strings.Contains(err.Error(), "не принял секрет") {
		t.Fatalf("чужая программа названа нашим дефектом: %v", err)
	}
}

// Обратный случай: на порту сидим МЫ, и тогда 401 это действительно наш
// дефект. Без этого теста первый чинится строкой «всегда перехват», и продукт
// начинает обвинять чужие программы в собственных ошибках.
func TestSvoyVladelecPortaEtoNashDefekt(t *testing.T) {
	staroe := ImyaYadra
	ImyaYadra = filepath.Base(os.Args[0])
	defer func() { ImyaYadra = staroe }()

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer s.Close()

	_, err := Zaderzhka(context.Background(), strings.TrimPrefix(s.URL, "http://"), "sekret", "teg")
	if err == nil {
		t.Fatal("403 принят за успех")
	}
	if strings.Contains(err.Error(), protokol.KodForeignProxyHijack) {
		t.Fatalf("свой же процесс назван перехватом: %v", err)
	}
}

func TestNesyotVRuchnomOtdayotVybrannogo(t *testing.T) {
	adres := podstavnoyKlash(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"type":"Selector","now":"srv-nl","all":["avto","srv-nl"]}`)
	})
	teg, err := Nesyot(context.Background(), adres, "sekret", "vybor", "avto")
	if err != nil {
		t.Fatalf("несущий не получен: %v", err)
	}
	if teg != "srv-nl" {
		t.Fatalf("несущий %q, ожидался srv-nl", teg)
	}
}

// Замерено на живом ядре: в авто «now» верхней группы это ИМЯ ВЛОЖЕННОЙ ГРУППЫ,
// а не сервер. Без второго этажа механизм отдаёт строку «avto» ровно в том
// режиме, ради которого он написан.
func TestNesyotVAvtoSpuskaetsyaEtazhomNizhe(t *testing.T) {
	var sprosheno []string
	adres := podstavnoyKlash(t, func(w http.ResponseWriter, r *http.Request) {
		sprosheno = append(sprosheno, r.URL.Path)
		switch r.URL.Path {
		case "/proxies/vybor":
			fmt.Fprint(w, `{"type":"Selector","now":"avto","all":["avto","srv-nl"]}`)
		case "/proxies/avto":
			fmt.Fprint(w, `{"type":"URLTest","now":"srv-de","all":["srv-nl","srv-de"]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	teg, err := Nesyot(context.Background(), adres, "sekret", "vybor", "avto")
	if err != nil {
		t.Fatalf("несущий не получен: %v", err)
	}
	if teg != "srv-de" {
		t.Fatalf("несущий %q, ожидался srv-de: спрошено %v", teg, sprosheno)
	}
	if len(sprosheno) != 2 {
		t.Fatalf("запросов %d (%v), а этажа два", len(sprosheno), sprosheno)
	}
}

// Третий этаж это уже не наша схема групп, а чей-то чужой конфиг или наш
// дефект. Спуск без предела превратил бы кольцо в вечный цикл внутри команды.
func TestNesyotNeSpuskaetsyaBeskonechno(t *testing.T) {
	adres := podstavnoyKlash(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"type":"Selector","now":"avto","all":["avto"]}`)
	})
	if _, err := Nesyot(context.Background(), adres, "sekret", "vybor", "avto"); err == nil {
		t.Fatal("кольцо групп принято за ответ")
	}
}

func TestPostavitVyborShlyotPutSTelom(t *testing.T) {
	var metod, put, telo, avt string
	adres := podstavnoyKlash(t, func(w http.ResponseWriter, r *http.Request) {
		metod, put, avt = r.Method, r.URL.Path, r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		telo = string(b)
		w.WriteHeader(http.StatusNoContent)
	})
	if err := PostavitVybor(context.Background(), adres, "sekret", "vybor", "srv-de"); err != nil {
		t.Fatalf("переключение не прошло: %v", err)
	}
	if metod != http.MethodPut || put != "/proxies/vybor" {
		t.Fatalf("ушёл %s %s, ожидался PUT /proxies/vybor", metod, put)
	}
	if telo != `{"name":"srv-de"}` {
		t.Fatalf("тело %s, ожидалось {\"name\":\"srv-de\"}", telo)
	}
	if avt != "Bearer sekret" {
		t.Fatalf("заголовок %q: без секрета ядро ответит 401, а мы решим, что переключились", avt)
	}
}

// Замерено: 400 приходит на три причины, кодом они неразличимы, телом
// различимы. Один код отказа на все три отправил бы человека переподключаться
// там, где переподключение не поможет, и наоборот.
func TestPostavitVyborRazlichaetPrichinyPo400(t *testing.T) {
	dlya := func(soobshchenie string) error {
		adres := podstavnoyKlash(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, `{"message":%q}`, soobshchenie)
		})
		return PostavitVybor(context.Background(), adres, "sekret", "vybor", "srv-de")
	}
	if err := dlya("Selector update error: not found"); !errors.Is(err, ErrTegaNetVYadre) {
		t.Fatalf("неизвестный тег дал %v, а это единственная причина, которую лечит переподключение", err)
	}
	if err := dlya("Must be a Selector"); err == nil || errors.Is(err, ErrTegaNetVYadre) {
		t.Fatalf("чужая группа дала %v, а переподключение её не лечит", err)
	}
}

// Успех это 204. Двести это чей-то другой сервер на нашем порту, и молчаливое
// согласие с ним означает «переключились» там, где не переключались.
func TestPostavitVyborNeSchitaet200Uspehom(t *testing.T) {
	adres := podstavnoyKlash(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"ok":true}`)
	})
	if err := PostavitVybor(context.Background(), adres, "sekret", "vybor", "srv-de"); err == nil {
		t.Fatal("200 принято за успех переключения")
	}
}

// Отвергнутое сервером рукопожатие обязано быть ОТЛИЧИМО от всего остального.
//
// Ядро живо и отвечает, а исходящий не работает: несовпавший uuid, чужой
// публичный ключ reality, неверный shortId. Пока это была безымянная
// fmt.Errorf, она схлопывалась в all-servers-down при подъёме и в
// switch-target-not-carrying при переключении, то есть человека отправляли
// проверять сеть вместо подписки.
func TestZaderzhkaNazyvaetOtvergnutyyServerSvoimImenem(t *testing.T) {
	adres := podstavnoyKlash(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"An error occurred in the delay test"}`,
			http.StatusServiceUnavailable)
	})
	_, err := Zaderzhka(context.Background(), adres, "sekret", "srv-1")
	if err == nil {
		t.Fatal("не-200 на пробе принят за успех")
	}
	if !errors.Is(err, ErrServerOtvergKlyuchi) {
		t.Fatalf("причина не типизирована: %v", err)
	}
}

// Обратная сторона: 401 и 403 это НЕ отвергнутый сервер, а не принятый секрет
// или чужой клиент на нашем порту. Один признак на оба случая отправил бы
// человека обновлять подписку там, где виноват сосед по порту.
func TestNeprinyatyySekretNeSchitaetsyaOtvergnutymServerom(t *testing.T) {
	for _, kod := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		adres := podstavnoyKlash(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(kod)
		})
		_, err := Zaderzhka(context.Background(), adres, "sekret", "srv-1")
		if err == nil {
			t.Fatalf("код %d принят за успех", kod)
		}
		if errors.Is(err, ErrServerOtvergKlyuchi) {
			t.Fatalf("код %d назван отвергнутым сервером: %v", kod, err)
		}
	}
}

// Пустой now у группы urltest это НЕ битый ответ, а фаза: группа ещё не
// закончила первую пробу задержки. Замерено 06.09.2026 на живом ядре: через
// 1.23 с после подъёма now пуст, через 5.63 с назван. Служба спрашивала один
// раз, глотала отказ и оставляла поле несущего пустым до следующего опроса
// через тридцать секунд, и приёмка ловила это как провал продукта.
//
// У селектора такой фазы нет, поэтому отличать обязана ОШИБКА, а не молчание:
// вызывающий сам решает, отказ это или «спроси ещё раз».
func TestVyborGruppyOtlichaetNeVybralaOtBitogoOtveta(t *testing.T) {
	adres := podstavnoyKlash(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"type":"URLTest","now":"","all":["srv-nl","srv-de"]}`)
	})
	_, err := VyborGruppy(context.Background(), adres, "sekret", "avto")
	if err == nil {
		t.Fatal("пустой выбор принят за ответ")
	}
	if !errors.Is(err, ErrNeVybrala) {
		t.Fatalf("ошибка %v не опознаётся как «не выбрала»: вызывающему нечем отличить фазу от поломки", err)
	}
}

// Nesyot спускается этажом ниже, и «не выбрала» обязано доезжать до вызывающего
// с того этажа, а не превращаться по дороге в общий отказ.
func TestNesyotDonositNeVybralaSNizhnegoEtazha(t *testing.T) {
	adres := podstavnoyKlash(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/proxies/vybor":
			fmt.Fprint(w, `{"type":"Selector","now":"avto","all":["avto","srv-nl"]}`)
		case "/proxies/avto":
			fmt.Fprint(w, `{"type":"URLTest","now":"","all":["srv-nl"]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	_, err := Nesyot(context.Background(), adres, "sekret", "vybor", "avto")
	if !errors.Is(err, ErrNeVybrala) {
		t.Fatalf("ошибка %v не опознаётся как «не выбрала»", err)
	}
}
