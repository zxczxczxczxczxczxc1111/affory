package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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
// Возвращает СПИСОК ПОЧИНОК, по строке на каждую. Пустой список значит «набор
// уже был в этом виде», и это обычный случай.
//
// Отчёт заведён 20.09.2026 разбором жалобы про подписку, которая возвращалась
// после каждого обновления. Разбор трижды упёрся в одно: все три починки здесь
// МОЛЧАЛИВЫЕ, то есть состав подписок менялся без единой команды и без единой
// строки в журнале, и доказать момент появления записи было физически нечем.
// Чинить вслепую дороже, чем сказать вслух.
func (n *Nabor) PrivestiPodpiski() []string {
	var pochinki []string
	if n.Podpiska != "" {
		if !n.estAdres(n.Podpiska) {
			n.Podpiski = append(n.Podpiski, ZapisPodpiski{
				Id:    IdPodpiski(n.Podpiska),
				Adres: n.Podpiska,
			})
			pochinki = append(pochinki, "старое поле подписки перенесено в список: "+
				UzelPodpiski(n.Podpiska))
		}
		// Гасится сразу: иначе очистка подписки командой воскресала бы записью
		// при следующем же чтении набора, и удалить подписку стало бы нельзя.
		n.Podpiska = ""
	}
	for i := range n.Podpiski {
		if n.Podpiski[i].Id == "" {
			n.Podpiski[i].Id = IdPodpiski(n.Podpiski[i].Adres)
			pochinki = append(pochinki, "подписке без идентификатора дан свой: "+
				UzelPodpiski(n.Podpiski[i].Adres))
		}
	}
	if len(n.Podpiski) == 0 {
		if n.Aktivnaya != "" {
			pochinki = append(pochinki, "активная снята: подписок не осталось")
		}
		n.Aktivnaya = ""
		return pochinki
	}
	// Активная, указывающая в пустоту, это набор из чужого профиля либо след
	// удалённой записи. Молча отдавать первую на каждом чтении нельзя: обновление
	// уходило бы в подписку, которую никто не выбирал, и это было бы невидимо.
	// Чиним ссылку явно и записываем, чтобы дальше все читали одно и то же.
	if n.zapisPodpiski(n.Aktivnaya) == nil {
		prezhnyaya := n.Aktivnaya
		n.Aktivnaya = n.Podpiski[0].Id
		// Ключи при этом НЕ перекладываются: рабочий список остаётся от прежней
		// активной. Сказать об этом обязаны отдельно, потому что на экране такая
		// подписка выглядит выбранной человеком.
		pochinki = append(pochinki, fmt.Sprintf(
			"активная указывала в пустоту (%q), взята первая: %s; ключи НЕ переложены",
			prezhnyaya, UzelPodpiski(n.Podpiski[0].Adres)))
	}
	return pochinki
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
		n.PereklyuchitAktivnuyu(z.Id, false)
		return
	}
	z := ZapisPodpiski{Id: IdPodpiski(adres), Adres: adres}
	n.Podpiski = append(n.Podpiski, z)
	n.PereklyuchitAktivnuyu(z.Id, false)
}

// UbratPodpisku удаляет запись. Активной становится первая из оставшихся, а на
// пустом списке активной не остаётся вовсе.
func (n *Nabor) UbratPodpisku(id string) {
	if n.Aktivnaya == id {
		for _, z := range n.Podpiski {
			if z.Id != id {
				n.PereklyuchitAktivnuyu(z.Id, false)
				break
			}
		}
		if n.Aktivnaya == id {
			var ruchnye []protokol.Server
			for _, srv := range aktualnyeServery(n.Servery) {
				if !srv.IzPodpiski {
					ruchnye = append(ruchnye, srv)
				}
			}
			n.Servery = ruchnye
		}
	}
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
		z.Otkaz = ""
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
		istochnik := z.Servery
		if aktivnaya {
			istochnik = nil
			for _, srv := range n.Servery {
				if srv.IzPodpiski {
					istochnik = append(istochnik, srv)
				}
			}
		}
		stroki := make([]protokol.Server, 0, len(istochnik))
		for _, srv := range aktualnyeServery(istochnik) {
			stroki = append(stroki, dlyaEkrana(srv))
		}
		spisok = append(spisok, map[string]any{
			"servery": stroki,
			"id":      z.Id,
			"uzel":    UzelPodpiski(z.Adres),
			"imya":    z.Imya,
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

	r, serverov, err := s.obnovitPodpiskuPoId(ctx, IdPodpiski(adres))
	if err != nil {
		return otkazPodpiski(k, r, err)
	}
	return otvet(k.Id, k.Imya, map[string]any{"id": IdPodpiski(adres), "aktivnaya": stalaAktivnoy, "serverov": serverov, "otkazy": r.Otkazy})
}

// removeSubscription убирает источник и его серверы из каталога.
// Адреса текущего соединения остаются только в снимке работающего ядра.
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
	adresKlash, _ := s.dostupKKlash()
	nashli := false
	pusto := false
	var zamenyId map[string]string
	if err := s.pravitNabor(func(n *Nabor) error {
		z := n.zapisPodpiski(telo.Id)
		if z == nil {
			return nil
		}
		nashli = true
		pusto = len(z.Servery) == 0
		zamenyId = n.PereklyuchitAktivnuyu(telo.Id, adresKlash != "")
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
		return otvet(k.Id, k.Imya, map[string]any{"aktivnaya": telo.Id, "serverov": len(n.Servery), "server_ids": zamenyId})
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

// PereklyuchitAktivnuyu перекладывает актуальные записи между подписками.
// Снимок работающего ядра хранится в Sluzhba и в кэши подписок не попадает.
// Второй аргумент оставлен для совместимости внутренних вызовов.
func (n *Nabor) PereklyuchitAktivnuyu(id string, _ bool) map[string]string {
	if n.Aktivnaya == id {
		return nil
	}
	novaya := n.zapisPodpiski(id)
	if novaya == nil {
		return nil
	}
	var ruchnye, prezhnie []protokol.Server
	for _, srv := range aktualnyeServery(n.Servery) {
		if srv.IzPodpiski {
			prezhnie = append(prezhnie, srv)
		} else {
			ruchnye = append(ruchnye, srv)
		}
	}
	if p := n.zapisPodpiski(n.Aktivnaya); p != nil {
		p.Servery = prezhnie
	}
	cached := aktualnyeServery(novaya.Servery)
	n.Servery = ssylki.Slit(ruchnye, cached)
	zameny := make(map[string]string)
	for _, old := range cached {
		for _, current := range n.Servery {
			if current.IzPodpiski && current.Id != old.Id && ssylki.TotZheProfil(old, current) {
				zameny[old.Id] = current.Id
				break
			}
		}
	}
	novaya.Servery = nil
	n.Aktivnaya = id
	return zameny
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
	var razbor ssylki.Razbor
	var serverov int
	var otkazAktivnoy error
	for _, z := range n.Podpiski {
		if ctx.Err() != nil {
			return razbor, serverov, ctx.Err()
		}
		r, count, err := s.obnovitPodpiskuPoId(ctx, z.Id)
		if z.Id == n.Aktivnaya {
			razbor, serverov, otkazAktivnoy = r, count, err
		}
	}
	return razbor, serverov, otkazAktivnoy
}

// otmetitOtkazPodpiski кладёт причину в строку подписки.
//
// Отдельной правкой набора, а не внутри обхода: отказ приходит из сети, и
// держать набор запертым на время похода значило бы подвесить любую команду
// человека на чужую панель.
// Вызывается под muNabor после проверки поколения сетевого запроса.
func (s *Sluzhba) otmetitOtkazPodpiski(id, adres string, prichina error) {
	if id == "" || prichina == nil {
		return
	}
	// Меняется только текст ошибки. Пересборка брандмауэра здесь не нужна
	// и задерживала возврат уже завершившегося сетевого запроса.
	n, err := s.nabor()
	if err == nil {
		if z := n.zapisPodpiski(id); z != nil && z.Adres == adres {
			z.Otkaz = prichina.Error()
			err = s.zapisatNabor(n)
		}
	}
	if err != nil {
		log.Printf("причина отказа подписки не записана: %v", err)
	}
}
