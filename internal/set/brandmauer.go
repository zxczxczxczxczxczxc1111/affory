package set

import (
	"errors"
	"fmt"
	"log"
	"net/netip"
	"slices"
	"strings"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/wfp"
)

// Режим «весь трафик» запирает машину фильтрами WFP, см. zamok_wfp.go.
//
// До 18.10.2026 запрет стоял политикой брандмауэра Windows (DefaultOutboundAction
// = Block) плюс именованными разрешающими правилами netsh. Эта схема здесь
// осталась только для снятия: машина, запертая версией 1.9.4 и раньше, после
// обновления распечатывается по старому файлу отката, см. snyatStaryyZamok.

var (
	ErrBrandmauerVyklyuchen = errors.New("фильтрация Windows недоступна: запрет не применится")
	ErrNetTunnelya          = errors.New("нет адреса TUN: разрешение некуда привязать")
)

// Имена правил прежней схемы. По ним снимается замок прежней версии, их
// подметает старт службы, и их перечисляет аварийный лист.
const (
	PravAllowTun = "Affory-Allow-Tun"
	PravAllowSrv = "Affory-Allow-Server"
	// Порт у netsh задаётся только вместе с протоколом, поэтому серверам было
	// два правила: TCP под прежним именем и UDP для hy2 и tuic.
	PravAllowSrvUdp = "Affory-Allow-Server-Udp"
	PravAllowLan    = "Affory-Allow-Local"
	PravAllowDns    = "Affory-Allow-Dns"
	PravAllowDnsTcp = "Affory-Allow-Dns-Tcp"
	PravAllowProc   = "Affory-Allow-Proc"
)

// VseImenaPravil это запасной список правил прежней схемы на случай, когда
// файла отката нет: после нештатной смерти службы снимать всё равно надо.
// Правил по процессам было сколько угодно, поэтому запас берётся с потолком:
// удаление несуществующего правила ничего не стоит, а оставшееся висеть стоит
// интернета.
//
// По маске снятие не идёт принципиально: маска однажды заденет чужое правило с
// похожим именем, и заметят это не сразу.
func VseImenaPravil() []string {
	rezhim := []string{PravAllowTun, PravAllowSrv, PravAllowSrvUdp, PravAllowLan, PravAllowDns, PravAllowDnsTcp}
	for i := 0; i < 10; i++ {
		rezhim = append(rezhim, fmt.Sprintf("%s-%d", PravAllowProc, i))
	}
	// Замены жили только посреди пересборки, но служба могла умереть и там.
	imena := append(slices.Clone(rezhim), ImyaPravilaIPv6)
	for _, imya := range rezhim {
		imena = append(imena, vremennoeImya(imya))
	}
	return imena
}

// suffiksZameny отличал правило-замену на время пересборки прежней схемы.
const suffiksZameny = "-Zamena"

func vremennoeImya(imya string) string { return imya + suffiksZameny }

// Razreshyonnoe это то, что запрет пропускает, выраженное данными.
type Razreshyonnoe struct {
	// AdresTun это адрес САМОГО TUN-адаптера: всё, что уходит с него, идёт
	// через туннель.
	AdresTun netip.Addr

	// Kandidaty это адреса ВСЕХ серверов, а не только выбранного: иначе
	// переключение сервера в запертом режиме отрезает само себя.
	Kandidaty []netip.Addr
	// Porty это порты серверов, подписок и загрузок: «443», «20000-30000».
	// Без них разрешение пускало любую программу на любой порт этих адресов
	// (С2 аудита 1.6.1). Пустой список значит «разрешения на серверы нет»:
	// разрешение без порта снова стало бы распахнутым.
	Porty []string

	// Shlyuz и Resolver берутся из системы (см. sistema.go). Без них умирают
	// локальная сеть, принтер и повторный резолв по TTL.
	Shlyuz   netip.Addr
	Resolver netip.Addr

	// Protsessy это полные пути ядер и самой службы.
	Protsessy []string

	// ChuzhieSeti это сети чужих туннелей, поднятых на этой машине прямо сейчас
	// (Radmin VPN, Hamachi, Tailscale). Пустой список это обычный случай:
	// разрешаются только те, чей адаптер в системе действительно есть, см.
	// chuzhie_seti.go.
	ChuzhieSeti []string

	// Служебных адресов отдельным полем здесь НЕТ, и это разобрано 02.09.2026.
	//
	// Пункт спеки просил три вещи: адрес подписки, адрес снимка состояния и
	// наборы rule_set. Адрес подписки уже едет через Kandidaty (его кладёт
	// SobratAdresa), наборы rule_set вычеркнуты решением волны 3, а у снимка
	// состояния адреса нет вовсе: он лежит файлом на диске. Поле стояло пустым
	// у всех вызывающих и создавало вид покрытия там, где покрывать нечего.
}

var imenaProfiley = []string{"domainprofile", "privateprofile", "publicprofile"}

// SostoyanieProfiley читает КАЖДЫЙ профиль брандмауэра из реестра, где у
// состояния нет языка. Нужен прежней схеме: по нему видно, стоит ли её замок.
// Недостающие значения реестра дают умолчание Windows, см.
// sostoyanieIzReestra.
func SostoyanieProfiley() ([]ProfilDo, error) {
	var itog []ProfilDo
	for _, p := range imenaProfiley {
		imya := strings.TrimSuffix(p, "profile")
		pr, err := chitatIzReestra(imya)
		if err != nil {
			return nil, fmt.Errorf("состояние профиля %s не прочитано: %w", p, err)
		}
		itog = append(itog, pr)
	}
	return itog, nil
}

// VklyuchitVesTrafik ставит запрет целиком или заменяет стоящий новым.
//
// Функция зовётся и на первое включение, и на каждую пересборку (добавили
// сервер, переподнялся туннель). Замена идёт одной транзакцией WFP: машина не
// бывает ни открытой посередине, ни запертой без разрешений.
func VklyuchitVesTrafik(r Razreshyonnoe, namerenno bool) error {
	if !r.AdresTun.IsValid() {
		return ErrNetTunnelya
	}
	nabor, err := naborFiltrov(r)
	if err != nil {
		return err
	}

	prezhniy, err := ProchitatOtkat()
	estFayl := err == nil
	if err != nil && !errors.Is(err, ErrOtkataNet) {
		return err
	}
	peresborka := estFayl && prezhniy.Wfp

	// Файл пишется ДО фильтров: служба, умершая между ними, оставит запрет, и
	// следующий старт обязан знать, что он наш и намеренный ли.
	//
	// Namerenno только поднимается. Понизить его пересборкой значило бы
	// разрешить следующему старту службы распечатать машину, которую заперли
	// осознанно.
	novyy := prezhniy
	novyy.Wfp = true
	novyy.Namerenno = namerenno || prezhniy.Namerenno
	if err := ZapisatOtkat(novyy); err != nil {
		return err
	}

	if err := zamok.Postavit(nabor); err != nil {
		err = sBfe(err)
		if peresborka {
			// Транзакция откатилась: стоит прежний запрет, и машина остаётся
			// запертой, что бы ни случилось (С3 аудита 1.6.1).
			return fmt.Errorf("запрет не пересобран, защита осталась прежней: %w", err)
		}
		// Транзакция откатилась целиком, в системе не изменилось ничего, кроме
		// файла. Оставленный, он заставил бы выключение режима снимать то, чего
		// нет, а старт службы считать машину запертой.
		vernutFayl(estFayl, prezhniy)
		return fmt.Errorf("запрет не поставлен: %w", err)
	}

	// Замок прежней версии снимается ПОСЛЕ того, как встал новый: наоборот
	// означало бы окно, в котором машина открыта. Отказ не рушит включение:
	// запрет уже стоит, а прежние имена и политика остаются в файле, и
	// выключение режима попробует снять их снова.
	if len(novyy.Profili) > 0 {
		if err := snyatStaryyZamok(novyy); err != nil {
			log.Printf("замок прежней версии не снят, запрет WFP стоит, сниму при выключении режима: %v", err)
			return nil
		}
		novyy.Profili, novyy.Pravila = nil, nil
		if err := ZapisatOtkat(novyy); err != nil {
			log.Printf("снятый замок прежней версии остался в файле отката: %v", err)
		}
	}
	return nil
}

// vernutFayl возвращает файл отката к состоянию до неудачного включения.
func vernutFayl(estFayl bool, prezhniy Otkat) {
	var err error
	if estFayl {
		err = ZapisatOtkat(prezhniy)
	} else {
		err = UdalitOtkat()
	}
	if err != nil {
		log.Printf("файл отката после отказа не возвращён: %v", err)
	}
}

// VyklyuchitVesTrafik снимает запрет и, если он остался, замок прежней версии.
//
// Файл отката удаляется последним и только после полного снятия: он
// единственное, по чему следующий старт поймёт, что машина осталась запертой.
func VyklyuchitVesTrafik() error {
	o, err := ProchitatOtkat()
	if errors.Is(err, ErrOtkataNet) {
		// Файла нет, но запрет или правила могли остаться: файл потерян, или
		// правила остались от сборки, у которой файла ещё не было.
		_, errW := snyatZapretBezFayla()
		_, _, errP := podmesti()
		return errors.Join(errW, errP)
	}
	if err != nil {
		return snyatBezOtkata(err)
	}
	if _, err := zamok.Snyat(); err != nil {
		return fmt.Errorf("запрет не снят: %w", sBfe(err))
	}
	if len(o.Profili) > 0 {
		if err := snyatStaryyZamok(o); err != nil {
			if o.Wfp {
				// Запрет WFP уже снят, в файле остаётся только прежний замок.
				o.Wfp = false
				if e := ZapisatOtkat(o); e != nil {
					log.Printf("файл отката не переписан после снятия запрета: %v", e)
				}
			}
			return err
		}
	}
	return UdalitOtkat()
}

// snyatZapretBezFayla снимает запрет, о котором файл ничего не знает.
// Недоступная BFE здесь не отказ: нашего замка за ней не видно, а установщик,
// которому этот вызов нужен на каждой машине, падать из-за неё не должен.
func snyatZapretBezFayla() (bool, error) {
	snyato, err := zamok.Snyat()
	if err != nil && errors.Is(err, wfp.ErrNedostupen) {
		log.Printf("запрет без файла отката не проверен: %v", err)
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("запрет без файла отката не снят: %w", err)
	}
	if snyato {
		log.Printf("запрет без файла отката снят: файл потерян, намеренность неизвестна")
	}
	return snyato, nil
}

// snyatStaryyZamok снимает замок прежней схемы в ОБРАТНОМ порядке: сначала
// политика, потом правила. Файл отката не трогает: решает вызывающий.
func snyatStaryyZamok(o Otkat) error {
	var oshibki []error
	for _, p := range o.Profili {
		if _, err := vypolnit([]string{"advfirewall", "set", p.Imya + "profile", "firewallpolicy", strings.ToLower(p.Politika)}); err != nil {
			oshibki = append(oshibki, fmt.Errorf("политика %s не возвращена: %w", p.Imya, err))
		}
	}
	if len(oshibki) > 0 {
		return sveritSMpsSvc(errors.Join(oshibki...))
	}
	// Имена берутся из файла отката: там записано то, что реально заведено.
	// Запасной список нужен, когда имён в файле нет, и он заведомо шире нужного.
	imena := o.Pravila
	if len(imena) == 0 {
		imena = VseImenaPravil()
	}
	for _, imya := range imena {
		if err := SnyatPravilo(imya); err != nil {
			oshibki = append(oshibki, err)
		}
	}
	for _, p := range o.Profili {
		if p.Vklyuchen {
			continue // был включён, включённым и оставляем
		}
		if _, err := vypolnit([]string{"advfirewall", "set", p.Imya + "profile", "state", "off"}); err != nil {
			oshibki = append(oshibki, fmt.Errorf("профиль %s не выключен обратно: %w", p.Imya, err))
		}
	}
	return sveritSMpsSvc(errors.Join(oshibki...))
}

// VesTrafikVklyuchyon отвечает, заперта ли машина СЕЙЧАС: стоит ли наш запрет
// WFP или замок прежней версии.
//
// Политика Block без нашего файла отката нашей не считается: человек мог
// поставить её себе сам, и снятие программы не повод её трогать.
func VesTrafikVklyuchyon() (bool, error) {
	stoit, err := zamok.Stoit()
	if err != nil {
		return false, sBfe(err)
	}
	if stoit {
		return true, nil
	}
	o, err := ProchitatOtkat()
	if errors.Is(err, ErrOtkataNet) || (err == nil && len(o.Profili) == 0) {
		return false, nil
	}
	return staryyZamokStoit()
}

// staryyZamokStoit: BlockOutbound хотя бы на одном ВКЛЮЧЁННОМ профиле.
// Выключенный профиль свою политику не применяет вовсе, это замерено, и считать
// его за блокировку значит принять открытую машину за запертую.
func staryyZamokStoit() (bool, error) {
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

// SnyatOsirotevshee зовётся при КАЖДОМ старте службы.
//
// Осиротевший замок это запрет, оставшийся после нештатной смерти службы или
// перезагрузки: туннеля нет, а машина заперта. Решает признак Namerenno в
// файле отката:
//
//	Namerenno = false -> сняли, это наш собственный мусор;
//	Namerenno = true  -> НЕ трогаем, но говорим вслух кодом killswitch-orphan.
//
// Возвращает: снято ли, и осталась ли машина запертой намеренно.
func SnyatOsirotevshee() (snyato bool, zapertaNamerenno bool, err error) {
	o, err := ProchitatOtkat()
	if errors.Is(err, ErrOtkataNet) {
		// Файл пишется ДО фильтров, поэтому запрет без файла значит, что файл
		// потерян. Намеренность записана в нём же, а запертая навсегда машина
		// хуже распечатанной: из первой человеку не выйти без аварийного листа.
		snyatoW, errW := snyatZapretBezFayla()
		snyatoP, _, errP := podmesti()
		return snyatoW || snyatoP, false, errors.Join(errW, errP)
	}
	if err != nil {
		if err := snyatBezOtkata(err); err != nil {
			return false, false, err
		}
		return true, false, snyatSirotuIPv6()
	}
	if o.Namerenno {
		// Файлу верить нельзя, не спросив саму машину: запрет могли снять
		// руками, а файл остался бы утверждать, что машина заперта.
		zaperta, err := VesTrafikVklyuchyon()
		if err != nil {
			return false, false, err
		}
		if zaperta {
			return false, true, nil
		}
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
// НЕ зовётся на ветке «заперта намеренно»: там сеть закрыта запретом целиком,
// вреда от правила нет, а трогать намеренно запертую машину нельзя вовсе.
func snyatSirotuIPv6() error { return VernutIPv6() }

// PolitikaWindowsPoUmolchaniyu это политика профиля у Windows из коробки.
const PolitikaWindowsPoUmolchaniyu = "blockinbound,allowoutbound"

// snyatBezOtkata распечатывает машину, когда файл отката есть, но не
// читается ни он, ни его прежняя версия (Н4 аудита 1.6.1).
//
// Файл есть, значит запирали мы. Снимается запрет WFP, а если стоит замок
// прежней версии, возвращается умолчание Windows: прежняя политика
// неизвестна. Открытую по брандмауэру машину это не трогает: её политика
// может быть чужой настройкой.
func snyatBezOtkata(prichina error) error {
	log.Printf("файл отката не читается (%v): снимаю запрет и правила Affory; замок прежней "+
		"версии, если стоит, сменяю политикой Windows по умолчанию %s",
		prichina, PolitikaWindowsPoUmolchaniyu)
	if _, err := zamok.Snyat(); err != nil {
		return fmt.Errorf("файл отката не читается (%v), а запрет не снят: %w", prichina, sBfe(err))
	}
	zaperta, err := staryyZamokStoit()
	if err != nil {
		return fmt.Errorf("файл отката не читается (%v), а состояние брандмауэра не прочитано: %w", prichina, err)
	}
	if zaperta {
		if _, err := vypolnit([]string{"advfirewall", "set", "allprofiles", "firewallpolicy", PolitikaWindowsPoUmolchaniyu}); err != nil {
			return fmt.Errorf("файл отката не читается (%v), а политика не возвращена: %w", prichina, err)
		}
	}
	if _, _, err := podmesti(); err != nil {
		return err
	}
	// Нечитаемый файл уходит: оставленный, он повторял бы то же на каждом
	// старте и мешал бы следующему запиранию записать свой.
	return UdalitOtkat()
}

// podmesti снимает правила прежней схемы, ничего не зная о прошлом.
//
// Политику при этом НЕ трогает: без файла отката неизвестно, ставили ли её мы,
// и вернуть чужой Block в Allow значит отключить чужую защиту, приняв её за
// свой мусор.
//
// Снимаются только правила, которые стоят: их имена читаются из реестра одним
// проходом (Г1 аудита 1.8.0). Сверка идёт по точным именам из VseImenaPravil,
// маски нет. Реестр не прочитался: снятие по всему списку, медленно, но без
// пропусков.
func podmesti() (bool, bool, error) {
	est, err := imenaPravilVReestre()
	if err != nil {
		log.Printf("имена правил из реестра не прочитаны, снимаю по полному списку: %v", err)
		return podmestiPoSpisku()
	}
	nashli := false
	var oshibki []error
	for _, imya := range VseImenaPravil() {
		if !est[imya] {
			continue
		}
		nashli = true
		if err := SnyatPravilo(imya); err != nil {
			oshibki = append(oshibki, err)
		}
	}
	return nashli, false, errors.Join(oshibki...)
}

func podmestiPoSpisku() (bool, bool, error) {
	nashli := false
	var oshibki []error
	for _, imya := range VseImenaPravil() {
		// Стояло ли правило, спрашивается ДО снятия и по имени: ответ «сняли»
		// от netsh это опять фраза на языке системы.
		est, _ := PraviloEst(imya)
		if err := SnyatPravilo(imya); err != nil {
			oshibki = append(oshibki, err)
			continue
		}
		if est {
			nashli = true
		}
	}
	return nashli, false, errors.Join(oshibki...)
}

// PerezavestiRazreshyonnyeServery меняет ОДНО разрешение: список серверов.
//
// Зачем отдельно от VklyuchitVesTrafik. Та собирает набор целиком и требует
// адрес TUN-адаптера. При упавшем туннеле адреса нет, пересобрать набор нечем,
// и удалённый сервер сохранял бы разрешение наружу ровно там, где режим и
// включают. Список серверов от туннеля не зависит вовсе.
//
// Запрета нет (выключили, или стоит замок прежней версии): менять нечего, и
// разрешение без запрета осталось бы постоянным мусором в BFE.
func PerezavestiRazreshyonnyeServery(a Adresa) error {
	nabor, err := filtryServerov(a.Vse, a.Porty)
	if err != nil {
		return err
	}
	stoit, err := zamok.Zamenit(klyuchiVida(vidServery), nabor)
	if err != nil {
		return fmt.Errorf("разрешение серверов не переписано: %w", sBfe(err))
	}
	if !stoit {
		log.Printf("запрет не стоит, разрешение серверов переписывать некуда")
	}
	return nil
}

// PerezavestiPravilaDns переписывает разрешение DNS под резолвер, пока машина
// заперта без туннеля (L4 аудита 1.8.0). Разрешение стояло под резолвером
// момента запирания, и после смены сети служба не могла разрешить имена
// серверов: подъём не проходил, и восстановление крутилось взаперти вечно.
func PerezavestiPravilaDns(resolver netip.Addr) error {
	stoit, err := zamok.Zamenit(klyuchiVida(vidDns), filtryDns(resolver))
	if err != nil {
		return fmt.Errorf("разрешение DNS не переписано: %w", sBfe(err))
	}
	if !stoit {
		log.Printf("запрет не стоит, разрешение DNS переписывать некуда")
	}
	return nil
}
