package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/netip"
	"strconv"
	"sync"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/genkonfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/hranenie"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/set"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/sostoyanie"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/ssylki"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/yadra"
)

// Константы целей больше нет: сервер берётся из списка (задача 3.8). Пока она
// была, критерий «сервер добавляется ссылкой» был недостижим физически, потому
// что разобранному protokol.Server было некуда лечь.
const (
// Ядро одно, и его имя с путём живут в tunnel.go рядом с подъёмом туннеля.
// Прежде здесь стояли имя и путь конфига ВТОРОГО ядра (Xray), потому что волне 1
// нужен был процесс, ОТДАЮЩИЙ SOCKS. Второго ядра больше нет (П5).
)

// How long connect waits for the first successful probe before giving up. Longer
// than the 4 second threshold on purpose: the threshold is what we promise, this
// is when we stop hoping.
// A var, not a const, and only so the test can shorten it. A test that honestly
// waits fifteen seconds is a test that gets commented out in a month.
var zhdatPodyoma = 15 * time.Second

// popytokPodyoma это число стартов ядра за один connect (см. Connect).
var popytokPodyoma = 2

// Значения по умолчанию для расписания наблюдения. Они КОПИРУЮТСЯ в поля
// службы, а не читаются из горутин напрямую.
//
// Раньше это были var, которые тест правил у себя и возвращал в defer. Горутина
// наблюдателя при этом переживала свой тест и читала их, пока следующий писал:
// детектор гонок назвал это гонкой в тестах, но общая изменяемая переменная,
// которую читает фоновая горутина, это дефект устройства, а не теста.
const periodNablyudeniyaPoUmolchaniyu = yadra.PeriodProby

// Сколько ждать выбора у группы, которая на подъёме его ещё не назвала, и с
// каким шагом переспрашивать. Замерено 06.09.2026 на живом ядре 1.14.0-rc.5:
// через 1.23 с после подъёма now у группы avto пуст, через 5.63 с назван.
// Двадцать секунд это запас втрое; шаг в полсекунды дёшев, потому что вопрос
// уходит на 127.0.0.1, а не наружу.
// Как часто спрашивать ЯДРО, кто несёт. Отдельно от периода пробы живости:
// вопрос уходит на 127.0.0.1 и стоит копейки, а проба идёт наружу и стоит
// трафика. Пока оба сидели на одном тике в тридцать секунд, поле несущего
// после переключения называло прежний сервер ровно этот период (замерено
// 06.09.2026 трижды: 26.8, 29.5 и 31.1 с).
const periodNesushchegoPoUmolchaniyu = 2 * time.Second

const srokDosprosaPoUmolchaniyu = 20 * time.Second
const shagDosprosaPoUmolchaniyu = 500 * time.Millisecond

// Сколько провалов подряд считаем смертью туннеля. Один это норма жизни на
// мобильной сети, и рвать по нему связь значит рвать её на ровном месте.
const provalovPodryadPoUmolchaniyu = 2

// Отступы между попытками вернуться после аварии. Растущие, а не постоянные:
// постоянный отступ на мёртвой сети это ddos самого себя и съеденная батарея.
// Последний повторяется, пока сеть не вернётся; потолка попыток нет намеренно,
// зато каждая попытка попадает в журнал: молчаливый бесконечный цикл хуже
// отказа. Var ради теста.
var otstupyPoUmolchaniyu = []time.Duration{
	time.Second, 2 * time.Second, 5 * time.Second, 10 * time.Second, 30 * time.Second,
}

type Sluzhba struct {
	mu          sync.Mutex
	speed       *speedJob
	speedRunner speedRun
	// zhurnalKomand пишет строку на каждую команду: имя, кто прислал, исход.
	// nil значит «молчим»: тесты и стенд создают службу без журнала. Тел в нём
	// нет НИКОГДА, там ключи (см. zhurnalKomand.go).
	zhurnalKomand *log.Logger
	// Статистика (6.1): подписчики по id соединения, отмена опроса, период и
	// снимок подставляются тестами. adresVyhoda заполняет checkExitIp (6.3).
	statPodp    map[uint64]bool
	statOtmena  context.CancelFunc
	periodStat  time.Duration
	snimokStat  func(ctx context.Context, adres, sekret, teg string) (yadra.Snimok, error)
	adresVyhoda string
	// Проверки 6.3: эндпоинт и два шва для сети и брандмауэра.
	adresProverki string
	sprositVyhod  func(ctx context.Context, endpoint string, portProksi int) (string, error)
	ipv6Zaglushen func() (bool, error)
	// Обновление (6.5): запуск подменщика подставляется тестами.
	zapustitPodmenshchika func(prog, novaya string) error
	// Каталог данных: исход обновления читается оттуда; тесты подставляют свой.
	dirDannyh    string
	dirProgrammy string
	// Снимок сервера (6.4): загрузчик подставляется тестами.
	zagruzitSnimok func(ctx context.Context, adres string) (set.Snimok, error)
	// Журнал соединений (6.2): файл, период опроса и снимок соединений
	// подставляются тестами.
	zhurnalSoed      *sostoyanie.ZhurnalSoedineniy
	periodZhurnala   time.Duration
	soedineniyaYadra func(ctx context.Context, adres, sekret string) ([]yadra.Soedinenie, error)
	// muVybor держит ВЕСЬ цикл живого переключения: чтение текущего выбора,
	// PUT, проба, откат, запись набора. Отдельный от mu намеренно: под mu
	// отвечают Status и izvestit, и держать его через два похода в ядро значит
	// подвесить интерфейс на время переключения.
	//
	// ПРАВИЛО, без которого второй замок это взаимная блокировка: muVybor
	// берётся ПЕРВЫМ и никогда не берётся под mu.
	muVybor sync.Mutex
	sost    protokol.Sostoyanie
	// vybranId это то, что человек выбрал сам, и в авто он пуст по построению.
	// nesushchiyId это то, что реально несёт трафик. Ш7-5 научит службу
	// спрашивать второе у ядра, а не переписывать собственное намерение.
	vybranId       string
	nesushchiyId   string
	nesushchiyImya string
	// Считать режим только в sobratTun нельзя: Status отвечает с первого кадра,
	// задолго до первого подъёма, и на провод уехала бы пустая строка, то есть
	// третье значение у типа с двумя.
	rezhim protokol.Rezhim
	// Доступ к clash_api живущего ядра. Рождается в sobratTun на каждый старт и
	// нужен пробе ПРЯМО СЕЙЧАС, а не следующему запуску: в файл состояния он
	// пишется для восстановления после смерти службы, и читать оттуда данные
	// собственного живого подключения значило бы ходить кругом через диск.
	portClash int
	// Порт нашего входа mixed. Ноль означает, что прокси мы не поднимали:
	// сторож при этом считает чужим любой включённый системный прокси, как и до
	// появления входа.
	portProksiNash int
	sekretClash    string
	podnyatS       *time.Time
	oshib          *protokol.Oshibka
	otmena         context.CancelFunc
	// pokolenieP это счётчик поколений подъёма, и он существует потому, что
	// отменять подъём контекстом НЕ РАБОТАЕТ.
	//
	// Цикл проб смотрел ctx команды, то есть контекст СОЕДИНЕНИЯ: его не
	// отменяет ни одна команда, он живёт, пока живёт труба. Отменять было
	// нечего и вторым способом: s.otmena к этому моменту уже забрал disconnect,
	// а следующая попытка строила себе новый контекст от Background и поднимала
	// туннель заново. Человеку отвечали «выключено», и через пятнадцать секунд
	// туннель поднимался сам.
	//
	// Счётчик решает это без контекстов вовсе: подъём запоминает своё поколение
	// на входе и на каждом шаге сверяет. Изменилось значит отключили, и подъём
	// выходит, не трогая состояние: его уже поставил тот, кто отключал.
	pokolenieP     int
	pravilaKonfiga string
	trafikKonfiga  protokol.MarshrutTrafika
	oshibkaIPv6    error
	// ostanovlena это конец жизни службы, и живёт он под тем же замком, что и
	// поколения подъёма, потому что стережёт то же самое: регистрацию горутин.
	//
	// sync.WaitGroup запрещает Add из нуля ОДНОВРЕМЕННО с Wait, и обе группы
	// службы это нарушали. Наблюдатель заводил восстановление (s.fon.Add) уже
	// после того, как Zavershit прошёл s.fon.Wait(); та горутина звала Connect,
	// тот доходил до s.nabl.Add(1), а Disconnect остановки в это время сидел на
	// s.nabl.Wait(). Детектор гонок называл это записью Zavershit против чтения
	// Connect.func4, и найти по такой подписи причину без счётчика в руках
	// нельзя.
	//
	// Флаг ставится ПОД ЗАМКОМ и раньше любого Wait, а обе точки регистрации
	// берут тот же замок. Поэтому всякая удавшаяся регистрация случилась строго
	// до флага, то есть строго до Wait, а всякая опоздавшая отвергнута.
	ostanovlena bool
	// nabl держит наблюдателя. Disconnect обязан ДОЖДАТЬСЯ его, а не только
	// отменить: отменённая горутина живёт ещё столько, сколько ей нужно
	// дойти до select, и всё это время она читает поля службы и общие
	// переменные. Детектор гонок нашёл это первым же прогоном.
	nabl sync.WaitGroup
	// Расписание наблюдения и восстановления живёт В СЛУЖБЕ, а не в пакете:
	// иначе тест, правящий его у себя, правит его и у чужой фоновой горутины.
	// snimok это ПОСЛЕДНЕЕ записанное состояние файла. Без него каждая запись
	// шла полным снимком и затирала предыдущую: подъём туннеля клал порт
	// clash_api, секрет и адаптер, а следующая запись, сидящая на УСПЕШНОМ
	// пути, стирала всё это обратно. Восстановление после нештатной смерти
	// службы оставалось без данных ровно тогда, когда они нужны.
	snimok sostoyanie.SostoyanieFayla

	period time.Duration
	// Срок и шаг доспроса несущего, см. srokDosprosaPoUmolchaniyu. Поля, а не
	// пакетные переменные: тест укорачивает их у СВОЕЙ службы, не задевая чужую.
	// Период опроса несущего, см. periodNesushchegoPoUmolchaniyu.
	periodNesushchego time.Duration
	srokDosprosa      time.Duration
	shagDosprosa      time.Duration
	provalov          int
	otstupy           []time.Duration
	// Период опроса системного прокси. Поле, а не пакетная переменная: см.
	// periodProksiPoUmolchaniyu.
	periodProksi time.Duration
	// Судья собранного конфига. Шов, потому что настоящий судья это отдельный
	// процесс ядра, а проверяется здесь НАШЕ поведение при его отказе.
	proveritKonfig func(putKonfiga string) error
	// Восстановление это ВТОРАЯ фоновая горутина, и до сегодня её не отменял и
	// не ждал никто. Она переживала и отключение, и остановку службы, а внутри
	// зовёт Connect, то есть умеет поднять туннель после того, как его опустили.
	//
	// Два уровня отмены, потому что причин две. fonOtmena это конец жизни
	// службы. otmenaVosst это решение человека отключиться: оно обязано
	// останавливать восстановление СРАЗУ, а не после текущего отступа, который
	// на последних попытках достигает минуты.
	fon         sync.WaitGroup
	fonCtx      context.Context
	fonOtmena   context.CancelFunc
	otmenaVosst context.CancelFunc

	podp   map[uint64]chan protokol.Kadr
	sledId uint64

	// Seams. Not for elegance: without them the only way to test a state machine
	// is to own a hypervisor, and a state machine nobody tests is where the app
	// learns to lie about being connected.
	storozhit func(ctx context.Context, imya, konfig string, sob func(protokol.Sostoyanie)) error
	// Замер идёт у ТОГО ядра, которое ведёт трафик, по ТОМУ исходящему, которым
	// он пойдёт. Прежняя проба ходила в локальный SOCKS второго ядра, то есть
	// мимо туннеля, и на сервере, не несущем ничего, отвечала успехом.
	zamerit func(ctx context.Context, adres, sekret, teg string) (time.Duration, error)
	// Несущего называет ЯДРО, а не наше намерение. Два имени групп параметрами:
	// пакет yadra не знает про генератор конфига и знать не должен.
	nesyot func(ctx context.Context, adres, sekret, gruppa, tegAvto string) (string, error)
	// Швы живого переключения. Оба сетевые, и оба ОБЯЗАНЫ быть заглушены в
	// фикстуре: она кладёт zapomnitKlash(52715, ...), и незаглушенный шов уходит
	// стучаться на этот порт по-настоящему.
	postavitVybor func(ctx context.Context, adres, sekret, gruppa, teg string) error
	vyborGruppy   func(ctx context.Context, adres, sekret, gruppa string) (string, error)
	zhdatKlash    func(ctx context.Context, adres, sekret string) error
	zapisat       func(sostoyanie.SostoyanieFayla) error
	// Волна 2. Шов на ВЕСЬ подъём туннеля, а не на ожидание адаптера внутри
	// него. Разница практическая: подъём ещё и пишет конфиг в каталог данных и
	// выдаёт порт, и тест с узким швом молча гадил бы в живой ProgramData.
	podnyatTunnel func(ctx context.Context) (set.Adapter, error)

	// Проверка обновлений (план «шесть удобств» §5): адрес каталога с
	// versiya.json и архивами, загрузчик (подставляется тестами), находка.
	adresObnovleniy     string
	skachatFayl         func(ctx context.Context, adres string, predel int64) ([]byte, error)
	obnovlenie          *protokol.ObnovlenieOtvet
	obnovlenieProvereno *time.Time
	// Правило IPv6 живёт ровно столько же, сколько туннель, и НЕ зависит от
	// режима. Шов нужен не для красоты: без него тест опускания идёт заводить
	// настоящее правило в брандмауэре машины разработчика.
	glushitIPv6     func() error
	vernutIPv6      func() error
	vklyuchitVes    func(set.Razreshyonnoe, bool) error
	prochitatProksi func() (set.Proksi, error)
	vyklyuchitVes   func() error
	// Узкая пересборка ОДНОГО правила, списка адресов серверов. Отдельный шов
	// от vklyuchitVes, потому что зовётся ровно там, где тот бессилен: ядро
	// умерло, адреса TUN нет, набор целиком не собрать.
	suzitServery func([]netip.Addr) error

	// killSwitch это НЕ производная от состояния брандмауэра: система может
	// быть заперта чужим правилом, а мы про это ничего не знаем. Здесь только
	// то, что включили мы сами.
	killSwitch    bool
	zaslonAktiven bool
	muZaslon      sync.Mutex

	// Адаптер туннеля, известен только после подъёма. На его индексе держится
	// поиск канала ПОД туннелем, на алиасе порядок правил задачи 2.5.
	tun set.Adapter

	// Хранилище секретов. Шов, потому что настоящее пишет в ProgramData, и тест
	// без шва гадил бы в живой каталог машины разработчика.
	sekretyChitat func() ([]byte, error)
	sekretyPisat  func([]byte) error

	// Набор серверов. Шов, потому что настоящий ходит в хранилище секретов, а
	// тесту нужен список без DPAPI и без ProgramData.
	nabor func() (Nabor, error)

	// Запись конфига второго ядра. Шов, потому что настоящая пишет в живой
	// C:\ProgramData, а тест не должен трогать машину, на которой идёт.

	// Загрузка подписки. Шов, потому что настоящая ходит в сеть, а тест обязан
	// уметь показать и истекшую подписку, и недоступную, не поднимая сервера.
	zagruzitPodpisku func(ctx context.Context, adres string) (ssylki.Razbor, error)

	// Адреса для правила петли И для разрешающих правил брандмауэра. ОДИН
	// источник на оба списка: два независимых сборщика неизбежно разошлись бы,
	// и разошлись бы тихо. Адрес в петле, но не в брандмауэре, даёт неработающий
	// туннель в запертом режиме; адрес в брандмауэре, но не в петле, даёт петлю.
	//
	// Шов ещё и потому, что настоящий сборщик ходит в системный резолвер, а тест
	// не должен зависеть от чужого DNS.
	sobratAdresa func() ([]netip.Addr, error)
	// Сам сборщик по спискам. Шов, чтобы тест мог увидеть, ЧТО ему ушло
	// (адреса наборов), не трогая резолвер машины.
	sobratAdresaSet func(servery []protokol.Server, podpiska string, zagruzki ...string) ([]netip.Addr, error)

	// Наборы rule_set. Желаемые: шов, потому что настоящие лежат в ProgramData.
	// Загрузка: шов, потому что настоящая ходит в сеть.
	naboryZhelaemye func() []genkonfig.NaborPravil
	skachatNabor    func(ctx context.Context, adres string) ([]byte, error)

	// podRezhim поднят только на время перезапуска, затеянного включением
	// режима «весь трафик». Живёт под тем же замком, что и killSwitch.
	podRezhim bool

	seychas   func() time.Time
	zhdat     func(ctx context.Context, d time.Duration) bool
	prochitat func() (sostoyanie.SostoyanieFayla, error)
}

func NovayaSluzhba() *Sluzhba {
	s := &Sluzhba{
		sost:      protokol.SostVyklyuchen,
		podp:      map[uint64]chan protokol.Kadr{},
		statPodp:  map[uint64]bool{},
		storozhit: yadra.Storozhit,
		zapisat:   sostoyanie.Zapisat,
	}
	s.period = periodNablyudeniyaPoUmolchaniyu
	s.periodNesushchego = periodNesushchegoPoUmolchaniyu
	s.srokDosprosa = srokDosprosaPoUmolchaniyu
	s.shagDosprosa = shagDosprosaPoUmolchaniyu
	s.periodStat = periodStatPoUmolchaniyu
	s.snimokStat = yadra.Statistika
	s.adresProverki = set.AdresProverkiPoUmolchaniyu
	s.sprositVyhod = set.AdresVyhoda
	s.ipv6Zaglushen = set.PravilaIPv6Est
	s.zapustitPodmenshchika = zapustitPodmenshchikaVTemp
	s.dirDannyh = sostoyanie.KatalogDannyh()
	s.dirProgrammy = sostoyanie.KatalogProgrammy()
	s.zagruzitSnimok = set.ZagruzitSnimok
	s.adresObnovleniy = AdresObnovleniyPoUmolchaniyu
	s.skachatFayl = skachatPoSeti
	s.zhurnalSoed = sostoyanie.NovyyZhurnalSoedineniy()
	s.periodZhurnala = periodZhurnalaPoUmolchaniyu
	s.soedineniyaYadra = yadra.Soedineniya
	s.provalov = provalovPodryadPoUmolchaniyu
	s.otstupy = otstupyPoUmolchaniyu
	s.periodProksi = periodProksiPoUmolchaniyu
	s.proveritKonfig = func(put string) error { return yadra.Proverit(imyaYadraTun, put) }
	s.fonCtx, s.fonOtmena = context.WithCancel(context.Background())
	s.speedRunner = runSpeed
	s.glushitIPv6 = set.GlushitIPv6
	s.vernutIPv6 = set.VernutIPv6
	s.vklyuchitVes = set.VklyuchitVesTrafik
	s.prochitatProksi = set.SistemnyyProksi
	s.vyklyuchitVes = set.VyklyuchitVesTrafik
	s.suzitServery = set.PerezavestiRazreshyonnyeServery
	// Значение метода берётся после создания: раньше её просто не у чего взять.
	s.podnyatTunnel = s.podnyatTunSistemno
	hr := hranenie.Novyy()
	s.sekretyChitat = hr.Zagruzit
	s.sekretyPisat = hr.Sohranit
	s.sobratAdresa = s.adresaKandidatov
	s.sobratAdresaSet = set.SobratAdresa
	s.naboryZhelaemye = naboryIzUmolchaniy
	s.skachatNabor = skachatNaborPoSeti
	s.nabor = s.naborIzHranilishcha
	// Читаем набор ПРИ СТАРТЕ, а не ждём первой записи. Поведение считает
	// rezhimNabora по набору из хранилища, и одного умолчания в конструкторе
	// мало: после перезапуска с сохранённым Vybran провод сказал бы «авто», а
	// конфиг собрался бы ручным. Отказ чтения (хранилище заперто, DPAPI не
	// отдал) это не повод падать: набор всё равно недоступен, серверов ноль, и
	// умолчание тут честнее пустоты.
	s.rezhim = rezhimPoUmolchaniyu
	if n, err := s.naborIzHranilishcha(); err == nil {
		s.rezhim = rezhimNabora(n)
	}
	// Пять попыток: служба стартует Automatic, то есть раньше, чем поднимается
	// сеть, и первая загрузка подписки почти всегда приходится на этот момент.
	zagr := ssylki.NovyyZagruzchik()
	s.zagruzitPodpisku = func(ctx context.Context, adres string) (ssylki.Razbor, error) {
		return zagr.ZagruzitSPovtorami(ctx, adres, 5)
	}
	s.seychas = time.Now
	s.zhdat = zhdatPoChasam
	s.prochitat = sostoyanie.Prochitat
	s.zagruzitNastroyki()
	s.zamerit = yadra.Zaderzhka
	s.nesyot = yadra.Nesyot
	s.postavitVybor = yadra.PostavitVybor
	s.vyborGruppy = yadra.VyborGruppy
	s.zhdatKlash = yadra.ZhdatKlash
	return s
}

func (s *Sluzhba) Podpisatsya() (uint64, <-chan protokol.Kadr) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sledId++
	id := s.sledId
	// Buffered: a subscriber that stopped reading is a UI that is gone, and the
	// service must not block on its ghost.
	c := make(chan protokol.Kadr, 32)
	s.podp[id] = c
	return id, c
}

func (s *Sluzhba) Otpisatsya(id uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c, est := s.podp[id]; est {
		delete(s.podp, id)
		close(c)
	}
	// Смерть соединения это отписка от статистики: интерфейс, убитый
	// диспетчером задач, сам отписаться не успевает.
	delete(s.statPodp, id)
	s.pogasitOprosStat()
}

func (s *Sluzhba) izvestit(imya string, telo any) {
	syroe, err := json.Marshal(telo)
	if err != nil {
		return
	}
	k := protokol.Kadr{Tip: "sobytie", Imya: imya, Telo: syroe}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.podp {
		select {
		case c <- k:
		default:
		}
	}
}

// The service owns the state. The UI never computes it from parts, because two
// places computing the same truth is how they start disagreeing.
func (s *Sluzhba) postavit(n protokol.Sostoyanie, oshib *protokol.Oshibka) {
	s.mu.Lock()
	if s.sost != n {
		s.cancelSpeedLocked("Подключение изменилось. Запустите замер заново.")
	}
	s.sost = n
	s.oshib = oshib
	if n == protokol.SostPodnyat && s.podnyatS == nil {
		t := time.Now()
		s.podnyatS = &t
	}
	if n == protokol.SostVyklyuchen {
		s.podnyatS = nil
	}
	s.mu.Unlock()
	s.izvestit("state", s.Status())
	// Адрес выхода по подъёму, не по таймеру (§5).
	if n == protokol.SostPodnyat {
		go s.obnovitAdresVyhoda()
	}
}

func (s *Sluzhba) Status() protokol.StatusOtvet {
	s.mu.Lock()
	defer s.mu.Unlock()
	return protokol.StatusOtvet{
		Sostoyanie:          s.sost,
		TrafikPoUmolchaniyu: s.trafikKonfiga,
		VybranId:            s.vybranId,
		NesushchiyId:        s.nesushchiyId,
		NesushchiyImya:      s.nesushchiyImya,
		RezhimMarshruta:     s.rezhim,
		VersiyaSluzh:        fmt.Sprintf("%d", protokol.Versiya),
		PodnyatS:            s.podnyatS,
		KillSwitch:          s.killSwitch,
		PortProksi:          s.portProksiNash,
		// Read from the registry every time, never cached: the Run key is the
		// only truth, and a cached flag would disagree with a key edited by hand.
		Avtozapusk:           avtozapuskVklyuchen(),
		PodklyuchatPriStarte: s.snimok.PodklyuchatPriStarte,
		Zhurnal:              s.snimok.Zhurnal,
		PolosaVverh:          s.snimok.PolosaVverh,
		PolosaVniz:           s.snimok.PolosaVniz,
		Oshib:                s.oshib,
		VersiyaProgrammy:     versiyaDlyaEkrana(),
		Obnovlenie:           s.obnovlenie,
		ObnovlenieProvereno:  s.obnovlenieProvereno,
	}
}

// zagruzitNastroyki читает с диска то, что является НАСТРОЙКОЙ, а не
// состоянием: без этого первая же запись snimok затёрла бы флаг нулём.
// Отказ чтения это первый запуск или запертый каталог, умолчание честнее.
func (s *Sluzhba) zagruzitNastroyki() {
	f, err := s.prochitat()
	if err != nil {
		return
	}
	s.mu.Lock()
	s.snimok.PodklyuchatPriStarte = f.PodklyuchatPriStarte
	s.snimok.KillSwitch, s.killSwitch = f.KillSwitch, f.KillSwitch
	s.snimok.Zhurnal = f.Zhurnal
	s.snimok.PolosaVverh, s.snimok.PolosaVniz = f.PolosaVverh, f.PolosaVniz
	if f.AdresProverki != "" {
		s.adresProverki = f.AdresProverki
	}
	s.mu.Unlock()
}

// SetConnectOnStart запоминает второе решение (§9.2, задача 4.8).
//
// Отдаёт отказ записи наружу: флаг живёт ТОЛЬКО в файле, и незаписанный флаг
// это настройка, которой не существует. Отвечать успехом на неё значит обещать
// подъём при старте, которого не будет.
func (s *Sluzhba) SetConnectOnStart(vkl bool) error {
	return s.pravitSost(func(f *sostoyanie.SostoyanieFayla) { f.PodklyuchatPriStarte = vkl })
}

// SetPolosa запоминает полосу канала человека в мегабитах.
//
// Пара нулей это законное «снять объявление»: без полосы hysteria2 идёт на BBR,
// и это рабочее состояние, а не поломка. Всё остальное проверяется ЗДЕСЬ, а не
// в генераторе: там отказ означает несобранный конфиг всего туннеля, то есть
// человек увидит «туннель не поднялся» вместо «в поле полосы ерунда».
//
// Потолок стоит намеренно. Brutal гонит трафик РОВНО с объявленной скоростью,
// поэтому опечатка в поле это не косметика: замер 05.09.2026 на полке 20 Мбит
// при объявленных 440 дал восемь провалов и p95 3214 мс против нуля и 92 мс у
// честного объявления.
const PotolokPolosy = 10000

func (s *Sluzhba) SetPolosa(vverh, vniz int) error {
	switch {
	case vverh == 0 && vniz == 0:
	case vverh <= 0 || vniz <= 0:
		return fmt.Errorf("полоса задаётся парой положительных чисел либо снимается парой нулей: вверх %d, вниз %d", vverh, vniz)
	case vverh > PotolokPolosy || vniz > PotolokPolosy:
		return fmt.Errorf("полоса больше %d Мбит: вверх %d, вниз %d", PotolokPolosy, vverh, vniz)
	}
	return s.pravitSost(func(f *sostoyanie.SostoyanieFayla) {
		f.PolosaVverh, f.PolosaVniz = vverh, vniz
	})
}

// popytokPriStarte: служба стартует Automatic, раньше сети, и первый подъём
// почти наверняка упрётся в отсутствие маршрута. Шесть попыток по десять
// секунд это минута, за которую сеть поднимается на любой машине.
const popytokPriStarte = 6

// PodklyuchitPriStarte поднимает туннель при старте службы, если человек это
// выбрал. Выключенный флаг означает ровно ничего: туннель, поднявшийся сам
// без просьбы, это самый неприятный сюрприз у VPN.
func (s *Sluzhba) PodklyuchitPriStarte(ctx context.Context) {
	s.mu.Lock()
	vkl := s.snimok.PodklyuchatPriStarte
	// Замок ПОДРАЗУМЕВАЕТ туннель, и это не удобство, а единственный выход.
	// Запертая машина без туннеля не имеет связи вовсе: правила пускают наружу
	// только ядро, а его нет. Ф1 от 05.09.2026, замерено в госте: после
	// убийства службы SCM вернул её, замок помнился, флаг был выключен, и
	// машина осталась без сети навсегда.
	//
	// Самовольный подъём на НЕЗАПЕРТОЙ машине по-прежнему запрещён: там он
	// сюрприз, а здесь возврат отнятого.
	zaperta := s.zaslonAktiven
	s.mu.Unlock()
	if !vkl && !zaperta {
		return
	}
	for i := 0; i < popytokPriStarte; i++ {
		err := s.Connect(ctx)
		if err == nil {
			log.Printf("туннель поднят при старте, попытка %d", i+1)
			return
		}
		log.Printf("подъём при старте, попытка %d: %v", i+1, err)
		if i < popytokPriStarte-1 && !s.zhdat(ctx, 10*time.Second) {
			return
		}
	}
}

// StatusS отдаёт статус с РАЗОВОЙ ошибкой вместо запомненной.
//
// Нужна там, где сбой надо показать человеку, но не запоминать: postavit
// переписал бы s.sost и затёр чужую ошибку, а Status() положить разовую ошибку
// нечем. Замок берётся и отпускается внутри Status, поэтому вызывать её из-под
// s.mu так же нельзя.
func (s *Sluzhba) StatusS(oshib *protokol.Oshibka) protokol.StatusOtvet {
	st := s.Status()
	if oshib != nil {
		st.Oshib = oshib
	}
	return st
}

// tegDlyaZamera отвечает, ЧТО именно мы спрашиваем у ядра про здоровье пути.
//
// Группу, а не кандидата. Тег кандидата берётся один раз при подъёме и живёт
// всё подключение, а выбор внутри группы может смениться без нас. Замер по
// группе уходит через её текущий выбор: замерено на живом ядре, задержка
// приходит числом и падает в 503, когда выбранный кандидат мёртв.
//
// Функция, а не константа на месте вызова: так подмена аргумента видна мутацией.
func tegDlyaZamera() string { return genkonfig.TegSelector }

func (s *Sluzhba) Connect(ctx context.Context) error {
	return s.connect(ctx, nil)
}

func (s *Sluzhba) connect(ctx context.Context, expected *int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	sost := s.sost
	s.mu.Unlock()

	// Отказ это НЕ занятое состояние, это провалившаяся попытка, и повторную
	// обязаны принять. Замерено на стенде 01.09.2026: импорт профиля с неверным
	// паролем, connect, честный отказ «серверов нет», затем импорт с верным
	// паролем и connect, который ответил «туннель уже в состоянии otkaz».
	// Серверы к тому моменту были на месте. Лечилось только перезапуском службы,
	// то есть человек, у которого один раз не поднялось, оставался с кнопкой,
	// которая больше ничего не делает.
	//
	// Уборка перед повторной попыткой это ВТОРОЙ барьер, а не необходимость, и
	// названо это так по факту проверки: каждая ветка отказа, наступающая после
	// запуска ядер, уже зовёт Disconnect сама, а ранние ветки отказывают до
	// того, как появляется что убирать. Мутационный прогон это подтвердил, сняв
	// вызов без единого красного теста. Барьер оставлен потому, что веток отказа
	// шесть, растут они по одной, и достаточно седьмой забыть про уборку, чтобы
	// повторный подъём пошёл поверх живого сторожа и второй пары ядер.
	// ne-neset здесь наравне с otkaz, и по той же причине: это провалившаяся
	// попытка, а не занятое состояние. Без него автовосстановление невозможно
	// физически: наблюдатель ставит именно ne-neset.
	if expected == nil && (sost == protokol.SostOtkaz || sost == protokol.SostNeNeset) {
		s.Disconnect()
	}

	// Проверка и переход ПОД ОДНИМ замком. Прежде состояние читалось, замок
	// отпускался, и только потом начиналась работа: человек жал «подключить»
	// ровно тогда, когда срабатывало автовосстановление, обе горутины видели
	// vyklyuchen и обе шли поднимать. Два ядра, два TUN-адаптера, два набора
	// правил брандмауэра, и вдобавок второй вызывающий затирал s.otmena
	// первого, после чего первый наблюдатель становился неубиваемым.
	//
	// Само по себе это старый код, который никто не правил. Дефектным он стал в
	// тот день, когда у Connect появился второй вызывающий.
	s.mu.Lock()
	// A command names the state the human wants, and podnyat/podnimaetsya are
	if s.ostanovlena || ctx.Err() != nil || (expected != nil && s.pokolenieP != *expected) {
		s.mu.Unlock()
		return errPodyomOtmenyon
	}
	// that state already reached. Answering with an error there is not honesty,
	// it is a red banner over a working tunnel: the dispatcher had no code to
	// put on such an error, so it fell through to all-servers-down and blamed
	// servers that were carrying traffic at that very moment.
	//
	// The other busy states stay errors on purpose. otkaz and ne-neset are
	// handled above by Disconnect and fall through to a real second attempt,
	// and vosstanavlivaetsya means another goroutine is already lifting: two
	// lifts at once are two cores, two adapters and two rule sets.
	if s.sost == protokol.SostPodnyat || s.sost == protokol.SostPodnimaetsya {
		s.mu.Unlock()
		return nil
	}
	if s.sost != protokol.SostVyklyuchen {
		zanyato := s.sost
		s.mu.Unlock()
		return fmt.Errorf("туннель уже в состоянии %s", zanyato)
	}
	s.sost = protokol.SostPodnimaetsya
	s.cancelSpeedLocked("Началось подключение к VPN. Запустите замер после подключения.")
	// Поколение берётся ЗДЕСЬ, под тем же замком, что и переход в podnimaetsya.
	// Это точка, с которой отключение нас уже видит: раньше её отключать было
	// нечего, позже осталось бы окно, в котором disconnect уже прошёл, а мы про
	// это ещё не знаем.
	moyo := s.pokolenieP
	s.mu.Unlock()

	// Сервер берётся из списка ДО всего остального: без него подниматься некуда,
	// и узнать это лучше до того, как выставлено состояние «поднимается».
	n, err := s.nabor()
	if err != nil {
		log.Printf("подключение: набор серверов не прочитан: %v", err)
		s.postavit(protokol.SostOtkaz, &protokol.Oshibka{
			Kod: protokol.KodSecretsUnreadable, Tekst: err.Error()})
		return err
	}
	srv, err := n.VybrannyyServer()
	if err != nil {
		s.postavit(protokol.SostOtkaz, &protokol.Oshibka{
			Kod: protokol.KodSelectedServerGone, Tekst: "серверов нет: добавь сервер ссылкой или подпиской"})
		return err
	}

	nach := time.Now()
	// podnimaetsya is not decoration: the UI draws a different screen for it, and
	// jumping straight to podnyat makes the app look frozen for four seconds.
	s.postavit(protokol.SostPodnimaetsya, nil)

	s.mu.Lock()
	// Только ВЫБОР, и только из n.Vybran как есть. srv это VybrannyyServer(), а
	// он при пустом Vybran отдаёт Servery[0]: для подъёма это верно, человеку с
	// одним сервером не надо сначала его «выбирать», но записать это же в поле
	// выбора значило бы назвать сервер, которого человек не выбирал.
	//
	// Несущего здесь НЕ пишем. Ш7-1 писала его отсюда, до всякого опроса ядра, и
	// при отказе опроса поле показывало бы Servery[0] с той же уверенностью, что
	// и при живом ядре. Несущего называет ядро, ниже, после удачной пробы.
	s.vybranId = n.Vybran
	s.mu.Unlock()

	// Отказ записи файла НЕ роняет подъём: файл нужен восстановлению после
	// нештатной смерти службы, а туннель человеку нужен сейчас. Но молчать о
	// нём нельзя, иначе восстановление однажды не найдёт данных и объяснить это
	// будет нечем.
	if err := s.pravitSost(func(f *sostoyanie.SostoyanieFayla) {
		f.Sostoyanie, f.ConnectNach = protokol.SostPodnimaetsya, &nach
	}); err != nil {
		log.Printf("файл состояния не записан на начале подъёма: %v", err)
	}

	// Две попытки, а не одна. 03.09.2026 дважды за день connect отвечал
	// all-servers-down через 15 с, а через минуты та же запись поднималась за
	// секунду; причина не поймана. Человек видит это как «не работает».
	// Вторая попытка это перезапуск ядра, не ещё одна проба того же: если
	// беда в ядре или в первом рукопожатии, свежий старт её лечит, а если в
	// сети, вторые 15 с ничего не стоят. Причина последней пробы теперь
	// пишется и в журнал, и в текст отказа: прежде она терялась.
	var poslednyaya error
	for popytka := 1; popytka <= popytokPodyoma; popytka++ {
		// Состояние службы в начале попытки прежде не перечитывалось НИ РАЗУ, и
		// это половина дефекта: вторая попытка строила себе новый контекст от
		// Background и поднимала туннель, которого уже никто не просил.
		if s.podyomOtmenyon(moyo) {
			return errPodyomOtmenyon
		}
		vnutr, otmena := context.WithCancel(context.Background())
		s.mu.Lock()
		s.otmena = otmena
		s.mu.Unlock()

		// Слежение за чужим прокси живёт столько же, сколько подключение: вне его
		// предупреждать не о чем, туннеля всё равно нет.
		go s.slediZaProksi(vnutr)

		// Туннель поднимается ДО замера, и это обратный порядок к прежнему.
		//
		// Прежний порядок (проба, потом TUN) защищал от состояния ne-neset: туннель,
		// заворачивающий трафик в ядро, которое никуда не дозвонилось. Защита была
		// настоящей, но куплена ценой, которую заметили только на чужих серверах:
		// пробовать ДО подъёма умеет лишь тот путь, что идёт мимо туннеля, а он
		// меряет не то, чем трафик пойдёт. Теперь от ne-neset защищает не порядок, а
		// откат: замер идёт по живому туннелю, и его провал туннель опускает.
		a, err := s.podnyatTunnel(vnutr)
		if err != nil {
			// Отмена это НЕ отказ подъёма, и спрашиваем мы об этом ДО того, как
			// выдумывать диагноз. Полоса Б убрала выдуманный диагноз из ОТВЕТА
			// команды connect и до состояния не дошла: ответ был честный, а
			// экран после него всё равно показывал «TUN-адаптер не появился».
			// Человек нажал «отключить» и получил за своё же действие красный
			// экран. Замерено раундом 3 04.09.2026, судья отмены.
			//
			// Уборка та же, что и у отказа: подъём адаптера не удался, значит
			// за собой могло остаться недоделанное, и Disconnect убирает это
			// целиком. Разница ровно в одном: состояние не трогаем, его уже
			// поставил тот, кто отменял.
			if s.podyomOtmenyon(moyo) {
				log.Printf("подключение отменено, пока поднимался туннель: %v", err)
				s.Disconnect()
				return errPodyomOtmenyon
			}
			log.Printf("подключение: туннель не поднялся: %v", err)
			s.Disconnect()
			s.postavit(protokol.SostOtkaz, &protokol.Oshibka{
				Kod: kodPodyomaTunnelya(err), Tekst: err.Error()})
			return err
		}
		// Отключение, успевшее ПОКА поднимался туннель. Проверка стоит до
		// записи файла и до glushitIPv6 намеренно: opustitYadro отключавшего уже
		// прошёл, его vernutIPv6 отработал, и правило, заведённое следующей
		// строкой, осталось бы висеть без туннеля до следующего подъёма.
		// Замерено тестом: без этой проверки правил оставалось два.
		//
		// Убираем за собой САМИ: адаптер и ядро наши, отключавший про них не
		// знал. Состояние при этом не трогаем, его уже поставил он.
		if s.podyomOtmenyon(moyo) {
			s.otmenit()
			s.opustitYadro()
			return errPodyomOtmenyon
		}
		// Доступ к clash_api и адаптер пишутся СРАЗУ. Именно они нужны службе,
		// поднявшейся после нештатной смерти: без порта не у кого спросить, жив ли
		// туннель, без индекса адаптера нечего убирать за собой.
		s.mu.Lock()
		portKlash, sekretKlash := s.portClash, s.sekretClash
		s.mu.Unlock()
		if err := s.pravitSost(func(f *sostoyanie.SostoyanieFayla) {
			f.Sostoyanie = protokol.SostPodnyat
			f.PortClash, f.SekretClash = portKlash, sekretKlash
			f.AdapterTun, f.IndeksTun = a.Imya, a.Indeks
		}); err != nil {
			log.Printf("доступ к ядру и адаптер не записаны в файл состояния: %v", err)
		}
		s.mu.Lock()
		s.tun = a
		s.mu.Unlock()

		// Порядок из задачи 2.5: адаптер есть -> правило IPv6 -> (дальше
		// разрешающие правила и политика). Раньше адаптера ставить нечего, позже уже
		// поздно: промежуток между поднятым туннелем и глушением v6 это окно утечки.
		if err := s.glushitIPv6(); err != nil {
			s.Disconnect()
			s.postavit(protokol.SostOtkaz, &protokol.Oshibka{
				Kod: protokol.KodFirewallFailed, Tekst: err.Error()})
			return err
		}

		adres, sekret := s.dostupKKlash()
		// Ядро отвечает не мгновенно, и спросить его сразу после старта значит
		// измерить пустоту.
		if err := s.zhdatKlash(vnutr, adres, sekret); err != nil {
			// Не KodTunCreateFailed: адаптер к этому моменту создан, молчит
			// ядро. Прежний код отправлял человека чинить исправный туннель.
			log.Printf("подключение: ядро не отвечает по управляющему порту: %v", err)
			s.Disconnect()
			s.postavit(protokol.SostOtkaz, &protokol.Oshibka{
				Kod: protokol.KodYadroNeOtvechaet, Tekst: err.Error()})
			return err
		}

		// Выбор НАВЯЗЫВАЕТСЯ ядру, а не оставляется на поле default в конфиге.
		//
		// Ядро запоминает выбранный исходящий: store_selected у sing-box
		// «включено по умолчанию, когда задан cache_file.enabled», а он у нас
		// задан. При старте ядро восстанавливает ПРОШЛЫЙ выбор и перебивает им
		// default, то есть выбор человека доезжает до конфига и проигрывает
		// кэшу. Найдено живым прогоном 04.09.2026: на чистом наборе из четырёх
		// серверов дважды подряд просили один, а несущим выходил кэшированный,
		// при верном vybran_id на экране.
		//
		// В авто перебивается ровно так же и хуже: кэш держит КОНКРЕТНЫЙ
		// сервер, и автоматический режим молча выродился бы в один навсегда
		// выбранный.
		if err := s.navyazatVybor(vnutr, adres, sekret); err != nil {
			// Отказ закрытый: поднятый туннель через ЧУЖОЙ сервер хуже
			// неподнятого. Человек выбрал страну, и отдать ему другую молча
			// значит соврать ровно в том, ради чего программу ставят.
			log.Printf("подключение: выбор не доехал до ядра: %v", err)
			s.Disconnect()
			s.postavit(protokol.SostOtkaz, &protokol.Oshibka{
				Kod: protokol.KodPereklyuchenieNeDoehalo, Tekst: err.Error()})
			return err
		}

		teg := tegDlyaZamera()
		srok := time.Now().Add(zhdatPodyoma)
		// Номер пробы и её длительность пишутся КАЖДЫЙ раз. Две пробы за 15 с
		// и полсотни это разные диагнозы: первое значит, что ядро думает над
		// каждой, второе, что контекст уже отменён и они возвращаются мгновенно.
		// Различить их можно только по числу строк, поэтому строк не жалеем.
		nomer := 0
		for time.Now().Before(srok) {
			// Отмена замечается на БЛИЖАЙШЕЙ итерации, а не через пятнадцать
			// секунд. Выход не ставит состояние: его поставил disconnect, и
			// написать поверх него свой отказ значило бы вернуть на экран
			// «поднимается» после того, как человеку ответили «выключено».
			if s.podyomOtmenyon(moyo) {
				s.otmenit()
				s.opustitYadro()
				return errPodyomOtmenyon
			}
			nomer++
			nachProby := time.Now()
			d, err := s.zamerit(vnutr, adres, sekret, teg)
			if err != nil {
				log.Printf("подключение: проба %d за %v (с начала подъёма %v): %v",
					nomer, time.Since(nachProby).Round(time.Millisecond),
					time.Since(nach).Round(time.Millisecond), err)
			}
			if err == nil {
				// Наблюдатель регистрируется ПЕРВЫМ делом на удачном пути, до
				// единой записи о подъёме.
				//
				// Порядок не косметический. Регистрация это ещё и место, где
				// проверяется, не отключили ли нас и не остановлена ли служба, а
				// объявить podnyat и только потом это заметить значит оставить
				// экран с поднятым туннелем поверх чужого vyklyuchen.
				//
				// С этой строки и до запуска горутины ниже НЕТ ни одного
				// возврата, и это условие договора zavestiNablyudatelya: уйти
				// отсюда, не дойдя до defer s.nabl.Done(), значит подвесить
				// Disconnect навсегда.
				//
				// Обратная сторона тоже работает на нас: раз регистрация прошла,
				// отключающийся ждёт нас на s.nabl.Wait(), и весь остаток этого
				// блока случается ДО его уборки, а не наперегонки с ней.
				if !s.zavestiNablyudatelya(moyo) {
					s.otmenit()
					s.opustitYadro()
					return errPodyomOtmenyon
				}
				pervaya := time.Now()
				// Именно podnyat, а не podnimaetsya: следующей строкой это и
				// объявляется наружу, а файл до сих пор оставался с промежуточным
				// состоянием навсегда.
				if err := s.pravitSost(func(f *sostoyanie.SostoyanieFayla) {
					f.Sostoyanie, f.ConnectNach = protokol.SostPodnyat, &nach
					f.ProbaPervaya = &pervaya
					// Оба поля пишутся ЗДЕСЬ, а не в Status(): Status только читает,
					// файл наполняет pravitSost. Без этой пары поля появились бы в
					// структуре без единого писателя, то есть повтор находки 52.
					// srv и n локальные, замка им не нужно: замыкание и так
					// выполняется под s.mu (pravitSost, :520).
					f.NesushchiyId, f.VybranId = srv.Id, n.Vybran
				}); err != nil {
					log.Printf("файл состояния не записан на удачном подъёме: %v", err)
				}
				// Спрашиваем ЯДРО, кто несёт, а не переписываем своё намерение.
				// Отказ опроса НЕ роняет подъём: туннель уже несёт, а неизвестный
				// несущий это пустое поле и строка в журнале. Ронять рабочее
				// подключение из-за неотвеченного вопроса значило бы поменять его на
				// отказ ради надписи на экране.
				//
				// IdIzTega отдаёт ДВА значения, и второе не декорация: тег, не
				// начинающийся с префикса кандидата, это имя группы, а не сервер.
				// «Ещё не выбрала» это НЕ отказ, и сдаваться на нём нельзя: поле
				// осталось бы пустым до следующего опроса наблюдателя, то есть на
				// тридцать секунд. Замеренная цена ожидания около шести секунд.
				dosprosNuzhen := false
				if nteg, err := s.nesyot(vnutr, adres, sekret, genkonfig.TegSelector, genkonfig.TegAvto); err != nil {
					if errors.Is(err, yadra.ErrNeVybrala) {
						dosprosNuzhen = true
					} else {
						log.Printf("несущий не спрошен у ядра: %v", err)
					}
				} else if id, ok := genkonfig.IdIzTega(nteg); ok {
					s.zapomnitNesushchego(id)
				} else {
					log.Printf("ядро назвало несущим %q, а это не тег кандидата", nteg)
				}
				log.Printf("исходящий %s отвечает за %v", teg, d)
				s.postavit(protokol.SostPodnyat, nil)
				// Правила пересобираются на КАЖДЫЙ подъём при включённом режиме.
				// Прежде это делалось только на изменение списка серверов, поэтому
				// после автовосстановления или ручного disconnect/connect правило
				// продолжало разрешать localip СТАРОГО адаптера. Работало лишь
				// потому, что sing-tun обычно выдаёт тот же 172.19.0.1.
				//
				// Отказ пересборки НЕ роняет подъём: туннель уже несёт, а запертая
				// машина с отставшим правилом чинится следующим подъёмом. Но
				// молчать нельзя.
				//
				// Кроме одного случая: подъём, затеянный САМИМ включением режима.
				// Там запирание делает SetKillSwitch, и оно обязано доложить об
				// отказе человеку, а не спрятать его в журнал. Два вызова подряд
				// трогали бы брандмауэр дважды одним и тем же.
				s.mu.Lock()
				podRezhim := s.podRezhim
				s.mu.Unlock()
				if err := s.peresobratEsliNado(podRezhim); err != nil {
					log.Printf("правила брандмауэра отстали от подъёма: %v", err)
					s.postavit(protokol.SostPodnyat, &protokol.Oshibka{
						Kod: protokol.KodFirewallFailed, Tekst: err.Error()})
				}
				// Наблюдатель живёт столько же, сколько подключение. До него подъём
				// проверялся ровно один раз, и туннель, умерший через минуту, до
				// перезагрузки продолжал числиться поднятым.
				//
				// Доспрос ОТДЕЛЬНОЙ горутиной, а не первой фазой наблюдателя:
				// иначе ядро, не назвавшее выбор никогда, отодвинуло бы первую
				// проверку живости туннеля на срок доспроса. Регистрация своя, и
				// отказ в ней означает лишь, что нас уже отключают.
				if dosprosNuzhen && s.zavestiNablyudatelya(moyo) {
					go func() {
						defer s.nabl.Done()
						s.dosprositNesushchego(vnutr, adres, sekret)
					}()
				}
				go func() {
					defer s.nabl.Done()
					s.nablyudat(vnutr, adres, sekret, teg)
				}()
				return nil
			}
			poslednyaya = err
			select {
			case <-ctx.Done():
				log.Printf("подключение: команда отменена на пробе %d: %v", nomer, ctx.Err())
				s.Disconnect()
				return ctx.Err()
			case <-time.After(300 * time.Millisecond):
			}
		}
		// Строка пишется на каждую попытку, а не только на неудачную
		// промежуточную: прежде исход ПОСЛЕДНЕЙ попытки не попадал в журнал
		// никогда, а именно она и объявляет отказ.
		log.Printf("подключение: попытка подъёма %d из %d, ядро не понесло за %s, проб %d, последняя: %v",
			popytka, popytokPodyoma, zhdatPodyoma, nomer, poslednyaya)
		if popytka < popytokPodyoma {
			s.otmenit()
			s.opustitYadro()
		}
	}

	// Up but carrying nothing is its own state, and it is the dangerous one.
	//
	// Вердикт пишется в журнал СО ВСЕЙ обстановкой. Разовый отказ 03.09.2026
	// разбирать было нечем именно потому, что здесь молчали: состояние живёт
	// в памяти до следующей команды, а в журнале команд пишется только код.
	log.Printf("подключение: отказ %s, сервер %s (%s, %s), попыток %d по %s, последняя проба: %v",
		protokol.KodAllServersDown, srv.Id, srv.Imya, srv.Transport,
		popytokPodyoma, zhdatPodyoma, poslednyaya)
	s.Disconnect()
	// Код по ПРИЧИНЕ последней пробы, а не all-servers-down насмерть: ответ
	// команды выбирает kodPodklyucheniya, а состояние живёт на Главной до
	// следующей команды, и разойтись им нельзя.
	s.postavit(protokol.SostNeNeset, &protokol.Oshibka{
		Kod:   kodNepodnyavshegosya(poslednyaya),
		Tekst: fmt.Sprintf("туннель поднялся, но не понёс трафик (%d попытки по %s): %v", popytokPodyoma, zhdatPodyoma, poslednyaya),
	})
	return fmt.Errorf("туннель не понёс трафик за %d попытки по %s: %w", popytokPodyoma, zhdatPodyoma, poslednyaya)
}

// kodPodyomaTunnelya различает пропавший драйвер и не созданный адаптер.
//
// Оба приходят одним таймаутом ожидания, и до этой ветки человек с невставшим
// драйвером читал «не удалось создать адаптер», то есть шёл повторять там, где
// повторять нечего. Признак ставит ядро, разбор в otkazOzhidaniyaAdaptera.
func kodPodyomaTunnelya(err error) string {
	if errors.Is(err, yadra.ErrDrayverNeVstal) {
		return protokol.KodWintunMissing
	}
	return protokol.KodTunCreateFailed
}

// kodNepodnyavshegosya называет причину последней пробы.
//
// Отдельной функцией, а не веткой на месте: тот же вопрос задаёт
// kodPodklyucheniya для ответа команды, и разные ответы на один вопрос это
// экран, который спорит сам с собой.
func kodNepodnyavshegosya(poslednyaya error) string {
	if errors.Is(poslednyaya, yadra.ErrServerOtvergKlyuchi) {
		return protokol.KodServerAuthFailed
	}
	return protokol.KodAllServersDown
}

// Disconnect опускает туннель по решению ЧЕЛОВЕКА или по отказу подъёма.
//
// Три шага раздельно, и порядок важен: отменить, ДОЖДАТЬСЯ наблюдателя, убрать.
// Без ожидания уборка идёт наперегонки с горутиной, которая ещё спрашивает
// clash_api и ещё имеет право объявить аварию.
//
// Сам наблюдатель зовёт otmenit и opustit ПОРОЗНЬ, а не Disconnect: ждать
// здесь он стал бы ждать самого себя.
func (s *Sluzhba) Disconnect() {
	// otmenitPodyom, а не otmenit: идущий подъём обязан выйти, а не досидеть до
	// своего срока и поднять туннель поверх отключения.
	s.otmenitPodyom()
	s.nabl.Wait()
	s.opustit()
}

// Zavershit гасит ВСЁ фоновое и опускает туннель. Зовётся на остановке службы
// и в уборке тестов.
//
// Без него горутина восстановления переживала процесс: тест заканчивался,
// следующий писал общие переменные, а она их читала. Детектор гонок называл это
// гонкой в тестах, хотя болезнь была в продукте.
// pravitSost меняет ЧАСТЬ файла состояния, оставляя остальное на месте.
//
// Полные снимки на каждой записи и были дефектом: четыре независимых
// вызывающих, каждый знает про свои три поля и обнуляет чужие семь.
// peresobratEsliNado пропускает пересборку, когда подъём затеян самим
// включением режима: там запирание делает SetKillSwitch и оно обязано доложить
// об отказе человеку.
func (s *Sluzhba) peresobratEsliNado(podRezhim bool) error {
	if podRezhim {
		return nil
	}
	return s.PeresobratRazresheniya()
}

// pravitSost ОТДАЁТ отказ записи, а не глотает его.
//
// Прежде тут стояло _ = s.zapisat(f), и один вызывающий на этом врал человеку:
// setConnectOnStart отвечала успехом при непрошедшей записи, экран рисовал
// галочку, а после перезагрузки туннель не поднимался, потому что флага в файле
// не было. Остальные вызывающие сидят на пути подъёма, им ронять подключение
// из-за файла не за что, и они пишут причину в журнал.
func (s *Sluzhba) pravitSost(pravka func(*sostoyanie.SostoyanieFayla)) error {
	s.mu.Lock()
	pravka(&s.snimok)
	f := s.snimok
	s.mu.Unlock()
	return s.zapisat(f)
}

// sbrositSost стирает файл целиком. Здесь полная перезапись ВЕРНА: туннеля
// больше нет, и оставленные порт с адаптером это данные о ядре, которого не
// существует. Устаревший индекс адаптера опаснее пустого.
func (s *Sluzhba) sbrositSost() error {
	s.mu.Lock()
	// Отметка обновления подписки к туннелю отношения не имеет и переживает
	// сброс. У неё появился второй читатель, расписание, и без этой строки
	// каждое отключение стоило бы лишнего похода за подпиской.
	podpiska := s.snimok.PodpiskaObnovlena
	priStarte := s.snimok.PodklyuchatPriStarte
	zhurnal := s.snimok.Zhurnal
	// Полоса канала это настройка машины, а не след поднятого туннеля: канал от
	// отключения не меняется. Без этой пары каждое отключение молча снимало бы
	// объявление, и hysteria2 возвращался бы на BBR без единого слова.
	polosaVverh, polosaVniz := s.snimok.PolosaVverh, s.snimok.PolosaVniz
	s.snimok = sostoyanie.SostoyanieFayla{
		Sostoyanie:           protokol.SostVyklyuchen,
		PodpiskaObnovlena:    podpiska,
		PodklyuchatPriStarte: priStarte,
		KillSwitch:           s.killSwitch,
		Zhurnal:              zhurnal,
		PolosaVverh:          polosaVverh,
		PolosaVniz:           polosaVniz,
	}
	f := s.snimok
	s.mu.Unlock()
	return s.zapisat(f)
}

func (s *Sluzhba) Zavershit() {
	// Флаг ПЕРВЫМ, до всякого Wait: с этой строки ни одна горутина больше не
	// регистрируется, и оба ожидания ниже досчитываются до нуля навсегда, а не
	// до следующего опоздавшего Add.
	//
	// Поколение подъёма меняется заодно: идущий Connect обязан выйти, а не
	// досидеть до своего срока на службе, которой уже нет.
	s.mu.Lock()
	s.ostanovlena = true
	s.pokolenieP++
	s.mu.Unlock()
	s.fonOtmena()
	s.fon.Wait()
	s.Disconnect()
	s.osvoboditSet()
}

func (s *Sluzhba) otmenit() {
	s.mu.Lock()
	otmena := s.otmena
	s.otmena = nil
	s.mu.Unlock()
	if otmena != nil {
		otmena()
	}
}

// otmenitPodyom это otmenit плюс смена поколения: отмена, которую цикл проб
// обязан ЗАМЕТИТЬ.
//
// Отдельный метод, а не строка внутри otmenit, потому что вызывающих у otmenit
// два рода. Disconnect отменяет чужой подъём и хочет, чтобы тот вышел. Сам
// подъём между своими двумя попытками отменяет СЕБЯ и обязан продолжить: смена
// поколения там означала бы, что вторая попытка не начнётся никогда.
//
// Счётчик и отмена контекста меняются ПОД ОДНИМ замком. Порознь они дали бы
// окно, в котором подъём видит прежнее поколение и живой контекст, то есть
// ровно то, чего эта пара и должна не допускать.
func (s *Sluzhba) otmenitPodyom() {
	s.mu.Lock()
	s.cancelSpeedLocked("VPN отключается. Замер остановлен.")
	s.pokolenieP++
	otmena := s.otmena
	s.otmena = nil
	s.mu.Unlock()
	if otmena != nil {
		otmena()
	}
}

// zavestiNablyudatelya регистрирует наблюдателя подъёма и отвечает, можно ли
// его пускать.
//
// Проверка и s.nabl.Add(1) ПОД ОДНИМ замком с отменой и остановкой. Порознь
// они и давали гонку: подъём видел живую службу, отпускал замок, и Add уходил
// уже наперегонки с Wait.
//
// Вызывающий, получивший true, ОБЯЗАН дойти до горутины с defer s.nabl.Done():
// возврат между регистрацией и запуском подвесит Disconnect навсегда.
func (s *Sluzhba) zavestiNablyudatelya(moyo int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ostanovlena || s.pokolenieP != moyo {
		return false
	}
	s.nabl.Add(1)
	return true
}

// zavestiFonovuyu регистрирует фоновую горутину (восстановление) по тем же
// правилам. Отдельная группа, потому что Zavershit ждёт её раньше туннеля.
func (s *Sluzhba) zavestiFonovuyu() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ostanovlena {
		return false
	}
	s.fon.Add(1)
	return true
}

// podyomOtmenyon отвечает, сменилось ли поколение с начала этого подъёма.
func (s *Sluzhba) podyomOtmenyon(moyo int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pokolenieP != moyo
}

// errPodyomOtmenyon оборачивает context.Canceled НАМЕРЕННО: отмена подъёма это
// отмена, и граница протокола уже умеет отвечать на неё состоянием, а не
// выдуманным кодом отказа (см. kodPodklyucheniya в dispetcher.go).
var errPodyomOtmenyon = fmt.Errorf("подъём отменён отключением: %w", context.Canceled)

// Otklyuchit это отключение по решению ЧЕЛОВЕКА, и оно единственное гасит
// восстановление.
//
// Отделено от Disconnect намеренно. Disconnect зовут и внутренние ветки отказа
// самого Connect, в том числе тот Connect, который запустило восстановление.
// Гасить восстановление оттуда значило бы прекращать его первой же неудачной
// попыткой, а вся его суть в повторах: замерено на стенде, одна попытка при
// ещё не поднявшейся сети проваливается всегда.
func (s *Sluzhba) Otklyuchit() {
	s.mu.Lock()
	otmenaV := s.otmenaVosst
	s.otmenaVosst = nil
	s.mu.Unlock()
	if otmenaV != nil {
		otmenaV()
	}
	s.Disconnect()
	s.osvoboditSet()
}

// opustitYadro убирает то, что относится к ЯДРУ, не трогая состояние службы:
// между двумя попытками подъёма служба остаётся в podnimaetsya.
func (s *Sluzhba) opustitYadro() {
	// Правило IPv6 снимается вместе с туннелем, и его отказ не должен мешать
	// остальному опусканию: осиротевшее правило чинится проверкой при старте
	// службы, а незавершённое опускание чинить нечем.
	if s.vernutIPv6 != nil {
		err := s.vernutIPv6()
		s.mu.Lock()
		s.oshibkaIPv6 = err
		s.mu.Unlock()
		if err != nil {
			log.Printf("правило IPv6 не снято: %v", err)
		}
	}
	// Секрет живёт ровно столько, сколько поднят туннель: он новый на каждый
	// старт ядра, и оставленный после опускания это секрет от ядра, которого
	// больше нет. Забыть адаптер обязательно по той же причине. Устаревший индекс опаснее пустого: по нему
	// исключат из правил брандмауэра адаптер, которого уже нет, а то и чужой,
	// занявший освободившийся индекс.
	s.mu.Lock()
	s.tun = set.Adapter{}
	s.portClash, s.sekretClash = 0, ""
	s.pravilaKonfiga, s.trafikKonfiga = "", ""
	s.mu.Unlock()
}

func (s *Sluzhba) opustit() {
	s.opustitYadro()
	s.mu.Lock()
	// Несущего больше нет: ядро опущено. Оставить имя значило бы показывать
	// экран с сервером, который ничего не несёт. Выбор снимается вместе с ним:
	// он поле ЖИВОГО подключения, а не настройка, живущая между сессиями. До
	// Ш7-1 serverId не снимался нигде, и служба отдавала vyklyuchen вместе с
	// именем сервера.
	s.vybranId, s.nesushchiyId, s.nesushchiyImya = "", "", ""
	s.mu.Unlock()
	s.postavit(protokol.SostVyklyuchen, nil)
	s.mu.Lock()
	oshibkaIPv6 := s.oshibkaIPv6
	s.mu.Unlock()
	if oshibkaIPv6 != nil {
		s.postavit(protokol.SostVyklyuchen, &protokol.Oshibka{Kod: protokol.KodFirewallFailed, Tekst: "Не удалось восстановить IPv6: " + oshibkaIPv6.Error()})
	}
	// Файл переживает процесс, поэтому написанное в нём после опускания это то,
	// во что поверит следующий старт. Оставить там podnyat с индексом мёртвого
	// адаптера значит подсунуть следующему запуску ровно то, что абзацем выше
	// названо опаснее пустого: освободившийся индекс переиспользуется системой.
	if err := s.sbrositSost(); err != nil {
		log.Printf("файл состояния не переписан при опускании: %v", err)
	}
}

// zapomnitKlash запоминает доступ к clash_api поднявшегося ядра.
func (s *Sluzhba) zapomnitKlash(port int, sekret string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.portClash, s.sekretClash = port, sekret
}

// dostupKKlash отдаёт адрес и секрет для пробы. Пустой адрес означает, что
// ядро не поднято: спрашивать нечего и некого.
// zapomnitNesushchego пишет ответ ядра под замком.
//
// Сеттер, а не голое присваивание: вызывающие живут ВНЕ s.mu (подъём после
// пробы и наблюдатель), и каждый брал бы замок сам. В Ш7-1 присваивание стояло
// внутри уже взятого замка, поэтому сеттера там не было: sync.Mutex
// нереентрантный.
func (s *Sluzhba) zapomnitNesushchego(id string) {
	// Тот же несущий значит «ничего не изменилось», и платить за это чтением
	// набора нельзя: опрос теперь идёт каждые две секунды, а набор лежит на
	// диске зашифрованным.
	s.mu.Lock()
	tot := s.nesushchiyId == id
	s.mu.Unlock()
	if tot {
		return
	}
	// Имя ищется ДО замка: набор читается с диска и расшифровывается.
	imya := ""
	if id != "" {
		if n, err := s.nabor(); err == nil {
			for _, srv := range n.Servery {
				if srv.Id == id {
					imya = srv.Imya
					break
				}
			}
		}
	}
	s.mu.Lock()
	smenilsya := s.nesushchiyId != id
	if smenilsya {
		s.cancelSpeedLocked("Сервер изменился. Запустите замер заново.")
	}
	s.nesushchiyId, s.nesushchiyImya = id, imya
	s.mu.Unlock()
	if !smenilsya || id == "" {
		return
	}
	// Смена несущего меняет и адрес выхода (§5: при смене сервера).
	go s.obnovitAdresVyhoda()
	// И это событие: в авто ядро переключает само, без команды человека, и
	// до сих пор экран узнавал при следующем опросе, а трей не узнавал никогда
	// (план «шесть удобств» §4). Тот же несущий событием не является.
	s.izvestit("state", s.Status())
}

func (s *Sluzhba) dostupKKlash() (adres, sekret string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.portClash == 0 {
		return "", ""
	}
	// Адрес петлевой и совпадает с тем, что генератор кладёт в
	// external_controller. Спрашивать конфиг обратно с диска было бы вторым
	// источником правды о том, что мы сами же и записали.
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(s.portClash)), s.sekretClash
}

// dosprositNesushchego добивается ответа у группы, которая на подъёме ещё не
// назвала выбор.
//
// Группа avto это urltest, и до конца первой пробы задержки её now пуст. Это
// фаза, а не поломка, поэтому единственный вопрос при подъёме её не ловит:
// замерено 06.09.2026, вопрос уходит примерно на 1.2 с, ответ появляется на
// 5.6 с. Без доспроса поле несущего оставалось пустым до следующего опроса
// наблюдателя, и приёмка ловила это как провал продукта.
func (s *Sluzhba) dosprositNesushchego(ctx context.Context, adres, sekret string) {
	srok := time.Now().Add(s.srokDosprosa)
	for time.Now().Before(srok) {
		select {
		case <-ctx.Done():
			return
		case <-time.After(s.shagDosprosa):
		}
		nteg, err := s.nesyot(ctx, adres, sekret, genkonfig.TegSelector, genkonfig.TegAvto)
		if errors.Is(err, yadra.ErrNeVybrala) {
			continue
		}
		if err != nil {
			log.Printf("несущий не спрошен у ядра: %v", err)
			return
		}
		if id, ok := genkonfig.IdIzTega(nteg); ok {
			s.zapomnitNesushchego(id)
		} else {
			log.Printf("ядро назвало несущим %q, а это не тег кандидата", nteg)
		}
		return
	}
	// Молчание тут тоже ответ, и его надо назвать: пустое поле несущего при
	// живом туннеле иначе выглядит как наша забывчивость.
	log.Printf("группа не назвала выбор за %v: поле несущего осталось пустым", s.srokDosprosa)
}

// nablyudat спрашивает живой туннель, несёт ли он ещё трафик.
//
// Смысл ровно в том состоянии, ради которого в проекте заведено отдельное имя:
// поднятый туннель, не несущий ничего, со стороны сокета неотличим от рабочего.
// Заметить его может только тот, кто спрашивает регулярно.
func (s *Sluzhba) nablyudat(ctx context.Context, adres, sekret, teg string) {
	podryad := 0
	// Тик идёт по МЕНЬШЕМУ из двух периодов, а проба живости отсчитывается
	// отдельно. Так частый вопрос о несущем не превращается в частый выход
	// наружу, а тест, укорачивающий period, по-прежнему получает частую пробу.
	shag := s.periodNesushchego
	if s.period < shag {
		shag = s.period
	}
	sledZamer := time.Now().Add(s.period)
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(shag):
		}
		// В авто ядро меняет выбор САМО, без единой нашей команды. Без
		// периодического опроса экран показывал бы выбор, сделанный при подъёме,
		// и расходился бы с действительностью тем сильнее, чем дольше сессия.
		if nteg, err := s.nesyot(ctx, adres, sekret, genkonfig.TegSelector, genkonfig.TegAvto); err == nil {
			if id, ok := genkonfig.IdIzTega(nteg); ok {
				s.zapomnitNesushchego(id)
			}
		}
		if time.Now().Before(sledZamer) {
			continue
		}
		sledZamer = time.Now().Add(s.period)
		if _, err := s.zamerit(ctx, adres, sekret, teg); err != nil {
			// Отменённый контекст это НЕ авария, а нас самих опускают. Считать
			// его провалом значило бы поставить ne-neset поверх выключенного и
			// запустить восстановление, которое подключит туннель обратно.
			// Кнопка «отключить» переставала работать, и заметил это только
			// тест, доводящий замер до отказа ровно в момент Disconnect.
			if ctx.Err() != nil {
				return
			}
			podryad++
			log.Printf("замер исходящего %s не прошёл (%d подряд): %v", teg, podryad, err)
			if podryad < s.provalov {
				continue
			}
			s.otmenit()
			s.opustit()
			s.postavit(protokol.SostNeNeset, &protokol.Oshibka{
				Kod: protokol.KodTunnelNeNeset, Tekst: "туннель перестал нести трафик"})
			// Возвращаемся САМИ, и только после аварии. Осознанное отключение
			// человеком не переподключает никогда, иначе кнопка «отключить»
			// перестаёт работать: это и есть граница между обычным режимом и
			// постоянным, ради которой у чужих клиентов заведено два kill switch.
			vosstCtx, otmenaV := context.WithCancel(s.fonCtx)
			s.mu.Lock()
			s.otmenaVosst = otmenaV
			s.mu.Unlock()
			// Не зарегистрировались значит службу останавливают, и
			// возвращаться некуда: восстановление подняло бы туннель уже после
			// того, как его опустили насовсем.
			if !s.zavestiFonovuyu() {
				otmenaV()
				return
			}
			go func() {
				defer s.fon.Done()
				defer otmenaV()
				s.vosstanavlivat(vosstCtx)
			}()
			return
		}
		// Счётчик сбрасывается только успехом: два провала ПОДРЯД, а не два
		// провала за всё время работы.
		podryad = 0
	}
}

// vosstanavlivat поднимает туннель обратно после АВАРИИ.
//
// Своим контекстом, а не контекстом подключения: тот уже отменён Disconnect'ом,
// и наследовать от него значило бы отменить восстановление в момент рождения.
func (s *Sluzhba) vosstanavlivat(ctx context.Context) {
	for popytka := 0; ; popytka++ {
		otstup := s.otstupy[min(popytka, len(s.otstupy)-1)]
		// Сон прерываемый. Голый Sleep означал, что отключение человеком
		// доходит до цикла только после текущего отступа, а он на последних
		// попытках достигает минуты: кнопка нажата, а туннель ещё поднимется.
		select {
		case <-ctx.Done():
			return
		case <-time.After(otstup):
		}

		// Человек мог отключиться руками, пока мы ждали. Его решение старше
		// нашего: проверяем состояние ПЕРЕД каждой попыткой.
		//
		// Продолжаем и из otkaz тоже, и это не мелочь: после провалившейся
		// попытки состояние именно otkaz, а не ne-neset. Замерено 01.09.2026 на
		// стенде: погасили адаптер, служба упала за 40.3 с, сделала одну
		// попытку, та честно провалилась (сети ещё нет), и цикл остановился на
		// собственной проверке. Сеть вернули, туннель не вернулся.
		s.mu.Lock()
		sost := s.sost
		s.mu.Unlock()
		if sost != protokol.SostNeNeset && sost != protokol.SostOtkaz {
			log.Printf("восстановление прекращено: состояние %s", sost)
			return
		}

		log.Printf("восстановление, попытка %d", popytka+1)
		if err := s.Connect(context.Background()); err != nil {
			log.Printf("восстановление не удалось: %v", err)
			continue
		}
		log.Printf("туннель восстановлен с попытки %d", popytka+1)
		return
	}
}
