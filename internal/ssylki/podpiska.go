package ssylki

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/obhoddns"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/sboi"
)

var (
	// Пустой список это ОТКАЗ, а не «ноль серверов». Разница в том, что при
	// отказе прежний список остаётся жить: одна опечатка в публикации не должна
	// оставлять запертую машину без единого адреса.
	ErrPodpiskaPusta = errors.New("в подписке нет ни одного сервера")
	// Истекшая подписка приезжает кодом 200 и исправными ссылками, см. Razobrat.
	ErrPodpiskaIstekla    = errors.New("подписка истекла")
	ErrPodpiskaNedostupna = errors.New("подписка не загрузилась")
	ErrPodpiskaVelika     = errors.New("тело подписки больше потолка")
	ErrPonizhenieTLS      = errors.New("перенаправление понижает https до http")
	// Панель с лимитом устройств отвечает пустым телом и заголовком, а не
	// кодом ошибки (subscription.service.ts у Remnawave).
	ErrPodpiskaUstroystvo = errors.New("панель подписки не пускает это устройство")
)

// Ustroystvo это то, что панель подписки узнаёт о машине (П9 аудита 1.6.1).
// Пустые поля не отправляются.
type Ustroystvo struct {
	Versiya string // версия клиента для User-Agent
	// Постоянный номер машины. Наружу уходит только хеш вместе с хостом
	// подписки: общий номер позволил бы поставщикам сопоставить машину.
	Id        string
	OS        string
	VersiyaOS string
	Model     string
}

func (u Ustroystvo) zagolovki(h http.Header, host string) {
	ua := "Affory"
	if u.Versiya != "" {
		ua += "/" + u.Versiya
	}
	h.Set("User-Agent", ua)
	h.Del("x-hwid")
	if u.Id != "" {
		// hex от SHA-256 это 64 знака: панель принимает от 10 до 64 из
		// латиницы, цифр, «=» и «-».
		sum := sha256.Sum256([]byte(u.Id + "\n" + strings.ToLower(host)))
		h.Set("x-hwid", hex.EncodeToString(sum[:]))
	}
	for k, v := range map[string]string{"x-device-os": u.OS, "x-ver-os": u.VersiyaOS, "x-device-model": u.Model} {
		if v != "" {
			h.Set(k, v)
		}
	}
}

// otkazUstroystva читает ответ панели о лимите устройств при любом коде
// ответа: старые версии отвечали 404, нынешние 200 с пустым телом.
func otkazUstroystva(h http.Header) error {
	switch {
	case strings.EqualFold(h.Get("x-hwid-max-devices-reached"), "true"):
		return fmt.Errorf("%w: на подписке заняты все места под устройства", ErrPodpiskaUstroystvo)
	case strings.EqualFold(h.Get("x-hwid-not-supported"), "true"):
		return fmt.Errorf("%w: панель не получила номер устройства", ErrPodpiskaUstroystvo)
	}
	return nil
}

// OtkazZagruzki это отказ загрузки вместе с ШАГОМ, на котором он случился.
//
// До 22.09.2026 всё, что мешало забрать список, приезжало одной строкой
// «подписка недоступна»: не разрешившееся имя, закрытый порт, сорванное
// рукопожатие TLS, молчание в срок и отказ панели по праву доступа выглядели
// одинаково и вели человека в одну сторону, хотя лечатся они по-разному.
//
// Оборачивает ErrPodpiskaNedostupna, поэтому все прежние errors.Is продолжают
// работать: код разбирается по виду только там, где совет от него зависит.
type OtkazZagruzki struct {
	Vid sboi.Vid
	// Kod это код ответа, когда ответ всё-таки пришёл, и ноль, если загрузка
	// сорвалась раньше. По нему человеку отличают «по ссылке ничего нет» от
	// «сервер сломался у себя»: лечится одно ссылкой, другое ожиданием.
	Kod int
	err error
}

func (o OtkazZagruzki) Error() string { return o.err.Error() }
func (o OtkazZagruzki) Unwrap() error { return o.err }

// OtkazSVidom помечает отказ шагом, на котором он случился. Наружу ради
// подставных загрузчиков: тест обязан уметь построить тот же отказ, иначе
// проверяется не то, что приезжает из сети.
func OtkazSVidom(vid sboi.Vid, err error) error {
	return otkazSKodom(vid, 0, err)
}

// OtkazOtveta это отказ по коду пришедшего ответа. Наружу по той же причине,
// что и OtkazSVidom.
func OtkazOtveta(kod int, err error) error {
	return otkazSKodom(sboi.PoKoduOtveta(kod), kod, err)
}

// vidObryva это шаг сбоя запроса, у которого не пришло ни байта ответа.
//
// Соединение, которое открылось и оборвалось (EOF или сброс на чтении), по
// типу шага не называет: EOF не называл вовсе, и человек видел общее
// «подписка не загрузилась», а сброс читался как «не отвечает на
// подключение», хотя подключение было. Замерено в госте 02.10.2026: порт
// принимал соединение и рвал его на рукопожатии, ровно как фильтр
// провайдера. У https такой обрыв это сорванное рукопожатие, у http
// соединение, закрытое без ответа. Отказ в самом подключении (dial) и срок
// остаются как есть.
func vidObryva(err error, shema string) sboi.Vid {
	vid := sboi.Klassifitsirovat(err)
	var op *net.OpError
	oborvano := errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
		(errors.As(err, &op) && (op.Op == "read" || op.Op == "write"))
	if !oborvano || (vid != sboi.Neyasno && vid != sboi.TCP) {
		return vid
	}
	if shema == "https" {
		return sboi.TLS
	}
	return sboi.TCP
}

func otkazSKodom(vid sboi.Vid, kod int, err error) error {
	if vid == sboi.Neyasno {
		return err
	}
	return OtkazZagruzki{Vid: vid, Kod: kod, err: fmt.Errorf("%w (%s)", err, vid.Opisanie())}
}

// Потолок тела. Подписка на тысячу серверов это примерно двести килобайт, так
// что мегабайта хватает с запасом, а вот скачивать чужой дистрибутив, потому
// что на том конце подменили ответ, мы не станем.
const PotolokPoUmolchaniyu int64 = 1 << 20

// Раз в двенадцать часов. Чаще незачем: список серверов у панелей меняется
// днями, а каждый запрос это ещё один шанс засветить токен подписки.
const PeriodObnovleniya = 12 * time.Hour

// Chasy это инъекция времени. Без неё расписание в тестах недостижимо, и
// проверять его пришлось бы ожиданием, то есть никогда.
type Chasy interface{ Seychas() time.Time }

type nastoyashchieChasy struct{}

func (nastoyashchieChasy) Seychas() time.Time { return time.Now() }

// OtkazStroki это строка, которая не разобралась, вместе с её номером.
//
// Номер настоящий, от начала тела, и считает пустые строки тоже: человек будет
// смотреть в подписку глазами, а редактор нумерует именно так.
type OtkazStroki struct {
	Stroka   int    `json:"stroka"`
	Prichina string `json:"prichina"`
}

// Uvedomlenie это сообщение панели, приехавшее вместо сервера.
type Uvedomlenie struct {
	Stroka int    `json:"stroka"`
	Tekst  string `json:"tekst"`
}

type Razbor struct {
	Servery      []protokol.Server `json:"servery"`
	Otkazy       []OtkazStroki     `json:"otkazy,omitempty"`
	Uvedomleniya []Uvedomlenie     `json:"uvedomleniya,omitempty"`
}

// RazobratSpisok превращает тело подписки в список серверов.
//
// Отказ и частичный успех это РАЗНЫЕ вещи, и обе возвращают заполненный Razbor:
// причины нужны человеку и при отказе, иначе на экране остаётся слово «пусто».
func RazobratSpisok(telo []byte) (Razbor, error) {
	// Настройки чужого клиента по строкам дали бы сотни отказов, из которых не
	// понять главного: панель отдаёт не список ссылок (П7 аудита 1.6.1).
	if pohozheNaNastroyki(string(telo)) {
		return Razbor{}, ErrNeSsylki
	}
	r := razobratStroki(telo, true)
	if len(r.Servery) > 0 {
		return r, nil
	}
	// Ни одного сервера. Исходов два, и путать их нельзя: истекшая подписка это
	// ответ панели, который надо показать дословно, а всё остальное это отказ
	// с причинами.
	if len(r.Uvedomleniya) > 0 {
		return r, ErrPodpiskaIstekla
	}
	return r, ErrPodpiskaPusta
}

// razobratStroki общая часть подписки и вставки пачкой (26.09.2026): снять
// BOM и base64, разрезать по строкам, разобрать каждую, убрать повторы.
//
// izPodpiski=false это вставка руками. В ней строки http(s) пропускаются: их
// окно отправляет отдельно, подпиской, и в отказах они были бы ложью.
func razobratStroki(telo []byte, izPodpiski bool) Razbor {
	var r Razbor

	// BOM. Панели, собранные на Windows, ставят его молча, и без снятия первая
	// строка перестаёт начинаться с «vless».
	telo = bytes.TrimPrefix(telo, []byte{0xEF, 0xBB, 0xBF})
	tekst := string(telo)

	if b, ok := dekodirovatSpisok(tekst); ok {
		tekst = string(b)
	}
	// Переносы приводятся к одному виду ДО разбора строк.
	//
	// Хвостовой \r от CRLF снимает и TrimSpace ниже, так что первая замена это
	// подстраховка, а не то, что спасает: мутационный прогон 01.09.2026 показал,
	// что её удаление тест не замечает, и это честно записано в тесте.
	//
	// А вот вторая замена работает всерьёз. Тело, разделённое ОДНИМ \r, при
	// разбиении по \n остаётся единственной строкой, и подписка целиком
	// объявляется пустой: не «битой», а именно пустой, то есть человеку сообщают
	// неправду о причине.
	tekst = strings.ReplaceAll(tekst, "\r\n", "\n")
	tekst = strings.ReplaceAll(tekst, "\r", "\n")

	// Дедупликация после разбора сравнивает всю конфигурацию. Общий endpoint
	// сам по себе не означает, что пароли, SNI и параметры транспорта совпали.
	for i, stroka := range strings.Split(tekst, "\n") {
		nomer := i + 1
		stroka = strings.TrimSpace(stroka)
		if stroka == "" {
			continue
		}
		if !izPodpiski && AdresPodpiski(stroka) {
			continue
		}
		srv, err := Razobrat(stroka)
		switch {
		case err == nil:
			srv.IzPodpiski = izPodpiski
			r.Servery = append(r.Servery, srv)
		case errors.Is(err, ErrUvedomleniePodpiski):
			// Отдельным полем, а НЕ в отказы: там текст утонет среди номеров
			// строк, а он и есть ответ панели, включая код оплаты.
			r.Uvedomleniya = append(r.Uvedomleniya, Uvedomlenie{
				Stroka: nomer,
				Tekst:  strings.TrimPrefix(err.Error(), ErrUvedomleniePodpiski.Error()+": "),
			})
		default:
			r.Otkazy = append(r.Otkazy, OtkazStroki{Stroka: nomer, Prichina: prichinaStroki(err, stroka)})
		}
	}

	r.Servery = unikalnyeProfili(r.Servery)
	return r
}

// dekodirovatSpisok снимает base64, если тело в нём.
//
// Проверка «внутри есть ://» обязательна. Без неё любая строка, случайно
// оказавшаяся годным base64, декодируется в мусор, и подписка объявляется битой
// по причине, которой нет.
func dekodirovatSpisok(s string) ([]byte, bool) {
	// Панели, выросшие вокруг почтовых библиотек, ломают base64 переносами
	// каждые 76 символов. Пробелы внутри тела значат, что это не ссылки.
	szhatoe := strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, s)
	if szhatoe == "" {
		return nil, false
	}
	b, ok := dekodirovat(szhatoe)
	if !ok || !strings.Contains(string(b), "://") {
		return nil, false
	}
	return b, true
}

// Zagruzchik держит всё, что в тестах должно подменяться.
type Zagruzchik struct {
	Klient  *http.Client
	Potolok int64
	Chasy   Chasy
	// Spat отдельно от Chasy: расписание смотрит на часы, а повторы именно
	// спят, и подменять их надо порознь.
	Spat       func(time.Duration)
	Ustroystvo Ustroystvo
}

func NovyyZagruzchik() *Zagruzchik {
	// Своя копия транспорта ради одного поля: имя подписки, которого не нашёл
	// DNS системы, спрашивается у публичного DNS через HTTPS (жалоба
	// 02.10.2026: провайдер не отдавал имя, и подписка не грузилась вовсе).
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.DialContext = obhoddns.Nabrat
	return &Zagruzchik{
		// Таймаут задан, а не оставлен нулём. Клиент без таймаута висит на
		// молчащем сокете вечно, и служба вместе с ним.
		Klient:  &http.Client{Timeout: 30 * time.Second, Transport: tr},
		Potolok: PotolokPoUmolchaniyu,
		Chasy:   nastoyashchieChasy{},
	}
}

// NovyyZagruzchikCherez отдаёт загрузчик, который ходит через локальный вход
// ядра, то есть ЧЕРЕЗ туннель.
//
// Своя копия транспорта, а не правка общего: http.DefaultTransport один на
// процесс, и прокси в нём увёл бы в туннель заодно замеры и проверку выхода.
//
// TLS не трогается ни одним полем, и это не забывчивость. Запрос несёт пропуск
// к панели, и ослабить проверку сертификата ради доступности значило бы отдать
// этот пропуск любому, кто встанет на пути.
func NovyyZagruzchikCherez(proksi string) (*Zagruzchik, error) {
	z := NovyyZagruzchik()
	if proksi == "" {
		return z, nil
	}
	u, err := url.Parse("http://" + proksi)
	if err != nil {
		return nil, fmt.Errorf("адрес локального входа не разобран: %w", err)
	}
	z.Klient = &http.Client{
		Timeout:   z.Klient.Timeout,
		Transport: &http.Transport{Proxy: http.ProxyURL(u)},
	}
	return z, nil
}

// Zagruzit скачивает подписку и разбирает её.
//
// Адрес подписки это секрет того же класса, что и ключ: он и есть пропуск.
// Поэтому он не попадает ни в одну строку ошибки, а net/http кладёт полный URL
// в *url.Error совершенно бесплатно.
func (z *Zagruzchik) Zagruzit(ctx context.Context, adres string) (Razbor, error) {
	zapros, err := http.NewRequestWithContext(ctx, http.MethodGet, adres, nil)
	if err != nil {
		return Razbor{}, fmt.Errorf("%w: адрес не разобран", ErrPodpiskaNedostupna)
	}

	z.Ustroystvo.zagolovki(zapros.Header, zapros.URL.Hostname())

	// Копия, а не правка чужого клиента: вызывающий отдал нам клиент, а не
	// разрешение менять его поведение у себя за спиной.
	klient := *z.Klient
	zapret := zapretPonizheniya(zapros.URL.Scheme)
	klient.CheckRedirect = func(r *http.Request, bylo []*http.Request) error {
		if err := zapret(r, bylo); err != nil {
			return err
		}
		// Перенаправление на другой хост несёт номер машины для НЕГО, а не
		// для прежнего: net/http копирует заголовки как есть.
		z.Ustroystvo.zagolovki(r.Header, r.URL.Hostname())
		return nil
	}

	otvet, err := klient.Do(zapros)
	if err != nil {
		if errors.Is(err, ErrPonizhenieTLS) {
			return Razbor{}, ErrPonizhenieTLS
		}
		return Razbor{}, OtkazSVidom(vidObryva(err, zapros.URL.Scheme),
			fmt.Errorf("%w: %s", ErrPodpiskaNedostupna, bezAdresa(err, adres)))
	}
	defer otvet.Body.Close()

	if err := otkazUstroystva(otvet.Header); err != nil {
		return Razbor{}, err
	}

	if otvet.StatusCode != http.StatusOK {
		// Только код. Тело чужое, и печатать его целиком значит однажды
		// напечатать в журнал то, что панель туда положила.
		return Razbor{}, OtkazOtveta(otvet.StatusCode,
			fmt.Errorf("%w: код ответа %d", ErrPodpiskaNedostupna, otvet.StatusCode))
	}

	// Потолок+1 и сравнение, а НЕ LimitReader на потолок: второй усекает молча,
	// и на выходе получается исправный список из меньшего числа серверов вообще
	// без ошибки. Это худший исход из всех: он выглядит успехом.
	telo, err := io.ReadAll(io.LimitReader(otvet.Body, z.Potolok+1))
	if err != nil {
		return Razbor{}, fmt.Errorf("%w: %s", ErrPodpiskaNedostupna, bezAdresa(err, adres))
	}
	if int64(len(telo)) > z.Potolok {
		return Razbor{}, fmt.Errorf("%w: больше %d байт", ErrPodpiskaVelika, z.Potolok)
	}
	return RazobratSpisok(telo)
}

// zapretPonizheniya запрещает переход https -> http.
//
// Понижение TLS на запросе, несущем токен подписки, это выдача токена всем, кто
// стоит на пути. Перенаправления внутри https при этом совершенно законны:
// панели переносят свои пути и не спрашивают нас.
func zapretPonizheniya(ishodnayaShema string) func(*http.Request, []*http.Request) error {
	return func(zapros *http.Request, bylo []*http.Request) error {
		if len(bylo) >= 10 {
			return errors.New("слишком много перенаправлений")
		}
		if ishodnayaShema == "https" && zapros.URL.Scheme != "https" {
			return ErrPonizhenieTLS
		}
		return nil
	}
}

// bezAdresa вычищает адрес подписки из текста ошибки.
//
// Снять обёртку *url.Error мало: адрес встречается и внутри текста от
// резолвера. Поэтому после снятия обёртки идёт ещё и прямая замена, и это не
// перестраховка, а второй независимый барьер к секрету.
// bezSsylki вычищает саму строку подписки из текста отказа.
//
// Разбор чинится в источнике (razobratURL больше не печатает адрес), но отказ
// собирается из ЛЮБОЙ ошибки разбора, а их там десяток и добавятся новые.
// Барьер стоит на выходе, потому что цена промаха несимметрична: в строке
// лежат uuid и pbk, а отказы уезжают в кадр ответа и на экран.
func bezSsylki(err error, stroka string) string {
	tekst := err.Error()

	// Замена целой строки НЕДОСТАТОЧНА, и это выяснилось мутацией, а не
	// чтением. url.Parse отрезает фрагмент ДО разбора, поэтому url.Error несёт
	// ссылку без «#NL», и точное совпадение промахивается на один хвост.
	kandidaty := []string{stroka}
	if i := strings.IndexByte(stroka, '#'); i >= 0 {
		kandidaty = append(kandidaty, stroka[:i])
	}

	// Разбирать кусками, а не через url.Parse: сюда попадают ровно те строки,
	// на которых url.Parse уже отказал, и второй раз он откажет так же.
	bez := stroka
	if i := strings.Index(bez, "://"); i >= 0 {
		bez = bez[i+3:]
	}
	if i := strings.IndexByte(bez, '#'); i >= 0 {
		bez = bez[:i]
	}
	if i := strings.IndexByte(bez, '@'); i >= 0 {
		kandidaty = append(kandidaty, bez[:i]) // uuid или пароль
		bez = bez[i+1:]
	}
	if i := strings.IndexByte(bez, '?'); i >= 0 {
		kandidaty = append(kandidaty, bez[i+1:]) // строка запроса с pbk и sid
		bez = bez[:i]
	}
	if bez != "" {
		kandidaty = append(kandidaty, bez) // хост с портом
	}

	return zamenitKuski(tekst, kandidaty, "<ссылка>")
}

// prichinaStroki это причина отказа строки для окна: без самой ссылки и без
// английского хвоста разборщика («ss: illegal base64 data at input byte 7»).
// Если от причины ничего нашего не осталось, говорится главное: ссылка не
// разобрана, а номер строки окно ставит рядом само.
func prichinaStroki(err error, stroka string) string {
	if p := sboi.ObrezatTehniku(bezSsylki(err, stroka)); p != "" {
		return p
	}
	return "ссылка не разобрана"
}

// zamenitKuski заменяет каждый кусок секрета меткой. Общая для отказов
// подписки и для журналов в выгрузке диагностики (О6 аудита 1.6.1).
func zamenitKuski(tekst string, kuski []string, metka string) string {
	// От длинных к коротким: иначе короткий кусок съест часть длинного и
	// оставит от него огрызок, по которому секрет всё ещё собирается.
	kuski = append([]string(nil), kuski...)
	sort.Slice(kuski, func(i, j int) bool { return len(kuski[i]) > len(kuski[j]) })
	for _, k := range kuski {
		if len(k) > 3 {
			tekst = strings.ReplaceAll(tekst, k, metka)
		}
	}
	return tekst
}

func bezAdresa(err error, adres string) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		err = ue.Err
	}
	return zamenitAdres(err.Error(), adres)
}

// zamenitAdres убирает адрес подписки целиком и по частям: путь и запрос
// встречаются в текстах и без узла.
func zamenitAdres(tekst, adres string) string {
	tekst = strings.ReplaceAll(tekst, adres, "<адрес подписки>")
	if u, e := url.Parse(adres); e == nil {
		for _, chast := range []string{u.RequestURI(), u.EscapedPath(), u.RawQuery} {
			if chast != "" && chast != "/" {
				tekst = strings.ReplaceAll(tekst, chast, "<адрес подписки>")
			}
		}
	}
	return tekst
}

// otvetOkonchatelnyy отвечает, ответил ли сервер подписки отказом, который
// повтор не изменит: 4xx, кроме 408 (не дождался запроса) и 429 (просит
// подождать). Ссылка без подписки ждала пять заходов и 16 с там, где 404
// пришёл с первого раза, и каждый заход ещё раз светил адрес (приёмка 1.9.2
// в госте, 02.10.2026). 5xx повторяются: сервер сломан сейчас, а не навсегда.
func otvetOkonchatelnyy(err error) bool {
	var o OtkazZagruzki
	if !errors.As(err, &o) {
		return false
	}
	return o.Kod >= 400 && o.Kod < 500 && o.Kod != http.StatusRequestTimeout && o.Kod != http.StatusTooManyRequests
}

// ZagruzitSPovtorami повторяет попытку с нарастающей паузой.
//
// Служба стартует Automatic, то есть раньше, чем поднимается сеть. Без повторов
// свежепоставленный клиент до полусуток сидел бы со списком, который не смог
// обновить, и человек видел бы VPN без серверов.
//
// Повторяется ТОЛЬКО недоступность. Истекшая подписка, пустой список и слишком
// большое тело это содержательные ответы: пять повторов не изменят в них
// ничего, кроме времени, которое человек ждёт уже известный ответ.
func (z *Zagruzchik) ZagruzitSPovtorami(ctx context.Context, adres string, popytok int) (Razbor, error) {
	if popytok < 1 {
		popytok = 1
	}
	pauza := time.Second
	var posledn error
	for i := 0; i < popytok; i++ {
		if err := ctx.Err(); err != nil {
			return Razbor{}, err
		}
		r, err := z.Zagruzit(ctx, adres)
		if err == nil || !errors.Is(err, ErrPodpiskaNedostupna) || otvetOkonchatelnyy(err) {
			return r, err
		}
		posledn = err
		if i < popytok-1 {
			if z.Spat != nil {
				z.Spat(pauza)
			} else {
				timer := time.NewTimer(pauza)
				select {
				case <-ctx.Done():
					timer.Stop()
					return Razbor{}, ctx.Err()
				case <-timer.C:
				}
			}
			pauza *= 2
		}
	}
	return Razbor{}, posledn
}
