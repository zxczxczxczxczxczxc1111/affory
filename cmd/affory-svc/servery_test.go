package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/genkonfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/set"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/sostoyanie"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/yadra"
)

// Hysteria2 with pinSHA256: the screen says "pinned", the pin itself does not
// travel. It is public-key material, not a secret, but the screen has no use
// for 44 base64 characters and the channel policy is "nothing the screen
// cannot show".
func TestDlyaEkranaPomechaetPinNeOtdavayaEgo(t *testing.T) {
	e := dlyaEkrana(protokol.Server{Id: "a", Transport: "hy2", Pin: "AAAA"})
	if !e.SPinom || e.Pin != "" {
		t.Fatalf("экран %+v: ждали s_pinom без самого пина", e)
	}
	if dlyaEkrana(protokol.Server{Id: "b"}).SPinom {
		t.Fatal("без пина пометки быть не должно")
	}
}

func vtoroyServer() protokol.Server {
	return protokol.Server{
		Id: "de", Imya: "Германия", Transport: "reality-tcp",
		Host: "203.0.113.9", Port: 8443,
		Uuid: "11111111-2222-3333-4444-555555555555",
		// 43 символа base64url: check силён в значениях и отверг бы короче.
		PublicKey: "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8", ShortId: "01ab",
		Sni: "www.example.com",
	}
}

func TestServerBerotsyaIzSpiskaANeIzKonstanty(t *testing.T) {
	// Проверка греп-ом прошла бы и на вбитом гвоздями 10.0.0.1, поэтому идём
	// через шов: пустой список обязан сорвать подключение, а список из одного
	// сервера обязан довести пробу ровно до него.
	s := podstavnaya(t, nil)
	s.nabor = func() (Nabor, error) { return Nabor{}, nil }
	if err := s.Connect(context.Background()); err == nil {
		t.Fatal("пустой список серверов не помешал подключению: адрес взят откуда-то ещё")
	}

	// Инструмент сменился: замер теперь спрашивает про группу, и тег в нём
	// одинаков при любом сервере. Судим НЕСУЩЕГО: он ставится при подъёме из
	// поднятого кандидата и отвечает ровно на предмет теста. По VybranId судить
	// нельзя, набор его не задаёт и поле обязано быть пустым при любой
	// реализации (см. TestBezVyboraPoleVyboraPustoe).
	s = podstavnaya(t, nil)
	s.nabor = func() (Nabor, error) {
		return Nabor{Servery: []protokol.Server{vtoroyServer()}}, nil
	}
	if err := s.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := s.Status().NesushchiyId; got != "de" {
		t.Fatalf("поднят %q, а в списке был только de", got)
	}
}

func TestVybrannyyServerPobezhdaetPervogo(t *testing.T) {
	// Иначе выбор человека тихо подменяется порядком списка, а узнаёт он об этом
	// по чужой стране в браузере. Инструмент сменился на прямой: раньше выбор
	// ловился по тегу замера, а замер теперь про группу.
	//
	// Судится НЕСУЩИЙ, а не выбранный. Ш7-1 кладёт vybranId = n.Vybran, то есть
	// КОПИЮ литерала из этого же теста: поле равно "de" независимо от того, какой
	// сервер реально поднят, и тест был бы зелен при любой реализации подъёма.
	// Различает только несущий, который берётся из VybrannyyServer().
	s := podstavnaya(t, nil)
	n := Nabor{Servery: []protokol.Server{serverProby(), vtoroyServer()}, Vybran: "de"}
	s.nabor = func() (Nabor, error) { return n, nil }
	s.sekretyPisat = func(b []byte) error { return json.Unmarshal(b, &n) }
	if err := s.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := s.Status().NesushchiyId; got != "de" {
		t.Fatalf("поднят %q, а выбран был de", got)
	}
}

func TestIscheznuvshiyVybrannyyNeLomaetPodyom(t *testing.T) {
	// Подписка убрала сервер, который был выбран. Отказать значило бы оставить
	// человека без VPN там, где есть рабочая замена.
	s := podstavnaya(t, nil)
	s.nabor = func() (Nabor, error) {
		return Nabor{Servery: []protokol.Server{vtoroyServer()}, Vybran: "davno-net"}, nil
	}
	if err := s.Connect(context.Background()); err != nil {
		t.Fatalf("исчезнувший выбранный сервер сорвал подъём: %v", err)
	}
	s.Disconnect()
}

// --- команды ---

func vypolnit(t *testing.T, s *Sluzhba, imya string, telo any) protokol.Kadr {
	t.Helper()
	b, err := json.Marshal(telo)
	if err != nil {
		t.Fatal(err)
	}
	return s.Obrabotat(ctxAdmina(), protokol.Kadr{
		Tip: "cmd", Id: 1, Imya: imya, Telo: b})
}

const ssylkaProby = "vless://11111111-2222-3333-4444-555555555555@203.0.113.9:8443" +
	"?encryption=none&type=tcp&security=reality&sni=www.example.com&fp=chrome" +
	"&pbk=AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8&sid=01ab#Германия"

func TestDobavlenieSsylkoyIVidNaEkrane(t *testing.T) {
	s := podstavnaya(t, nil)
	if o := vypolnit(t, s, "addServer", map[string]string{"ssylka": ssylkaProby}); o.Oshib != nil {
		t.Fatalf("сервер не добавился: %v", o.Oshib)
	}
	o := vypolnit(t, s, "listServers", nil)
	if o.Oshib != nil {
		t.Fatal(o.Oshib)
	}
	var otvet struct {
		Servery []protokol.Server `json:"servery"`
		Vybran  string            `json:"vybran"`
	}
	if err := json.Unmarshal(o.Telo, &otvet); err != nil {
		t.Fatal(err)
	}
	if len(otvet.Servery) != 2 {
		t.Fatalf("серверов %d, ожидалось два", len(otvet.Servery))
	}
}

func TestSpisokServerovNeOtdayotKlyuchi(t *testing.T) {
	// Канал пускает INTERACTIVE осознанно, чтобы интерфейс не требовал админа. Из
	// этого прямо следует, что всё уехавшее в канал доступно любой интерактивной
	// сессии на машине, а uuid и pbk это и есть доступ к VPN.
	s := podstavnaya(t, nil)
	if o := vypolnit(t, s, "addServer", map[string]string{"ssylka": ssylkaProby}); o.Oshib != nil {
		t.Fatal(o.Oshib)
	}
	o := vypolnit(t, s, "listServers", nil)
	for _, tayna := range []string{
		"11111111-2222-3333-4444-555555555555",
		"AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8",
		"01ab",
	} {
		if strings.Contains(string(o.Telo), tayna) {
			t.Fatalf("ключ уехал в канал: %s", tayna)
		}
	}
	// Зеркало: то, что показывать НУЖНО, обязано доехать.
	for _, vidno := range []string{"203.0.113.9", "Германия", "reality-tcp"} {
		if !strings.Contains(string(o.Telo), vidno) {
			t.Fatalf("на экране не будет %s", vidno)
		}
	}
}

// zadatPodpisku это НАСТРОЙКА, а не проверка.
//
// С находки 35 задание адреса сразу тянет список, поэтому у подставной панели,
// которая отвечает «истекла» или «пусто», команда законно отказывает. Тесты
// ниже проверяют не это, им нужен сохранённый адрес. Сохранение и проверяем:
// молча глотать отказ нельзя, иначе настройка станет местом, где дефект прячется.
func zadatPodpisku(t *testing.T, s *Sluzhba, adres string) {
	t.Helper()
	vypolnit(t, s, "setSubscription", map[string]string{"adres": adres})
	n, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}
	if n.Podpiska != adres {
		t.Fatalf("адрес подписки не сохранён: %q", n.Podpiska)
	}
}

func TestAdresPodpiskiNeOtdayotsyaTselikom(t *testing.T) {
	// Адрес подписки это пропуск: он и есть секрет. Наружу только признак и узел.
	s := podstavnaya(t, nil)
	const adres = "https://panel.example/sub/OCHEN-SEKRETNYY-TOKEN"
	zadatPodpisku(t, s, adres)
	o := vypolnit(t, s, "listServers", nil)
	if strings.Contains(string(o.Telo), "OCHEN-SEKRETNYY-TOKEN") {
		t.Fatalf("адрес подписки уехал в канал целиком: %s", o.Telo)
	}
	if !strings.Contains(string(o.Telo), "panel.example") {
		t.Fatal("узел подписки не показан: человеку нечего опознать")
	}
}

func TestPovtornoeDobavlenieNeDvoit(t *testing.T) {
	// Человек скопировал ссылку дважды. Список из двух одинаковых серверов ему
	// только мешает, а идентификатор у них один и тот же по построению.
	s := podstavnaya(t, nil)
	for i := 0; i < 2; i++ {
		if o := vypolnit(t, s, "addServer", map[string]string{"ssylka": ssylkaProby}); o.Oshib != nil {
			t.Fatal(o.Oshib)
		}
	}
	n, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}
	if len(n.Servery) != 2 {
		t.Fatalf("серверов %d, ожидалось два (свой плюс добавленный один раз)", len(n.Servery))
	}
}

func TestUdalenieTekushchegoServeraPriPodnyatomTunneleOtklonyaetsya(t *testing.T) {
	// Убрать сервер под поднятым туннелем значит оставить машину с правилами
	// брандмауэра под адрес, которого больше нет в списке. Отказ понятнее, чем
	// тихое опускание туннеля под руками у человека.
	s := podstavnaya(t, nil)
	if err := s.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	o := vypolnit(t, s, "removeServer", map[string]string{"id": "nl"})
	if o.Oshib == nil {
		t.Fatal("текущий сервер удалён под поднятым туннелем")
	}
}

func TestIstekshayaPodpiskaDohoditTekstomPaneli(t *testing.T) {
	// Двадцать первый код и есть про это: человек должен увидеть сообщение
	// панели, а не нашу формулировку поверх него.
	s := podstavnaya(t, nil)
	zadatPodpisku(t, s, "https://panel.example/sub")
	o := vypolnit(t, s, "refreshSubscription", nil)
	if o.Oshib == nil {
		t.Fatal("истекшая подписка принята как успех")
	}
	if o.Oshib.Kod != protokol.KodSubscriptionExpired {
		t.Fatalf("код %s, ожидался subscription-expired", o.Oshib.Kod)
	}
	if !strings.Contains(o.Oshib.Tekst, "Подписка закончилась") {
		t.Fatalf("текст панели потерян: %s", o.Oshib.Tekst)
	}
}

func TestPustayaPodpiskaSohranyaetPrezhniySpisok(t *testing.T) {
	// Решение владельца: одна опечатка в публикации не должна оставлять запертую
	// машину без единого адреса.
	s := podstavnaya(t, nil)
	zadatPodpisku(t, s, "https://panel.example/pusto")
	do, _ := s.nabor()
	o := vypolnit(t, s, "refreshSubscription", nil)
	if o.Oshib == nil {
		t.Fatal("пустая подписка принята как успех")
	}
	posle, _ := s.nabor()
	if len(posle.Servery) != len(do.Servery) {
		t.Fatalf("прежний список стёрт: было %d, стало %d", len(do.Servery), len(posle.Servery))
	}
	// Проверка «список не изменился» сама по себе НИЧЕГО не значит: до слияния
	// дело не доходит ни в этой ветке, ни в ветке «подписка недоступна», и
	// мутационный прогон 01.09.2026 это показал. Значение имеет ВЕТКА: человек
	// обязан увидеть, что подписка ответила пустотой, а не что она недоступна,
	// потому что чинить это разные вещи.
	if o.Oshib.Kod != protokol.KodSubscriptionMalformed {
		t.Fatalf("код %s, ожидался subscription-malformed", o.Oshib.Kod)
	}
	if !strings.Contains(o.Oshib.Tekst, "прежний список сохранён") {
		t.Fatalf("человеку не сказано, что список цел: %s", o.Oshib.Tekst)
	}

	// Зеркало: недоступная подписка это ДРУГОЙ код. Без этого проверка выше
	// прошла бы и на «всё подряд считаем пустотой».
	zadatPodpisku(t, s, "https://panel.example/nedostupno")
	o = vypolnit(t, s, "refreshSubscription", nil)
	if o.Oshib == nil || o.Oshib.Kod != protokol.KodSubscriptionUnreach {
		t.Fatalf("недоступная подписка дала %v, ожидался subscription-unreachable", o.Oshib)
	}
}

// --- пересборка правил при включённом режиме ---

func TestDobavlenieServeraPriVklyuchennomRezhimePeresobiraetPravila(t *testing.T) {
	// Разрешающее правило заводится по адресам ВСЕХ серверов сразу, поэтому
	// переключение между уже известными серверами правил не трогает. А вот
	// сервер, ДОБАВЛЕННЫЙ при включённом режиме, в правила не попадал бы вовсе,
	// и переключиться на него означало бы отсутствие сети без единой ошибки.
	var peresborok int
	var poslednie []netip.Addr
	s := podstavnaya(t, nil)
	s.vklyuchitVes = func(r set.Razreshyonnoe, _ bool) error {
		peresborok++
		poslednie = r.Kandidaty
		return nil
	}
	s.prochitatProksi = func() (set.Proksi, error) { return set.Proksi{}, nil }
	s.sobratAdresa = func() ([]netip.Addr, error) {
		n, _ := s.nabor()
		out := make([]netip.Addr, 0, len(n.Servery))
		for _, srv := range n.Servery {
			out = append(out, netip.MustParseAddr(srv.Host))
		}
		return out, nil
	}

	if err := s.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.SetKillSwitch(true); err != nil {
		t.Fatal(err)
	}
	bylo := peresborok

	if o := vypolnit(t, s, "addServer", map[string]string{"ssylka": ssylkaProby}); o.Oshib != nil {
		t.Fatal(o.Oshib)
	}
	if peresborok == bylo {
		t.Fatal("правила не пересобраны: новый сервер не разрешён, переключение на него даст отсутствие сети")
	}
	var nashli bool
	for _, a := range poslednie {
		if a.String() == "203.0.113.9" {
			nashli = true
		}
	}
	if !nashli {
		t.Fatalf("адреса нового сервера нет в правилах: %v", poslednie)
	}
}

func TestPriVyklyuchennomRezhimePravilaNeTrogayutsya(t *testing.T) {
	// Зеркало. Без него правило выше могло бы означать «пересобираем всегда», и
	// добавление сервера включало бы человеку kill-switch, которого он не просил.
	var peresborok int
	s := podstavnaya(t, nil)
	s.vklyuchitVes = func(set.Razreshyonnoe, bool) error { peresborok++; return nil }
	if o := vypolnit(t, s, "addServer", map[string]string{"ssylka": ssylkaProby}); o.Oshib != nil {
		t.Fatal(o.Oshib)
	}
	if peresborok != 0 {
		t.Fatal("правила заведены при выключенном режиме: человеку включили запор, которого он не просил")
	}
}

func TestPustoyRezhimSVybrannymEtoRuchnoy(t *testing.T) {
	// Набор старой установки: поля режима в блобе нет, а сервер выбран руками.
	n := Nabor{Servery: []protokol.Server{serverProby(), vtoroyServer()}, Vybran: "de"}
	if r := rezhimNabora(n); r != protokol.RezhimRuchnoy {
		t.Fatalf("режим %q, а человек выбирал сервер руками: миграция уронила бы его в авто", r)
	}
}

func TestRezhimNaProvodeNikogdaNePustoy(t *testing.T) {
	// Статус отвечает с первого кадра, задолго до первого подъёма туннеля.
	// Пустая строка это третье значение у типа, заведённого ради двух.
	s := podstavnaya(t, nil)
	if r := s.Status().RezhimMarshruta; r != protokol.RezhimAvto && r != protokol.RezhimRuchnoy {
		t.Fatalf("режим на проводе %q, а объявленных значения два", r)
	}
}

func TestChuzhoyRezhimIzBlobaNePrinimaetsya(t *testing.T) {
	// importProfilya пишет расшифрованный блоб в хранилище КАК ЕСТЬ, без разбора
	// в Nabor. Значит режим приезжает не только командой, и проверять его надо
	// на чтении. Замерено: без этого мусорное значение проходит насквозь.
	s := podstavnaya(t, nil)
	s.sekretyChitat = func() ([]byte, error) {
		return []byte(`{"servery":[{"id":"nl","imya":"n","transport":"ws","host":"203.0.113.20","port":443}],"rezhim":"xyzzy"}`), nil
	}
	n, err := s.naborIzHranilishcha()
	if err != nil {
		t.Fatal(err)
	}
	if n.Rezhim != "" {
		t.Fatalf("режим %q принят из блоба: у типа два значения, и это не одно из них", n.Rezhim)
	}
}

func TestUdalenieServeraNeUvoditVAvto(t *testing.T) {
	// Режим считается по Vybran, а removeServer его чистит. Человек со старой
	// установкой удаляет свой сервер и молча меняет себе режим маршрута.
	//
	// Идём через НАСТОЯЩЕЕ чтение: фикстура подменяет s.nabor замыканием на
	// литерал, а литерал пришлось бы писать уже с полем режима, то есть тест
	// проверял бы сам себя. Блоб старой установки поля режима не содержит.
	s := podstavnaya(t, nil)
	blob := []byte(`{"servery":[` +
		`{"id":"nl","imya":"n","transport":"ws","host":"203.0.113.20","port":443},` +
		`{"id":"de","imya":"d","transport":"ws","host":"203.0.113.22","port":443}],` +
		`"vybran":"nl"}`)
	s.sekretyChitat = func() ([]byte, error) { return blob, nil }
	s.sekretyPisat = func(b []byte) error { blob = b; return nil }
	s.nabor = s.naborIzHranilishcha

	if o := vypolnit(t, s, "removeServer", map[string]string{"id": "nl"}); o.Oshib != nil {
		t.Fatalf("удаление не прошло: %s", o.Oshib.Kod)
	}
	n, err := s.naborIzHranilishcha()
	if err != nil {
		t.Fatal(err)
	}
	if n.Rezhim != protokol.RezhimRuchnoy {
		t.Fatalf("после удаления режим %q, а человек ничего про режим не говорил", n.Rezhim)
	}
}

// Ядро держит свой список кандидатов до следующего подъёма: горячей
// перезагрузки конфига у него нет. Сервер, выпавший из набора при живом ядре,
// остаётся достижимым для ядра и недостижимым для экрана.
func TestPriZhivomYadreNaborNeTeryaetServery(t *testing.T) {
	s := podstavnaya(t, nil)
	n := Nabor{Servery: []protokol.Server{serverProby(), vtoroyServer()}, Vybran: "nl"}
	s.nabor = func() (Nabor, error) { return n, nil }
	s.sekretyPisat = func(b []byte) error { return json.Unmarshal(b, &n) }
	if err := s.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Именно ПОСТОРОННИЙ сервер, не несущий: заслон шире, чем «нельзя удалить
	// текущий», и старый отказ в removeServer этот случай пропускал.
	o := vypolnit(t, s, "removeServer", map[string]string{"id": "de"})
	if o.Oshib == nil {
		t.Fatal("сервер удалён при живом ядре: ядро продолжит держать его кандидатом")
	}
	if o.Oshib.Kod != protokol.KodKandidatZanyat {
		t.Fatalf("код %q, а сервер занят живым ядром", o.Oshib.Kod)
	}
}

// Отказавшее подключение это НЕ «занято». Порт clash_api обнулён опусканием, а
// состояние otkaz держится, пока крутится восстановление. Судить по состоянию
// значит сказать человеку «сначала отключись», когда он уже отключён.
func TestPosleOtkazaSpisokPravitsyaSvobodno(t *testing.T) {
	s := podstavnaya(t, nil)
	n := Nabor{Servery: []protokol.Server{serverProby(), vtoroyServer()}, Vybran: "nl"}
	s.nabor = func() (Nabor, error) { return n, nil }
	s.sekretyPisat = func(b []byte) error { return json.Unmarshal(b, &n) }
	if err := s.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.opustit()
	s.postavit(protokol.SostOtkaz, &protokol.Oshibka{
		Kod: protokol.KodAllServersDown, Tekst: "сервер не отвечает"})
	if o := vypolnit(t, s, "removeServer", map[string]string{"id": "de"}); o.Oshib != nil {
		t.Fatalf("удаление после отказа не прошло: %s", o.Oshib.Kod)
	}
}

// Добавление при живом ядре РАЗРЕШЕНО: заслон про потерю, а не про изменение.
// Запрет означал бы, что подписка не может привезти новый узел, пока человек
// подключён, то есть ровно то, ради чего подписка и заводилась.
func TestPriZhivomYadreServerDobavlyaetsya(t *testing.T) {
	s := podstavnaya(t, nil)
	n := Nabor{Servery: []protokol.Server{serverProby()}, Vybran: "nl"}
	s.nabor = func() (Nabor, error) { return n, nil }
	s.sekretyPisat = func(b []byte) error { return json.Unmarshal(b, &n) }
	if err := s.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	// ssylkaProby это КОНСТАНТА, без скобок.
	o := vypolnit(t, s, "addServer", map[string]string{"ssylka": ssylkaProby})
	if o.Oshib != nil {
		t.Fatalf("добавление при живом ядре отказано: %s", o.Oshib.Kod)
	}
}

// Ветка connect зовёт ЗАПОМИНАНИЕ, а не переключение. Иначе connect --server
// при живом туннеле уводит трафик на другой сервер и следом отвечает
// all-servers-down: замерено на живом ядре.
func TestConnectSServeromNeHoditVYadro(t *testing.T) {
	s := podstavnaya(t, nil)
	n := Nabor{Servery: []protokol.Server{serverProby(), vtoroyServer()}}
	s.nabor = func() (Nabor, error) { return n, nil }
	s.sekretyPisat = func(b []byte) error { return json.Unmarshal(b, &n) }
	// Спрашивается МОМЕНТ обращения, а не сам его факт. Подъём с 04.09.2026
	// обязан навязать ядру выбор ПОСЛЕ старта: ядро восстанавливает прошлый
	// выбор из кэша и перебивает им default. Запрет на любое обращение за
	// команду краснел бы на этой правке, хотя запрещал он другое: разговор с
	// ядром, которого ещё нет.
	doYadra, posleYadra := false, false
	s.postavitVybor = func(context.Context, string, string, string, string) error {
		if adres, _ := s.dostupKKlash(); adres == "" {
			doYadra = true
		} else {
			posleYadra = true
		}
		return nil
	}
	if o := vypolnit(t, s, "connect", map[string]string{"server": "de"}); o.Oshib != nil {
		t.Fatalf("подключение не прошло: %s", o.Oshib.Kod)
	}
	if doYadra {
		t.Fatal("connect сходил в живое ядро: ядра в этот момент ещё нет")
	}
	if !posleYadra {
		t.Fatal("подъём не навязал выбор поднявшемуся ядру: трафик пойдёт через запомненный ядром сервер")
	}
	if s.Status().VybranId != "de" {
		t.Fatalf("выбран %q, а просили de", s.Status().VybranId)
	}
}

// Переключение обязано обновить несущего ДО ответа команде: оснастка снимает
// статус через 8 секунд, а человек смотрит на экран сразу. Фоновое обновление
// раз в 30 секунд это уточнение, а не источник.
func TestSetServerObnovlyaetNesushchegoSinhronno(t *testing.T) {
	s := podstavnaya(t, nil)
	konfigVyboraDlyaTesta(t, konfigProby)
	n := Nabor{Servery: []protokol.Server{serverProby(), vtoroyServer()}, Vybran: "nl"}
	s.nabor = func() (Nabor, error) { return n, nil }
	s.sekretyPisat = func(b []byte) error { return json.Unmarshal(b, &n) }
	if err := s.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.nesyot = func(context.Context, string, string, string, string) (string, error) {
		return genkonfig.TegKandidata("de"), nil
	}
	if o := vypolnit(t, s, "setServer", map[string]string{"id": "de"}); o.Oshib != nil {
		t.Fatalf("переключение не прошло: %s", o.Oshib.Kod)
	}
	st := s.Status()
	if st.VybranId != "de" || st.NesushchiyId != "de" {
		t.Fatalf("выбран %q, несёт %q: оба обязаны быть de до ответа команде", st.VybranId, st.NesushchiyId)
	}
}

// Тег, которого нет в конфиге ядра, лечится переподключением, и только им.
// Сервер, добавленный после подключения, это честный случай, а не редкость:
// добавление при живом ядре разрешено задачей Ш7-4.
func TestSetServerTrebuetPodyomaNaNeizvestnyyTeg(t *testing.T) {
	s := podstavnaya(t, nil)
	konfigVyboraDlyaTesta(t, konfigProby)
	n := Nabor{Servery: []protokol.Server{serverProby(), vtoroyServer()}, Vybran: "nl"}
	s.nabor = func() (Nabor, error) { return n, nil }
	s.sekretyPisat = func(b []byte) error { return json.Unmarshal(b, &n) }
	if err := s.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.postavitVybor = func(context.Context, string, string, string, string) error {
		return yadra.ErrTegaNetVYadre
	}
	o := vypolnit(t, s, "setServer", map[string]string{"id": "de"})
	if o.Oshib == nil || o.Oshib.Kod != protokol.KodNuzhenPodyom {
		t.Fatalf("код %v, ожидался switch-needs-reconnect", o.Oshib)
	}
}

// Мёртвый сервер не имеет права отобрать сеть у человека, который был подключён
// и работал. 204 говорит «команда принята», а не «трафик пошёл туда»: судит
// проба, и при её отказе выбор возвращается на прежний.
func TestSetServerVozvrashchaetPrezhniyVyborEsliNovyyNeNesyot(t *testing.T) {
	s := podstavnaya(t, nil)
	konfigVyboraDlyaTesta(t, konfigProby)
	n := Nabor{Servery: []protokol.Server{serverProby(), vtoroyServer()}, Vybran: "nl"}
	s.nabor = func() (Nabor, error) { return n, nil }
	s.sekretyPisat = func(b []byte) error { return json.Unmarshal(b, &n) }
	if err := s.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	var postavleno []string
	s.vyborGruppy = func(context.Context, string, string, string) (string, error) {
		return genkonfig.TegKandidata("nl"), nil
	}
	s.postavitVybor = func(_ context.Context, _, _, _, teg string) error {
		postavleno = append(postavleno, teg)
		return nil
	}
	// Проба через НОВЫЙ выбор не проходит. Заглушка фикстуры отвечает 42 мс без
	// сети, поэтому отказ ставится здесь явно.
	s.zamerit = func(context.Context, string, string, string) (time.Duration, error) {
		return 0, errors.New("503")
	}
	o := vypolnit(t, s, "setServer", map[string]string{"id": "de"})
	if o.Oshib == nil || o.Oshib.Kod != protokol.KodNovyyNeNesyot {
		t.Fatalf("код %v, ожидался switch-target-not-carrying", o.Oshib)
	}
	if len(postavleno) != 2 || postavleno[1] != genkonfig.TegKandidata("nl") {
		t.Fatalf("в ядро ушло %v, а прежний выбор обязан вернуться", postavleno)
	}
	if n.Vybran != "nl" {
		t.Fatalf("в наборе выбран %q: неудачное переключение не имеет права его переписать", n.Vybran)
	}
}

// Порог переключения меряется этими двумя отметками, и до сих пор их не писал
// никто. Тест сторожит ровно то, что четыре волны считалось проверенным, не
// будучи написанным.
func TestPereklyuchenieStavitObeOtmetki(t *testing.T) {
	s := podstavnaya(t, nil)
	konfigVyboraDlyaTesta(t, konfigProby)
	n := Nabor{Servery: []protokol.Server{serverProby(), vtoroyServer()}, Vybran: "nl"}
	s.nabor = func() (Nabor, error) { return n, nil }
	s.sekretyPisat = func(b []byte) error { return json.Unmarshal(b, &n) }
	if err := s.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	// ЧАСЫ ШАГА, а не time.Now. Замерено на этой машине: три вызова time.Now
	// подряд вернули одно и то же значение, совпав даже монотонной частью
	// (m=+0.029889501). На настоящих часах порядок отметок здесь недоказуем в
	// принципе, и прибор Ш7-7 упрётся в то же самое: разница двух отметок на
	// быстром пути равна нулю, и порог «уложились в две секунды» выполняется
	// реализацией, которая ставит обе отметки подряд и ничего между ними не
	// делает.
	//
	// atomic, а не голая переменная: часы читает и фоновый наблюдатель.
	var tikk int64
	nachalo := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	s.seychas = func() time.Time {
		return nachalo.Add(time.Duration(atomic.AddInt64(&tikk, 1)) * time.Millisecond)
	}
	// Отметка времени ПРОБЫ. Без неё тест судит только наличие пары: замерено
	// мутацией, перенос отметки gotov выше PUT не роняет ни одного теста, то
	// есть прибор Ш7-7 мерил бы окно, внутри которого доказуемо ничего нет.
	var vProbe time.Time
	s.zamerit = func(context.Context, string, string, string) (time.Duration, error) {
		vProbe = s.seychas()
		return 42 * time.Millisecond, nil
	}
	if o := vypolnit(t, s, "setServer", map[string]string{"id": "de"}); o.Oshib != nil {
		t.Fatalf("переключение не прошло: %s", o.Oshib.Kod)
	}
	s.mu.Lock()
	nach, gotov := s.snimok.SelectorNach, s.snimok.SelectorGotov
	s.mu.Unlock()
	if nach == nil || gotov == nil {
		t.Fatalf("отметки selector_nachalo=%v selector_gotov=%v: прибор порога мерить нечем", nach, gotov)
	}
	if gotov.Before(*nach) {
		t.Fatalf("selector_gotov раньше selector_nachalo: разница уйдёт в прибор отрицательной")
	}
	// Порог обязан ОХВАТЫВАТЬ работу, а не стоять рядом с ней.
	if vProbe.Before(*nach) || gotov.Before(vProbe) {
		t.Fatalf("проба в %v вне окна %v..%v: прибор меряет пустоту", vProbe, nach, gotov)
	}
}

// Отключение ПОСРЕДИ переключения не имеет права оставить на экране несущего.
//
// Окно настоящее: между PUT и опросом ядра стоят проба и запись набора, то есть
// сотни миллисекунд, за которые человек успевает нажать «отключить», а
// наблюдатель успевает опустить туннель по трём провалам подряд. Заслон стоит в
// коде с первой редакции, но до этого теста на его снятие не падало ничего.
func TestPereklyuchenieNePishetNesushchegoPosleOtklyucheniya(t *testing.T) {
	s := podstavnaya(t, nil)
	konfigVyboraDlyaTesta(t, konfigProby)
	n := Nabor{Servery: []protokol.Server{serverProby(), vtoroyServer()}, Vybran: "nl"}
	s.nabor = func() (Nabor, error) { return n, nil }
	s.sekretyPisat = func(b []byte) error { return json.Unmarshal(b, &n) }
	if err := s.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Ядро уходит ровно в тот момент, когда команда уже отдала PUT.
	s.postavitVybor = func(context.Context, string, string, string, string) error {
		s.Otklyuchit()
		return nil
	}
	// Ядро, которого уже нет, называет несущим кого угодно: если заслон снят,
	// это значение и уедет на экран поверх состояния vyklyuchen.
	s.nesyot = func(context.Context, string, string, string, string) (string, error) {
		return genkonfig.TegKandidata("de"), nil
	}
	vypolnit(t, s, "setServer", map[string]string{"id": "de"})
	st := s.Status()
	if st.NesushchiyId != "" {
		t.Fatalf("несущий %q при состоянии %q: экран показывает сервер, которого нет",
			st.NesushchiyId, st.Sostoyanie)
	}
}

// Умолчание это авто. Тест написан на ЗНАЧЕНИЕ, а не на константу: сверка
// константы с собой зелена при любом её значении.
func TestBezNastroekRezhimAvto(t *testing.T) {
	if r := rezhimNabora(Nabor{Servery: []protokol.Server{serverProby()}}); r != protokol.RezhimAvto {
		t.Fatalf("режим без настроек %q, а умолчание это авто", r)
	}
}

// В обе стороны намеренно. Проверка только «включить авто» зелена и у команды,
// которая ставит авто ВСЕГДА и вернуться не даёт.
func TestSetRouteModeRabotaetVObeStorony(t *testing.T) {
	// Хвост теста доводит команду до живого ядра, а значит и до переписи
	// конфига. Без шва она правит файл установленного клиента.
	konfigVyboraDlyaTesta(t, konfigProby)
	s := podstavnaya(t, nil)
	n := Nabor{Servery: []protokol.Server{serverProby(), vtoroyServer()}}
	s.nabor = func() (Nabor, error) { return n, nil }
	s.sekretyPisat = func(b []byte) error { return json.Unmarshal(b, &n) }

	if o := vypolnit(t, s, "setRouteMode", map[string]string{"rezhim": "ruchnoy"}); o.Oshib != nil {
		t.Fatalf("переход в ручной не прошёл: %s", o.Oshib.Kod)
	}
	if n.Rezhim != protokol.RezhimRuchnoy || s.Status().RezhimMarshruta != protokol.RezhimRuchnoy {
		t.Fatalf("в наборе %q, на проводе %q, ожидался ручной", n.Rezhim, s.Status().RezhimMarshruta)
	}
	if o := vypolnit(t, s, "setRouteMode", map[string]string{"rezhim": "avto"}); o.Oshib != nil {
		t.Fatalf("возврат в авто не прошёл: %s", o.Oshib.Kod)
	}
	if n.Rezhim != protokol.RezhimAvto || s.Status().RezhimMarshruta != protokol.RezhimAvto {
		t.Fatalf("в наборе %q, на проводе %q, ожидалось авто", n.Rezhim, s.Status().RezhimMarshruta)
	}

	// Ответ обязан СКАЗАТЬ про подъём на обеих сторонах шва. После И1 он на
	// обеих говорит «не нужен», и обе стороны проверяются намеренно: без ядра
	// подниматься нечему, а с ядром режим уже переключён в нём самом.
	if trebuet(t, vypolnit(t, s, "setRouteMode", map[string]string{"rezhim": "ruchnoy"})) {
		t.Fatal("на выключенном туннеле сказано, что нужен подъём: поднимать нечего")
	}
	if err := s.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if trebuet(t, vypolnit(t, s, "setRouteMode", map[string]string{"rezhim": "avto"})) {
		t.Fatal("на поднятом туннеле сказано, что нужен подъём: ядро переключает режим само")
	}
}

// trebuet достаёт из ответа признак «режим применится со следующего подъёма».
func trebuet(t *testing.T, k protokol.Kadr) bool {
	t.Helper()
	if k.Oshib != nil {
		t.Fatalf("команда отказала: %s", k.Oshib.Kod)
	}
	var telo struct {
		Trebuet bool `json:"trebuet_podyoma"`
	}
	if err := json.Unmarshal(k.Telo, &telo); err != nil {
		t.Fatalf("тело ответа не разбирается: %v", err)
	}
	return telo.Trebuet
}

// Чужое значение отвергается КОМАНДОЙ, а не только чтением набора. Проверка на
// чтении (Ш7-2) закрывает импорт профиля и молча приводит мусор к умолчанию;
// команда обязана сказать вслух, иначе интерфейс решит, что режим поставлен.
//
// ПЕРВОЕ утверждение здесь не про мусор, а про существование команды, и снять
// его нельзя. Хвост диспетчера отвечает на НЕИЗВЕСТНУЮ команду тем же самым
// protocol-mismatch, поэтому проверка одного лишь кода зелена на дереве, где
// команды нет вовсе. Такой тест не имеет красной фазы и не доказывает ничего.
func TestSetRouteModeOtvergaetChuzhoeZnachenie(t *testing.T) {
	s := podstavnaya(t, nil)
	n := Nabor{Servery: []protokol.Server{serverProby()}}
	s.nabor = func() (Nabor, error) { return n, nil }
	s.sekretyPisat = func(b []byte) error { return json.Unmarshal(b, &n) }

	// Команда обязана СУЩЕСТВОВАТЬ и принимать годное значение. Без этой строки
	// весь тест проходит на пустом дереве.
	if o := vypolnit(t, s, "setRouteMode", map[string]string{"rezhim": "avto"}); o.Oshib != nil {
		t.Fatalf("годное значение отвергнуто (%s): команды нет либо она сломана", o.Oshib.Kod)
	}
	o := vypolnit(t, s, "setRouteMode", map[string]string{"rezhim": "xyzzy"})
	if o.Oshib == nil || o.Oshib.Kod != protokol.KodProtocolMismatch {
		t.Fatalf("отказ %v, ожидался protocol-mismatch", o.Oshib)
	}
	// Мусор не имеет права доехать до набора.
	if n.Rezhim != protokol.RezhimAvto {
		t.Fatalf("в наборе %q, а мусор обязан был остановиться на границе", n.Rezhim)
	}
}

// Задача 4.9. Возраст последнего обновления подписки нужен экрану рядом с её
// узлом, и брать его больше неоткуда: статус этого поля не отдаёт, а сама
// отметка живёт в файле состояния службы.
func TestSpisokServerovNazyvaetVozrastPodpiski(t *testing.T) {
	s := podstavnaya(t, nil)
	kogda := time.Date(2026, 9, 2, 20, 0, 0, 0, time.UTC)
	s.pravitSost(func(f *sostoyanie.SostoyanieFayla) { f.PodpiskaObnovlena = &kogda })
	o := vypolnit(t, s, "listServers", nil)
	if o.Oshib != nil {
		t.Fatal(o.Oshib)
	}
	var otvet struct {
		PodpiskaObnovlena *time.Time `json:"podpiska_obnovlena"`
	}
	if err := json.Unmarshal(o.Telo, &otvet); err != nil {
		t.Fatal(err)
	}
	if otvet.PodpiskaObnovlena == nil || !otvet.PodpiskaObnovlena.Equal(kogda) {
		t.Fatalf("podpiska_obnovlena в ответе %v, ожидалось %v", otvet.PodpiskaObnovlena, kogda)
	}
}
