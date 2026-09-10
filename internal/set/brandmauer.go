package set

import (
	"errors"
	"fmt"
	"log"
	"net/netip"
	"regexp"
	"strings"
)

// Режим «весь трафик»: DefaultOutboundAction = Block плюс именованные
// разрешающие правила.
//
// Одним блокирующим правилом со списком исключений обойтись НЕЛЬЗЯ: в
// брандмауэре Windows блокирующие правила стоят выше разрешающих, явное Block
// всегда бьёт явное Allow, а единственное исключение требует IPsec. Разрешающие
// выигрывают только против умолчания профиля, то есть против политики.
//
// Плата названа прямо: снятие правил по имени больше НЕ возвращает интернет,
// возвращает его откат политики, и прежнее значение надо знать заранее.

var (
	ErrBrandmauerVyklyuchen = errors.New("брандмауэр выключен: политика не будет применяться")
	ErrNetTunnelya          = errors.New("нет адреса TUN: правило разрешения некуда привязать")
)

// Имена правил. Имя это опознание: по нему их находит снятие, проверка
// осиротевшего и человек с аварийным листом в руках.
const (
	PravAllowTun = "Affory-Allow-Tun"
	PravAllowSrv = "Affory-Allow-Server"
	PravAllowLan = "Affory-Allow-Local"
	PravAllowDns = "Affory-Allow-Dns"
	// Отдельным именем, а не вторым правилом с тем же: снятие идёт по именам,
	// и одно имя на два правила читается как «одно правило», пока кто-нибудь не
	// начнёт их считать.
	PravAllowDnsTcp = "Affory-Allow-Dns-Tcp"
	PravAllowProc   = "Affory-Allow-Proc"
)

// VseImenaPravil это запасной список на случай, когда файла отката нет: после
// нештатной смерти службы снимать всё равно надо. Правил по процессам может
// быть сколько угодно, поэтому запас берётся с потолком: удаление
// несуществующего правила ничего не стоит, а оставшееся висеть стоит интернета.
//
// По маске снятие не идёт принципиально: маска однажды заденет чужое правило с
// похожим именем, и заметят это не сразу.
func VseImenaPravil() []string {
	imena := []string{PravAllowTun, PravAllowSrv, PravAllowLan, PravAllowDns, PravAllowDnsTcp, ImyaPravilaIPv6}
	for i := 0; i < 10; i++ {
		imena = append(imena, fmt.Sprintf("%s-%d", PravAllowProc, i))
	}
	return imena
}

// Razreshyonnoe это шесть пунктов списка из спеки, выраженные данными.
type Razreshyonnoe struct {
	// AdresTun это адрес САМОГО TUN-адаптера. Привязка идёт по нему, а не по
	// имени адаптера: у netsh привязки к адаптеру по имени нет вовсе, а
	// interfacetype знает только wireless, lan и ras, и lan заодно накрыл бы
	// физический Ethernet.
	AdresTun netip.Addr

	// Kandidaty это адреса ВСЕХ серверов, а не только выбранного: иначе
	// переключение сервера в запертом режиме отрезает само себя.
	Kandidaty []netip.Addr

	// Shlyuz и Resolver берутся из системы (см. sistema.go). Без них умирают
	// локальная сеть, принтер и повторный резолв по TTL.
	Shlyuz   netip.Addr
	Resolver netip.Addr

	// Protsessy это полные пути ядер и самой службы.
	Protsessy []string

	// Служебных адресов отдельным полем здесь НЕТ, и это разобрано 02.09.2026.
	//
	// Пункт спеки просил три вещи: адрес подписки, адрес снимка состояния и
	// наборы rule_set. Адрес подписки уже едет через Kandidaty (его кладёт
	// SobratAdresa), наборы rule_set вычеркнуты решением волны 3, а у снимка
	// состояния адреса нет вовсе: он лежит файлом на диске. Поле стояло пустым
	// у всех вызывающих и создавало вид покрытия там, где покрывать нечего.
}

// Значения netsh НЕ локализуются, в отличие от ключей. Поэтому состояние
// опознаётся по значению строки, а не по имени поля слева: имя поля на другой
// локали станет другим, а ON и BlockInbound,AllowOutbound останутся собой.
var (
	reSostoyanie = regexp.MustCompile(`(?im)^\s*\S+\s+(ON|OFF)\s*$`)
	rePolitika   = regexp.MustCompile(`(?im)^\s*\S.*?\s+((?:Block|Allow)Inbound,(?:Block|Allow)Outbound)\s*$`)
)

var imenaProfiley = []string{"domainprofile", "privateprofile", "publicprofile"}

// SostoyanieProfiley спрашивает КАЖДЫЙ профиль отдельно.
//
// Один вызов show allprofiles потребовал бы резать вывод на блоки по заголовкам,
// а заголовки локализуются. Три вызова стоят миллисекунды и не зависят от языка
// системы вовсе.
func SostoyanieProfiley() ([]ProfilDo, error) {
	var itog []ProfilDo
	for _, p := range imenaProfiley {
		vyhod, err := vypolnit([]string{"advfirewall", "show", p})
		if err != nil {
			return nil, fmt.Errorf("состояние профиля %s не прочитано: %w", p, err)
		}
		pr := ProfilDo{Imya: strings.TrimSuffix(p, "profile")}
		if m := reSostoyanie.FindStringSubmatch(vyhod); m != nil {
			pr.Vklyuchen = strings.EqualFold(m[1], "ON")
		} else {
			return nil, fmt.Errorf("в выводе профиля %s не нашлось ON или OFF", p)
		}
		if m := rePolitika.FindStringSubmatch(vyhod); m != nil {
			pr.Politika = m[1]
		} else {
			return nil, fmt.Errorf("в выводе профиля %s не нашлось политики", p)
		}
		itog = append(itog, pr)
	}
	return itog, nil
}

// VklyuchitVesTrafik ставит режим целиком и в единственно верном порядке.
func VklyuchitVesTrafik(r Razreshyonnoe, namerenno bool) error {
	if !r.AdresTun.IsValid() {
		return ErrNetTunnelya
	}

	// 1. Снять прежнее состояние и записать на диск. До любых изменений.
	do, err := SostoyanieProfiley()
	if err != nil {
		return err
	}
	pravila := pravilaRazresheniya(r)
	// Правило IPv6 в этот список НЕ входит намеренно. Оно принадлежит туннелю, а
	// не режиму, и живёт ровно столько, сколько туннель. Попав сюда, оно снималось
	// бы при выключении режима, туннель при этом оставался бы поднятым, IPv6 шёл
	// бы мимо него, а служба считала бы правило стоящим, потому что VernutIPv6
	// никто не звал.
	imena := []string{}
	for _, k := range pravila {
		imena = append(imena, k[0])
	}

	// Прежнее состояние снимается ОДИН РАЗ, при первом включении. Дальше эта
	// функция зовётся повторно на каждую пересборку правил (добавили сервер при
	// включённом режиме), и машина к тому моменту уже заперта НАМИ. Записать
	// снятое сейчас значило бы положить в откат Block как "прежнее" и запереть
	// машину навсегда: выключение режима честно вернуло бы Block обратно, файл
	// отката удалило, а status при этом отвечал бы kill_switch: false.
	//
	// Имена правил, наоборот, обновляются всегда: их состав меняется вместе со
	// списком серверов и процессов, а снятие идёт именно по ним.
	//
	// Namerenno только поднимается. Понизить его пересборкой значило бы
	// разрешить следующему старту службы распечатать машину, которую заперли
	// осознанно.
	if prezhniy, err := ProchitatOtkat(); err == nil {
		do = prezhniy.Profili
		namerenno = namerenno || prezhniy.Namerenno
	} else if !errors.Is(err, ErrOtkataNet) {
		return err
	} else {
		// Вторая половина решения 28 плана волны 2. Говорится ровно ЗДЕСЬ:
		// это единственная ветка, где машина переходит из открытой в запертую,
		// а пересборка правил при уже включённом режиме идёт мимо.
		//
		// netsh отвергает возврат дословно: "Notconfigured value can only be
		// used when configuring a Group Policy object (GPO) store". Командлет
		// вернуть смог бы, но он запрещён правилом "одна утилита на всё".
		//
		// Замерено опытом 02.09.2026 (stend/opyt-hranilishche-politiki.ps1):
		// netsh show читает ПОСТОЯННОЕ хранилище и печатает NotConfigured тем же
		// текстом, что и явное blockinbound,allowoutbound. То есть след виден
		// только командлетом чтения, а человеку взяться о нём неоткуда, кроме
		// этой строки.
		log.Print("постоянная настройка профилей брандмауэра изменена БЕЗВОЗВРАТНО: " +
			"значение NotConfigured вернуть нечем, оно станет явным " +
			"blockinbound,allowoutbound. Поведение то же, состояние другое")
	}

	if err := ZapisatOtkat(Otkat{Profili: do, Namerenno: namerenno, Pravila: imena}); err != nil {
		return err
	}

	// 2. Включить брандмауэр там, где он выключен. Решено 01.09.2026:
	// программе разрешено менять настройки, нужные для работы.
	// Замерено в тот же день: при выключенном профиле политика Block не делает
	// НИЧЕГО, интернет работает, а интерфейс при этом зелёный. Тихий отказ тут
	// худший из возможных.
	for _, p := range do {
		if p.Vklyuchen {
			continue
		}
		if _, err := vypolnit([]string{"advfirewall", "set", p.Imya + "profile", "state", "on"}); err != nil {
			return fmt.Errorf("%w: профиль %s: %v", ErrBrandmauerVyklyuchen, p.Imya, err)
		}
		// Сказано ВСЛУХ, потому что это правка настройки всей машины, а не
		// нашей программы. Выключение режима вернёт профиль обратно по файлу
		// отката, но если файл потерять, вернуть будет нечем и человек об этом
		// узнает только из журнала. Решение 28 плана волны 2.
		log.Printf("профиль %s брандмауэра был ВЫКЛЮЧЕН и включён нами: "+
			"без включённого профиля запрет не применяется вовсе. "+
			"Выключение режима вернёт его обратно", p.Imya)
	}

	// 3. Разрешающие правила. Ставятся ДО политики: наоборот означает окно, в
	// котором машина уже заперта, а исключений ещё нет.
	for _, k := range pravila {
		_, _ = vypolnit([]string{"advfirewall", "firewall", "delete", "rule", "name=" + k[0]})
		if _, err := vypolnit(k[1:]); err != nil {
			_ = VyklyuchitVesTrafik()
			return fmt.Errorf("правило %s не заведено: %w", k[0], err)
		}
	}

	// 4. И только последним запрет по умолчанию.
	if _, err := vypolnit([]string{"advfirewall", "set", "allprofiles",
		"firewallpolicy", "blockinbound,blockoutbound"}); err != nil {
		_ = VyklyuchitVesTrafik()
		return fmt.Errorf("политика не поставлена: %w", err)
	}
	return nil
}

// VyklyuchitVesTrafik снимает режим в ОБРАТНОМ порядке.
//
// Сначала политика, потом правила. У снятия есть окно, где туннель ещё поднят, а
// защиты уже нет, и это осознанная плата: обратный порядок запирает машину, если
// снятие зависнет посередине, а это хуже.
func VyklyuchitVesTrafik() error {
	o, err := ProchitatOtkat()
	if errors.Is(err, ErrOtkataNet) {
		_, _, err = podmesti()
		return err
	}
	if err != nil {
		return err
	}

	var oshibki []error
	for _, p := range o.Profili {
		if _, err := vypolnit([]string{"advfirewall", "set", p.Imya + "profile", "firewallpolicy", strings.ToLower(p.Politika)}); err != nil {
			oshibki = append(oshibki, fmt.Errorf("политика %s не возвращена: %w", p.Imya, err))
		}
	}
	if len(oshibki) > 0 {
		return errors.Join(oshibki...)
	}
	// Имена берутся из файла отката: там записано то, что реально заведено.
	// Запасной список нужен, когда файла нет, и он заведомо шире нужного.
	imena := o.Pravila
	if len(imena) == 0 {
		imena = VseImenaPravil()
	}
	for _, imya := range imena {
		vyhod, err := vypolnit([]string{"advfirewall", "firewall", "delete", "rule", "name=" + imya})
		if err != nil && !netPravil(vyhod) {
			oshibki = append(oshibki, fmt.Errorf("правило %s не снято: %w", imya, err))
		}
	}
	if o.Profili != nil {
		for _, p := range o.Profili {
			if p.Vklyuchen {
				continue // был включён, включённым и оставляем
			}
			if _, err := vypolnit([]string{"advfirewall", "set", p.Imya + "profile", "state", "off"}); err != nil {
				oshibki = append(oshibki, fmt.Errorf("профиль %s не выключен обратно: %w", p.Imya, err))
			}
		}
	}
	if len(oshibki) > 0 {
		// Файл отката НЕ удаляется: он единственное, по чему следующий старт
		// поймёт, что машина осталась запертой.
		return errors.Join(oshibki...)
	}
	return UdalitOtkat()
}

// Возврат берёт значение из СВОЕГО файла, а не из живой системы.
//
// Причина проще, чем казалась, и прежняя формулировка здесь была неверна.
// Замерено опытом 02.09.2026 (stend/opyt-hranilishche-politiki.ps1): netsh show
// читает ПОСТОЯННОЕ хранилище, то есть ровно то, которое меняет netsh set.
// Действующее значение он не показывает вовсе: локальный GPO с Block дал
// ActiveStore=Block при netsh=AllowOutbound.
//
// Живую систему спрашивать нельзя по другой причине: к моменту возврата она
// заперта НАМИ, и "прежним" значением оказался бы наш собственный Block.
// Второе: netsh печатает NotConfigured тем же текстом, что и явное
// blockinbound,allowoutbound, поэтому отличить нетронутую настройку от
// настроенной по его выводу невозможно в принципе.
func politikaIzOtkata(o Otkat) string {
	for _, p := range o.Profili {
		if p.Politika != "" {
			return strings.ToLower(p.Politika)
		}
	}
	return ""
}

// pravilaRazresheniya возвращает пары: имя правила и команда его создания.
func pravilaRazresheniya(r Razreshyonnoe) [][]string {
	var itog [][]string
	dobavit := func(imya string, hvost ...string) {
		itog = append(itog, append([]string{imya,
			"advfirewall", "firewall", "add", "rule", "name=" + imya,
			"dir=out", "action=allow", "profile=any"}, hvost...))
	}

	// 1. Исходящий с адреса TUN-адаптера.
	dobavit(PravAllowTun, "localip="+r.AdresTun.String())

	// 2. Адреса серверов и ВСЕ кандидаты, включая узел подписки: без него режим
	// отрезает обновление списка ровно тогда, когда оно нужнее всего.
	if s := spisokAdresov(r.Kandidaty); s != "" {
		dobavit(PravAllowSrv, "remoteip="+s)
	}

	// 3. Частные диапазоны и шлюз: иначе умирают локальная сеть и принтер.
	mestnye := []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "169.254.0.0/16"}
	if r.Shlyuz.IsValid() {
		mestnye = append(mestnye, r.Shlyuz.String())
	}
	dobavit(PravAllowLan, "remoteip="+strings.Join(mestnye, ","))

	// 4. Порт 53 к локальному резолверу: на нём держится повторный резолв по TTL.
	//
	// ОБА протокола. Правило было только на UDP, а резолвер уходит на TCP при
	// усечённом ответе, и в запертом режиме этот переспрос молча не доезжал:
	// выглядит как случайно неработающие сайты, а не как правило брандмауэра.
	// Решено 01.09.2026.
	if r.Resolver.IsValid() {
		dobavit(PravAllowDns, "remoteip="+r.Resolver.String(), "remoteport=53", "protocol=udp")
		dobavit(PravAllowDnsTcp, "remoteip="+r.Resolver.String(), "remoteport=53", "protocol=tcp")
	}

	// 5. Процессы ядер и сама служба. У netsh одно правило это ОДНА программа,
	// списка тут нет, поэтому имена нумеруются.
	for i, p := range r.Protsessy {
		itog = append(itog, []string{fmt.Sprintf("%s-%d", PravAllowProc, i),
			"advfirewall", "firewall", "add", "rule",
			"name=" + fmt.Sprintf("%s-%d", PravAllowProc, i),
			"dir=out", "action=allow", "profile=any", "program=" + p})
	}
	return itog
}

func spisokAdresov(a []netip.Addr) string {
	var s []string
	for _, k := range a {
		if k.IsValid() {
			s = append(s, k.String())
		}
	}
	return strings.Join(s, ",")
}

// SnyatOsirotevshee зовётся при КАЖДОМ старте службы.
//
// Осиротевшая защита это правила и политика Block, оставшиеся после нештатной
// смерти службы: туннеля нет, а машина заперта, и распечатать её некому.
//
// Развилка, которую редакция 3 не разводила вовсе. Правило «политика Block
// есть, туннеля нет, значит осиротело» накрывает и НАМЕРЕННО запертую машину,
// и она после перезапуска службы распечаталась бы сама. Это ровно наоборот
// тому, ради чего kill-switch включают. Поэтому решает признак Namerenno в
// файле отката:
//
//	Namerenno = false -> сняли, это наш собственный мусор;
//	Namerenno = true  -> НЕ трогаем, но говорим вслух кодом killswitch-orphan.
//
// Возвращает: снято ли, и осталась ли машина запертой намеренно.
// VesTrafikVklyuchyon отвечает, заперта ли машина СЕЙЧАС, спрашивая брандмауэр.
//
// Заперта означает BlockOutbound хотя бы на одном ВКЛЮЧЁННОМ профиле:
// выключенный профиль свою политику не применяет вовсе, это замерено, и считать
// его за блокировку значит принять открытую машину за запертую.
func VesTrafikVklyuchyon() (bool, error) {
	profili, err := SostoyanieProfiley()
	if err != nil {
		return false, err
	}
	for _, p := range profili {
		if p.Vklyuchen && strings.Contains(p.Politika, "BlockOutbound") {
			return true, nil
		}
	}
	return false, nil
}

func SnyatOsirotevshee() (snyato bool, zapertaNamerenno bool, err error) {
	o, err := ProchitatOtkat()
	if errors.Is(err, ErrOtkataNet) {
		// Файла нет. Но правила могли остаться от сборки, у которой файла ещё не
		// было, поэтому мусор всё равно подметается по запасному списку.
		return podmesti()
	}
	if err != nil {
		return false, false, err
	}
	if o.Namerenno {
		// Файлу верить нельзя, не спросив саму машину. Выход из режима руками по
		// аварийному листу возвращает политику и снимает правила, но файл не
		// трогает: он остаётся лежать и утверждать, что машина заперта. Без этой
		// проверки служба докладывала бы о намеренной блокировке при каждом
		// старте до скончания века, а машина при этом открыта настежь.
		zaperta, err := VesTrafikVklyuchyon()
		if err != nil {
			return false, false, err
		}
		if zaperta {
			return false, true, nil
		}
		// Машина открыта, значит файл устарел. Удаляем: держать его дальше значит
		// врать следующему старту тем же самым способом.
		if err := UdalitOtkat(); err != nil {
			return false, false, err
		}
		return false, false, snyatSirotuIPv6()
	}
	if err := VyklyuchitVesTrafik(); err != nil {
		return false, false, err
	}
	return true, false, snyatSirotuIPv6()
}

// snyatSirotuIPv6 снимает Affory-IPv6-Block-Out, оставшийся от мёртвого туннеля.
//
// Правило принадлежит туннелю, а зовётся всё это при старте службы, то есть
// когда туннеля заведомо нет. Глушить IPv6 без туннеля это чистая потеря:
// защищать нечего, а половина сети у человека не работает.
//
// НЕ зовётся на ветке «заперта намеренно»: там сеть закрыта целиком политикой,
// вреда от правила нет, а трогать намеренно запертую машину нельзя вовсе.
// Прежде правило снималось только при отсутствии файла отката, потому что
// VyklyuchitVesTrafik перебирает имена ИЗ ФАЙЛА, а IPv6 туда не вносился.
func snyatSirotuIPv6() error { return VernutIPv6() }

// podmesti снимает наши правила, ничего не зная о прошлом.
//
// Политику при этом НЕ трогает: без файла отката неизвестно, ставили ли её мы,
// и вернуть чужой Block в Allow значит отключить чужую защиту, приняв её за
// свой мусор.
func podmesti() (bool, bool, error) {
	nashli := false
	var oshibki []error
	for _, imya := range VseImenaPravil() {
		vyhod, err := vypolnit([]string{"advfirewall", "firewall", "delete", "rule", "name=" + imya})
		if err != nil && !netPravil(vyhod) {
			oshibki = append(oshibki, fmt.Errorf("правило %s не снято: %w", imya, err))
		}
		if err == nil && !netPravil(vyhod) {
			nashli = true
		}
	}
	return nashli, false, errors.Join(oshibki...)
}

// PerezavestiRazreshyonnyeServery переписывает ОДНО правило: список адресов
// серверов.
//
// Зачем отдельно от VklyuchitVesTrafik. Та собирает набор целиком и требует
// адрес TUN-адаптера, потому что на нём держится правило туннеля. При упавшем
// туннеле адреса нет, пересобрать набор нечем, и весь набор оставался стоять
// под ПРЕЖНИЙ список серверов. Удалённый сервер при этом сохранял разрешение
// наружу в запертом режиме, то есть ровно там, где режим и включают. Список
// серверов от туннеля не зависит вовсе, и переписать его можно всегда.
//
// Порядок здесь ОБРАТНЫЙ общему: сначала снять, потом завести. У netsh
// заведение по существующему имени добавляет ВТОРОЕ правило с тем же именем, и
// «переписать» превратилось бы в «дописать», а старый адрес остался бы
// разрешённым. Окно между снятием и заведением машина проводит запертой
// плотнее нужного, и это верная сторона отказа.
//
// Имя правила в файл отката не пишется намеренно: режим включается только при
// живом туннеле, значит правило уже заведено и уже записано, а пустой список
// его снимает, и снятие несуществующего правила при выключении режима ничего
// не стоит.
func PerezavestiRazreshyonnyeServery(kandidaty []netip.Addr) error {
	// Правила могло не быть вовсе: это не отказ. Отличает netPravil по выводу,
	// и глотать здесь ЛЮБОЙ отказ нельзя: молчащий netsh означал бы, что старый
	// список остался стоять, а мы доложили об успехе.
	if vyhod, err := vypolnit([]string{"advfirewall", "firewall", "delete", "rule",
		"name=" + PravAllowSrv}); err != nil && !netPravil(vyhod) {
		return fmt.Errorf("правило %s не снято: %w", PravAllowSrv, err)
	}
	s := spisokAdresov(kandidaty)
	if s == "" {
		// Серверов не осталось: разрешать некуда, и правило без адресов у netsh
		// значит «любой адрес», то есть распахнутую дыру вместо снятой.
		return nil
	}
	if _, err := vypolnit([]string{"advfirewall", "firewall", "add", "rule",
		"name=" + PravAllowSrv, "dir=out", "action=allow", "profile=any",
		"remoteip=" + s}); err != nil {
		return fmt.Errorf("правило %s не переучреждено: %w", PravAllowSrv, err)
	}
	return nil
}
