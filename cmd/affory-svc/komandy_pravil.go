package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/katalog"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/set"
)

// Команды правил (задача 5.4). Правила живут в наборе рядом с серверами и
// пишутся через pravitNabor: замок, заслон Ш7-4 (набор не теряет живых) и
// пересборка разрешающих правил действуют на них так же, как на серверы.

// PravilaNabora это то, что человек попросил вести мимо туннеля.
type PravilaNabora struct {
	Trafik *protokol.PravilaTrafika `json:"trafik,omitempty"`
	// Пути уже нормализованы (set.NormalizovatPut): ядро сравнивает строку.
	Protsessy []string `json:"protsessy"`
	// Домены строчными, без точки в конце и в начале.
	Domeny []string `json:"domeny"`
	// BezRuSpiska это просьба человека вести российские сайты через туннель
	// наравне со всем остальным (владелец, 08.09.2026).
	//
	// Поле отрицательное намеренно. Наборы, уже лежащие на дисках, этого поля
	// не знают, и при разборе оно станет false, то есть «список на месте»: так
	// обновление никому не меняет поведение молча.
	BezRuSpiska bool `json:"bez_ru_spiska,omitempty"`
}

// zaprosPravil это ТЕЛО команды setRules, а не то, что ложится в набор.
//
// Отдельный тип ради одного поля: у выключателя на проводе три состояния, а в
// наборе два. Отсутствие поля значит «не трогали». Клиент, который про
// выключатель не знает (`affory-cli rules set --in файл`, окно прошлой
// версии), шлёт два списка, и считать его молчание за «включить обратно»
// значит менять маршрут российских сайтов у человека за спиной.
type zaprosPravil struct {
	Trafik      *protokol.PravilaTrafika `json:"trafik"`
	Protsessy   []string                 `json:"protsessy"`
	Domeny      []string                 `json:"domeny"`
	BezRuSpiska *bool                    `json:"bez_ru_spiska"`
}

// Шов для нормализации пути: настоящая открывает файл на диске, а тесту
// правил нужен и файл, который есть, и файл, которого нет.
var normalizovatPut = set.NormalizovatPut

var errPraviloNegodno = errors.New("правило не принято")

// Имя домена: метки из букв, цифр и дефиса, хотя бы одна точка. Схема, путь и
// пробел это не домен, а строка, которую человек вставил не туда, и правило по
// ней не совпало бы ни с чем, выглядя при этом рабочим.
var reDomen = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$`)

func (s *Sluzhba) listRules(k protokol.Kadr) protokol.Kadr {
	n, err := s.nabor()
	if err != nil {
		return otkaz(k.Id, k.Imya, protokol.KodSecretsUnreadable, err.Error())
	}
	return otvet(k.Id, k.Imya, teloPravil(n.Pravila, s.pravilaOzhidayut(n.Pravila), false))
}

func (s *Sluzhba) setRules(ctx context.Context, k protokol.Kadr) protokol.Kadr {
	var telo zaprosPravil
	if err := json.Unmarshal(k.Telo, &telo); err != nil {
		return otkaz(k.Id, k.Imya, protokol.KodProtocolMismatch, "тело команды не разбирается")
	}
	// Проверка идёт ВНУТРИ правки набора: годность правила теперь зависит от
	// того, что в наборе уже лежит, а читать набор отдельно от записи значит
	// проверять по одному списку, а писать в другой.
	var pravila PravilaNabora
	if err := s.pravitNabor(func(n *Nabor) error {
		// Прежнее значение выключателя берётся из набора ПОД ЗАМКОМ правки, а
		// не читается отдельно: между чтением и записью успевает пройти чужая
		// правка, и вернулось бы то, что уже отменили.
		vhod := PravilaNabora{Protsessy: telo.Protsessy, Domeny: telo.Domeny, BezRuSpiska: n.Pravila.BezRuSpiska}
		vhod.Trafik = n.Pravila.Trafik
		if telo.Trafik != nil {
			vhod.Trafik = telo.Trafik
			vhod.Protsessy, vhod.Domeny = nil, nil
		}
		if telo.BezRuSpiska != nil {
			vhod.BezRuSpiska = *telo.BezRuSpiska
		}
		p, err := proveritPravila(vhod, n.Pravila)
		if err != nil {
			return err
		}
		s.mu.Lock()
		strict := s.killSwitch
		s.mu.Unlock()
		if p.Trafik != nil && strict && estPryamoyTrafik(*p.Trafik) {
			return fmt.Errorf("%w: сначала отключите блокировку сети вне VPN в настройках защиты", errPraviloNegodno)
		}
		n.Pravila, pravila = p, p
		return nil
	}); err != nil {
		if errors.Is(err, errPraviloNegodno) {
			return otkaz(k.Id, k.Imya, protokol.KodPraviloNegodno, err.Error())
		}
		return otkaz(k.Id, k.Imya, kodSohraneniya(err), err.Error())
	}
	// Горячей перезагрузки правил у ядра нет: как и setRouteMode, ответ
	// говорит, что новое применится со следующего подъёма, а не молчит.
	//
	// Критерий «ядро живо» это ПУСТОЙ адрес clash_api, а не состояние: подъём
	// нужен ровно тогда, когда старые правила держит живое ядро, а не тогда,
	// когда на экране написано что-то кроме «выключен».
	adres, _ := s.dostupKKlash()
	if telo.Trafik != nil && adres != "" {
		if err := s.perepodklyuchit(ctx); err != nil {
			return otkaz(k.Id, k.Imya, kodPodklyucheniya(err, s.Status().Oshib), "Правила сохранены. Переподключение не завершено: "+err.Error())
		}
		return otvet(k.Id, k.Imya, teloPravil(pravila, s.pravilaOzhidayut(pravila), true))
	}
	return otvet(k.Id, k.Imya, teloPravil(pravila, adres != "", otlichaetsya(telo, pravila)))
}

// teloPravil отдаёт пустые списки как `[]`, а не `null`: null на экране это
// «не мерили» по контракту fallback, а здесь измерено и равно нулю.
//
// spisok_izmenyon значит «принятое не равно присланному, перерисуй список
// целиком». Дедуп и нормализация укорачивают и переписывают строки, и молчание
// про это оставляет на экране строки, которых в наборе нет.
func teloPravil(p PravilaNabora, trebuetPodyoma, izmenyon bool) map[string]any {
	protsessy, domeny := p.Protsessy, p.Domeny
	if protsessy == nil {
		protsessy = []string{}
	}
	if domeny == nil {
		domeny = []string{}
	}
	return map[string]any{
		"trafik":          trafikPravil(p),
		"katalog":         katalogDlyaPravil(),
		"protsessy":       protsessy,
		"domeny":          domeny,
		"trebuet_podyoma": trebuetPodyoma,
		"spisok_izmenyon": izmenyon,
		"bez_ru_spiska":   p.BezRuSpiska,
	}
}

// otlichaetsya сравнивает ПРИСЛАННОЕ с ПРИНЯТЫМ.
//
// Именно с присланным, а не с прежним набором: экран рисует то, что отправил, и
// перерисовать его надо тогда, когда служба приняла что-то другое. Нормализация
// пути и приведение домена к строчным считаются изменением наравне с дедупом.
func otlichaetsya(prislano zaprosPravil, prinyato PravilaNabora) bool {
	return !slices.Equal(prislano.Protsessy, prinyato.Protsessy) ||
		!slices.Equal(prislano.Domeny, prinyato.Domeny)
}

// proveritPravila нормализует каждую строку и отвергает весь список, если
// хоть одна негодна: набор либо принят целиком, либо не тронут. Половина
// правил, записанная молча, это правило, о котором человек думает, что оно
// есть.
//
// Нормализация пути это условие годности НОВОГО правила, а не уже принятого.
// Разница стоила продукту целого тупика: правило нормализуется открытием файла
// на диске, и стоило человеку снести игру, как отказ прилетал на ВЕСЬ список.
// Удалить нельзя было ничего, включая само осиротевшее правило, а выхода из
// этого состояния интерфейс не предлагал.
//
// Пропавший файл делает уже принятое правило неактивным (ядро не найдёт такого
// процесса), но не делает негодным список. Отказ rule-invalid остаётся ровно
// для правил, которых в наборе ещё не было, и называет конкретное правило.
func proveritPravila(t PravilaNabora, bylo PravilaNabora) (PravilaNabora, error) {
	itog := PravilaNabora{Protsessy: []string{}, Domeny: []string{}, BezRuSpiska: t.BezRuSpiska}
	if t.Trafik != nil {
		trafik, err := proveritTrafik(*t.Trafik, trafikPravil(bylo))
		if err != nil {
			return PravilaNabora{}, err
		}
		itog.Trafik = &trafik
	}
	vidno := map[string]bool{}
	for _, p := range t.Protsessy {
		syroy := strings.TrimSpace(p)
		norm, err := normalizovatPut(syroy)
		if err != nil {
			// Пути в наборе уже нормализованы, а экран присылает их обратно как
			// есть. Сравнение без учёта регистра: на Windows это один и тот же
			// файл, и требовать точного совпадения значило бы вернуть тот же
			// тупик другим путём.
			prinyatoRanee, est := sredi(bylo.Protsessy, syroy)
			if !est {
				return PravilaNabora{}, fmt.Errorf("%w: %s (%v)", errPraviloNegodno, syroy, err)
			}
			norm = prinyatoRanee
		}
		if !vidno["p:"+norm] {
			vidno["p:"+norm] = true
			itog.Protsessy = append(itog.Protsessy, norm)
		}
	}
	for _, d := range t.Domeny {
		norm := strings.Trim(strings.ToLower(strings.TrimSpace(d)), ".")
		if !reDomen.MatchString(norm) {
			return PravilaNabora{}, fmt.Errorf("%w: %q не похоже на имя домена", errPraviloNegodno, d)
		}
		if !vidno["d:"+norm] {
			vidno["d:"+norm] = true
			itog.Domeny = append(itog.Domeny, norm)
		}
	}
	return itog, nil
}

// sredi ищет путь в уже принятом списке без учёта регистра и отдаёт ту
// строку, которая в наборе, а не ту, что прислал экран.
func sredi(spisok []string, put string) (string, bool) {
	for _, s := range spisok {
		if strings.EqualFold(s, put) {
			return s, true
		}
	}
	return "", false
}

func katalogDlyaPravil() *katalog.Katalog {
	k, err := katalog.Chitat()
	if err != nil {
		return nil
	}
	return &k
}
