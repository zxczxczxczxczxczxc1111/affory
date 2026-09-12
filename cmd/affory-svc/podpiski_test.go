package main

import (
	"context"
	"encoding/json"
	"net/netip"
	"slices"
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/genkonfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/ssylki"
)

// Несколько подписок: одна активная, остальные про запас.
//
// Прежде адрес был ОДИН, полем `Podpiska`. У человека их две-три (панель,
// запасная панель, чужая), и держать их приходилось в блокноте: задал вторую,
// потерял первую. Модель хранит список, обновляется и попадает в список
// серверов только активная.

// Набор из установки, где подписка была одна. Прочитать его надо как список из
// одной записи, иначе обновление после установки новой версии пойдёт в никуда,
// а человек увидит «подписка не задана» там, где она задана год назад.
func TestStarayaPodpiskaChitaetsyaKakOdnaZapis(t *testing.T) {
	staryy := []byte(`{"servery":[],"podpiska":"https://panel.example.net/sub/tok"}`)

	var n Nabor
	if err := json.Unmarshal(staryy, &n); err != nil {
		t.Fatalf("старый набор не разобрался: %v", err)
	}
	n.PrivestiPodpiski()

	if len(n.Podpiski) != 1 {
		t.Fatalf("записей подписки %d, ждали одну: %+v", len(n.Podpiski), n.Podpiski)
	}
	if n.Podpiski[0].Adres != "https://panel.example.net/sub/tok" {
		t.Fatalf("адрес не перенесён: %q", n.Podpiski[0].Adres)
	}
	if n.Aktivnaya != n.Podpiski[0].Id {
		t.Fatalf("активная %q, а запись %q: единственная подписка обязана быть активной", n.Aktivnaya, n.Podpiski[0].Id)
	}
	if n.AdresAktivnoy() != "https://panel.example.net/sub/tok" {
		t.Fatalf("адрес активной %q", n.AdresAktivnoy())
	}
}

// Приведение обязано быть идемпотентным: его зовут при каждом чтении набора, и
// вторая запись той же подписки на каждом чтении означала бы список, растущий
// от одного только просмотра списка серверов.
func TestPrivedeniePodpisokIdempotentno(t *testing.T) {
	var n Nabor
	n.Podpiska = "https://panel.example.net/sub/tok"
	n.PrivestiPodpiski()
	n.PrivestiPodpiski()
	n.PrivestiPodpiski()

	if len(n.Podpiski) != 1 {
		t.Fatalf("записей %d после трёх приведений", len(n.Podpiski))
	}
}

// Набор без подписки вовсе: ни записей, ни активной, и пустой адрес. Пустая
// запись с пустым адресом означала бы «подписка задана» на первом же запуске.
func TestNaborBezPodpiskiOstaetsyaPustym(t *testing.T) {
	var n Nabor
	n.PrivestiPodpiski()

	if len(n.Podpiski) != 0 {
		t.Fatalf("на пустом наборе завелись записи: %+v", n.Podpiski)
	}
	if n.AdresAktivnoy() != "" {
		t.Fatalf("адрес активной %q при отсутствии подписок", n.AdresAktivnoy())
	}
}

// Активная указывает на запись, которой в списке нет (её удалили руками в
// профиле, приехавшем импортом). Молча отдать первую нельзя: обновление ушло бы
// не в ту подписку. Приведение чинит ссылку явно, выбирая первую и записывая
// это в поле, чтобы дальше все читали одно и то же.
func TestAktivnayaUkazyvaetNaPropavshuyuZapis(t *testing.T) {
	n := Nabor{
		Podpiski: []ZapisPodpiski{
			{Id: "aaa", Adres: "https://pervaya.example.net/sub"},
			{Id: "bbb", Adres: "https://vtoraya.example.net/sub"},
		},
		Aktivnaya: "net-takoy",
	}
	n.PrivestiPodpiski()

	if n.Aktivnaya != "aaa" {
		t.Fatalf("активная %q, ждали первую из списка", n.Aktivnaya)
	}
}

// Идентификатор считается от адреса: тот же адрес это та же подписка, сколько
// бы раз его ни вводили. Иначе повторный ввод плодит записи-двойники, и человек
// не понимает, какая из трёх одинаковых строк обновляется.
func TestIdPodpiskiSchitaetsyaOtAdresa(t *testing.T) {
	a := IdPodpiski("https://panel.example.net/sub/tok")
	b := IdPodpiski("https://panel.example.net/sub/tok")
	c := IdPodpiski("https://panel.example.net/sub/drugoy")

	if a == "" {
		t.Fatal("идентификатор пуст")
	}
	if a != b {
		t.Fatalf("один адрес дал разные идентификаторы: %q и %q", a, b)
	}
	if a == c {
		t.Fatal("разные адреса дали один идентификатор")
	}
}

// Идентификатор не должен быть обратимым в адрес: он уезжает на экран и в
// журнал, а адрес это секрет класса ключа. Проверка грубая и достаточная: в
// идентификаторе не встречается ни узел, ни хвост пути.
func TestIdPodpiskiNeSoderzhitAdresa(t *testing.T) {
	id := IdPodpiski("https://panel.example.net/sub/tok")
	for _, kusok := range []string{"panel", "example", "tok", "sub"} {
		if strings.Contains(id, kusok) {
			t.Fatalf("идентификатор %q несёт кусок адреса %q", id, kusok)
		}
	}
}

// Приведение обязано случаться на ЧТЕНИИ набора, а не по памяти того, кто
// пишет новую команду. Пока его звали руками, каждая новая дверь к набору была
// шансом забыть, и забытое приведение выглядит как «подписка не задана».
func TestChtenieNaboraPrivoditPodpiski(t *testing.T) {
	s := podstavnaya(t, nil)
	blob := []byte(`{"servery":[],"podpiska":"https://panel.example.net/sub/tok"}`)
	s.sekretyChitat = func() ([]byte, error) { return blob, nil }
	s.nabor = s.naborIzHranilishcha

	n, err := s.nabor()
	if err != nil {
		t.Fatalf("набор не прочитан: %v", err)
	}
	if len(n.Podpiski) != 1 || n.Aktivnaya == "" {
		t.Fatalf("чтение не привело подписки: %+v, активная %q", n.Podpiski, n.Aktivnaya)
	}
	if n.AdresAktivnoy() != "https://panel.example.net/sub/tok" {
		t.Fatalf("адрес активной %q", n.AdresAktivnoy())
	}
}

// Обновление идёт в АКТИВНУЮ подписку, а не в старое поле. Проверяется по тому,
// какой адрес увидела загрузка: пока она читала n.Podpiska, переключение
// активной не меняло ничего вовсе.
func TestObnovlenieBeretAdresAktivnoyPodpiski(t *testing.T) {
	s := podstavnaya(t, nil)
	hranilishcheProby(t, s, Nabor{
		Podpiski: []ZapisPodpiski{
			{Id: IdPodpiski("https://pervaya.example.net/sub"), Adres: "https://pervaya.example.net/sub"},
			{Id: IdPodpiski("https://vtoraya.example.net/sub"), Adres: "https://vtoraya.example.net/sub"},
		},
		Aktivnaya: IdPodpiski("https://vtoraya.example.net/sub"),
	})

	var sprosili string
	s.zagruzitPodpisku = func(ctx context.Context, adres string) (ssylki.Razbor, error) {
		sprosili = adres
		return ssylki.Razbor{Servery: []protokol.Server{
			{Id: "srv", Imya: "sr", Host: "s.example.net", Port: 443, Transport: "hy2", IzPodpiski: true},
		}}, nil
	}

	if _, _, err := s.obnovitPodpisku(context.Background()); err != nil {
		t.Fatalf("обновление отказало: %v", err)
	}
	if sprosili != "https://vtoraya.example.net/sub" {
		t.Fatalf("спросили %q, а активной была вторая", sprosili)
	}
}

// Запертый режим пускает мимо туннеля только то, что стоит в списке разрешённых.
// Адрес ЗАПАСНОЙ подписки обязан быть там наравне с активной: иначе первое же
// переключение в запертом режиме упирается в собственный killswitch, и
// починить это изнутри клиента нельзя.
func TestZapasnyePodpiskiPopadayutVRazreshyonnye(t *testing.T) {
	s := podstavnaya(t, nil)
	hranilishcheProby(t, s, Nabor{
		Servery: []protokol.Server{{Id: "srv", Host: "s.example.net", Port: 443, Transport: "hy2"}},
		Podpiski: []ZapisPodpiski{
			{Id: IdPodpiski("https://aktivnaya.example.net/sub"), Adres: "https://aktivnaya.example.net/sub"},
			{Id: IdPodpiski("https://zapasnaya.example.net/sub"), Adres: "https://zapasnaya.example.net/sub"},
		},
		Aktivnaya: IdPodpiski("https://aktivnaya.example.net/sub"),
	})
	s.naboryZhelaemye = func() []genkonfig.NaborPravil { return nil }

	var vidennye []string
	s.sobratAdresaSet = func(_ []protokol.Server, podpiska string, zagruzki ...string) ([]netip.Addr, error) {
		vidennye = append([]string{podpiska}, zagruzki...)
		return []netip.Addr{netip.MustParseAddr("192.0.2.1")}, nil
	}

	if _, err := s.adresaKandidatov(); err != nil {
		t.Fatalf("сбор адресов отказал: %v", err)
	}
	if !slices.Contains(vidennye, "https://zapasnaya.example.net/sub") {
		t.Fatalf("адрес запасной подписки не попал в разрешённые: %v", vidennye)
	}
	if !slices.Contains(vidennye, "https://aktivnaya.example.net/sub") {
		t.Fatalf("адрес активной подписки не попал в разрешённые: %v", vidennye)
	}
}

// Список подписок для экрана: идентификаторы и узлы, но НЕ адреса. Адрес это
// пропуск к ключам, и канал пускает интерактивного пользователя.
func TestListSubscriptionsNeOtdayotAdresa(t *testing.T) {
	s := podstavnaya(t, nil)
	hranilishcheProby(t, s, Nabor{})
	s.zagruzitPodpisku = func(context.Context, string) (ssylki.Razbor, error) {
		return ssylki.Razbor{Servery: []protokol.Server{serverProby()}}, nil
	}
	vypolnit(t, s, "addSubscription", map[string]string{"adres": "https://panel.example/sub/SEKRET-TOKEN"})

	o := vypolnit(t, s, "listSubscriptions", nil)
	if o.Oshib != nil {
		t.Fatalf("список подписок отказал: %+v", o.Oshib)
	}
	if strings.Contains(string(o.Telo), "SEKRET-TOKEN") {
		t.Fatalf("адрес подписки уехал в канал: %s", o.Telo)
	}
	if !strings.Contains(string(o.Telo), "panel.example") {
		t.Fatalf("узла подписки нет, опознать нечего: %s", o.Telo)
	}
}

// Первая подписка становится активной сама: человек, у которого она одна, не
// должен ещё и «выбирать» её отдельным действием.
func TestPervayaPodpiskaStanovitsyaAktivnoy(t *testing.T) {
	s := podstavnaya(t, nil)
	hranilishcheProby(t, s, Nabor{})
	s.zagruzitPodpisku = func(context.Context, string) (ssylki.Razbor, error) {
		return ssylki.Razbor{Servery: []protokol.Server{serverProby()}}, nil
	}

	vypolnit(t, s, "addSubscription", map[string]string{"adres": "https://pervaya.example/sub"})

	n, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}
	if n.AdresAktivnoy() != "https://pervaya.example/sub" {
		t.Fatalf("активной стала %q", n.AdresAktivnoy())
	}
}

// Вторая подписка ложится ПРО ЗАПАС и активную не подменяет. Иначе добавление
// адреса молча уводит список серверов на другую панель, а человек видел только
// «добавить».
func TestVtorayaPodpiskaNeMenyaetAktivnuyu(t *testing.T) {
	s := podstavnaya(t, nil)
	hranilishcheProby(t, s, Nabor{})
	s.zagruzitPodpisku = func(context.Context, string) (ssylki.Razbor, error) {
		return ssylki.Razbor{Servery: []protokol.Server{serverProby()}}, nil
	}
	vypolnit(t, s, "addSubscription", map[string]string{"adres": "https://pervaya.example/sub"})

	o := vypolnit(t, s, "addSubscription", map[string]string{"adres": "https://vtoraya.example/sub"})
	if o.Oshib != nil {
		t.Fatalf("вторая подписка отвергнута: %+v", o.Oshib)
	}

	n, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}
	if len(n.Podpiski) != 2 {
		t.Fatalf("записей %d, ждали две", len(n.Podpiski))
	}
	if n.AdresAktivnoy() != "https://pervaya.example/sub" {
		t.Fatalf("активная подменилась на %q", n.AdresAktivnoy())
	}
}

// Переключение активной тянет список серверов СРАЗУ: иначе человек переключил
// подписку и жмёт подключить, получая ключи прежней панели.
func TestPereklyuchenieAktivnoyTyanetSpisok(t *testing.T) {
	s := podstavnaya(t, nil)
	hranilishcheProby(t, s, Nabor{})
	var sprosili []string
	s.zagruzitPodpisku = func(_ context.Context, adres string) (ssylki.Razbor, error) {
		sprosili = append(sprosili, adres)
		return ssylki.Razbor{Servery: []protokol.Server{serverProby()}}, nil
	}
	vypolnit(t, s, "addSubscription", map[string]string{"adres": "https://pervaya.example/sub"})
	vypolnit(t, s, "addSubscription", map[string]string{"adres": "https://vtoraya.example/sub"})

	o := vypolnit(t, s, "setActiveSubscription", map[string]string{"id": IdPodpiski("https://vtoraya.example/sub")})
	if o.Oshib != nil {
		t.Fatalf("переключение отказало: %+v", o.Oshib)
	}
	if len(sprosili) == 0 || sprosili[len(sprosili)-1] != "https://vtoraya.example/sub" {
		t.Fatalf("после переключения спрашивали %v", sprosili)
	}
}

// Переключение на подписку, которой нет, это отказ с причиной, а не молчаливый
// выбор первой попавшейся.
func TestPereklyuchenieNaNesushchestvuyushchuyu(t *testing.T) {
	s := podstavnaya(t, nil)
	hranilishcheProby(t, s, Nabor{})

	o := vypolnit(t, s, "setActiveSubscription", map[string]string{"id": "net-takoy"})
	if o.Oshib == nil {
		t.Fatal("переключение на несуществующую подписку принято")
	}
}

// Удаление активной оставляет активной первую из оставшихся: набор без активной
// при непустом списке означал бы «подписка не задана» при заданных подписках.
func TestUdalenieAktivnoyPeredayotAktivnostOstavsheysya(t *testing.T) {
	s := podstavnaya(t, nil)
	hranilishcheProby(t, s, Nabor{})
	s.zagruzitPodpisku = func(context.Context, string) (ssylki.Razbor, error) {
		return ssylki.Razbor{Servery: []protokol.Server{serverProby()}}, nil
	}
	vypolnit(t, s, "addSubscription", map[string]string{"adres": "https://pervaya.example/sub"})
	vypolnit(t, s, "addSubscription", map[string]string{"adres": "https://vtoraya.example/sub"})

	o := vypolnit(t, s, "removeSubscription", map[string]string{"id": IdPodpiski("https://pervaya.example/sub")})
	if o.Oshib != nil {
		t.Fatalf("удаление отказало: %+v", o.Oshib)
	}

	n, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}
	if len(n.Podpiski) != 1 || n.AdresAktivnoy() != "https://vtoraya.example/sub" {
		t.Fatalf("после удаления активной: записей %d, активная %q", len(n.Podpiski), n.AdresAktivnoy())
	}
}

// Тело addSubscription несёт адрес, то есть секрет класса ключа. В журнал
// команд оно попадать не должно так же, как тело setSubscription.
func TestTeloAddSubscriptionNeLogiruetsya(t *testing.T) {
	if protokol.TeloMozhnoLogirovat("addSubscription") {
		t.Fatal("тело addSubscription печатается в журнал вместе с адресом подписки")
	}
}

// Отметка свежести принадлежит ЗАПИСИ, а не службе. Общая отметка одна на всех
// и после переключения врёт: говорит про свежесть чужой подписки.
func TestObnovlenieStavitOtmetkuAktivnoyZapisi(t *testing.T) {
	s := podstavnaya(t, nil)
	hranilishcheProby(t, s, Nabor{})
	s.zagruzitPodpisku = func(context.Context, string) (ssylki.Razbor, error) {
		return ssylki.Razbor{Servery: []protokol.Server{serverProby()}}, nil
	}
	vypolnit(t, s, "addSubscription", map[string]string{"adres": "https://pervaya.example/sub"})
	vypolnit(t, s, "addSubscription", map[string]string{"adres": "https://vtoraya.example/sub"})

	n, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}
	aktivnaya := n.zapisPodpiski(n.Aktivnaya)
	if aktivnaya == nil || aktivnaya.Obnovlena == nil {
		t.Fatal("у активной записи нет отметки обновления")
	}
	zapasnaya := n.zapisPodpiski(IdPodpiski("https://vtoraya.example/sub"))
	if zapasnaya == nil {
		t.Fatal("запасная запись пропала")
	}
	if zapasnaya.Obnovlena != nil {
		t.Fatal("у запасной записи стоит отметка обновления, хотя её никто не грузил")
	}
}

// Раз в 12 часов обновляются ВСЕ добавленные подписки, а не только активная.
// Ключи запасной складываются в её запись и в список серверов не лезут: список
// на экране и конфиг ядра остаются про ту подписку, по которой человек работает.
func TestObnovlenieHoditVoVsePodpiski(t *testing.T) {
	s := podstavnaya(t, nil)
	hranilishcheProby(t, s, Nabor{})
	var sprosili []string
	s.zagruzitPodpisku = func(_ context.Context, adres string) (ssylki.Razbor, error) {
		sprosili = append(sprosili, adres)
		if adres == "https://zapasnaya.example/sub" {
			return ssylki.Razbor{Servery: []protokol.Server{izPodpiski(vtoroyServer())}}, nil
		}
		return ssylki.Razbor{Servery: []protokol.Server{izPodpiski(serverProby())}}, nil
	}
	vypolnit(t, s, "addSubscription", map[string]string{"adres": "https://aktivnaya.example/sub"})
	vypolnit(t, s, "addSubscription", map[string]string{"adres": "https://zapasnaya.example/sub"})

	sprosili = nil
	if _, _, err := s.obnovitVsePodpiski(context.Background()); err != nil {
		t.Fatalf("обход подписок отказал: %v", err)
	}
	if !slices.Contains(sprosili, "https://aktivnaya.example/sub") || !slices.Contains(sprosili, "https://zapasnaya.example/sub") {
		t.Fatalf("обошли %v, а подписки две", sprosili)
	}

	n, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}
	zapas := n.zapisPodpiski(IdPodpiski("https://zapasnaya.example/sub"))
	if zapas == nil || len(zapas.Servery) != 1 {
		t.Fatalf("ключи запасной не сложены в её запись: %+v", zapas)
	}
	if zapas.Obnovlena == nil {
		t.Fatal("у запасной нет отметки обновления, хотя её только что тянули")
	}
	for _, srv := range n.Servery {
		if srv.Id == vtoroyServer().Id {
			t.Fatal("сервер запасной подписки попал в рабочий список")
		}
	}
}

// Отказ одной подписки не отменяет обход остальных и записывается в её строку:
// иначе одна просроченная панель молча останавливает обновление всех.
func TestOtkazOdnoyPodpiskiNeLomaetObhod(t *testing.T) {
	s := podstavnaya(t, nil)
	hranilishcheProby(t, s, Nabor{})
	s.zagruzitPodpisku = func(_ context.Context, adres string) (ssylki.Razbor, error) {
		if adres == "https://mertvaya.example/sub" {
			return ssylki.Razbor{}, ssylki.ErrPodpiskaNedostupna
		}
		return ssylki.Razbor{Servery: []protokol.Server{serverProby()}}, nil
	}
	vypolnit(t, s, "addSubscription", map[string]string{"adres": "https://aktivnaya.example/sub"})
	vypolnit(t, s, "addSubscription", map[string]string{"adres": "https://mertvaya.example/sub"})
	vypolnit(t, s, "addSubscription", map[string]string{"adres": "https://tretya.example/sub"})

	if _, _, err := s.obnovitVsePodpiski(context.Background()); err != nil {
		t.Fatalf("обход упал целиком из-за одной подписки: %v", err)
	}

	n, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}
	mertvaya := n.zapisPodpiski(IdPodpiski("https://mertvaya.example/sub"))
	if mertvaya == nil || mertvaya.Otkaz == "" {
		t.Fatalf("причина отказа не записана в строку подписки: %+v", mertvaya)
	}
	tretya := n.zapisPodpiski(IdPodpiski("https://tretya.example/sub"))
	if tretya == nil || len(tretya.Servery) != 1 {
		t.Fatalf("третью подписку не обошли после отказа второй: %+v", tretya)
	}
}

// Переключение берёт ключи ИЗ ЗАПИСИ и в сеть не ходит: они уже приехали
// расписанием. Это и есть смысл обхода всех подписок, и заодно переключение
// работает, когда панель молчит.
func TestPereklyuchenieNeHoditVSetIRabotaetPriMolchashcheyPaneli(t *testing.T) {
	s := podstavnaya(t, nil)
	hranilishcheProby(t, s, Nabor{})
	s.zagruzitPodpisku = func(_ context.Context, adres string) (ssylki.Razbor, error) {
		if adres == "https://zapasnaya.example/sub" {
			return ssylki.Razbor{Servery: []protokol.Server{izPodpiski(vtoroyServer())}}, nil
		}
		return ssylki.Razbor{Servery: []protokol.Server{izPodpiski(serverProby())}}, nil
	}
	vypolnit(t, s, "addSubscription", map[string]string{"adres": "https://aktivnaya.example/sub"})
	vypolnit(t, s, "addSubscription", map[string]string{"adres": "https://zapasnaya.example/sub"})
	if _, _, err := s.obnovitVsePodpiski(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Панель молчит: после этого ни один поход в сеть не может помочь.
	s.zagruzitPodpisku = func(context.Context, string) (ssylki.Razbor, error) {
		return ssylki.Razbor{}, ssylki.ErrPodpiskaNedostupna
	}
	o := vypolnit(t, s, "setActiveSubscription", map[string]string{"id": IdPodpiski("https://zapasnaya.example/sub")})
	if o.Oshib != nil {
		t.Fatalf("переключение отказало при готовых ключах: %+v", o.Oshib)
	}

	n, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}
	if len(n.Servery) != 1 || n.Servery[0].Id != vtoroyServer().Id {
		t.Fatalf("после переключения в списке %+v, ждали сервер запасной", n.Servery)
	}
}

// Ручные серверы не принадлежат ни одной подписке и переключение их не трогает.
func TestPereklyuchenieNeTeryaetRuchnyeServery(t *testing.T) {
	s := podstavnaya(t, nil)
	ruchnoy := vtoroyServer()
	hranilishcheProby(t, s, Nabor{Servery: []protokol.Server{ruchnoy}})
	s.zagruzitPodpisku = func(context.Context, string) (ssylki.Razbor, error) {
		return ssylki.Razbor{Servery: []protokol.Server{izPodpiski(serverProby())}}, nil
	}
	vypolnit(t, s, "addSubscription", map[string]string{"adres": "https://pervaya.example/sub"})
	vypolnit(t, s, "addSubscription", map[string]string{"adres": "https://vtoraya.example/sub"})
	if _, _, err := s.obnovitVsePodpiski(context.Background()); err != nil {
		t.Fatal(err)
	}
	vypolnit(t, s, "setActiveSubscription", map[string]string{"id": IdPodpiski("https://vtoraya.example/sub")})

	n, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}
	nashli := false
	for _, srv := range n.Servery {
		if srv.Id == ruchnoy.Id {
			nashli = true
		}
	}
	if !nashli {
		t.Fatalf("ручной сервер пропал при переключении подписки: %+v", n.Servery)
	}
}

// Разбор подписки метит свои серверы (ssylki/podpiska.go). Фикстура, которая
// этого не делает, подсовывает службе ручной сервер под видом подписочного, и
// тест начинает судить не то, что думает.
func izPodpiski(s protokol.Server) protokol.Server {
	s.IzPodpiski = true
	return s
}

// Строка запасной подписки обязана говорить, есть ли у неё готовые ключи и не
// отказала ли она в прошлый обход. Без этих двух полей живая запасная и
// просроченная выглядят на экране одинаково, а узнать разницу можно только
// переключившись на неё.
func TestSpisokPodpisokNazyvaetKlyuchiIOtkaz(t *testing.T) {
	s := podstavnaya(t, nil)
	hranilishcheProby(t, s, Nabor{})
	s.zagruzitPodpisku = func(_ context.Context, adres string) (ssylki.Razbor, error) {
		if adres == "https://mertvaya.example/sub" {
			return ssylki.Razbor{}, ssylki.ErrPodpiskaIstekla
		}
		return ssylki.Razbor{Servery: []protokol.Server{izPodpiski(serverProby()), izPodpiski(vtoroyServer())}}, nil
	}
	vypolnit(t, s, "addSubscription", map[string]string{"adres": "https://aktivnaya.example/sub"})
	vypolnit(t, s, "addSubscription", map[string]string{"adres": "https://zapasnaya.example/sub"})
	vypolnit(t, s, "addSubscription", map[string]string{"adres": "https://mertvaya.example/sub"})
	if _, _, err := s.obnovitVsePodpiski(context.Background()); err != nil {
		t.Fatal(err)
	}

	o := vypolnit(t, s, "listSubscriptions", nil)
	var telo struct {
		Podpiski []struct {
			Id       string `json:"id"`
			Serverov int    `json:"serverov"`
			Otkaz    string `json:"otkaz"`
		} `json:"podpiski"`
	}
	if err := json.Unmarshal(o.Telo, &telo); err != nil {
		t.Fatalf("ответ не разбирается: %v", err)
	}
	po := map[string]struct {
		Id       string `json:"id"`
		Serverov int    `json:"serverov"`
		Otkaz    string `json:"otkaz"`
	}{}
	for _, p := range telo.Podpiski {
		po[p.Id] = p
	}
	if n := po[IdPodpiski("https://zapasnaya.example/sub")].Serverov; n != 2 {
		t.Fatalf("у запасной %d серверов, а обход привёз два", n)
	}
	if po[IdPodpiski("https://mertvaya.example/sub")].Otkaz == "" {
		t.Fatal("просроченная подписка выглядит как живая: причины нет")
	}
	if n := po[IdPodpiski("https://aktivnaya.example/sub")].Serverov; n != 2 {
		t.Fatalf("у активной %d серверов: её ключи лежат в рабочем списке и считаются оттуда", n)
	}
}
