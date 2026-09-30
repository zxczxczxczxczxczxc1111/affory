package set

import (
	"errors"
	"fmt"
	"net/netip"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/sostoyanie"
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
	t.Cleanup(func() { vypolnit = prezhniy; katalogDannyh = prezhniyKat; sostoyanieMpsSvc = prezhneeMps })
	return &zhurnal
}

// L16 аудита 1.8.0: netsh отказывает, потому что служба брандмауэра
// остановлена или отключена. Человек видит firewall-disabled с причиной, а
// не общий отказ.
func TestOtkazPriOstanovlennomMpsSvcEtoVyklyuchennyyBrandmauer(t *testing.T) {
	perehvat(t, func([]string) (string, error) { return "", errors.New("netsh: служба не запущена") })
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
		err := VklyuchitVesTrafik(obraztsovoeRazreshyonnoe(), true)
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
	if err := VklyuchitVesTrafik(obraztsovoeRazreshyonnoe(), true); errors.Is(err, ErrBrandmauerVyklyuchen) {
		t.Fatalf("непрочитанное состояние выдано за выключенный брандмауэр: %v", err)
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
	var snyali []string
	for _, z := range *zhurnal {
		snyali = append(snyali, strings.Join(z.argumenty, " "))
	}
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

func TestOtkatZapisyvaetsyaDoIzmeneniya(t *testing.T) {
	// Change first, remember later is how you end up with a machine that has no
	// internet and no idea what it looked like before. Order is the test.
	zhurnal := perehvat(t, otvetProfiley(vyvodProfilyaRu))
	if err := VklyuchitVesTrafik(obraztsovoeRazreshyonnoe(), true); err != nil {
		t.Fatal(err)
	}
	pervoeIzmenenie := -1
	posledneeChtenie := -1
	for i, z := range *zhurnal {
		k := strings.Join(z.argumenty, " ")
		if strings.Contains(k, " show ") {
			posledneeChtenie = i
		}
		if pervoeIzmenenie == -1 && (strings.Contains(k, " add ") || strings.Contains(k, "firewallpolicy") ||
			strings.Contains(k, "state on")) {
			pervoeIzmenenie = i
		}
	}
	if posledneeChtenie > pervoeIzmenenie {
		t.Fatalf("состояние читалось ПОСЛЕ изменения: чтение %d, изменение %d", posledneeChtenie, pervoeIzmenenie)
	}
	if _, err := ProchitatOtkat(); err != nil {
		t.Fatalf("файл отката не записан: %v", err)
	}
}

func TestPolitikaStavitsyaPoslednim(t *testing.T) {
	// Rules first, policy last. The other way round leaves a window where the
	// machine is already locked and the exceptions are not there yet.
	zhurnal := perehvat(t, otvetProfiley(vyvodProfilyaRu))
	if err := VklyuchitVesTrafik(obraztsovoeRazreshyonnoe(), true); err != nil {
		t.Fatal(err)
	}
	poslednyaya := strings.Join((*zhurnal)[len(*zhurnal)-1].argumenty, " ")
	if !strings.Contains(poslednyaya, "firewallpolicy blockinbound,blockoutbound") {
		t.Fatalf("последней командой была %q, а должна быть постановка политики", poslednyaya)
	}
}

func TestSnyatieVObratnomPoryadke(t *testing.T) {
	zhurnal := perehvat(t, otvetProfiley(vyvodProfilyaRu))
	if err := VklyuchitVesTrafik(obraztsovoeRazreshyonnoe(), true); err != nil {
		t.Fatal(err)
	}
	nachalo := len(*zhurnal)
	if err := VyklyuchitVesTrafik(); err != nil {
		t.Fatal(err)
	}
	pervaya := strings.Join((*zhurnal)[nachalo].argumenty, " ")
	if !strings.Contains(pervaya, "firewallpolicy") {
		t.Fatalf("снятие началось с %q, а должно с возврата политики", pervaya)
	}
}

func TestVozvratBeryotPolitikuIzFayla(t *testing.T) {
	// netsh show prints the EFFECTIVE value while set changes the PERSISTENT
	// one. Reading the live system and writing it back is a quiet substitution
	// of somebody else's setting; the rollback file is the only honest source.
	zhurnal := perehvat(t, otvetProfiley(strings.Replace(vyvodProfilyaRu,
		"BlockInbound,AllowOutbound", "AllowInbound,AllowOutbound", 1)))
	if err := VklyuchitVesTrafik(obraztsovoeRazreshyonnoe(), true); err != nil {
		t.Fatal(err)
	}
	nachalo := len(*zhurnal)
	if err := VyklyuchitVesTrafik(); err != nil {
		t.Fatal(err)
	}
	pervaya := strings.Join((*zhurnal)[nachalo].argumenty, " ")
	if !strings.Contains(pervaya, "allowinbound,allowoutbound") {
		t.Fatalf("возвращена политика %q, а в файле записана allowinbound,allowoutbound", pervaya)
	}
}

func TestVyklyuchennyyProfilVklyuchaetsyaIVozvrashchaetsya(t *testing.T) {
	// Measured 01.09.2026: with the profile off, a Block policy does NOTHING and
	// the internet keeps working while the UI shows green. Turning it on is the
	// owner's decision; turning it back off afterwards is the part that keeps it
	// honest.
	zhurnal := perehvat(t, otvetProfiley(vyvodProfilyaEn)) // OFF
	if err := VklyuchitVesTrafik(obraztsovoeRazreshyonnoe(), true); err != nil {
		t.Fatal(err)
	}
	nashli := false
	for _, z := range *zhurnal {
		if strings.Contains(strings.Join(z.argumenty, " "), "state on") {
			nashli = true
		}
	}
	if !nashli {
		t.Fatal("выключенный профиль не включён: политика будет надписью, а не защитой")
	}

	nachalo := len(*zhurnal)
	if err := VyklyuchitVesTrafik(); err != nil {
		t.Fatal(err)
	}
	vernuli := false
	for _, z := range (*zhurnal)[nachalo:] {
		if strings.Contains(strings.Join(z.argumenty, " "), "state off") {
			vernuli = true
		}
	}
	if !vernuli {
		t.Fatal("профиль остался включённым: мы поменяли чужую настройку и не вернули")
	}
}

func TestSnyatieUbiraetImennoZavedyonnye(t *testing.T) {
	// Process rules are numbered, so a fixed list would leave the extras hanging
	// and a prefix mask would one day eat somebody else's rule. The rollback file
	// records what was actually created.
	zhurnal := perehvat(t, otvetProfiley(vyvodProfilyaRu))
	r := obraztsovoeRazreshyonnoe()
	if err := VklyuchitVesTrafik(r, true); err != nil {
		t.Fatal(err)
	}
	nachalo := len(*zhurnal)
	if err := VyklyuchitVesTrafik(); err != nil {
		t.Fatal(err)
	}
	snyato := map[string]bool{}
	for _, z := range (*zhurnal)[nachalo:] {
		for _, a := range z.argumenty {
			if strings.HasPrefix(a, "name=") {
				snyato[strings.TrimPrefix(a, "name=")] = true
			}
		}
	}
	for _, imya := range []string{PravAllowTun, PravAllowSrv, PravAllowSrvUdp, PravAllowLan, PravAllowDns,
		PravAllowProc + "-0", PravAllowProc + "-1"} {
		if !snyato[imya] {
			t.Fatalf("правило %s не снято", imya)
		}
	}
	// Правило IPv6 в этом списке БЫЛО и требовало его снятия, то есть тест
	// закреплял дефект: блокировка принадлежит туннелю, а не режиму, и снимать её
	// вместе с режимом значит оставить поднятый туннель с открытым IPv6.
	// Отдельная проверка на это лежит в TestVyklyuchenieRezhimaNeSnimaetBlokirovkuIPv6.
	if snyato[ImyaPravilaIPv6] {
		t.Fatal("режим снял блокировку IPv6, хотя она к режиму не относится")
	}
}

func TestPraviloNaProtsessOdnaProgrammaNaPravilo(t *testing.T) {
	// netsh takes one program per rule. Two paths in one rule silently becomes a
	// rule that matches nothing.
	p := pravilaRazresheniya(obraztsovoeRazreshyonnoe())
	programmy := 0
	for _, k := range p {
		// Правила DNS тоже с программой, но с системной службой DNS, а не с
		// нашими процессами: их считает TestDnsRazreshyonTolkoSistemnoySluzhbeDns.
		if !strings.HasPrefix(k[0], PravAllowProc) {
			continue
		}
		for _, a := range k {
			if strings.HasPrefix(a, "program=") {
				programmy++
				if strings.Contains(a, ",") {
					t.Fatalf("в правиле два пути сразу: %s", a)
				}
			}
		}
	}
	if programmy != 2 {
		t.Fatalf("правил по программам %d, ожидалось два: ядро и служба", programmy)
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

func TestNeudachaPravilaOtkatyvaetVsyo(t *testing.T) {
	// A half-applied kill-switch is the worst of both worlds: no protection and
	// no internet. If a rule refuses, everything comes back down.
	var stavili bool
	prezhniy := vypolnit
	prezhniyKat := katalogDannyh
	kat := t.TempDir()
	katalogDannyh = func() string { return kat }
	var vernuliPolitiku bool
	vypolnit = func(a []string) (string, error) {
		k := strings.Join(a, " ")
		switch {
		case strings.Contains(k, " show "):
			return vyvodProfilyaRu, nil
		case strings.Contains(k, " add rule") && strings.Contains(k, PravAllowDns):
			stavili = true
			return "", errors.New("netsh отказал")
		case strings.Contains(k, "firewallpolicy blockinbound,allowoutbound"):
			vernuliPolitiku = true
		}
		return "Ok.", nil
	}
	defer func() { vypolnit = prezhniy; katalogDannyh = prezhniyKat }()

	if err := VklyuchitVesTrafik(obraztsovoeRazreshyonnoe(), true); err == nil {
		t.Fatal("режим объявлен включённым при неудачном правиле")
	}
	if !stavili {
		t.Fatal("тест не дошёл до правила DNS")
	}
	if !vernuliPolitiku {
		t.Fatal("после неудачи политика не возвращена: машина осталась наполовину запертой")
	}
}

func TestOsirotevsheeSnimaetsyaBezTunnelya(t *testing.T) {
	zhurnal := perehvat(t, otvetProfiley(vyvodProfilyaRu))
	// Режим включён НЕ намеренно: это наш собственный переходный мусор.
	if err := VklyuchitVesTrafik(obraztsovoeRazreshyonnoe(), false); err != nil {
		t.Fatal(err)
	}
	nachalo := len(*zhurnal)

	snyato, zaperta, err := SnyatOsirotevshee()
	if err != nil {
		t.Fatal(err)
	}
	if !snyato || zaperta {
		t.Fatalf("snyato=%v zaperta=%v, ожидалось снятие", snyato, zaperta)
	}
	vernuli := false
	for _, z := range (*zhurnal)[nachalo:] {
		if strings.Contains(strings.Join(z.argumenty, " "), "firewallpolicy") {
			vernuli = true
		}
	}
	if !vernuli {
		t.Fatal("политика не возвращена: машина осталась запертой")
	}
}

func TestNamerennoZapertuyuMashinuNeRaspechatyvayem(t *testing.T) {
	// The kill-switch exists to survive exactly this: the service dying while the
	// tunnel is down. Unlocking on the next start would undo the one thing the
	// mode was turned on for, and it would do it silently.
	zhurnal := perehvat(t, otvetProfiley(vyvodProfilyaRu))
	if err := VklyuchitVesTrafik(obraztsovoeRazreshyonnoe(), true); err != nil {
		t.Fatal(err)
	}
	nachalo := len(*zhurnal)

	snyato, zaperta, err := SnyatOsirotevshee()
	if err != nil {
		t.Fatal(err)
	}
	if snyato {
		t.Fatal("намеренно запертая машина распечатана сама")
	}
	if !zaperta {
		t.Fatal("не сказано вслух, что машина осталась запертой намеренно")
	}
	// Чтение это не вмешательство: запертость положено проверять у самой машины,
	// а не у файла. Считаем только команды, которые что-то МЕНЯЮТ.
	tronuli := 0
	for _, z := range (*zhurnal)[nachalo:] {
		if !strings.Contains(strings.Join(z.argumenty, " "), "show") {
			tronuli++
		}
	}
	if tronuli != 0 {
		t.Fatalf("система тронута %d командами, а трогать её было нельзя", tronuli)
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
	for _, z := range *zhurnal {
		if strings.Contains(strings.Join(z.argumenty, " "), "firewallpolicy") {
			t.Fatal("политика тронута без файла отката")
		}
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

// Н4 аудита 1.6.1. Нечитаемый файл отката (испорчены и он, и прежняя версия)
// оставлял машину запертой навсегда: и выключение режима, и уборка при старте
// возвращали ошибку разбора, ничего не сняв. Прежняя политика неизвестна, но
// файл есть, значит запирали мы, и возвращается умолчание Windows.
func TestNechitaemyyOtkatNeZapiraetMashinu(t *testing.T) {
	for _, sluchay := range []struct {
		imya  string
		snyat func() error
	}{
		{"выключение режима", VyklyuchitVesTrafik},
		{"уборка при старте", func() error { _, _, err := SnyatOsirotevshee(); return err }},
	} {
		t.Run(sluchay.imya, func(t *testing.T) {
			vremennyyKatalog(t)
			bezReestra(t)
			for _, p := range []string{putOtkata(), putOtkata() + sostoyanie.RasshirenieZapasa} {
				if err := os.WriteFile(p, make([]byte, 64), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			var komandy []string
			zhivoy := otvetProfiley(vyvodProfilyaRu) // профиль включён
			staryy := vypolnit
			t.Cleanup(func() { vypolnit = staryy })
			vypolnit = func(a []string) (string, error) {
				komandy = append(komandy, strings.Join(a, " "))
				return zhivoy(a)
			}
			// Машина заперта нами до сбоя.
			if _, err := vypolnit([]string{"advfirewall", "set", "allprofiles", "firewallpolicy", "blockinbound,blockoutbound"}); err != nil {
				t.Fatal(err)
			}

			if err := sluchay.snyat(); err != nil {
				t.Fatalf("снятие упало на нечитаемом откате: %v", err)
			}
			if zaperta, err := VesTrafikVklyuchyon(); err != nil || zaperta {
				t.Fatalf("машина осталась запертой (%v, %v)", zaperta, err)
			}
			snyatoPravil := 0
			for _, k := range komandy {
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

func TestUstarevshiyOtkatNeDelaetMashinuZapertoy(t *testing.T) {
	// A rollback file left behind by a manual emergency exit says Namerenno, but
	// the machine is wide open: the policy is back to AllowOutbound and no rule of
	// ours survives. Believing the file here means reporting an intentional lock
	// forever, on every start, about a machine nobody locked.
	vremennyyKatalog(t)
	if err := ZapisatOtkat(Otkat{
		Namerenno: true,
		Profili:   []ProfilDo{{Imya: "domain", Vklyuchen: false, Politika: "BlockInbound,AllowOutbound"}},
	}); err != nil {
		t.Fatal(err)
	}
	staryy := vypolnit
	defer func() { vypolnit = staryy }()
	vypolnit = func(a []string) (string, error) {
		if len(a) > 2 && a[1] == "show" {
			return "State                                 ON\nFirewall Policy                       BlockInbound,AllowOutbound\nOk.\n", nil
		}
		return "No rules match the specified criteria.\n", nil
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

func TestRealnoZapertayaMashinaOstayotsyaZapertoy(t *testing.T) {
	// The mirror case. The file says Namerenno AND the policy really is Block:
	// unlocking here would cancel the one thing the mode exists for.
	vremennyyKatalog(t)
	if err := ZapisatOtkat(Otkat{
		Namerenno: true,
		Profili:   []ProfilDo{{Imya: "domain", Vklyuchen: true, Politika: "BlockInbound,AllowOutbound"}},
	}); err != nil {
		t.Fatal(err)
	}
	bezReestra(t)
	staryy := vypolnit
	defer func() { vypolnit = staryy }()
	vypolnit = func(a []string) (string, error) {
		if len(a) > 2 && a[1] == "show" {
			return "State                                 ON\nFirewall Policy                       BlockInbound,BlockOutbound\nOk.\n", nil
		}
		return "", nil
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

func TestVyklyuchenieRezhimaNeSnimaetBlokirovkuIPv6(t *testing.T) {
	// The IPv6 rule belongs to the tunnel, not to the mode: it lives exactly as
	// long as the tunnel does. Putting it in the rollback list makes the mode
	// delete it on the way out, and the tunnel keeps running with IPv6 walking
	// straight past it. The service meanwhile believes the rule still stands,
	// because nobody called VernutIPv6.
	zhurnal := perehvat(t, otvetProfiley(vyvodProfilyaRu))
	if err := VklyuchitVesTrafik(obraztsovoeRazreshyonnoe(), true); err != nil {
		t.Fatal(err)
	}
	o, err := ProchitatOtkat()
	if err != nil {
		t.Fatal(err)
	}
	for _, imya := range o.Pravila {
		if imya == ImyaPravilaIPv6 {
			t.Fatal("правило IPv6 записано в откат режима, значит выключение режима его снимет")
		}
	}

	nachalo := len(*zhurnal)
	if err := VyklyuchitVesTrafik(); err != nil {
		t.Fatal(err)
	}
	for _, z := range (*zhurnal)[nachalo:] {
		soed := strings.Join(z.argumenty, " ")
		if strings.Contains(soed, "delete") && strings.Contains(soed, ImyaPravilaIPv6) {
			t.Fatal("выключение режима сняло блокировку IPv6 при живом туннеле: утечка")
		}
	}
}

func TestPovtornoeVklyuchenieNeZatiraetPervyyOtkat(t *testing.T) {
	// Пересборка правил (добавили сервер при включённом режиме) зовёт
	// VklyuchitVesTrafik ПОВТОРНО, уже поверх запертой машины. Если она снова
	// снимет состояние профилей "как есть", в откат уедет Block, и выключение
	// режима честно вернёт Block обратно. Машина без сети навсегда, а status
	// при этом отвечает kill_switch: false.
	perehvat(t, otvetProfiley(vyvodProfilyaRu))

	if err := VklyuchitVesTrafik(obraztsovoeRazreshyonnoe(), true); err != nil {
		t.Fatal(err)
	}
	pervyy, err := ProchitatOtkat()
	if err != nil {
		t.Fatal(err)
	}
	if got := politikaIzOtkata(pervyy); got != "blockinbound,allowoutbound" {
		t.Fatalf("первый откат уже неверен: %q", got)
	}

	// Второй вызов: netsh теперь отвечает BlockOutbound, потому что заглушка
	// живая и помнит, что мы заперли машину сами.
	r := obraztsovoeRazreshyonnoe()
	r.Protsessy = append(r.Protsessy, `C:\Program Files\Affory\sing-box.exe`)
	if err := VklyuchitVesTrafik(r, true); err != nil {
		t.Fatal(err)
	}
	vtoroy, err := ProchitatOtkat()
	if err != nil {
		t.Fatal(err)
	}
	if got := politikaIzOtkata(vtoroy); got != "blockinbound,allowoutbound" {
		t.Fatalf("пересборка затёрла откат: %q. Возврат поставит Block обратно", got)
	}

	// Список ИМЁН при этом обязан обновиться: правил стало больше, и снятие
	// идёт именно по нему. Сохранить весь файл целиком значило бы оставить
	// висеть Affory-Allow-Proc-2 навсегда.
	nashli := false
	for _, imya := range vtoroy.Pravila {
		if imya == "Affory-Allow-Proc-2" {
			nashli = true
		}
	}
	if !nashli {
		t.Fatalf("имена правил не обновились: %v", vtoroy.Pravila)
	}
}

func TestDnsRazreshyonPoTCPToZhe(t *testing.T) {
	// Порт 53 по UDP разрешён с самого начала, TCP нет. Резолвер уходит на TCP
	// при усечённом ответе, и в запертом режиме этот перес прос молча не доедет:
	// выглядит как случайно неработающие сайты, а не как правило брандмауэра.
	//
	// Решено 01.09.2026: открыть TCP/53 к тому же резолверу.
	pravila := pravilaRazresheniya(obraztsovoeRazreshyonnoe())

	var udp, tcp bool
	for _, p := range pravila {
		soed := strings.Join(p, " ")
		if !strings.Contains(soed, "remoteport=53") {
			continue
		}
		if strings.Contains(soed, "protocol=udp") {
			udp = true
		}
		if strings.Contains(soed, "protocol=tcp") {
			tcp = true
		}
	}
	if !udp {
		t.Error("правило DNS по UDP пропало")
	}
	if !tcp {
		t.Error("DNS по TCP запрещён: усечённый ответ в запертом режиме не переспросится")
	}
}

// С4 аудита 1.6.1. Порт 53 к резолверу был открыт любой программе: в запертом
// режиме всякая, что шлёт запросы сама мимо туннеля, резолвила открытым текстом.
// Нужен он одной системной службе DNS: через неё резолвит и наша служба, а ядру
// и службе и так разрешён любой выход правилами процессов.
func TestDnsRazreshyonTolkoSistemnoySluzhbeDns(t *testing.T) {
	for _, k := range pravilaRazresheniya(obraztsovoeRazreshyonnoe()) {
		if k[0] != PravAllowDns && k[0] != PravAllowDnsTcp {
			continue
		}
		s := strings.Join(k, " ")
		if !strings.Contains(strings.ToLower(s), `\svchost.exe`) || !strings.Contains(s, "service=dnscache") {
			t.Fatalf("правило DNS открыто не только службе DNS: %s", s)
		}
	}
}

func TestPraviloDnsTcpSnimaetsyaVmesteSOstalnymi(t *testing.T) {
	// Имя обязано попасть и в запасной список, иначе после смерти службы без
	// файла отката правило останется висеть навсегда.
	nashli := false
	for _, imya := range VseImenaPravil() {
		if imya == PravAllowDnsTcp {
			nashli = true
		}
	}
	if !nashli {
		t.Fatal("Affory-Allow-Dns-Tcp не в запасном списке имён: правило переживёт уборку")
	}
}

// Правило серверов переписывается ОТДЕЛЬНО от остальных, потому что при мёртвом
// ядре пересобрать остальные нечем: они привязаны к адресу TUN, а адаптера уже
// нет. Список серверов от туннеля не зависит вовсе, и оставлять в разрешающих
// удалённый адрес только потому, что рядом нет туннеля, значит держать дыру
// ровно в том режиме, ради которого его включали.
func TestPerezavestiServeryMenyaetTolkoSvoyoPravilo(t *testing.T) {
	zhurnal := perehvat(t, func([]string) (string, error) { return "Ok.", nil })

	err := PerezavestiRazreshyonnyeServery(Adresa{
		Vse:   []netip.Addr{netip.MustParseAddr("203.0.113.7"), netip.MustParseAddr("198.51.100.9")},
		Porty: []string{"443", "20000-30000"},
	})
	if err != nil {
		t.Fatalf("переучреждение правила: %v", err)
	}

	var stroki []string
	for _, z := range *zhurnal {
		stroki = append(stroki, strings.Join(z.argumenty, " "))
	}
	if len(stroki) != 4 {
		t.Fatalf("вызовов netsh %d, ждали четыре (снять и завести оба): %v", len(stroki), stroki)
	}
	if !strings.Contains(stroki[0], "delete rule name="+PravAllowSrv) || !strings.Contains(stroki[1], "delete rule name="+PravAllowSrvUdp) {
		t.Errorf("первым обязано идти снятие прежних правил, а идёт: %v", stroki[:2])
	}
	for i, pr := range map[int]string{2: "tcp", 3: "udp"} {
		if !strings.Contains(stroki[i], "add rule name=") || !strings.Contains(stroki[i], "protocol="+pr) {
			t.Errorf("вызов %d обязан завести правило %s, а это: %s", i, pr, stroki[i])
		}
		if !strings.Contains(stroki[i], "remoteip=203.0.113.7,198.51.100.9") || !strings.Contains(stroki[i], "remoteport=443,20000-30000") {
			t.Errorf("в правиле не те адреса или порты: %s", stroki[i])
		}
	}
	// Ни политика, ни чужие правила, ни файл отката: у этой функции ровно один
	// предмет. Тронуть политику при мёртвом ядре значило бы распечатать машину.
	for _, s := range stroki {
		if strings.Contains(s, "firewallpolicy") || strings.Contains(s, PravAllowTun) {
			t.Errorf("тронуто чужое: %s", s)
		}
	}
}

// L4 аудита 1.8.0: под замком без туннеля правило DNS переписывается под
// новый резолвер, а его имена попадают в откат, даже если замок вставал без
// резолвера.
func TestPerezavestiDnsDopisyvaetOtkatIMenyaetTolkoSvoyo(t *testing.T) {
	zhurnal := perehvat(t, func([]string) (string, error) { return "Ok.", nil })
	if err := ZapisatOtkat(Otkat{
		Profili:   []ProfilDo{{Imya: "domain", Vklyuchen: true, Politika: "BlockInbound,AllowOutbound"}},
		Namerenno: true, Pravila: []string{PravAllowTun, PravAllowSrv},
	}); err != nil {
		t.Fatal(err)
	}

	if err := PerezavestiPravilaDns(netip.MustParseAddr("192.168.5.1")); err != nil {
		t.Fatalf("правило DNS не переписано: %v", err)
	}
	o, err := ProchitatOtkat()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(o.Pravila, PravAllowDns) || !slices.Contains(o.Pravila, PravAllowDnsTcp) || !o.Namerenno {
		t.Fatalf("откат после записи %+v: выключение режима оставит правила DNS", o)
	}
	var stroki []string
	for _, z := range *zhurnal {
		stroki = append(stroki, strings.Join(z.argumenty, " "))
	}
	if len(stroki) != 4 {
		t.Fatalf("вызовов netsh %d, ждали четыре: %v", len(stroki), stroki)
	}
	for i, s := range stroki {
		snyatie := strings.Contains(s, "delete rule name=")
		if (i < 2) != snyatie {
			t.Errorf("вызов %d не на своём месте, сначала снятие, потом заведение: %s", i, s)
		}
		if !snyatie && (!strings.Contains(s, "remoteip=192.168.5.1") || !strings.Contains(s, "service=dnscache")) {
			t.Errorf("правило не под новый резолвер или не только для dnscache: %s", s)
		}
		if strings.Contains(s, "firewallpolicy") || strings.Contains(s, PravAllowSrv) || strings.Contains(s, PravAllowTun) {
			t.Errorf("тронуто чужое: %s", s)
		}
	}

	*zhurnal = nil
	if err := PerezavestiPravilaDns(netip.Addr{}); err != nil {
		t.Fatal(err)
	}
	if len(*zhurnal) != 2 {
		t.Fatalf("без резолвера вызовов %d, ждали только снятие двух правил", len(*zhurnal))
	}
}

// Пустой список это НЕ повод оставить прежнее правило: серверов не осталось,
// разрешать некуда, и отказ в сторону «заперто плотнее» тут единственный верный.
func TestPerezavestiServeryBezAdresovSnimaetPravilo(t *testing.T) {
	zhurnal := perehvat(t, func([]string) (string, error) { return "Ok.", nil })

	if err := PerezavestiRazreshyonnyeServery(Adresa{Porty: []string{"443"}}); err != nil {
		t.Fatalf("переучреждение пустым списком: %v", err)
	}
	if len(*zhurnal) != 2 {
		t.Fatalf("вызовов netsh %d, ждали два (только снятие): %v", len(*zhurnal), *zhurnal)
	}
	for _, z := range *zhurnal {
		if s := strings.Join(z.argumenty, " "); !strings.Contains(s, "delete rule name=") {
			t.Errorf("вызовом обязано быть снятие, а это: %s", s)
		}
	}
}

// imyaIz достаёт имя правила из команды netsh.
func imyaIz(a []string) string {
	for _, s := range a {
		if strings.HasPrefix(s, "name=") {
			return strings.TrimPrefix(s, "name=")
		}
	}
	return ""
}

// С3 аудита 1.6.1. Пересборка под запертой машиной снимала правило и заводила
// его заново: между двумя вызовами netsh туннель стоял без разрешения, и ping
// рвался на каждом переподъёме. Теперь на каждом шаге под каждым настоящим
// именем стоит либо само правило, либо его замена под другим именем.
func TestPeresborkaPodZashchitoyBezOkna(t *testing.T) {
	zhurnal := perehvat(t, otvetProfiley(vyvodProfilyaRu))
	r := obraztsovoeRazreshyonnoe()
	if err := VklyuchitVesTrafik(r, true); err != nil {
		t.Fatal(err)
	}
	nachalo := len(*zhurnal)
	if err := VklyuchitVesTrafik(r, true); err != nil {
		t.Fatal(err)
	}
	nastoyashchie := map[string]bool{}
	stoit := map[string]int{}
	for _, k := range KomandyRazresheniya(r) {
		nastoyashchie[k[0]] = true
		stoit[k[0]] = 1
	}
	pokryto := func(x string) bool {
		for imya, n := range stoit {
			if n > 0 && (imya == x || strings.HasPrefix(imya, x) && !nastoyashchie[imya]) {
				return true
			}
		}
		return false
	}
	for _, z := range (*zhurnal)[nachalo:] {
		s := strings.Join(z.argumenty, " ")
		switch {
		case strings.Contains(s, " add rule "):
			stoit[imyaIz(z.argumenty)]++
		case strings.Contains(s, " delete rule "):
			stoit[imyaIz(z.argumenty)] = 0
		default:
			continue
		}
		for x := range nastoyashchie {
			if !pokryto(x) {
				t.Fatalf("после «%s» правила %s нет ни под каким именем", s, x)
			}
		}
	}
	for x := range nastoyashchie {
		if stoit[x] != 1 {
			t.Fatalf("после пересборки правил %s стоит %d", x, stoit[x])
		}
	}
}

// Отказ пересборки не снимает защиту: машина заперта нами, и распечатать её
// отказом netsh значит снять защиту ровно тогда, когда что-то пошло не так.
func TestOtkazPeresborkiOstavlyaetZashchitu(t *testing.T) {
	otvet := otvetProfiley(vyvodProfilyaRu)
	lomat := false
	zhurnal := perehvat(t, func(a []string) (string, error) {
		if s := strings.Join(a, " "); lomat && strings.Contains(s, " add rule ") && strings.Contains(s, PravAllowLan) {
			return "", errors.New("netsh отказал")
		}
		return otvet(a)
	})
	if err := VklyuchitVesTrafik(obraztsovoeRazreshyonnoe(), true); err != nil {
		t.Fatal(err)
	}
	lomat = true
	nachalo := len(*zhurnal)
	if err := VklyuchitVesTrafik(obraztsovoeRazreshyonnoe(), true); err == nil {
		t.Fatal("отказ пересборки проглочен")
	}
	for _, z := range (*zhurnal)[nachalo:] {
		if s := strings.Join(z.argumenty, " "); strings.Contains(s, "firewallpolicy") && strings.Contains(s, "allowoutbound") {
			t.Fatalf("отказ пересборки распечатал машину: %s", s)
		}
	}
	if _, err := ProchitatOtkat(); err != nil {
		t.Fatalf("файл отката снят: %v", err)
	}
}

// С2 аудита 1.6.1. Правило серверов без порта пускало любую программу на любой
// порт этих адресов, а среди них адреса подписки и GitHub. Без портов правила
// нет вовсе: без remoteport оно снова стало бы распахнутым.
func TestPraviloServerovTolkoNaIhPortah(t *testing.T) {
	r := obraztsovoeRazreshyonnoe()
	r.Porty = []string{"443", "20000-30000"}
	nashli := map[string]bool{}
	for _, k := range KomandyRazresheniya(r) {
		if k[0] != PravAllowSrv && k[0] != PravAllowSrvUdp {
			continue
		}
		s := strings.Join(k, " ")
		if !strings.Contains(s, "remoteport=443,20000-30000") || !strings.Contains(s, "remoteip=192.0.2.225") {
			t.Fatalf("правило серверов без своих портов: %s", s)
		}
		nashli[k[0]] = true
	}
	if !nashli[PravAllowSrv] || !nashli[PravAllowSrvUdp] {
		t.Fatalf("правил серверов %v, ждали TCP и UDP", nashli)
	}
	r.Porty = nil
	for _, k := range KomandyRazresheniya(r) {
		if k[0] == PravAllowSrv || k[0] == PravAllowSrvUdp {
			t.Fatalf("правило серверов без портов: %v", k)
		}
	}
}

func TestPraviloMestnyhNesyotSetiChuzhihTunneley(t *testing.T) {
	// Список собирает вызывающий по живым адаптерам; правило обязано его
	// донести. Без этого Radmin VPN умирал под блокировкой молча.
	r := Razreshyonnoe{
		AdresTun:    netip.MustParseAddr("172.19.0.1"),
		ChuzhieSeti: []string{"26.0.0.0/8"},
	}
	nashli := false
	for _, k := range pravilaRazresheniya(r) {
		if k[0] != PravAllowLan {
			continue
		}
		nashli = true
		stroka := strings.Join(k, " ")
		if !strings.Contains(stroka, "26.0.0.0/8") {
			t.Fatalf("сеть чужого туннеля не доехала до правила: %s", stroka)
		}
		if !strings.Contains(stroka, "192.168.0.0/16") {
			t.Fatalf("частные сети потеряны: %s", stroka)
		}
	}
	if !nashli {
		t.Fatal("правила местных сетей нет вовсе")
	}
}
