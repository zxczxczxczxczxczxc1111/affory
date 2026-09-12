package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/ssylki"
)

// Несколько подписок в одном наборе: одна активная, остальные про запас.
//
// Поле `Podpiska` было одно, и вторая подписка в клиенте не жила: задал новый
// адрес, потерял прежний. Панели меняются, у запасной свой адрес, и держать его
// в блокноте рядом с клиентом означает, что клиент половину работы не делает.
//
// Активная РОВНО одна намеренно. Сливать серверы двух панелей в один список
// значит сложить вместе два источника с одинаковыми именами узлов и разными
// ключами, и человек не сможет сказать, чей сервер перед ним. Запасная лежит
// адресом, а не списком: её ключи приедут, когда она станет активной.

// ZapisPodpiski это одна подписка набора.
type ZapisPodpiski struct {
	// Id считается от адреса. Стабилен между запусками, наружу отдаётся вместо
	// адреса и не обратим в него.
	Id string `json:"id"`
	// Adres это секрет класса ключа: на экран уезжает только узел.
	Adres string `json:"adres"`
	// Imya задаётся человеком. Пустое значит «показывать узел»: две подписки
	// одной панели без имён различаются только хвостом пути, а его не покажешь.
	Imya string `json:"imya,omitempty"`
	// Obnovlena это отметка ПОСЛЕДНЕГО удачного обновления именно этой записи.
	// Общая отметка в состоянии службы одна на всех и после переключения врёт:
	// говорит про свежесть чужой подписки.
	Obnovlena *time.Time `json:"obnovlena,omitempty"`
	// Otkaz это причина последней НЕУДАЧИ обновления именно этой записи. Пустая
	// строка значит «прошлый обход удался»: без отдельного поля просроченная
	// запасная панель выглядела бы на экране так же, как живая.
	Otkaz string `json:"otkaz,omitempty"`
	// Servery это ключи ЭТОЙ подписки, сложенные обходом расписания.
	//
	// Хранятся у записи, а не в общем списке набора, и это главное решение всей
	// затеи. Общий список читают ядро, экран, killswitch и замеры; свались туда
	// ключи трёх панелей разом, каждое из этих мест пришлось бы учить
	// фильтровать, а забытое место означало бы чужой сервер в конфиге ядра.
	// Здесь же переключение это перекладка двух списков в одной функции.
	//
	// У АКТИВНОЙ записи поле пустое: её ключи лежат в Nabor.Servery, потому что
	// с ними работают. Две копии одного списка разъехались бы в первый же день.
	Servery []protokol.Server `json:"servery,omitempty"`
}

// IdPodpiski считает устойчивое имя подписки от её адреса.
//
// От адреса целиком, а не от узла: у одной панели бывает две подписки, и
// различаются они как раз хвостом пути. Хеш, а не кусок адреса: идентификатор
// уезжает на экран и в журнал команд, а адрес это пропуск к ключам.
func IdPodpiski(adres string) string {
	if adres == "" {
		return ""
	}
	h := sha256.Sum256([]byte(adres))
	return hex.EncodeToString(h[:6])
}

// PrivestiPodpiski чинит набор до вида, в котором его читают остальные.
//
// Зовётся на КАЖДОМ чтении набора, поэтому обязана быть идемпотентной: иначе
// список подписок растёт от одного только просмотра списка серверов.
//
// Делает ровно три вещи: переносит старое одиночное поле в список, дописывает
// пропавшие идентификаторы и чинит ссылку на активную. Набор без подписок
// остаётся пустым: запись с пустым адресом означала бы «подписка задана» на
// первом же запуске.
func (n *Nabor) PrivestiPodpiski() {
	if n.Podpiska != "" {
		if !n.estAdres(n.Podpiska) {
			n.Podpiski = append(n.Podpiski, ZapisPodpiski{
				Id:    IdPodpiski(n.Podpiska),
				Adres: n.Podpiska,
			})
		}
		// Гасится сразу: иначе очистка подписки командой воскресала бы записью
		// при следующем же чтении набора, и удалить подписку стало бы нельзя.
		n.Podpiska = ""
	}
	for i := range n.Podpiski {
		if n.Podpiski[i].Id == "" {
			n.Podpiski[i].Id = IdPodpiski(n.Podpiski[i].Adres)
		}
	}
	if len(n.Podpiski) == 0 {
		n.Aktivnaya = ""
		return
	}
	// Активная, указывающая в пустоту, это набор из чужого профиля либо след
	// удалённой записи. Молча отдавать первую на каждом чтении нельзя: обновление
	// уходило бы в подписку, которую никто не выбирал, и это было бы невидимо.
	// Чиним ссылку явно и записываем, чтобы дальше все читали одно и то же.
	if n.zapisPodpiski(n.Aktivnaya) == nil {
		n.Aktivnaya = n.Podpiski[0].Id
	}
}

func (n *Nabor) estAdres(adres string) bool {
	for _, z := range n.Podpiski {
		if z.Adres == adres {
			return true
		}
	}
	return false
}

// zapisPodpiski отдаёт запись по идентификатору, либо nil.
func (n *Nabor) zapisPodpiski(id string) *ZapisPodpiski {
	if id == "" {
		return nil
	}
	for i := range n.Podpiski {
		if n.Podpiski[i].Id == id {
			return &n.Podpiski[i]
		}
	}
	return nil
}

// AdresAktivnoy отдаёт адрес подписки, которую обновляем.
//
// Единственная дверь к адресу для всей службы: пока обновление читало поле
// `Podpiska` напрямую, переключение активной ничего бы не меняло.
func (n *Nabor) AdresAktivnoy() string {
	if z := n.zapisPodpiski(n.Aktivnaya); z != nil {
		return z.Adres
	}
	return ""
}

// UzelPodpiski это то, что от адреса МОЖНО показать человеку.
func UzelPodpiski(adres string) string {
	if adres == "" {
		return ""
	}
	u, err := url.Parse(adres)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// ZadatAktivnuyu кладёт адрес в список и делает его активным.
//
// Тот же адрес не плодит двойника: идентификатор считается от адреса, и
// повторный ввод это «сделать активной ту же самую». Пустой адрес удаляет
// активную запись, активной становится первая из оставшихся.
func (n *Nabor) ZadatAktivnuyu(adres string) {
	if adres == "" {
		n.UbratPodpisku(n.Aktivnaya)
		return
	}
	if z := n.zapisPoAdresu(adres); z != nil {
		n.Aktivnaya = z.Id
		return
	}
	z := ZapisPodpiski{Id: IdPodpiski(adres), Adres: adres}
	n.Podpiski = append(n.Podpiski, z)
	n.Aktivnaya = z.Id
}

// UbratPodpisku удаляет запись. Активной становится первая из оставшихся, а на
// пустом списке активной не остаётся вовсе.
func (n *Nabor) UbratPodpisku(id string) {
	ostatok := make([]ZapisPodpiski, 0, len(n.Podpiski))
	for _, z := range n.Podpiski {
		if z.Id != id {
			ostatok = append(ostatok, z)
		}
	}
	n.Podpiski = ostatok
	if n.Aktivnaya == id {
		n.Aktivnaya = ""
	}
	n.PrivestiPodpiski()
}

func (n *Nabor) zapisPoAdresu(adres string) *ZapisPodpiski {
	for i := range n.Podpiski {
		if n.Podpiski[i].Adres == adres {
			return &n.Podpiski[i]
		}
	}
	return nil
}

// OtmetitObnovlenie ставит запись отметку времени последнего удачного обновления.
func (n *Nabor) OtmetitObnovlenie(id string, kogda time.Time) {
	if z := n.zapisPodpiski(id); z != nil {
		kopiya := kogda
		z.Obnovlena = &kopiya
	}
}

// Четыре команды экрана подписок. Отдельно от setSubscription, которая осталась
// дверью «задать подписку одним действием» и живёт в komandy_serverov.go.

// listSubscriptions отдаёт список для экрана: идентификаторы, узлы, отметки.
// Адресов здесь нет и быть не может: канал пускает интерактивного пользователя,
// а адрес подписки это пропуск к ключам.
func (s *Sluzhba) listSubscriptions(k protokol.Kadr) protokol.Kadr {
	n, err := s.nabor()
	if err != nil {
		return otkaz(k.Id, k.Imya, protokol.KodSecretsUnreadable, err.Error())
	}
	// Ключи активной лежат в рабочем списке, а не в её записи, поэтому её
	// серверы считаются оттуда. Одно число в двух местах разъехалось бы.
	vRabochem := 0
	for _, srv := range n.Servery {
		if srv.IzPodpiski {
			vRabochem++
		}
	}
	spisok := make([]map[string]any, 0, len(n.Podpiski))
	for _, z := range n.Podpiski {
		aktivnaya := z.Id == n.Aktivnaya
		serverov := len(z.Servery)
		if aktivnaya {
			serverov = vRabochem
		}
		spisok = append(spisok, map[string]any{
			"id":   z.Id,
			"uzel": UzelPodpiski(z.Adres),
			"imya": z.Imya,
			// Число ключей и причина отказа: без них живая запасная и
			// просроченная выглядят на экране одинаково, а узнать разницу можно
			// было бы только переключившись.
			"serverov":  serverov,
			"otkaz":     z.Otkaz,
			"obnovlena": z.Obnovlena,
			"aktivnaya": aktivnaya,
		})
	}
	return otvet(k.Id, k.Imya, map[string]any{"podpiski": spisok})
}

// addSubscription кладёт подписку про запас.
//
// Активной она становится только если активной ещё нет: иначе добавление
// адреса молча уводило бы список серверов на другую панель, а человек нажимал
// «добавить», а не «переключиться». Переключение это отдельное действие.
func (s *Sluzhba) addSubscription(ctx context.Context, k protokol.Kadr) protokol.Kadr {
	var telo struct {
		Adres string `json:"adres"`
		Imya  string `json:"imya"`
	}
	if err := json.Unmarshal(k.Telo, &telo); err != nil {
		return otkaz(k.Id, k.Imya, protokol.KodProtocolMismatch, "тело команды не разбирается")
	}
	adres := strings.TrimSpace(telo.Adres)
	if oshib := proveritAdresPodpiski(adres); oshib != "" {
		return otkaz(k.Id, k.Imya, protokol.KodSubscriptionMalformed, oshib)
	}

	stalaAktivnoy := false
	if err := s.pravitNabor(func(n *Nabor) error {
		if len(n.Podpiski) >= PredelPodpisok && n.zapisPoAdresu(adres) == nil {
			return errSlishkomMnogoPodpisok
		}
		if n.Aktivnaya == "" {
			n.ZadatAktivnuyu(adres)
			stalaAktivnoy = true
		} else if n.zapisPoAdresu(adres) == nil {
			n.Podpiski = append(n.Podpiski, ZapisPodpiski{
				Id: IdPodpiski(adres), Adres: adres, Imya: strings.TrimSpace(telo.Imya),
			})
		}
		return nil
	}); err != nil {
		if errors.Is(err, errSlishkomMnogoPodpisok) {
			return otkaz(k.Id, k.Imya, protokol.KodSubscriptionMalformed, err.Error())
		}
		return otkaz(k.Id, k.Imya, kodSohraneniya(err), err.Error())
	}

	// Список серверов тянется ТОЛЬКО если подписка стала активной: запасная
	// лежит адресом, её ключи приедут при переключении.
	if !stalaAktivnoy {
		return otvet(k.Id, k.Imya, map[string]any{"id": IdPodpiski(adres), "aktivnaya": false})
	}
	r, serverov, err := s.obnovitPodpisku(ctx)
	if err != nil {
		return otkazPodpiski(k, r, err)
	}
	return otvet(k.Id, k.Imya, map[string]any{
		"id": IdPodpiski(adres), "aktivnaya": true, "serverov": serverov, "otkazy": r.Otkazy,
	})
}

// removeSubscription убирает запись. Серверы удалённой подписки уходят при
// следующем обновлении активной, а пока ядро поднято, они держатся тем же
// правилом, что и пропавшие из публикации.
func (s *Sluzhba) removeSubscription(k protokol.Kadr) protokol.Kadr {
	var telo struct {
		Id string `json:"id"`
	}
	if err := json.Unmarshal(k.Telo, &telo); err != nil {
		return otkaz(k.Id, k.Imya, protokol.KodProtocolMismatch, "тело команды не разбирается")
	}
	nashli := false
	if err := s.pravitNabor(func(n *Nabor) error {
		if n.zapisPodpiski(telo.Id) == nil {
			return nil
		}
		nashli = true
		n.UbratPodpisku(telo.Id)
		return nil
	}); err != nil {
		return otkaz(k.Id, k.Imya, kodSohraneniya(err), err.Error())
	}
	if !nashli {
		return otkaz(k.Id, k.Imya, protokol.KodSubscriptionMalformed, "такой подписки нет")
	}
	return otvet(k.Id, k.Imya, map[string]any{"udalena": telo.Id})
}

// setActiveSubscription переключает активную подписку.
//
// Ключи берутся ИЗ ЗАПИСИ: их привёз обход расписания, и в сеть идти незачем.
// Поэтому переключение мгновенно и работает, когда панель молчит. В сеть
// команда идёт ровно в одном случае: подписку ещё ни разу не обошли, и список
// её ключей пуст.
func (s *Sluzhba) setActiveSubscription(ctx context.Context, k protokol.Kadr) protokol.Kadr {
	var telo struct {
		Id string `json:"id"`
	}
	if err := json.Unmarshal(k.Telo, &telo); err != nil {
		return otkaz(k.Id, k.Imya, protokol.KodProtocolMismatch, "тело команды не разбирается")
	}
	// Живое ядро значит, что его конфиг собран при подъёме и знает теги прежней
	// подписки. Её ключи тогда остаются в списке пометкой удержания и уходят
	// сами при переподключении, иначе мы отдали бы экрану список, которого ядро
	// не видит, и упёрлись в заслон против потери живых.
	adresKlash, _ := s.dostupKKlash()
	nashli := false
	pusto := false
	if err := s.pravitNabor(func(n *Nabor) error {
		z := n.zapisPodpiski(telo.Id)
		if z == nil {
			return nil
		}
		nashli = true
		pusto = len(z.Servery) == 0
		n.PereklyuchitAktivnuyu(telo.Id, adresKlash != "")
		return nil
	}); err != nil {
		return otkaz(k.Id, k.Imya, kodSohraneniya(err), err.Error())
	}
	if !nashli {
		return otkaz(k.Id, k.Imya, protokol.KodSubscriptionMalformed, "такой подписки нет")
	}
	if !pusto {
		n, err := s.nabor()
		if err != nil {
			return otkaz(k.Id, k.Imya, protokol.KodSecretsUnreadable, err.Error())
		}
		return otvet(k.Id, k.Imya, map[string]any{"aktivnaya": telo.Id, "serverov": len(n.Servery)})
	}
	r, serverov, err := s.obnovitPodpisku(ctx)
	if err != nil {
		return otkazPodpiski(k, r, err)
	}
	return otvet(k.Id, k.Imya, map[string]any{
		"aktivnaya": telo.Id, "serverov": serverov, "otkazy": r.Otkazy,
	})
}

// PredelPodpisok это потолок списка.
//
// Не ради памяти: каждая подписка это адрес в списке разрешённых при запертом
// режиме, то есть дырка мимо туннеля. Десяток таких дырок заводится незаметно,
// а убирается руками по одной.
const PredelPodpisok = 10

var errSlishkomMnogoPodpisok = errors.New("подписок уже десять, больше не поместится")

// proveritAdresPodpiski отвечает текстом отказа, либо пустой строкой.
//
// Текст строим сами: err.Error() у url.Parse несёт САМ адрес, а он секрет.
func proveritAdresPodpiski(adres string) string {
	if adres == "" {
		return "адрес подписки пуст"
	}
	u, err := url.Parse(adres)
	if err != nil || u.Host == "" {
		return "адрес подписки не разобран"
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return "поддерживаются только http и https"
	}
	return ""
}

// PereklyuchitAktivnuyu перекладывает ключи между записью и рабочим списком.
//
// Ключи активной подписки живут в Nabor.Servery, ключи остальных в их записях.
// Переключение это ровно перекладка: прежние уезжают в свою запись, новые
// приезжают оттуда. В сеть при этом ходить незачем, их уже привёз обход.
//
// uderzhat означает «ядро поднято»: тогда прежние ключи ОСТАЮТСЯ в рабочем
// списке пометкой удержания. Конфиг ядра собран при подъёме и знает их теги;
// вычеркнуть их из набора значило бы отдать экрану список, которого ядро не
// видит, и упереться в заслон против потери живых.
func (n *Nabor) PereklyuchitAktivnuyu(id string, uderzhat bool) {
	if n.Aktivnaya == id {
		return
	}
	prezhnyaya := n.zapisPodpiski(n.Aktivnaya)
	novaya := n.zapisPodpiski(id)
	if novaya == nil {
		return
	}

	ostavshiesya := make([]protokol.Server, 0, len(n.Servery))
	uehavshie := make([]protokol.Server, 0, len(n.Servery))
	for _, srv := range n.Servery {
		if !srv.IzPodpiski && !srv.Uderzhan {
			// Ручной сервер не принадлежит подписке и переключения не замечает.
			ostavshiesya = append(ostavshiesya, srv)
			continue
		}
		uehavshie = append(uehavshie, srv)
		if uderzhat {
			srv.IzPodpiski = false
			srv.Uderzhan = true
			ostavshiesya = append(ostavshiesya, srv)
		}
	}
	if prezhnyaya != nil {
		// В запись едут ключи КАК БЫЛИ, с пометкой «из подписки»: удержание это
		// свойство рабочего списка, а не запаса.
		for i := range uehavshie {
			uehavshie[i].Uderzhan = false
			uehavshie[i].IzPodpiski = true
		}
		prezhnyaya.Servery = uehavshie
	}

	zanyato := make(map[string]bool, len(ostavshiesya))
	for _, srv := range ostavshiesya {
		zanyato[srv.Id] = true
	}
	for _, srv := range novaya.Servery {
		// Один и тот же адрес с портом и транспортом у двух панелей даёт один
		// идентификатор. Второй экземпляр не кладём: два исходящих с одним тегом
		// это конфиг, который ядро не соберёт.
		if zanyato[srv.Id] {
			continue
		}
		ostavshiesya = append(ostavshiesya, srv)
	}
	n.Servery = ostavshiesya
	novaya.Servery = nil
	n.Aktivnaya = id
}

// obnovitVsePodpiski обходит ВСЕ добавленные подписки, а не только активную.
//
// Запасная, чьи ключи приезжают раз в 12 часов вместе с остальными, делает
// переключение мгновенным и работающим при молчащей панели. Пока тянулась одна
// активная, «подписка про запас» означала «адрес про запас», то есть половину
// обещания.
//
// Отказ ОДНОЙ подписки не отменяет обход остальных и записывается в её строку:
// иначе одна просроченная панель молча останавливала бы обновление всех.
func (s *Sluzhba) obnovitVsePodpiski(ctx context.Context) (ssylki.Razbor, int, error) {
	n, err := s.nabor()
	if err != nil {
		return ssylki.Razbor{}, 0, oshibkaNabora{err}
	}
	if len(n.Podpiski) == 0 {
		return ssylki.Razbor{}, 0, errPodpiskaNeZadana
	}

	// Активная идёт прежним путём: её ключи лежат в рабочем списке, их надо
	// слить, удержать живых и сохранить одной правкой набора.
	aktivnaya := n.Aktivnaya
	// Отказ АКТИВНОЙ это отказ обхода: её список человек видит на экране, и
	// молчать о нём значит показывать вчерашние ключи как сегодняшние.
	var otkazAktivnoy error
	razbor, serverov, err := s.obnovitPodpisku(ctx)
	if err != nil {
		otkazAktivnoy = err
		s.otmetitOtkazPodpiski(aktivnaya, err)
	}

	for _, z := range n.Podpiski {
		if z.Id == aktivnaya {
			continue
		}
		r, err := s.zagruzitPodpisku(ctx, z.Adres)
		if err != nil {
			// Отказ ЗАПАСНОЙ не поднимается наверх: команду вызвал человек ради
			// активной, и просроченная запасная панель не повод объявить всё
			// обновление неудавшимся. Причина лежит в строке подписки, там её и
			// читают.
			s.otmetitOtkazPodpiski(z.Id, err)
			continue
		}
		teper := s.seychas()
		id := z.Id
		if err := s.pravitNabor(func(n *Nabor) error {
			zapis := n.zapisPodpiski(id)
			if zapis == nil {
				// Запись удалили, пока шла загрузка: класть ключи некуда, и это
				// не отказ, а гонка с человеком, которую выиграл человек.
				return nil
			}
			zapis.Servery = ssylki.Slit(zapis.Servery, r.Servery)
			zapis.Obnovlena = &teper
			zapis.Otkaz = ""
			return nil
		}); err != nil {
			log.Printf("ключи подписки не сохранены: %v", err)
		}
	}
	return razbor, serverov, otkazAktivnoy
}

// otmetitOtkazPodpiski кладёт причину в строку подписки.
//
// Отдельной правкой набора, а не внутри обхода: отказ приходит из сети, и
// держать набор запертым на время похода значило бы подвесить любую команду
// человека на чужую панель.
func (s *Sluzhba) otmetitOtkazPodpiski(id string, prichina error) {
	if id == "" || prichina == nil {
		return
	}
	if err := s.pravitNabor(func(n *Nabor) error {
		if z := n.zapisPodpiski(id); z != nil {
			z.Otkaz = prichina.Error()
		}
		return nil
	}); err != nil {
		log.Printf("причина отказа подписки не записана: %v", err)
	}
}
