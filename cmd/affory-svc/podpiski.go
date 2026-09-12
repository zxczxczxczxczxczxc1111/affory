package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
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
	spisok := make([]map[string]any, 0, len(n.Podpiski))
	for _, z := range n.Podpiski {
		spisok = append(spisok, map[string]any{
			"id":        z.Id,
			"uzel":      UzelPodpiski(z.Adres),
			"imya":      z.Imya,
			"obnovlena": z.Obnovlena,
			"aktivnaya": z.Id == n.Aktivnaya,
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

// setActiveSubscription переключает активную и СРАЗУ тянет её список.
//
// Без немедленного обновления человек переключает подписку, жмёт подключить и
// уходит по ключам прежней панели: список серверов до следующего расписания
// остался бы чужим.
func (s *Sluzhba) setActiveSubscription(ctx context.Context, k protokol.Kadr) protokol.Kadr {
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
		n.Aktivnaya = telo.Id
		return nil
	}); err != nil {
		return otkaz(k.Id, k.Imya, kodSohraneniya(err), err.Error())
	}
	if !nashli {
		return otkaz(k.Id, k.Imya, protokol.KodSubscriptionMalformed, "такой подписки нет")
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
