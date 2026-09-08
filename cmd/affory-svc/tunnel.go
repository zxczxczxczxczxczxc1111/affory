package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"encoding/json"
	"strings"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/genkonfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/set"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/sostoyanie"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/yadra"
)

// Ядро одно. Волна 2 поднимала два, и второй процесс отдавал SOCKS, который
// первый потреблял; с 01.09.2026 sing-box несёт все транспорты сам. Имя файла
// живёт в yadra: по нему же сверяется владелец порта clash_api.
const imyaAdapteraTun = "tun0"

// Имя файла ядра живёт в yadra и здесь только берётся: по нему же там
// сверяется владелец порта clash_api, а две копии строки разъезжаются молча.
var imyaYadraTun = yadra.ImyaYadra

// Отдельная переменная, а не константа, ровно ради теста: честно ждать двадцать
// секунд умеет только тест, который через месяц закомментируют.
var zhdatAdaptera = 20 * time.Second

func putKonfigaTun() string {
	return sostoyanie.KatalogDannyh() + `\sing-box.json`
}

// podnyatTun собирает конфиг, поднимает sing-box и ждёт появления адаптера.
//
// Порядок «сначала второе ядро, потом TUN» отменён вместе со вторым ядром
// (задача П5). От состояния ne-neset, ради которого он держался, теперь
// защищает не порядок, а откат: замер идёт по живому туннелю, и его провал
// туннель опускает.
func (s *Sluzhba) podnyatTunSistemno(ctx context.Context) (set.Adapter, error) {
	portClash, sekret, err := s.sobratTunProverennyy(putKonfigaTun())
	if err != nil {
		return set.Adapter{}, err
	}
	// Запоминаем ДО запуска ядра, а не после подъёма адаптера: проба готовности
	// нужна ровно в промежутке между стартом ядра и появлением адаптера. Если
	// подъём сорвётся, Disconnect это забудет вместе с остальным.
	s.zapomnitKlash(portClash, sekret)

	// Жалобы прошлого подъёма забываются ЗДЕСЬ, до старта ядра: оставленные,
	// они обвинили бы драйвер в отказе, к которому он отношения не имеет.
	yadra.ZabytZhalobyYadra()

	go func() {
		// Состояние туннеля ведёт основной путь Connect, поэтому обработчика у
		// сторожа нет вовсе.
		//
		// Прежде здесь стояла ветка на SostOtkaz. Сторож её не шлёт НИКОГДА: он
		// перезапускает ядро вечно, с растущими отступами, и сдачи у него нет.
		// Отказы при этом не теряются: не поднявшееся ядро ловит ожидание
		// адаптера в подъёме, а умершее позже ловит наблюдатель по clash_api и
		// опускает туннель кодом tunnel-not-carrying. Ветка описывала событие,
		// которого не бывает, и создавала вид обработки.
		if err := s.storozhit(ctx, imyaYadraTun, putKonfigaTun(), nil); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("сторож ядра туннеля завершился: %v", err)
		}
	}()

	ozhid, otm := context.WithTimeout(ctx, zhdatAdaptera)
	defer otm()
	a, err := set.ZhdatAdapterPolno(ozhid, imyaAdapteraTun)
	if err != nil {
		return set.Adapter{}, otkazOzhidaniyaAdaptera(err, zhalobaNaDrayver())
	}

	// Запись в файл состояния живёт у вызывающего, а НЕ здесь. Здесь она
	// оказывалась под швом подъёма: тест подменял весь подъём целиком, запись
	// вместе с ним не выполнялась, и проверить её было нечем. Ровно так порт
	// clash_api и адаптер пропадали из файла годами при зелёных тестах.
	return a, nil
}

// sobratTun готовит конфиг sing-box из того, что известно ПРЯМО СЕЙЧАС.
//
// ЗДЕСЬ ЛЕЖАТ КЛЮЧИ СЕРВЕРА, и это изменилось задачей П5. Пока xhttp нёс Xray,
// sing-box получал безобидный исходящий SOCKS на локальный порт, а весь секрет
// был в чужом конфиге. Теперь исходящий прямой: uuid, публичный ключ и shortId
// reality уезжают именно сюда. Отсюда 0600 на файле и каталог с разорванным
// наследованием: это не перестраховка, а единственное, что стоит между ключами
// и любым процессом в системе.
// sobratTun собирает конфиг ядра. isklyucheny это идентификаторы серверов,
// которые ядро уже отвергло: их не должно быть ни среди кандидатов urltest, ни
// среди исходящих, иначе следующая проверка отвергнет конфиг ровно так же.
func (s *Sluzhba) sobratTun(isklyucheny map[string]bool) ([]byte, int, string, error) {
	n, err := s.nabor()
	if err != nil {
		return nil, 0, "", err
	}
	vybrannyy, err := n.VybrannyyServer()
	if err != nil {
		return nil, 0, "", err
	}
	if len(isklyucheny) > 0 {
		ostavshiesya := make([]protokol.Server, 0, len(n.Servery))
		for _, srv := range n.Servery {
			if !isklyucheny[srv.Id] {
				ostavshiesya = append(ostavshiesya, srv)
			}
		}
		n.Servery = ostavshiesya
	}

	kandidaty, err := s.kandidatySIsklyucheniem()
	if err != nil {
		return nil, 0, "", err
	}

	resolver, err := set.LokalnyyResolver()
	if err != nil {
		return nil, 0, "", fmt.Errorf("локальный резолвер не определён: %w", err)
	}
	puti, err := s.putiProtsessov()
	if err != nil {
		return nil, 0, "", err
	}
	portClash, err := yadra.VydatPort()
	if err != nil {
		return nil, 0, "", fmt.Errorf("порт для clash_api не выдан: %w", err)
	}
	sekret, err := sluchaynyySekret()
	if err != nil {
		return nil, 0, "", err
	}

	// Режим «весь трафик» это часть конфига ядра, а не только брандмауэра.
	// Без него правило ip_is_private остаётся, частные сети идут мимо туннеля,
	// и инвариант 6 не наступает ни разу, хотя генератор его умеет.
	s.mu.Lock()
	vesTrafik := s.killSwitch
	// Полоса канала: настройка человека, живёт в файле состояния и читается
	// отсюда так же, как режим выше. Ноль означает «не измерена», и генератор
	// тогда не пишет полей вовсе, то есть hysteria2 остаётся на BBR.
	polosaVverh, polosaVniz := s.snimok.PolosaVverh, s.snimok.PolosaVniz
	s.mu.Unlock()

	var trafik *protokol.PravilaTrafika
	if n.Pravila.Trafik != nil {
		merged := trafikPravil(n.Pravila)
		trafik = &merged
	}
	telo, err := genkonfig.SingBox(genkonfig.Vhod{
		Trafik: trafik,
		Server: vybrannyy,
		Rezhim: rezhimNabora(n),
		// Кандидаты это ВСЕ серверы, а не только выбранный: urltest пробит их
		// все, и адрес, не попавший в правило петли, это петля на старте.
		Servery: n.Servery,
		// Адреса берутся из ТОГО ЖЕ сборщика, что и разрешающие правила
		// брандмауэра. Своя сборка здесь однажды разошлась бы с той, и молча.
		Kandidaty:      kandidaty,
		Resolver:       resolver,
		PutiProtsessov: puti,
		ClashApi:       genkonfig.ClashApi{Adres: "127.0.0.1", Port: portClash, Sekret: sekret},
		PortProksi:     s.vybratPortProksi(),
		VesTrafik:      vesTrafik,
		// Только наборы с файлом на диске: ядро поднимается с initial_path, а
		// не с сети, и неудача загрузки остаётся неудачей загрузки.
		Nabory:    s.podgotovitNabory(),
		FaylKesha: putKesha(),
		// Исключения человека. Генератор сам убирает их в режиме «весь трафик».
		Protsessy: n.Pravila.Protsessy,
		Domeny:    n.Pravila.Domeny,
		// Полоса нужна ровно hysteria2 и ровно как переключатель Brutal.
		// Генератор сам решает, кому её писать: транспорту, который её не
		// понимает, поле сломало бы весь конфиг.
		PolosaVverh: polosaVverh,
		PolosaVniz:  polosaVniz,
	})
	if err != nil {
		return nil, 0, "", fmt.Errorf("конфиг туннеля не собран: %w", err)
	}
	s.mu.Lock()
	s.pravilaKonfiga, s.trafikKonfiga = otpechatokPravil(n.Pravila), trafikPravil(n.Pravila).PoUmolchaniyu
	s.mu.Unlock()
	return telo, portClash, sekret, nil
}

// Секрет заново на каждый старт: постоянный секрет в файле это постоянный
// секрет, а этот живёт ровно столько, сколько поднят туннель.
func sluchaynyySekret() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("не удалось получить случайные байты: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func putYadraTun() string {
	return sostoyanie.KatalogProgrammy() + `\` + imyaYadraTun
}

// PortProksiPoUmolchaniyu это тот самый порт, который прописан системным
// прокси Windows у половины клиентов этого семейства: v2rayN, nekoray, Sota.
// Совпадение НАМЕРЕННОЕ. Смысл прокси рядом с туннелем ровно в том, чтобы
// приложение с уже настроенным прокси продолжало работать, а не в том, чтобы
// человек шёл править настройки под нас.
const PortProksiPoUmolchaniyu = 10809

// portProksi отдаёт порт, если он свободен, и ноль, если занят.
//
// Занятый порт нельзя отдавать в конфиг: sing-box падает на бинде ЦЕЛИКОМ, то
// есть чужой клиент на 10809 унёс бы наш туннель, который сам по себе исправен.
// Туннель важнее удобства, поэтому прокси здесь необязательная надстройка.
// vybratPortProksi выбирает порт и ЗАПОМИНАЕТ его.
//
// Запоминать обязательно: сторож системного прокси иначе не может отличить наш
// вход от чужого перехвата и обвиняет в перехвате нас самих. Интерфейсу волны 4
// этот же порт нужен, чтобы показать человеку, куда прописывать прокси.
func (s *Sluzhba) vybratPortProksi() int {
	port := portProksi(PortProksiPoUmolchaniyu)
	s.mu.Lock()
	s.portProksiNash = port
	s.mu.Unlock()
	return port
}

func portProksi(port int) int {
	imya, err := yadra.VladelecPorta(port)
	if err != nil {
		// Таблица TCP не прочиталась. Это «не знаю», а не «свободен», и рисковать
		// туннелем ради догадки не станем.
		return 0
	}
	if imya != "" {
		return 0
	}
	return port
}

// sobratTunProverennyy собирает конфиг, СПРАШИВАЕТ ЯДРО и исключает серверы,
// которые оно не приняло, пока конфиг не станет годным.
//
// Находка 43, и это второй заход на класс находки 33: один негодный сервер в
// списке не давал подняться туннелю вообще. Тогда закрыли частный случай,
// проверку ключа при разборе ссылки, и в записи о ней прямо сказано, чего не
// сделали: «ядро не приняло то, что собрал генератор» не проверялось никогда.
// Через сутки чужая подписка из 39 серверов принесла второй случай.
//
// Судья именно ядро, а не наша проверка: конфиг собираем мы, а принимает его
// оно, и полный список того, что оно отвергнет, знает только оно. Своя проверка
// повторила бы ту же ошибку, закрыв известные случаи и оставив класс.
//
// Конфиг пишется на своё место КАЖДЫЙ круг: ядро судит файл, а не память, и
// проверять один текст, запуская другой, значило бы проверять не то.
func (s *Sluzhba) sobratTunProverennyy(put string) (int, string, error) {
	isklyucheny := map[string]bool{}
	for {
		telo, portClash, sekret, err := s.sobratTun(isklyucheny)
		if err != nil {
			return 0, "", err
		}
		// 0600 и в каталоге, куда пускают только SYSTEM и админов. Конфиг несёт
		// адрес домашнего резолвера и секрет clash_api, то есть не является
		// безобидным текстом.
		if err := os.WriteFile(put, telo, 0o600); err != nil {
			return 0, "", fmt.Errorf("конфиг туннеля не записан: %w", err)
		}
		if s.proveritKonfig == nil {
			return portClash, sekret, nil
		}
		err = s.proveritKonfig(put)
		if err == nil {
			return portClash, sekret, nil
		}

		var o *yadra.OshibkaKonfiga
		// Отказ без номера исходящего исключать нечего: конфиг не разобрался
		// целиком. Круг здесь означал бы вечное перестроение одного и того же.
		if !errors.As(err, &o) || !o.EstNomer {
			return 0, "", fmt.Errorf("конфиг туннеля не принят ядром: %w", err)
		}
		id, est := idServeraPoNomeru(telo, o.Nomer)
		if !est || isklyucheny[id] {
			// Либо номер не сводится к серверу (это группа или петля), либо мы
			// его уже исключали и ядро всё равно недовольно. Круг прерывается.
			return 0, "", fmt.Errorf("конфиг туннеля не принят ядром: %w", err)
		}
		vybrannyy, oshib := s.vybrannyyId()
		if oshib == nil && id == vybrannyy {
			// Выбранный сервер исключать НЕЛЬЗЯ. Молча подняться на другом
			// значило бы увести трафик не туда, куда просили, и человек об этом
			// не узнал бы.
			return 0, "", fmt.Errorf("ядро не приняло выбранный сервер %s: %w", id, err)
		}

		isklyucheny[id] = true
		// Молча выкинуть сервер значит оставить человека с подпиской, которая
		// тихо стала короче. То же правило, что у исключения по имени.
		log.Printf("сервер %s исключён: ядро не приняло его исходящий (%s): %s",
			id, protokol.KodServerRejectedByCore, o.Vyhod)
		s.izvestit("serversRejected", map[string]any{
			"kod":   protokol.KodServerRejectedByCore,
			"id":    id,
			"tekst": "ядро не приняло сервер " + id + " и он исключён: " + o.Vyhod,
		})
	}
}

// vybrannyyId отвечает, какой сервер человек просил поднять.
func (s *Sluzhba) vybrannyyId() (string, error) {
	n, err := s.nabor()
	if err != nil {
		return "", err
	}
	// В авто выбранного сервера НЕТ, и защищать от исключения нечего. Пока здесь
	// стоял VybrannyyServer(), при пустом Vybran возвращался Servery[0], и на
	// этом ответе висел запрет исключать выбранный: одна плохая ссылка в
	// подписке роняла подъём целиком вместо исключения одного сервера. В ручном
	// режиме запрет верен, в авто он становится своей противоположностью.
	//
	// Вызывающий сравнивает id == vybrannyy и на пустой строке просто не
	// совпадёт, то есть исключение пройдёт как для любого кандидата. Отдельной
	// ветки там не нужно.
	if rezhimNabora(n) == protokol.RezhimAvto {
		return "", nil
	}
	v, err := n.VybrannyyServer()
	if err != nil {
		return "", err
	}
	return v.Id, nil
}

// idServeraPoNomeru сводит номер исходящего, названный ядром, к серверу.
//
// Читается ТОТ ЖЕ текст, который проверяло ядро, а не собственное представление
// о порядке: порядок исходящих задаёт генератор, и всякая вторая его копия
// разъехалась бы с первой молча.
func idServeraPoNomeru(telo []byte, nomer int) (string, bool) {
	var k struct {
		Outbounds []struct {
			Tag string `json:"tag"`
		} `json:"outbounds"`
	}
	if err := json.Unmarshal(telo, &k); err != nil {
		return "", false
	}
	if nomer < 0 || nomer >= len(k.Outbounds) {
		return "", false
	}
	return genkonfig.IdIzTega(strings.TrimSpace(k.Outbounds[nomer].Tag))
}

// zhalobaNaDrayver это шов над жалобой ядра: тесту нужны обе половины ответа,
// а живого ядра у него нет.
var zhalobaNaDrayver = yadra.ZhalobaNaDrayver

// otkazOzhidaniyaAdaptera называет причину таймаута ожидания адаптера.
//
// Таймаут у пропавшего драйвера и у занятого имени адаптера ОДИН И ТОТ ЖЕ, и
// догадка «раз таймаут, значит драйвера нет» запрещена. Судья тут ядро: оно
// единственное знает, обо что споткнулось, и жалоба доезжает до текста, потому
// что §9.1 обещает человеку «плюс причина от системы».
//
// Пустая жалоба означает «виновник не установлен», а не «драйвер цел»: молчание
// оставляет прежний tun-create-failed. Ровно то же правило, что у chuzhoyNaPortu.
func otkazOzhidaniyaAdaptera(err error, zhaloba string) error {
	if err == nil || zhaloba == "" {
		return err
	}
	return fmt.Errorf("%w: %s: %w", yadra.ErrDrayverNeVstal, zhaloba, err)
}
