package set

import (
	"errors"
	"fmt"
	"maps"
	"net/netip"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"golang.org/x/sys/windows"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/sostoyanie"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/wfp"
)

// Правдоподобный вывод netsh для одного профиля. Ключи слева намеренно
// РУССКИЕ: разбор обязан опираться на значения, а не на имена полей.
const vyvodProfilyaRu = `
Параметры профиля "Домен":
----------------------------------------------------------------------
Состояние                             ON
Политика брандмауэра                  BlockInbound,AllowOutbound
InboundUserNotification               Disable
`

const vyvodProfilyaEn = `
Domain Profile Settings:
----------------------------------------------------------------------
State                                 OFF
Firewall Policy                       BlockInbound,AllowOutbound
`

type zapis struct {
	argumenty []string
}

// perehvat подменяет netsh, каталог данных, реестр, службу брандмауэра и
// запрет WFP. Запрет подставной свежий на каждый вызов: состояние одного
// теста не доезжает до следующего.
func perehvat(t *testing.T, otvet func([]string) (string, error)) *[]zapis {
	t.Helper()
	var zhurnal []zapis
	prezhniy := vypolnit
	prezhniyKat := katalogDannyh
	// t.TempDir() отдаёт НОВЫЙ каталог на каждый вызов, поэтому запись и чтение
	// отката разъезжались бы по разным папкам, а тест винил бы код.
	kat := t.TempDir()
	katalogDannyh = func() string { return kat }
	vypolnit = func(a []string) (string, error) {
		zhurnal = append(zhurnal, zapis{argumenty: append([]string{}, a...)})
		return otvet(a)
	}
	bezReestra(t)
	// Служба брандмауэра живой машины не должна решать исход теста.
	prezhneeMps := sostoyanieMpsSvc
	sostoyanieMpsSvc = func() (bool, bool, error) { return true, false, nil }
	podstavitZamok(t)
	t.Cleanup(func() { vypolnit = prezhniy; katalogDannyh = prezhniyKat; sostoyanieMpsSvc = prezhneeMps })
	return &zhurnal
}

// staryyOtkat это файл отката замка версии 1.9.4 и раньше: политика профилей
// и имена правил netsh.
func staryyOtkat(namerenno bool) Otkat {
	return Otkat{
		Namerenno: namerenno,
		Pravila:   []string{PravAllowTun, PravAllowSrv},
		Profili:   []ProfilDo{{Imya: "domain", Vklyuchen: true, Politika: "BlockInbound,AllowOutbound"}},
	}
}

func komandy(zhurnal []zapis) []string {
	var itog []string
	for _, z := range zhurnal {
		itog = append(itog, strings.Join(z.argumenty, " "))
	}
	return itog
}

// L16 аудита 1.8.0: netsh отказывает, потому что служба брандмауэра
// остановлена или отключена. Теперь это касается только снятия замка прежней
// версии, и человек видит firewall-disabled с причиной, а не общий отказ.
func TestOtkazSnyatiyaStarogoZamkaPriVyklyuchennomMpsSvc(t *testing.T) {
	perehvat(t, func([]string) (string, error) { return "", errors.New("netsh: служба не запущена") })
	if err := ZapisatOtkat(staryyOtkat(true)); err != nil {
		t.Fatal(err)
	}
	for _, sl := range []struct {
		imya                  string
		rabotaet, otklyuchena bool
		vyklyuchen            bool
	}{
		{"остановлена", false, false, true},
		{"отключена", false, true, true},
		{"работает", true, false, false},
	} {
		sostoyanieMpsSvc = func() (bool, bool, error) { return sl.rabotaet, sl.otklyuchena, nil }
		err := VyklyuchitVesTrafik()
		if err == nil {
			t.Fatalf("%s: отказ netsh проглочен", sl.imya)
		}
		if errors.Is(err, ErrBrandmauerVyklyuchen) != sl.vyklyuchen {
			t.Errorf("%s: %v", sl.imya, err)
		}
		if sl.vyklyuchen && !strings.Contains(err.Error(), "MpsSvc") {
			t.Errorf("%s: причина не названа: %v", sl.imya, err)
		}
	}
	// Состояние не прочиталось: отказ прежний, без догадок.
	sostoyanieMpsSvc = func() (bool, bool, error) { return false, false, errors.New("тест: нет доступа") }
	if err := VyklyuchitVesTrafik(); errors.Is(err, ErrBrandmauerVyklyuchen) {
		t.Fatalf("непрочитанное состояние выдано за выключенный брандмауэр: %v", err)
	}
	if _, err := ProchitatOtkat(); err != nil {
		t.Fatalf("файл отката снят при неснятом замке: %v", err)
	}
}

func TestSostoyanieMpsSvcChitaetsyaBezAdmina(t *testing.T) {
	if _, _, err := sostoyanieMpsSvcSistemnoe(); err != nil {
		t.Fatalf("состояние службы брандмауэра не прочитано: %v", err)
	}
}

// bezReestra отключает чтение состояния из реестра.
//
// Тест описывает ответы netsh фикстурой, а реестр живой машины прошёл бы мимо
// неё: судья спорил бы сам с собой и винил код. Реестровый путь проверяется
// своими судьями, они рядом.
func bezReestra(t *testing.T) {
	t.Helper()
	prezhniy := chitatIzReestra
	chitatIzReestra = profilIzFikstury
	// Имена правил тоже: уборка идёт по полному списку, как видит её фикстура.
	prezhnieImena := imenaPravilVReestre
	imenaPravilVReestre = func() (map[string]bool, error) {
		return nil, errors.New("тест: реестр правил подменён netsh")
	}
	t.Cleanup(func() { chitatIzReestra = prezhniy; imenaPravilVReestre = prezhnieImena })
}

// Разбор вывода netsh живёт только в тестах (П2 аудита 1.8.0): продукт
// читает реестр, а тесты описывают машину выводом show, и живая фикстура
// меняет его вслед за set. Значения netsh не переводятся, ключи слева
// переводятся, поэтому опознание идёт по значению.
var (
	reSostoyanie = regexp.MustCompile(`(?im)^\s*\S+\s+(ON|OFF)\s*$`)
	rePolitika   = regexp.MustCompile(`(?im)^\s*\S.*?\s+((?:Block|Allow)Inbound,(?:Block|Allow)Outbound)\s*$`)
)

// profilIzFikstury читает профиль из вывода show подставного netsh.
func profilIzFikstury(profil string) (ProfilDo, error) {
	vyhod, err := vypolnit([]string{"advfirewall", "show", profil + "profile"})
	if err != nil {
		return ProfilDo{}, err
	}
	pr := ProfilDo{Imya: profil}
	m := reSostoyanie.FindStringSubmatch(vyhod)
	if m == nil {
		return ProfilDo{}, fmt.Errorf("в фикстуре профиля %s нет ON или OFF", profil)
	}
	pr.Vklyuchen = strings.EqualFold(m[1], "ON")
	mp := rePolitika.FindStringSubmatch(vyhod)
	if mp == nil {
		return ProfilDo{}, fmt.Errorf("в фикстуре профиля %s нет политики", profil)
	}
	pr.Politika = mp[1]
	return pr, nil
}

// Г1 аудита 1.8.0: уборка на старте зовёт netsh только для правил, которые
// стоят, и узнаёт их по точному имени.
func TestPodmestiSnimaetTolkoStoyashchie(t *testing.T) {
	zhurnal := perehvat(t, func([]string) (string, error) { return "Ok.", nil })
	imenaPravilVReestre = func() (map[string]bool, error) {
		return map[string]bool{
			PravAllowTun: true, PravAllowProc + "-3": true,
			// Похожие имена чужие: снятие по маске запрещено.
			PravAllowTun + "-Chuzhoe": true, "Chuzhoe-Pravilo": true,
		}, nil
	}
	nashli, _, err := podmesti()
	if err != nil || !nashli {
		t.Fatalf("nashli=%v err=%v", nashli, err)
	}
	snyali := komandy(*zhurnal)
	ozhidali := []string{
		"advfirewall firewall delete rule name=" + PravAllowTun,
		"advfirewall firewall delete rule name=" + PravAllowProc + "-3",
	}
	if strings.Join(snyali, "\n") != strings.Join(ozhidali, "\n") {
		t.Fatalf("команды netsh:\n%s\nожидались:\n%s", strings.Join(snyali, "\n"), strings.Join(ozhidali, "\n"))
	}
}

func TestPodmestiBezNashihPravilNeZovyotNetsh(t *testing.T) {
	zhurnal := perehvat(t, func([]string) (string, error) { return "Ok.", nil })
	imenaPravilVReestre = func() (map[string]bool, error) { return map[string]bool{"Chuzhoe-Pravilo": true}, nil }
	if nashli, _, err := podmesti(); err != nil || nashli || len(*zhurnal) != 0 {
		t.Fatalf("nashli=%v err=%v, команд netsh %d", nashli, err, len(*zhurnal))
	}
}

func TestImyaPravilaIzZapisiReestra(t *testing.T) {
	zapis := `v2.33|Action=Allow|Active=TRUE|Dir=Out|Protocol=17|RA4=10.0.0.1|Name=Affory-Allow-Server-Udp|`
	if got := imyaIzZapisi(zapis); got != PravAllowSrvUdp {
		t.Fatalf("имя %q", got)
	}
	if got := imyaIzZapisi(`v2.33|Action=Block|Dir=In|`); got != "" {
		t.Fatalf("имя без поля Name: %q", got)
	}
}

// Живой реестр читается и без прав на запись: там стоят встроенные правила
// Windows, и пустой ответ значит, что читали не то.
func TestImenaPravilChitayutsyaIzZhivogoReestra(t *testing.T) {
	est, err := chitatImenaPravil()
	if err != nil {
		t.Fatal(err)
	}
	if len(est) == 0 {
		t.Fatal("в постоянном хранилище брандмауэра не нашлось ни одного правила")
	}
}

func otvetProfiley(vyvod string) func([]string) (string, error) {
	// Заглушка ЖИВАЯ: смена политики через set меняет то, что отвечает show.
	// Мёртвая заглушка утверждала бы, что машина открыта, сразу после того как мы
	// её заперли, и проверка запертости прошла бы мимо предмета.
	zaperta := false
	return func(a []string) (string, error) {
		soed := strings.Join(a, " ")
		if strings.Contains(soed, "set") && strings.Contains(soed, "firewallpolicy") {
			zaperta = strings.Contains(soed, "blockoutbound")
			return "Ok.", nil
		}
		if len(a) > 2 && a[1] == "show" {
			if zaperta {
				return strings.Replace(vyvod, "AllowOutbound", "BlockOutbound", -1), nil
			}
			return vyvod, nil
		}
		return "Ok.", nil
	}
}

func obraztsovoeRazreshyonnoe() Razreshyonnoe {
	return Razreshyonnoe{
		AdresTun:  netip.MustParseAddr("172.19.0.1"),
		Kandidaty: []netip.Addr{netip.MustParseAddr("192.0.2.225")},
		Porty:     []string{"443"},
		Shlyuz:    netip.MustParseAddr("10.7.0.1"),
		Resolver:  netip.MustParseAddr("10.7.0.1"),
		Protsessy: []string{`C:\Program Files\Affory\xray.exe`, `C:\Program Files\Affory\affory-svc.exe`},
	}
}

func TestSostoyanieChitaetsyaPoZnacheniyuANePoKlyuchu(t *testing.T) {
	// netsh localises the KEYS but not the VALUES. Parsing by key name works
	// until the first machine with another display language, and then it fails
	// by reporting "firewall is off" on a firewall that is on.
	perehvat(t, otvetProfiley(vyvodProfilyaRu))
	p, err := SostoyanieProfiley()
	if err != nil {
		t.Fatal(err)
	}
	if len(p) != 3 || !p[0].Vklyuchen || p[0].Politika != "BlockInbound,AllowOutbound" {
		t.Fatalf("разобрано неверно: %+v", p)
	}

	perehvat(t, otvetProfiley(vyvodProfilyaEn))
	p, err = SostoyanieProfiley()
	if err != nil {
		t.Fatal(err)
	}
	if p[0].Vklyuchen {
		t.Fatal("OFF прочитан как включённый")
	}
}

// Файл отката пишется ДО запрета: служба, умершая между ними, оставила бы
// машину запертой без записи о том, что запрет наш и намеренный ли.
func TestOtkatZapisyvaetsyaDoZapreta(t *testing.T) {
	perehvat(t, otvetProfiley(vyvodProfilyaRu))
	z := podstavitZamok(t)
	var vMoment Otkat
	var oshibkaVMoment error
	z.priPostanovke = func() { vMoment, oshibkaVMoment = ProchitatOtkat() }
	if err := VklyuchitVesTrafik(obraztsovoeRazreshyonnoe(), true); err != nil {
		t.Fatal(err)
	}
	if oshibkaVMoment != nil || !vMoment.Wfp || !vMoment.Namerenno {
		t.Fatalf("в момент постановки файл %+v, %v", vMoment, oshibkaVMoment)
	}
	if !z.stoit() {
		t.Fatal("запрет не встал")
	}
}

// Опыт в госте 18.10.2026: при выключенной службе брандмауэра netsh отказывает
// на всём. Запрет на WFP брандмауэра не касается вовсе: ни включение, ни
// выключение не зовут netsh, и режим работает при остановленной MpsSvc.
//
// Заодно здесь стережётся прежний дефект: блокировка IPv6 принадлежит
// туннелю, а не режиму, и выключение режима её не снимает.
func TestZapretNeZovyotNetsh(t *testing.T) {
	zhurnal := perehvat(t, func(a []string) (string, error) {
		return "", errors.New("netsh: An error occurred while attempting to contact the Windows Defender Firewall service")
	})
	sostoyanieMpsSvc = func() (bool, bool, error) { return false, true, nil }
	z := podstavitZamok(t)
	if err := VklyuchitVesTrafik(obraztsovoeRazreshyonnoe(), true); err != nil {
		t.Fatalf("режим не включился при выключенной MpsSvc: %v", err)
	}
	if !z.stoit() {
		t.Fatal("запрет не встал")
	}
	if err := VyklyuchitVesTrafik(); err != nil {
		t.Fatalf("режим не выключился при выключенной MpsSvc: %v", err)
	}
	if z.stoit() {
		t.Fatal("запрет остался")
	}
	if len(*zhurnal) != 0 {
		t.Fatalf("netsh звался: %v", komandy(*zhurnal))
	}
	if _, err := ProchitatOtkat(); !errors.Is(err, ErrOtkataNet) {
		t.Fatalf("файл отката остался: %v", err)
	}
}

// Отказ постановки откатывает транзакцию целиком: в системе не меняется
// ничего, и файл отката обязан вернуться к прежнему. Оставленный, он заставлял
// выключение режима и «Отключить» раз за разом снимать то, чего нет (опыт в
// госте 18.10.2026).
func TestOtkazPostanovkiNeOstavlyaetOtkata(t *testing.T) {
	t.Run("первое включение", func(t *testing.T) {
		perehvat(t, otvetProfiley(vyvodProfilyaRu))
		z := podstavitZamok(t)
		z.otkaz = errors.New("тест: транзакция отклонена")
		if err := VklyuchitVesTrafik(obraztsovoeRazreshyonnoe(), true); err == nil {
			t.Fatal("отказ проглочен")
		}
		if _, err := ProchitatOtkat(); !errors.Is(err, ErrOtkataNet) {
			t.Fatalf("файл отката остался: %v", err)
		}
	})
	t.Run("поверх замка прежней версии", func(t *testing.T) {
		zhurnal := perehvat(t, otvetProfiley(vyvodProfilyaRu))
		z := podstavitZamok(t)
		if err := ZapisatOtkat(staryyOtkat(true)); err != nil {
			t.Fatal(err)
		}
		z.otkaz = errors.New("тест: транзакция отклонена")
		if err := VklyuchitVesTrafik(obraztsovoeRazreshyonnoe(), false); err == nil {
			t.Fatal("отказ проглочен")
		}
		o, err := ProchitatOtkat()
		if err != nil || o.Wfp || len(o.Profili) != 1 || !o.Namerenno {
			t.Fatalf("прежний файл не возвращён: %+v, %v", o, err)
		}
		// Прежний замок остаётся стоять: снимать его, не поставив новый, значит
		// распечатать машину.
		if len(*zhurnal) != 0 {
			t.Fatalf("прежний замок тронут при отказе нового: %v", komandy(*zhurnal))
		}
	})
}

// Недоступная служба базовой фильтрации это «режим недоступен», а не
// «повтори»: повтор без неё не сработает никогда.
func TestBfeNedostupnaEtoRezhimNedostupen(t *testing.T) {
	perehvat(t, otvetProfiley(vyvodProfilyaRu))
	z := podstavitZamok(t)
	z.otkaz = fmt.Errorf("FwpmEngineOpen0: %w", wfp.ErrNedostupen)
	err := VklyuchitVesTrafik(obraztsovoeRazreshyonnoe(), true)
	if !errors.Is(err, ErrBrandmauerVyklyuchen) || !errors.Is(err, wfp.ErrNedostupen) {
		t.Fatalf("отказ %v", err)
	}
}

// Пересборка (добавили сервер, переподнялся туннель) идёт одной заменой, без
// снятия между старым и новым: прежде между двумя вызовами netsh туннель стоял
// без разрешения, и ping рвался на каждом переподъёме (С3 аудита 1.6.1).
func TestPeresborkaOdnoyZamenoy(t *testing.T) {
	perehvat(t, otvetProfiley(vyvodProfilyaRu))
	z := podstavitZamok(t)
	r := obraztsovoeRazreshyonnoe()
	if err := VklyuchitVesTrafik(r, true); err != nil {
		t.Fatal(err)
	}
	r.Kandidaty = append(r.Kandidaty, netip.MustParseAddr("198.51.100.9"))
	if err := VklyuchitVesTrafik(r, true); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(z.vyzovy, []string{"postavit", "postavit"}) {
		t.Fatalf("вызовы %v", z.vyzovy)
	}
	srv, _ := z.vid(vidServery, false)
	if !estUslovie(srv, usSet("198.51.100.9/32")) {
		t.Fatal("новый сервер не попал в разрешение")
	}
}

// Отказ пересборки не снимает защиту: машина заперта нами, и распечатать её
// отказом значит снять защиту ровно тогда, когда что-то пошло не так.
func TestOtkazPeresborkiOstavlyaetZashchitu(t *testing.T) {
	perehvat(t, otvetProfiley(vyvodProfilyaRu))
	z := podstavitZamok(t)
	if err := VklyuchitVesTrafik(obraztsovoeRazreshyonnoe(), true); err != nil {
		t.Fatal(err)
	}
	do := maps.Clone(z.filtry)
	z.otkaz = errors.New("тест: транзакция отклонена")
	if err := VklyuchitVesTrafik(obraztsovoeRazreshyonnoe(), true); err == nil {
		t.Fatal("отказ пересборки проглочен")
	}
	if !reflect.DeepEqual(do, z.filtry) {
		t.Fatal("отказ пересборки тронул запрет")
	}
	if o, err := ProchitatOtkat(); err != nil || !o.Wfp {
		t.Fatalf("файл отката после отказа пересборки: %+v, %v", o, err)
	}
}

// Namerenno только поднимается: понизить его пересборкой значило бы
// разрешить следующему старту службы распечатать машину, которую заперли
// осознанно.
func TestPeresborkaNeSnizhaetNamerennost(t *testing.T) {
	perehvat(t, otvetProfiley(vyvodProfilyaRu))
	if err := VklyuchitVesTrafik(obraztsovoeRazreshyonnoe(), true); err != nil {
		t.Fatal(err)
	}
	if err := VklyuchitVesTrafik(obraztsovoeRazreshyonnoe(), false); err != nil {
		t.Fatal(err)
	}
	if o, err := ProchitatOtkat(); err != nil || !o.Namerenno {
		t.Fatalf("намеренность понижена: %+v, %v", o, err)
	}
}

// Машина, запертая версией 1.9.4: включение ставит запрет WFP и только ПОСЛЕ
// этого снимает прежний замок. Наоборот означало бы окно, в котором машина
// открыта.
func TestPerehodSoStarogoZamka(t *testing.T) {
	zhurnal := perehvat(t, otvetProfiley(vyvodProfilyaRu))
	z := podstavitZamok(t)
	if err := ZapisatOtkat(staryyOtkat(true)); err != nil {
		t.Fatal(err)
	}
	z.priPostanovke = func() {
		if len(*zhurnal) != 0 {
			t.Errorf("прежний замок тронут до постановки запрета: %v", komandy(*zhurnal))
		}
	}
	if err := VklyuchitVesTrafik(obraztsovoeRazreshyonnoe(), false); err != nil {
		t.Fatal(err)
	}
	k := komandy(*zhurnal)
	for _, nuzhno := range []string{
		"advfirewall set domainprofile firewallpolicy blockinbound,allowoutbound",
		"advfirewall firewall delete rule name=" + PravAllowTun,
		"advfirewall firewall delete rule name=" + PravAllowSrv,
	} {
		if !slices.Contains(k, nuzhno) {
			t.Errorf("не было команды %q: %v", nuzhno, k)
		}
	}
	o, err := ProchitatOtkat()
	if err != nil || !o.Wfp || len(o.Profili) != 0 || len(o.Pravila) != 0 || !o.Namerenno {
		t.Fatalf("файл после перехода %+v, %v", o, err)
	}
	if !z.stoit() {
		t.Fatal("запрет не стоит")
	}
}

// Прежний замок не снялся (служба брандмауэра отключена): включение не
// падает, запрет уже стоит, а прежний замок остаётся в файле, и выключение
// режима снимает его, когда netsh снова работает.
func TestStaryyZamokNeSnyatZapretStoit(t *testing.T) {
	lomat := true
	zhivoy := otvetProfiley(vyvodProfilyaRu)
	zhurnal := perehvat(t, func(a []string) (string, error) {
		if lomat && len(a) > 1 && a[1] != "show" {
			return "", errors.New("netsh: служба брандмауэра не отвечает")
		}
		return zhivoy(a)
	})
	z := podstavitZamok(t)
	if err := ZapisatOtkat(staryyOtkat(true)); err != nil {
		t.Fatal(err)
	}
	if err := VklyuchitVesTrafik(obraztsovoeRazreshyonnoe(), true); err != nil {
		t.Fatalf("включение упало на прежнем замке: %v", err)
	}
	o, err := ProchitatOtkat()
	if err != nil || !o.Wfp || len(o.Profili) != 1 {
		t.Fatalf("прежний замок пропал из файла: %+v, %v", o, err)
	}

	lomat = false
	*zhurnal = nil
	if err := VyklyuchitVesTrafik(); err != nil {
		t.Fatal(err)
	}
	if z.stoit() {
		t.Fatal("запрет остался")
	}
	if !slices.Contains(komandy(*zhurnal), "advfirewall set domainprofile firewallpolicy blockinbound,allowoutbound") {
		t.Fatalf("прежняя политика не возвращена: %v", komandy(*zhurnal))
	}
	if _, err := ProchitatOtkat(); !errors.Is(err, ErrOtkataNet) {
		t.Fatalf("файл отката остался: %v", err)
	}
}

// Замок прежней версии снимается в ОБРАТНОМ порядке: сначала политика, потом
// правила, и выключенный нами профиль выключается обратно.
func TestStaryyZamokSnimaetsyaVObratnomPoryadke(t *testing.T) {
	zhurnal := perehvat(t, otvetProfiley(vyvodProfilyaRu))
	o := staryyOtkat(true)
	o.Profili = append(o.Profili, ProfilDo{Imya: "public", Vklyuchen: false, Politika: "AllowInbound,AllowOutbound"})
	o.Pravila = append(o.Pravila, PravAllowProc+"-1")
	if err := ZapisatOtkat(o); err != nil {
		t.Fatal(err)
	}
	if err := VyklyuchitVesTrafik(); err != nil {
		t.Fatal(err)
	}
	k := komandy(*zhurnal)
	ozhidali := []string{
		"advfirewall set domainprofile firewallpolicy blockinbound,allowoutbound",
		"advfirewall set publicprofile firewallpolicy allowinbound,allowoutbound",
		"advfirewall firewall delete rule name=" + PravAllowTun,
		"advfirewall firewall delete rule name=" + PravAllowSrv,
		"advfirewall firewall delete rule name=" + PravAllowProc + "-1",
		"advfirewall set publicprofile state off",
	}
	if !slices.Equal(k, ozhidali) {
		t.Fatalf("команды:\n%s\nожидались:\n%s", strings.Join(k, "\n"), strings.Join(ozhidali, "\n"))
	}
	// Правило IPv6 принадлежит туннелю, а не режиму.
	for _, s := range k {
		if strings.Contains(s, ImyaPravilaIPv6) {
			t.Fatal("выключение режима сняло блокировку IPv6")
		}
	}
}

func TestVyklyuchenieSnimaetZapretIFayl(t *testing.T) {
	perehvat(t, otvetProfiley(vyvodProfilyaRu))
	z := podstavitZamok(t)
	if err := VklyuchitVesTrafik(obraztsovoeRazreshyonnoe(), true); err != nil {
		t.Fatal(err)
	}
	if zaperta, err := VesTrafikVklyuchyon(); err != nil || !zaperta {
		t.Fatalf("запертая машина не опознана: %v, %v", zaperta, err)
	}
	if err := VyklyuchitVesTrafik(); err != nil {
		t.Fatal(err)
	}
	if len(z.filtry) != 0 {
		t.Fatalf("фильтры остались: %d", len(z.filtry))
	}
	if zaperta, err := VesTrafikVklyuchyon(); err != nil || zaperta {
		t.Fatalf("открытая машина числится запертой: %v, %v", zaperta, err)
	}
	if _, err := ProchitatOtkat(); !errors.Is(err, ErrOtkataNet) {
		t.Fatalf("файл отката остался: %v", err)
	}
}

// Файл отката удаляется только после полного снятия: он единственное, по
// чему следующий старт поймёт, что машина осталась запертой.
func TestOtkazSnyatiyaOstavlyaetFayl(t *testing.T) {
	perehvat(t, otvetProfiley(vyvodProfilyaRu))
	z := podstavitZamok(t)
	if err := VklyuchitVesTrafik(obraztsovoeRazreshyonnoe(), true); err != nil {
		t.Fatal(err)
	}
	z.otkazSnyatiya = errors.New("тест: транзакция отклонена")
	if err := VyklyuchitVesTrafik(); err == nil {
		t.Fatal("отказ снятия проглочен")
	}
	if _, err := ProchitatOtkat(); err != nil {
		t.Fatalf("файл отката удалён при стоящем запрете: %v", err)
	}
}

// Без файла отката недоступная BFE не отказ: нашего замка за ней не видно, а
// установщик зовёт снятие на каждой машине и падать из-за неё не должен.
func TestBezFaylaNedostupnayaBfeNeOtkaz(t *testing.T) {
	perehvat(t, otvetProfiley(vyvodProfilyaRu))
	z := podstavitZamok(t)
	z.otkazSnyatiya = fmt.Errorf("FwpmEngineOpen0: %w", wfp.ErrNedostupen)
	if err := VyklyuchitVesTrafik(); err != nil {
		t.Fatalf("снятие без файла упало: %v", err)
	}
	z.otkazSnyatiya = errors.New("тест: отказано в доступе")
	if err := VyklyuchitVesTrafik(); err == nil {
		t.Fatal("настоящий отказ снятия проглочен")
	}
}

func TestOsirotevsheeSnimaetsyaBezTunnelya(t *testing.T) {
	perehvat(t, otvetProfiley(vyvodProfilyaRu))
	z := podstavitZamok(t)
	// Режим включён НЕ намеренно: это наш собственный переходный мусор.
	if err := VklyuchitVesTrafik(obraztsovoeRazreshyonnoe(), false); err != nil {
		t.Fatal(err)
	}
	snyato, zaperta, err := SnyatOsirotevshee()
	if err != nil {
		t.Fatal(err)
	}
	if !snyato || zaperta || z.stoit() {
		t.Fatalf("snyato=%v zaperta=%v стоит=%v, ожидалось снятие", snyato, zaperta, z.stoit())
	}
}

// Ради этого режим и включают: служба умерла или машину перезагрузили при
// опущенном туннеле. Распечатать на следующем старте значило бы отменить
// ровно то, ради чего режим включён, и молча.
func TestNamerennoZapertuyuMashinuNeRaspechatyvayem(t *testing.T) {
	zhurnal := perehvat(t, otvetProfiley(vyvodProfilyaRu))
	z := podstavitZamok(t)
	if err := VklyuchitVesTrafik(obraztsovoeRazreshyonnoe(), true); err != nil {
		t.Fatal(err)
	}
	vyzovy := len(z.vyzovy)
	snyato, zaperta, err := SnyatOsirotevshee()
	if err != nil {
		t.Fatal(err)
	}
	if snyato || !zaperta {
		t.Fatalf("snyato=%v zaperta=%v", snyato, zaperta)
	}
	if len(z.vyzovy) != vyzovy || !z.stoit() || len(*zhurnal) != 0 {
		t.Fatalf("намеренно запертая машина тронута: %v, netsh %v", z.vyzovy[vyzovy:], komandy(*zhurnal))
	}
}

// Файл пишется ДО фильтров, поэтому запрет без файла значит потерянный файл.
// Намеренность записана в нём же, а запертая навсегда машина хуже
// распечатанной.
func TestZapretBezFaylaSnimaetsyaNaStarte(t *testing.T) {
	perehvat(t, otvetProfiley(vyvodProfilyaRu))
	z := podstavitZamok(t)
	nabor, err := naborFiltrov(obraztsovoeRazreshyonnoe())
	if err != nil {
		t.Fatal(err)
	}
	if err := z.Postavit(nabor); err != nil {
		t.Fatal(err)
	}
	snyato, zaperta, err := SnyatOsirotevshee()
	if err != nil || !snyato || zaperta || z.stoit() {
		t.Fatalf("snyato=%v zaperta=%v err=%v стоит=%v", snyato, zaperta, err, z.stoit())
	}
}

// Запрет сняли руками, а файл остался утверждать, что машина заперта
// намеренно. Верить ему значило бы докладывать о блокировке при каждом
// старте про машину, которую никто не запирал.
func TestZapretSnyatRukamiFaylUstarel(t *testing.T) {
	perehvat(t, otvetProfiley(vyvodProfilyaRu))
	if err := ZapisatOtkat(Otkat{Wfp: true, Namerenno: true}); err != nil {
		t.Fatal(err)
	}
	_, zaperta, err := SnyatOsirotevshee()
	if err != nil || zaperta {
		t.Fatalf("zaperta=%v err=%v", zaperta, err)
	}
	if _, err := ProchitatOtkat(); !errors.Is(err, ErrOtkataNet) {
		t.Fatal("устаревший файл отката не удалён")
	}
}

func TestBezFaylaOtkataPolitikaNeTrogaetsya(t *testing.T) {
	// Without the rollback file we do not know whether that Block is ours. Turning
	// somebody else's protection off, mistaking it for our own litter, is the one
	// mistake a cleanup routine must never make.
	zhurnal := perehvat(t, otvetProfiley(vyvodProfilyaRu))
	if _, _, err := SnyatOsirotevshee(); err != nil {
		t.Fatal(err)
	}
	for _, s := range komandy(*zhurnal) {
		if strings.Contains(s, "firewallpolicy") {
			t.Fatal("политика тронута без файла отката")
		}
	}
	if zaperta, err := VesTrafikVklyuchyon(); err != nil || zaperta {
		t.Fatalf("чужая политика без нашего файла числится нашим замком: %v, %v", zaperta, err)
	}
}

func TestBezAdresaTunnelyaRezhimNeVklyuchaetsya(t *testing.T) {
	perehvat(t, otvetProfiley(vyvodProfilyaRu))
	r := obraztsovoeRazreshyonnoe()
	r.AdresTun = netip.Addr{}
	if err := VklyuchitVesTrafik(r, true); !errors.Is(err, ErrNetTunnelya) {
		t.Fatalf("ошибка %v, ожидалась ErrNetTunnelya", err)
	}
}

func TestPraviloDnsTcpSnimaetsyaVmesteSOstalnymi(t *testing.T) {
	// Имя обязано попасть и в запасной список, иначе после смерти службы без
	// файла отката правило прежней версии останется висеть навсегда.
	if !slices.Contains(VseImenaPravil(), PravAllowDnsTcp) {
		t.Fatal("Affory-Allow-Dns-Tcp не в запасном списке имён: правило переживёт уборку")
	}
}

// vremennyyKatalog уводит файл отката во временный каталог. Отдельной функцией,
// потому что t.TempDir() отдаёт НОВЫЙ каталог на каждый вызов, и запись с
// чтением разъехались бы по разным местам.
func vremennyyKatalog(t *testing.T) {
	t.Helper()
	prezhniy := katalogDannyh
	kat := t.TempDir()
	katalogDannyh = func() string { return kat }
	t.Cleanup(func() { katalogDannyh = prezhniy })
}

// Н3 аудита 1.6.1: файл отката, испорченный пропавшим питанием, берётся из
// прежней версии, а удаление отката снимает и её, иначе снятый замок
// воскрес бы из копии.
func TestBityyOtkatChitaetsyaIzZapasa(t *testing.T) {
	vremennyyKatalog(t)
	for _, politika := range []string{"BlockInbound,AllowOutbound", "BlockInbound,BlockOutbound"} {
		if err := ZapisatOtkat(Otkat{Profili: []ProfilDo{{Imya: "domain", Vklyuchen: true, Politika: politika}}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(putOtkata(), make([]byte, 64), 0o600); err != nil {
		t.Fatal(err)
	}
	o, err := ProchitatOtkat()
	if err != nil {
		t.Fatalf("битый откат не заменён запасом: %v", err)
	}
	if len(o.Profili) != 1 || o.Profili[0].Politika != "BlockInbound,AllowOutbound" {
		t.Fatalf("из запаса прочитано не то: %+v", o)
	}
	if err := UdalitOtkat(); err != nil {
		t.Fatal(err)
	}
	if _, err := ProchitatOtkat(); !errors.Is(err, ErrOtkataNet) {
		t.Fatalf("после удаления откат читается: %v", err)
	}
	if _, err := os.Stat(putOtkata() + sostoyanie.RasshirenieZapasa); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("запас отката пережил удаление: снятый замок воскрес бы из копии")
	}
}

// Файл с одним запретом WFP годен: профилей в нём нет, и это не «пусто».
func TestOtkatSOdnimZapretomGoden(t *testing.T) {
	vremennyyKatalog(t)
	if err := ZapisatOtkat(Otkat{Wfp: true}); err != nil {
		t.Fatal(err)
	}
	if o, err := ProchitatOtkat(); err != nil || !o.Wfp {
		t.Fatalf("%+v, %v", o, err)
	}
	if err := razobratOtkat([]byte(`{"namerenno":true}`), new(Otkat)); err == nil {
		t.Fatal("откат без замка и без запрета принят")
	}
}

// Н4 аудита 1.6.1. Нечитаемый файл отката (испорчены и он, и прежняя версия)
// оставлял машину запертой навсегда. Файл есть, значит запирали мы: снимается
// запрет, а замок прежней версии сменяется умолчанием Windows.
func TestNechitaemyyOtkatNeZapiraetMashinu(t *testing.T) {
	for _, sluchay := range []struct {
		imya  string
		snyat func() error
	}{
		{"выключение режима", VyklyuchitVesTrafik},
		{"уборка при старте", func() error { _, _, err := SnyatOsirotevshee(); return err }},
	} {
		t.Run(sluchay.imya, func(t *testing.T) {
			zhurnal := perehvat(t, otvetProfiley(vyvodProfilyaRu))
			z := podstavitZamok(t)
			nabor, err := naborFiltrov(obraztsovoeRazreshyonnoe())
			if err != nil {
				t.Fatal(err)
			}
			if err := z.Postavit(nabor); err != nil {
				t.Fatal(err)
			}
			for _, p := range []string{putOtkata(), putOtkata() + sostoyanie.RasshirenieZapasa} {
				if err := os.WriteFile(p, make([]byte, 64), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			// И замок прежней версии стоит.
			if _, err := vypolnit([]string{"advfirewall", "set", "allprofiles", "firewallpolicy", "blockinbound,blockoutbound"}); err != nil {
				t.Fatal(err)
			}

			if err := sluchay.snyat(); err != nil {
				t.Fatalf("снятие упало на нечитаемом откате: %v", err)
			}
			if z.stoit() {
				t.Fatal("запрет остался")
			}
			if zaperta, err := staryyZamokStoit(); err != nil || zaperta {
				t.Fatalf("политика осталась запертой (%v, %v)", zaperta, err)
			}
			snyatoPravil := 0
			for _, k := range komandy(*zhurnal) {
				if strings.Contains(k, "delete rule") {
					snyatoPravil++
				}
			}
			if snyatoPravil == 0 {
				t.Fatal("правила Affory не сняты")
			}
			if _, err := ProchitatOtkat(); !errors.Is(err, ErrOtkataNet) {
				t.Fatalf("нечитаемый откат остался и повторит то же на следующем старте: %v", err)
			}
		})
	}
}

func TestUstarevshiyStaryyOtkatNeDelaetMashinuZapertoy(t *testing.T) {
	// A rollback file left behind by a manual emergency exit says Namerenno, but
	// the machine is wide open: the policy is back to AllowOutbound and no rule of
	// ours survives. Believing the file here means reporting an intentional lock
	// forever, on every start, about a machine nobody locked.
	perehvat(t, func(a []string) (string, error) {
		if len(a) > 2 && a[1] == "show" {
			return "State                                 ON\nFirewall Policy                       BlockInbound,AllowOutbound\nOk.\n", nil
		}
		return "No rules match the specified criteria.\n", nil
	})
	if err := ZapisatOtkat(staryyOtkat(true)); err != nil {
		t.Fatal(err)
	}
	_, zaperta, err := SnyatOsirotevshee()
	if err != nil {
		t.Fatal(err)
	}
	if zaperta {
		t.Fatal("машина числится намеренно запертой, хотя политика открыта: файл устарел, а не машина заперта")
	}
	if _, err := ProchitatOtkat(); !errors.Is(err, ErrOtkataNet) {
		t.Fatal("устаревший файл отката не удалён, и следующий старт соврёт снова")
	}
}

func TestRealnoZapertayaStarymZamkomOstayotsyaZapertoy(t *testing.T) {
	// The mirror case. The file says Namerenno AND the policy really is Block:
	// unlocking here would cancel the one thing the mode exists for.
	perehvat(t, func(a []string) (string, error) {
		if len(a) > 2 && a[1] == "show" {
			return "State                                 ON\nFirewall Policy                       BlockInbound,BlockOutbound\nOk.\n", nil
		}
		return "", nil
	})
	if err := ZapisatOtkat(staryyOtkat(true)); err != nil {
		t.Fatal(err)
	}
	_, zaperta, err := SnyatOsirotevshee()
	if err != nil {
		t.Fatal(err)
	}
	if !zaperta {
		t.Fatal("намеренно запертая машина не опознана")
	}
	if _, err := ProchitatOtkat(); err != nil {
		t.Fatal("файл отката удалён у РЕАЛЬНО запертой машины: выйти из режима станет нечем")
	}
}

// Разрешение серверов переписывается ОТДЕЛЬНО от остальных, потому что при
// мёртвом ядре пересобрать остальные нечем: туннельное привязано к адресу
// TUN, а адаптера уже нет. Оставлять разрешённым удалённый адрес только
// потому, что рядом нет туннеля, значит держать дыру ровно в том режиме, ради
// которого его включали.
func TestPerezavestiServeryMenyaetTolkoSvoyo(t *testing.T) {
	perehvat(t, otvetProfiley(vyvodProfilyaRu))
	z := podstavitZamok(t)
	if err := VklyuchitVesTrafik(obraztsovoeRazreshyonnoe(), true); err != nil {
		t.Fatal(err)
	}
	krome := func(m map[windows.GUID]wfp.Filtr) map[windows.GUID]wfp.Filtr {
		c := maps.Clone(m)
		for _, k := range klyuchiVida(vidServery) {
			delete(c, k)
		}
		return c
	}
	do := krome(z.filtry)

	err := PerezavestiRazreshyonnyeServery(Adresa{
		Vse:   []netip.Addr{netip.MustParseAddr("203.0.113.7"), netip.MustParseAddr("198.51.100.9")},
		Porty: []string{"443", "20000-30000"},
	})
	if err != nil {
		t.Fatal(err)
	}
	srv, est := z.vid(vidServery, false)
	if !est {
		t.Fatal("разрешения серверов нет")
	}
	for _, u := range []wfp.Uslovie{usSet("203.0.113.7/32"), usSet("198.51.100.9/32"),
		wfp.UdalyonnyePorty(443, 443), wfp.UdalyonnyePorty(20000, 30000)} {
		if !estUslovie(srv, u) {
			t.Errorf("нет условия %+v", u)
		}
	}
	if estUslovie(srv, usSet("192.0.2.225/32")) {
		t.Error("удалённый сервер остался разрешённым")
	}
	if !reflect.DeepEqual(do, krome(z.filtry)) {
		t.Error("тронуто чужое")
	}
	if o, err := ProchitatOtkat(); err != nil || !o.Wfp || !o.Namerenno {
		t.Errorf("файл отката тронут: %+v, %v", o, err)
	}
}

// Пустой список это НЕ повод оставить прежнее разрешение: серверов не
// осталось, разрешать некуда, и отказ в сторону «заперто плотнее» тут
// единственный верный.
func TestPerezavestiServeryBezAdresovSnimaetRazreshenie(t *testing.T) {
	perehvat(t, otvetProfiley(vyvodProfilyaRu))
	z := podstavitZamok(t)
	if err := VklyuchitVesTrafik(obraztsovoeRazreshyonnoe(), true); err != nil {
		t.Fatal(err)
	}
	if err := PerezavestiRazreshyonnyeServery(Adresa{Porty: []string{"443"}}); err != nil {
		t.Fatal(err)
	}
	if _, est := z.vid(vidServery, false); est {
		t.Fatal("разрешение серверов осталось")
	}
	if !z.stoit() {
		t.Fatal("запрет снят вместе с разрешением")
	}
}

// Без запрета переписывать нечего: разрешение без него осталось бы
// постоянным мусором в BFE.
func TestPerezavestiBezZapretaNichegoNeStavit(t *testing.T) {
	perehvat(t, otvetProfiley(vyvodProfilyaRu))
	z := podstavitZamok(t)
	if err := PerezavestiRazreshyonnyeServery(Adresa{Vse: []netip.Addr{netip.MustParseAddr("203.0.113.7")}, Porty: []string{"443"}}); err != nil {
		t.Fatal(err)
	}
	if err := PerezavestiPravilaDns(netip.MustParseAddr("192.168.5.1")); err != nil {
		t.Fatal(err)
	}
	if len(z.filtry) != 0 {
		t.Fatalf("без запрета поставлено %d фильтров", len(z.filtry))
	}
}

// L4 аудита 1.8.0: под замком без туннеля разрешение DNS переписывается под
// новый резолвер, и только оно.
func TestPerezavestiDnsPodNovyyResolver(t *testing.T) {
	perehvat(t, otvetProfiley(vyvodProfilyaRu))
	z := podstavitZamok(t)
	if err := VklyuchitVesTrafik(obraztsovoeRazreshyonnoe(), true); err != nil {
		t.Fatal(err)
	}
	vsego := len(z.filtry)
	if err := PerezavestiPravilaDns(netip.MustParseAddr("192.168.5.1")); err != nil {
		t.Fatal(err)
	}
	dns, est := z.vid(vidDns, false)
	if !est || !estUslovie(dns, usSet("192.168.5.1/32")) || estUslovie(dns, usSet("10.7.0.1/32")) {
		t.Fatalf("DNS не под новый резолвер: %+v", dns)
	}
	if len(z.filtry) != vsego {
		t.Fatalf("фильтров было %d, стало %d", vsego, len(z.filtry))
	}
	if err := PerezavestiPravilaDns(netip.Addr{}); err != nil {
		t.Fatal(err)
	}
	if _, est := z.vid(vidDns, false); est {
		t.Fatal("без резолвера разрешение DNS осталось")
	}
}
