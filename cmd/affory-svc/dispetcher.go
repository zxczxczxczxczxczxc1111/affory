package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/yadra"
)

// Every command of the protocol, including the ones nobody has written yet. The
// shape and the name are fixed HERE, before the interface leans on them, rather
// than in wave 4 under deadline pressure. A command that will exist later and
// answers "not yet, wave 3" today costs nothing; a command that does not exist
// and hangs costs an evening.
var pozzhe = map[string]int{}

func otvet(id uint64, imya string, telo any) protokol.Kadr {
	k := protokol.Kadr{Tip: "otvet", Id: id, Imya: imya}
	if telo != nil {
		if b, err := json.Marshal(telo); err == nil {
			k.Telo = b
		}
	}
	return k
}

func otkaz(id uint64, imya, kod, tekst string) protokol.Kadr {
	return protokol.Kadr{Tip: "otvet", Id: id, Imya: imya,
		Oshib: &protokol.Oshibka{Kod: kod, Tekst: tekst}}
}

// Obrabotat never panics and never blocks forever. The half that panics here is
// the half holding the firewall rules, and a machine with rules and no service
// has no internet at all.
// komandyDlyaAdmina это ГРАНИЦА привилегий, и она одна на всю службу.
//
// Решено 03.09.2026. ОТМЕНЯЕТ прежнюю границу от 01.09.2026, где
// админа требовали десять команд. Администратор остаётся ровно там, где
// действие меняет машину целиком или выносит секреты за её пределы:
//
//   - setKillSwitch правит правила брандмауэра всей машины;
//   - installUpdate и downloadUpdate подменяют файлы программы;
//   - exportProfile и importProfile выносят наружу и заносят внутрь ключи.
//
// Всё остальное это работа с собственным набором серверов на собственной
// машине: добавить сервер, сменить подписку, выбрать сервер, поправить правила
// и автозапуск. Прежняя граница требовала UAC на добавление сервера, то есть на
// первое же действие после установки, и упиралась в тупик у человека без прав
// администратора вовсе. Программа, которая на первом шаге просит того, чего у
// человека нет, это программа, которую удаляют.
//
// Довод «другая программа от тебя» при этом не отброшен, он переехал: канал
// остаётся под INTERACTIVE, и чужой процесс по-прежнему не запрёт машину, не
// подменит файлы и не унесёт профиль. Увести трафик своим сервером он теперь
// может, и это принятая цена за программу, которой пользуются.
//
// Таблица, а не проверка в каждом обработчике: проверок было две на двенадцать
// команд, и заметить это чтением не удалось никому. Полноту таблицы стережёт
// TestKazhdayaKomandaImeetResheniyeOPravah, который берёт имена разбором
// диспетчера, а не рукописным списком.
var komandyDlyaAdmina = map[string]bool{
	"setKillSwitch":  true,
	"exportProfile":  true,
	"importProfile":  true,
	"installUpdate":  true,
	"downloadUpdate": true,
}

func (s *Sluzhba) Obrabotat(ctx context.Context, k protokol.Kadr) protokol.Kadr {
	otv := s.obrabotat(ctx, k)
	s.zapisatKomandu(ctx, k, otv)
	return otv
}

func (s *Sluzhba) obrabotat(ctx context.Context, k protokol.Kadr) protokol.Kadr {
	if komandyDlyaAdmina[k.Imya] {
		if o := s.trebuetAdmina(ctx, k); o != nil {
			return *o
		}
	}

	switch k.Imya {
	case "hello":
		// Сверка ДВУСТОРОННЯЯ. Прежде служба свою версию объявляла, а чужую не
		// читала вовсе: старый интерфейс против новой службы получал зелёное
		// приветствие и падал позже, на поле, которого не знает, то есть ровно
		// в том месте, где сверка должна была его остановить.
		//
		// Пустое тело тоже отказ: клиент, не назвавший версию, ничем не лучше
		// назвавшего чужую, а трактовать молчание в свою пользу это способ
		// раздать совместимость по недосмотру.
		var privet struct {
			Protocol int `json:"protocol"`
		}
		if err := json.Unmarshal(k.Telo, &privet); err != nil {
			return otkaz(k.Id, k.Imya, protokol.KodProtocolMismatch,
				"приветствие не разбирается")
		}
		if privet.Protocol != protokol.Versiya {
			return otkaz(k.Id, k.Imya, protokol.KodProtocolMismatch,
				fmt.Sprintf("интерфейс говорит на версии %d, служба на %d",
					privet.Protocol, protokol.Versiya))
		}
		// Отложенные команды с номерами волн (задача 4.10): экран над такой
		// командой показывает волну из ЭТОГО ответа, а не пробует запись.
		return otvet(k.Id, k.Imya, map[string]any{"protocol": protokol.Versiya, "pozzhe": pozzhe})

	case "status":
		// Исход обновления появляется ПОСЛЕ старта новой службы: подменщик
		// пишет его, дождавшись ответа. Поэтому забирается здесь, а не на старте.
		s.pokazatItogObnovleniya()
		return otvet(k.Id, k.Imya, s.Status())

	case "subscribeStats":
		return s.subscribeStats(ctx, k)

	case "setJournal":
		return s.setJournal(k)
	case "setDiagnostics":
		return s.setDiagnostics(k)

	case "clearJournal":
		return s.clearJournal(k)

	case "installUpdate":
		return s.installUpdate(k)

	case "checkUpdate":
		return s.checkUpdate(ctx, k)

	case "downloadUpdate":
		return s.downloadUpdate(ctx, k)

	case "getServerHealth":
		return s.getServerHealth(ctx, k)

	case "measureDelays":
		return s.measureDelays(ctx, k)

	case "measureBandwidth":
		return s.measureBandwidth(ctx, k)
	case "startSpeedTest":
		return s.startSpeedTest(k)
	case "speedTestStatus":
		return s.speedTestStatus(k)
	case "cancelSpeedTest":
		return s.cancelSpeedTest(k)

	case "checkExitIp":
		return s.checkExitIp(ctx, k)

	case "checkLeaks":
		return s.checkLeaks(ctx, k)

	case "setKillSwitch":
		var telo struct {
			Vkl bool `json:"vkl"`
		}
		if len(k.Telo) > 0 {
			if err := json.Unmarshal(k.Telo, &telo); err != nil {
				return otkaz(k.Id, k.Imya, protokol.KodProtocolMismatch, err.Error())
			}
		}
		if err := s.SetKillSwitch(telo.Vkl); err != nil {
			// kodRezhima, а не firewall-failed насмерть: молчащий резолвер и
			// выключенный брандмауэр это разные причины с разными экранами.
			return otkaz(k.Id, k.Imya, kodRezhima(err), err.Error())
		}
		return otvet(k.Id, k.Imya, s.Status())

	case "setAutostart":
		var telo struct {
			Vkl bool `json:"vkl"`
		}
		if len(k.Telo) > 0 {
			if err := json.Unmarshal(k.Telo, &telo); err != nil {
				return otkaz(k.Id, k.Imya, protokol.KodProtocolMismatch, err.Error())
			}
		}
		// Interface autostart, not the tunnel. The registry is the only
		// truth, so there is no field to update here: Status() reads it.
		var err error
		if telo.Vkl {
			err = vklyuchitAvtozapusk(putInterfeysa())
		} else {
			err = vyklyuchitAvtozapusk()
		}
		if err != nil {
			// НЕ admin-required. Ключ пишет СЛУЖБА, а она LocalSystem: прав
			// вызывающего тут нет вовсе (ustanovka.go, avtozapusk.go открывает
			// HKLM сам). Прежний код вёл на кнопку «Повторить от
			// администратора», то есть на прогон UAC, после которого отказ
			// повторяется слово в слово: тупик из тех, что учат не верить
			// запросам прав вообще.
			//
			// kodSohraneniya, потому что это ровно тот же класс, что и отказ
			// записи набора: настройка не легла туда, где она живёт. Своего кода
			// у «настройка не записалась» в §9.1 нет, а заводить его в этой
			// задаче нельзя, коды это отдельная полоса.
			return otkaz(k.Id, k.Imya, kodSohraneniya(err), err.Error())
		}
		return otvet(k.Id, k.Imya, s.Status())

	case "setConnectOnStart":
		var telo struct {
			Vkl bool `json:"vkl"`
		}
		if len(k.Telo) > 0 {
			if err := json.Unmarshal(k.Telo, &telo); err != nil {
				return otkaz(k.Id, k.Imya, protokol.KodProtocolMismatch, err.Error())
			}
		}
		// The tunnel, not the interface: the service acts on this at boot.
		//
		// Отказ записи ДОЕЗЖАЕТ до человека. Флаг живёт только в файле, и
		// успех при непрошедшей записи это галочка на экране и невыполненное
		// обещание поднять туннель при следующем старте.
		if err := s.SetConnectOnStart(telo.Vkl); err != nil {
			return otkaz(k.Id, k.Imya, kodSohraneniya(err), err.Error())
		}
		return otvet(k.Id, k.Imya, s.Status())

	case "setBandwidth":
		var telo struct {
			Vverh int `json:"vverh"`
			Vniz  int `json:"vniz"`
		}
		if len(k.Telo) > 0 {
			if err := json.Unmarshal(k.Telo, &telo); err != nil {
				return otkaz(k.Id, k.Imya, protokol.KodProtocolMismatch, err.Error())
			}
		}
		// Негодная пара это отказ ЗДЕСЬ, а не молчание. Полоса влияет только на
		// следующий подъём, поэтому тихо проглоченное значение человек заметит
		// не раньше, чем по проваленным замерам, и связать одно с другим уже не
		// сможет.
		//
		// kodSohraneniya по тому же доводу, что у соседних настроек: своего кода
		// у «настройка не записалась» в §9.1 нет.
		if err := s.SetPolosa(telo.Vverh, telo.Vniz); err != nil {
			return otkaz(k.Id, k.Imya, kodSohraneniya(err), err.Error())
		}
		return otvet(k.Id, k.Imya, s.Status())

	case "connect":
		var telo struct {
			Server string `json:"server,omitempty"`
		}
		if len(k.Telo) > 0 {
			_ = json.Unmarshal(k.Telo, &telo)
		}
		// Ядро ЖИВО, значит connect --server это ПЕРЕКЛЮЧЕНИЕ, а не подъём.
		//
		// Идемпотентный Connect отвечает на поднятом туннеле успехом, ничего не
		// подняв, и записанный перед ним выбор оставался обещанием: набор
		// говорил «Германия», трафик шёл через Нидерланды, а человеку было
		// сказано «готово». Откат полосы Б живёт только в ветке отказа, а
		// отказа тут и не было. Разъезд нашли два разбора независимо, судит его
		// строка 2 приёмки proverit-otvety-sostoyaniy.ps1.
		//
		// Делегируем в setServer, а не отказываем: «подключись к X» при живом
		// туннеле означает ровно то же, что «переключись на X», и у setServer
		// для этого есть и проба, и откат на прежний сервер.
		if telo.Server != "" {
			if adres, _ := s.dostupKKlash(); adres != "" {
				if err := s.setServer(ctx, telo.Server); err != nil {
					return otkaz(k.Id, k.Imya, kodPereklyucheniya(err), err.Error())
				}
				return otvet(k.Id, k.Imya, s.Status())
			}
			// sostoyanie-a-ne-yadro: вопрос здесь НЕ «живо ли ядро», а «идёт ли
			// подъём». Идущий подъём уже прочитал набор и понесёт прежний
			// сервер, а Connect ответит на него успехом по идемпотентности.
			// Записать выбор и сказать «готово» значит соврать с отсрочкой на
			// те четыре секунды, что осталось подниматься.
			if s.Status().Sostoyanie == protokol.SostPodnimaetsya {
				return otkaz(k.Id, k.Imya, protokol.KodPereklyuchenieNeDoehalo,
					"подъём уже идёт: выбери сервер, когда он закончится")
			}
		}

		// Выбор делается ДО подъёма и отдельной ошибкой: несуществующий
		// идентификатор иначе привёл бы к подъёму первого попавшегося сервера,
		// то есть к молчаливой подмене того, что человек выбрал.
		//
		// zapomnitVybor, а НЕ setServer: ядра здесь ещё нет, и поход в него
		// увёл бы трафик живого туннеля на другой сервер прямо перед подъёмом.
		//
		// Запись остаётся ДО подъёма, а откат делается ПОСЛЕ отказа. Перенести
		// саму запись за Connect нельзя: Connect берёт сервер из n.Vybran и
		// параметра не имеет, поэтому connect --server de поднял бы тот сервер,
		// что был выбран прежде, то есть ту самую молчаливую подмену, о которой
		// абзац выше. Откат стоит ровно столько же для человека и не трогает
		// границу с полосой В.
		vernut := func() {}
		if telo.Server != "" {
			doKomandy, err := s.nabor()
			if err != nil {
				return otkaz(k.Id, k.Imya, protokol.KodSecretsUnreadable, err.Error())
			}
			if err := s.zapomnitVybor(telo.Server); err != nil {
				return otkaz(k.Id, k.Imya, protokol.KodSelectedServerGone, err.Error())
			}
			vybranDo, rezhimDo := doKomandy.Vybran, doKomandy.Rezhim
			vernut = func() {
				if err := s.vernutVyborNazad(vybranDo, rezhimDo); err != nil {
					// Молчать нельзя: набор остался с чужим выбором, и следующий
					// подъём поднимет не тот сервер. Показать это человеку нечем,
					// отказ у команды уже свой и он о подъёме.
					log.Printf("выбор сервера не откачен после отказа подъёма: %v", err)
				}
			}
		}
		if err := s.Connect(ctx); err != nil {
			// Отказавшая команда не оставляет следов на диске. zapomnitVybor
			// пишет набор, ставит ручной режим и пересобирает правила
			// брандмауэра, и до сегодня всё это переживало отказ подъёма:
			// человек, у которого не поднялось, оставался с чужим выбранным
			// сервером и сменённым режимом.
			//
			// Отмена тоже откатывается. Она отвечает успехом (состояние), но
			// подключения не случилось, и запоминать выбор не за что.
			vernut()
			// Отмена это НЕ отказ подключения. Человек нажал «отключить» или
			// закрыл соединение, и подъём проиграл его собственной следующей
			// команде: это ровно то, чего он хотел. Отвечать отказом здесь
			// значит выдумать диагноз на ровном месте, а выдумывался он самый
			// пугающий: цикл проб возвращает ctx.Err(), Disconnect по пути
			// зануляет s.oshib, и человек получал all-servers-down с текстом
			// "context canceled".
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return otvet(k.Id, k.Imya, s.Status())
			}
			return otkaz(k.Id, k.Imya, kodPodklyucheniya(err, s.Status().Oshib), err.Error())
		}
		return otvet(k.Id, k.Imya, s.Status())

	case "setServer":
		// Тело разбирается ЗДЕСЬ, как во всех соседних ветках: общего разбора у
		// диспетчера нет.
		var telo struct {
			Id string `json:"id"`
		}
		if err := json.Unmarshal(k.Telo, &telo); err != nil {
			return otkaz(k.Id, k.Imya, protokol.KodProtocolMismatch, "тело команды не разбирается")
		}
		if err := s.setServer(ctx, telo.Id); err != nil {
			return otkaz(k.Id, k.Imya, kodPereklyucheniya(err), err.Error())
		}
		return otvet(k.Id, k.Imya, s.Status())

	case "setRouteMode":
		var telo struct {
			Rezhim protokol.Rezhim `json:"rezhim"`
		}
		if err := json.Unmarshal(k.Telo, &telo); err != nil {
			return otkaz(k.Id, k.Imya, protokol.KodProtocolMismatch, "тело команды не разбирается")
		}
		trebuetPodyoma, err := s.setRouteMode(ctx, telo.Rezhim)
		if err != nil {
			if errors.Is(err, ErrChuzhoyRezhim) {
				return otkaz(k.Id, k.Imya, protokol.KodProtocolMismatch, err.Error())
			}
			return otkaz(k.Id, k.Imya, kodSmenyMarshruta(err), err.Error())
		}
		// Признак приходит ОТ КОМАНДЫ, а не считается здесь по живому ядру.
		// Прежний счёт (adresYadra != "") отвечал «нужен переподъём» ровно
		// тогда, когда ядро живо, то есть ровно тогда, когда переключить можно
		// на лету: человека посылали рвать соединения там, где рвать нечего.
		//
		// Ключ остаётся в ответе, хотя после И1 отвечает «нет» на обеих ветках.
		// Его читает приёмка, и снять его значит снять единственное место, где
		// расхождение «экран сменил режим, а трафик нет» наблюдаемо снаружи.
		return otvet(k.Id, k.Imya, map[string]any{
			"status":          s.Status(),
			"trebuet_podyoma": trebuetPodyoma,
		})

	case "disconnect":
		s.Otklyuchit()
		return otvet(k.Id, k.Imya, s.Status())

	case "listServers":
		return s.listServers(k)

	case "listRules":
		return s.listRules(k)

	case "setRules":
		return s.setRules(ctx, k)

	case "addServer":
		return s.addServer(k)

	case "removeServer":
		return s.removeServer(k)

	case "setSubscription":
		return s.setSubscription(ctx, k)

	case "refreshSubscription":
		return s.refreshSubscription(ctx, k)

	case "exportProfile":
		return s.eksportProfilya(ctx, k)

	case "importProfile":
		return s.importProfilya(ctx, k)
	}

	if volna, est := pozzhe[k.Imya]; est {
		return otkaz(k.Id, k.Imya, protokol.KodNeRealizovano,
			fmt.Sprintf("команда %s появится в волне %d", k.Imya, volna))
	}
	return otkaz(k.Id, k.Imya, protokol.KodProtocolMismatch,
		fmt.Sprintf("команда %s не существует", k.Imya))
}

// imenaKomand отдаёт полный список команд службы.
//
// Существует ради ворот на границу прав: новая команда обязана получить решение
// «нужен админ или нет», а не унаследовать его умолчанием.
//
// Список руками ПРОВЕРЯЕТСЯ разбором: с 03.09.2026 vetki_test.go читает ветки
// switch через go/ast и требует совпадения в обе стороны. Прежний довод «разбор
// своего же кода ничего не доказывает» стоил двух команд: setAutostart и
// setConnectOnStart жили в диспетчере, но не в этом списке, и ни одни ворота их
// не видели, потому что все ворота смотрели сюда, а не в код.
func imenaKomand() []string {
	return []string{
		"hello", "status", "listServers", "connect", "disconnect", "setServer",
		"setRouteMode",
		"addServer", "removeServer", "setSubscription", "refreshSubscription",
		"setKillSwitch", "exportProfile", "importProfile",
		"setAutostart", "setConnectOnStart", "setBandwidth",
		"listRules", "setRules",
		"subscribeStats", "setJournal", "setDiagnostics", "clearJournal",
		"checkExitIp", "checkLeaks",
		"installUpdate", "getServerHealth", "checkUpdate", "downloadUpdate",
		"measureDelays", "measureBandwidth",
		"startSpeedTest", "speedTestStatus", "cancelSpeedTest",
	}
}

// kodPodklyucheniya выбирает код отказа подъёма ТРЕМЯ ступенями, и порядок в
// них главное.
//
// 1. Сама ошибка, если она типизирована. Состояние службы пишут все: наблюдатель,
// восстановление, пересборка правил. Замерено чтением 03.09.2026: провал
// peresobratEsliNado кладёт в состояние firewall-failed на ПОДНЯТОМ туннеле, и
// код, взятый оттуда, описывает последнее случившееся, а не то, обо что
// споткнулась эта команда.
//
// 2. Состояние. Каждая ветка отказа Connect кладёт туда точный код прямо перед
// возвратом, и это по-прежнему лучший источник для всего, что не типизировано:
// нечитаемые ключи, молчащее ядро, не давшийся брандмауэр, не создавшийся TUN.
//
// 3. Умолчание, и оно ПОСЛЕДНЕЕ, а не первое. Прежде оно стояло первым по факту:
// любая ошибка без кода в состоянии уезжала человеку как all-servers-down, то
// есть отправляла его проверять серверы при исправных серверах. Оставлено
// потому, что «туннель поднялся и не понёс» это и есть настоящий смысл кода, а
// он приходит именно ошибкой без кода в состоянии.
func kodPodklyucheniya(err error, sost *protokol.Oshibka) string {
	switch {
	case errors.Is(err, ErrNetServerov), errors.Is(err, ErrServerNeNayd):
		return protokol.KodSelectedServerGone
	// Ядро живо и ответило, а исходящий не отработал пробу: сервер отверг
	// рукопожатие. all-servers-down тут отправлял человека проверять сеть при
	// исправной сети, а чинить надо подписку.
	case errors.Is(err, yadra.ErrServerOtvergKlyuchi):
		return protokol.KodServerAuthFailed
	}
	if sost != nil && sost.Kod != "" {
		return sost.Kod
	}
	return protokol.KodAllServersDown
}

// vernutVyborNazad возвращает набор к тому выбору и режиму, что были до
// команды.
//
// Через pravitNabor, а не zapisatNabor напрямую: запись выбора шла туда же, и
// откат обязан пройти тем же путём, иначе правила брандмауэра останутся
// собранными под отменённый выбор.
//
// Сшито на приёмке 03.09.2026. Полоса Б писала откат через sohranitIPeresobrat,
// которую полоса В к моменту слияния уже заменила на pravitNabor с общим
// замком. Слияние при этом было чистым текстуально: файлы у полос не
// пересекаются, а вызов чужой функции никакой merge не проверяет.
//
// Читать-править-писать целиком ВНУТРИ замка, а не снаружи. Прежний вид брал
// набор до отката и сравнивал с ним, то есть сравнивал с набором, который сосед
// мог переписать между чтением и записью: ровно тот TOCTOU, против которого
// pravitNabor и заведена.
func (s *Sluzhba) vernutVyborNazad(vybran string, rezhim protokol.Rezhim) error {
	err := s.pravitNabor(func(n *Nabor) error {
		// Ничего не менялось: второй поход в брандмауэр не за чем. Достижимо,
		// когда человек назвал уже выбранный сервер в уже ручном режиме.
		if n.Vybran == vybran && n.Rezhim == rezhim {
			return errOtkatNeNuzhen
		}
		n.Vybran, n.Rezhim = vybran, rezhim
		return nil
	})
	if errors.Is(err, errOtkatNeNuzhen) {
		return nil
	}
	return err
}

// errOtkatNeNuzhen выходит из правки набора, НЕ записывая его. Наружу не
// уезжает: откат нечего откатывать это успех, а не отказ.
var errOtkatNeNuzhen = errors.New("выбор не менялся, откат не нужен")
