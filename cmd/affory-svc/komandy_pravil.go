package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"regexp"
	"slices"
	"strings"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/katalog"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/reklama"
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
	// наравне со всем остальным (решено 08.09.2026).
	//
	// Поле отрицательное намеренно. Наборы, уже лежащие на дисках, этого поля
	// не знают, и при разборе оно станет false, то есть «список на месте»: так
	// обновление никому не меняет поведение молча.
	BezRuSpiska bool `json:"bez_ru_spiska,omitempty"`
	// StaryeProgrammySnyaty значит «правила приложений на программы сервисов уже
	// сняты» (D2, 22.09.2026). Поле положительное: набор с диска этого поля не
	// знает, при разборе оно станет false, и чистка случится ровно один раз - на
	// первом чтении после обновления.
	//
	// Без флага чистка повторялась бы на каждом чтении и снимала правило,
	// которое человек завёл руками уже после обновления.
	StaryeProgrammySnyaty bool `json:"starye_programmy_snyaty,omitempty"`
	// Reklama это блокировка рекламы (28.09.2026). Указатель с omitempty, а
	// нулевая настройка сворачивается в nil (proveritReklamu): наборы с диска
	// поля не знают, JSON у них прежний, и после обновления не меняются ни
	// отпечаток правил, ни ревизия.
	Reklama *ReklamaPravila `json:"reklama,omitempty"`
}

// ReklamaPravila это настройка блокировки рекламы, как её задал человек.
type ReklamaPravila struct {
	Vkl        bool     `json:"vkl,omitempty"`
	Uroven     string   `json:"uroven,omitempty"` // light | multi
	Razresheno []string `json:"razresheno,omitempty"`
}

// zaprosPravil это ТЕЛО команды setRules, а не то, что ложится в набор.
//
// Отдельный тип ради одного поля: у выключателя на проводе три состояния, а в
// наборе два. Отсутствие поля значит «не трогали». Клиент, который про
// выключатель не знает (`affory-cli rules set --in файл`, окно прошлой
// версии), шлёт два списка, и считать его молчание за «включить обратно»
// значит менять маршрут российских сайтов у человека за спиной.
type zaprosPravil struct {
	Reviziya    string                   `json:"reviziya_pravil,omitempty"`
	Trafik      *protokol.PravilaTrafika `json:"trafik"`
	Protsessy   []string                 `json:"protsessy"`
	Domeny      []string                 `json:"domeny"`
	BezRuSpiska *bool                    `json:"bez_ru_spiska"`
	// nil значит «не трогали»: CLI и окно прошлой версии поля не шлют.
	Reklama *ReklamaPravila `json:"reklama"`
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
	// Прежние правила запоминаются ДО правки: если ядро не примет новый конфиг,
	// набор откатывается на них (A6). Откат ровно на одну операцию - ту, что
	// сейчас; архива прежних наборов у нас нет и не нужно.
	var bylo PravilaNabora
	if err := s.pravitNabor(func(n *Nabor) error {
		bylo = n.Pravila
		if telo.Reviziya != "" && telo.Reviziya != reviziyaPravil(n.Pravila) {
			return fmt.Errorf("%w: правила уже изменены другим запросом. Перечитай набор перед применением черновика", errPraviloNegodno)
		}
		// Прежнее значение выключателя берётся из набора ПОД ЗАМКОМ правки, а
		// не читается отдельно: между чтением и записью успевает пройти чужая
		// правка, и вернулось бы то, что уже отменили.
		vhod := PravilaNabora{Protsessy: telo.Protsessy, Domeny: telo.Domeny, BezRuSpiska: n.Pravila.BezRuSpiska,
			// Флаг чистки берётся из НАБОРА: окно про него не знает и слать его
			// не будет, а потеря флага вернула бы чистку на каждую правку.
			StaryeProgrammySnyaty: n.Pravila.StaryeProgrammySnyaty,
			// Блокировка рекламы тоже из набора: клиент, который о ней не знает,
			// не имеет права стереть её своей правкой.
			Reklama: n.Pravila.Reklama}
		vhod.Trafik = n.Pravila.Trafik
		if telo.Trafik != nil {
			vhod.Trafik = telo.Trafik
			vhod.Protsessy, vhod.Domeny = nil, nil
		}
		if telo.BezRuSpiska != nil {
			vhod.BezRuSpiska = *telo.BezRuSpiska
		}
		if telo.Reklama != nil {
			vhod.Reklama = telo.Reklama
		}
		p, err := proveritPravila(vhod, n.Pravila)
		if err != nil {
			return err
		}
		// Прямое правило при включённой защите принимается и сохраняется: под
		// защитой оно просто не применяется, а после её выключения работает.
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
	// Тело с одной рекламой тоже меняет конфиг: без второго условия включение
	// блокировки сохранилось бы и ждало следующего подъёма.
	if (telo.Trafik != nil || telo.Reklama != nil) && adres != "" && s.pravilaOzhidayut(pravila) {
		if err := s.perepodklyuchit(ctx); err != nil {
			// Кандидат забракован ДО остановки: рабочее подключение цело, а
			// сохранённые правила надо снять - иначе следующий подъём соберётся
			// с тем же негодным набором, и человек потеряет VPN уже без
			// единого своего действия.
			if errors.Is(err, errKandidatNegoden) {
				vernuli := s.vernutPravila(bylo)
				tekst := "Правила не применены: " + err.Error() + ". VPN продолжает работать по прежним правилам"
				if !vernuli {
					tekst += ". Вернуть прежние правила не удалось, проверь список"
				}
				return otkaz(k.Id, k.Imya, protokol.KodPravilaNePrinyaty, tekst)
			}
			return otkaz(k.Id, k.Imya, kodPodklyucheniya(err, s.vnutriOshib()), "Правила сохранены. Переподключение не завершено: "+err.Error())
		}
		// После подъёма, а не до: кэш сбрасывается, когда новое ядро уже
		// отвечает, иначе Windows успела бы запомнить ответ старого.
		s.posleSmenyReklamy(bylo.Reklama, pravila.Reklama)
		return otvet(k.Id, k.Imya, teloPravil(pravila, s.pravilaOzhidayut(pravila), true))
	}
	s.posleSmenyReklamy(bylo.Reklama, pravila.Reklama)
	return otvet(k.Id, k.Imya, teloPravil(pravila, s.pravilaOzhidayut(pravila), otlichaetsya(telo, pravila)))
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
		"reviziya_pravil": reviziyaPravil(p),
		"trafik":          trafikPravil(p),
		"katalog":         katalogDlyaPravil(),
		"protsessy":       protsessy,
		"domeny":          domeny,
		"trebuet_podyoma": trebuetPodyoma,
		"spisok_izmenyon": izmenyon,
		"bez_ru_spiska":   p.BezRuSpiska,
		"reklama":         teloReklamy(p.Reklama),
	}
}

// teloReklamy всегда объект со всеми тремя полями и списком: по нему окно
// показывает вкладку, а null значил бы «служба о рекламе не знает». Не
// ReklamaPravila: её omitempty выбросил бы выключатель и пустой список.
func teloReklamy(r *ReklamaPravila) map[string]any {
	t := map[string]any{"vkl": false, "uroven": string(reklama.Bazovyy), "razresheno": []string{}}
	if r == nil {
		return t
	}
	t["vkl"] = r.Vkl
	if r.Uroven != "" {
		t["uroven"] = r.Uroven
	}
	if r.Razresheno != nil {
		t["razresheno"] = r.Razresheno
	}
	return t
}

// reviziyaPravil берёт отпечаток ПОЛНОГО набора, а не конфига: уровень списка
// в конфиг не входит, но смена уровня из двух окон обязана дать конфликт
// черновиков. Без рекламы это те же байты, что у otpechatokPravil.
func reviziyaPravil(p PravilaNabora) string {
	b, err := json.Marshal(p)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%x", sha256.Sum256(b))
}

// otlichaetsya сравнивает ПРИСЛАННОЕ с ПРИНЯТЫМ.
//
// Именно с присланным, а не с прежним набором: экран рисует то, что отправил, и
// перерисовать его надо тогда, когда служба приняла что-то другое. Нормализация
// пути и приведение домена к строчным считаются изменением наравне с дедупом.
func otlichaetsya(prislano zaprosPravil, prinyato PravilaNabora) bool {
	return !slices.Equal(prislano.Protsessy, prinyato.Protsessy) ||
		!slices.Equal(prislano.Domeny, prinyato.Domeny) ||
		(prislano.Reklama != nil && !slices.Equal(prislano.Reklama.Razresheno, razreshenoIz(prinyato.Reklama)))
}

func razreshenoIz(r *ReklamaPravila) []string {
	if r == nil {
		return nil
	}
	return r.Razresheno
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
	itog := PravilaNabora{Protsessy: []string{}, Domeny: []string{}, BezRuSpiska: t.BezRuSpiska,
		StaryeProgrammySnyaty: t.StaryeProgrammySnyaty}
	// itog строится с нуля: без переноса любая правка правил стирала бы
	// настройку рекламы.
	r, err := proveritReklamu(t.Reklama)
	if err != nil {
		return PravilaNabora{}, err
	}
	itog.Reklama = r
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

// Предел ручного списка: это исключения, а не второй список блокировки.
const predelIsklyucheniyReklamy = 256

// proveritReklamu нормализует так же, как домены правил (reDomen, строчные,
// без точек по краям, без повторов). Нулевая настройка сворачивается в nil.
func proveritReklamu(r *ReklamaPravila) (*ReklamaPravila, error) {
	if r == nil {
		return nil, nil
	}
	u, izvesten := reklama.Privesti(r.Uroven)
	if !izvesten {
		return nil, fmt.Errorf("%w: уровень списка %q неизвестен", errPraviloNegodno, r.Uroven)
	}
	itog := &ReklamaPravila{Vkl: r.Vkl, Uroven: string(u)}
	vidno := map[string]bool{}
	for _, d := range r.Razresheno {
		norm := strings.Trim(strings.ToLower(strings.TrimSpace(d)), ".")
		// IP совпадает с reDomen, но domain_suffix сравнивается только с именем:
		// такое исключение выглядело бы рабочим и не делало бы ничего.
		if !reDomen.MatchString(norm) || net.ParseIP(norm) != nil {
			return nil, fmt.Errorf("%w: %q не похоже на имя сайта", errPraviloNegodno, d)
		}
		if !vidno[norm] {
			vidno[norm] = true
			itog.Razresheno = append(itog.Razresheno, norm)
		}
	}
	if len(itog.Razresheno) > predelIsklyucheniyReklamy {
		return nil, fmt.Errorf("%w: исключений %d, больше %d", errPraviloNegodno, len(itog.Razresheno), predelIsklyucheniyReklamy)
	}
	if !itog.Vkl && len(itog.Razresheno) == 0 && u == reklama.Bazovyy {
		return nil, nil
	}
	return itog, nil
}

// privestiReklamu мягко чинит настройку, прочитанную с диска. Блоб приходит и
// импортом чужого профиля, и отказ читать набор из-за одной строки оставил бы
// человека без единого сервера: незнакомый уровень становится базовым,
// негодные имена и адреса выбрасываются, лишнее сверх предела обрезается.
// Каждая починка называется вызывающему.
func privestiReklamu(r *ReklamaPravila) (*ReklamaPravila, []string) {
	if r == nil {
		return nil, nil
	}
	var pochinki []string
	u, izvesten := reklama.Privesti(r.Uroven)
	if !izvesten {
		pochinki = append(pochinki, fmt.Sprintf("уровень списка рекламы %q заменён на %s", r.Uroven, u))
	}
	itog := &ReklamaPravila{Vkl: r.Vkl, Uroven: string(u)}
	vidno := map[string]bool{}
	for _, d := range r.Razresheno {
		norm := strings.Trim(strings.ToLower(strings.TrimSpace(d)), ".")
		if !reDomen.MatchString(norm) || net.ParseIP(norm) != nil {
			pochinki = append(pochinki, fmt.Sprintf("исключение рекламы %q выброшено: не имя сайта", d))
			continue
		}
		if !vidno[norm] {
			vidno[norm] = true
			itog.Razresheno = append(itog.Razresheno, norm)
		}
	}
	if len(itog.Razresheno) > predelIsklyucheniyReklamy {
		pochinki = append(pochinki, fmt.Sprintf("исключений рекламы %d, оставлены первые %d", len(itog.Razresheno), predelIsklyucheniyReklamy))
		itog.Razresheno = itog.Razresheno[:predelIsklyucheniyReklamy]
	}
	if !itog.Vkl && len(itog.Razresheno) == 0 && u == reklama.Bazovyy {
		return nil, pochinki
	}
	return itog, pochinki
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
