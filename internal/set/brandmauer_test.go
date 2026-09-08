package set

import (
	"errors"
	"net/netip"
	"strings"
	"testing"
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
	t.Cleanup(func() { vypolnit = prezhniy; katalogDannyh = prezhniyKat })
	return &zhurnal
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
	for _, imya := range []string{PravAllowTun, PravAllowSrv, PravAllowLan, PravAllowDns,
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
	// Решение владельца 01.09.2026: открыть TCP/53 к тому же резолверу.
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

	err := PerezavestiRazreshyonnyeServery([]netip.Addr{
		netip.MustParseAddr("203.0.113.7"),
		netip.MustParseAddr("198.51.100.9"),
	})
	if err != nil {
		t.Fatalf("переучреждение правила: %v", err)
	}

	var stroki []string
	for _, z := range *zhurnal {
		stroki = append(stroki, strings.Join(z.argumenty, " "))
	}
	if len(stroki) != 2 {
		t.Fatalf("вызовов netsh %d, ждали два (снять и завести): %v", len(stroki), stroki)
	}
	if !strings.Contains(stroki[0], "delete rule name="+PravAllowSrv) {
		t.Errorf("первым обязано идти снятие прежнего правила, а идёт: %s", stroki[0])
	}
	if !strings.Contains(stroki[1], "add rule name="+PravAllowSrv) {
		t.Errorf("вторым обязано идти заведение, а идёт: %s", stroki[1])
	}
	if !strings.Contains(stroki[1], "remoteip=203.0.113.7,198.51.100.9") {
		t.Errorf("в правиле не тот список адресов: %s", stroki[1])
	}
	// Ни политика, ни чужие правила, ни файл отката: у этой функции ровно один
	// предмет. Тронуть политику при мёртвом ядре значило бы распечатать машину.
	for _, s := range stroki {
		if strings.Contains(s, "firewallpolicy") || strings.Contains(s, PravAllowTun) {
			t.Errorf("тронуто чужое: %s", s)
		}
	}
}

// Пустой список это НЕ повод оставить прежнее правило: серверов не осталось,
// разрешать некуда, и отказ в сторону «заперто плотнее» тут единственный верный.
func TestPerezavestiServeryBezAdresovSnimaetPravilo(t *testing.T) {
	zhurnal := perehvat(t, func([]string) (string, error) { return "Ok.", nil })

	if err := PerezavestiRazreshyonnyeServery(nil); err != nil {
		t.Fatalf("переучреждение пустым списком: %v", err)
	}
	if len(*zhurnal) != 1 {
		t.Fatalf("вызовов netsh %d, ждали один (только снятие): %v", len(*zhurnal), *zhurnal)
	}
	if s := strings.Join((*zhurnal)[0].argumenty, " "); !strings.Contains(s, "delete rule name="+PravAllowSrv) {
		t.Errorf("единственным вызовом обязано быть снятие, а это: %s", s)
	}
}
