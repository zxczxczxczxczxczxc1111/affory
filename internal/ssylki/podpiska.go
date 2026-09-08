package ssylki

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
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
)

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

	// Повторы В САМОЙ подписке. Замерено на живой подписке 01.09.2026: из
	// одиннадцати строк две вели на один и тот же узел с одним и тем же
	// идентификатором. Модель этого не предусматривала, а последствие тяжёлое:
	// сверка кандидатов из задачи 3.6 отвергает список с повторяющимся Id,
	// конфиг ядра не собирается, и подъёма нет вовсе. Человек вставляет рабочую
	// подписку и получает клиент, который не подключается ни к чему.
	//
	// Схлопывается ТИХО и без отказа: для человека это одна и та же точка, а не
	// ошибка публикации, о которой ему есть что делать.
	vzyaty := make(map[string]bool)

	for i, stroka := range strings.Split(tekst, "\n") {
		nomer := i + 1
		stroka = strings.TrimSpace(stroka)
		if stroka == "" {
			continue
		}
		srv, err := Razobrat(stroka)
		switch {
		case err == nil:
			if vzyaty[srv.Id] {
				continue
			}
			vzyaty[srv.Id] = true
			srv.IzPodpiski = true
			r.Servery = append(r.Servery, srv)
		case errors.Is(err, ErrUvedomleniePodpiski):
			// Отдельным полем, а НЕ в отказы: там текст утонет среди номеров
			// строк, а он и есть ответ панели, включая код оплаты.
			r.Uvedomleniya = append(r.Uvedomleniya, Uvedomlenie{
				Stroka: nomer,
				Tekst:  strings.TrimPrefix(err.Error(), ErrUvedomleniePodpiski.Error()+": "),
			})
		default:
			r.Otkazy = append(r.Otkazy, OtkazStroki{Stroka: nomer, Prichina: bezSsylki(err, stroka)})
		}
	}

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
	Spat func(time.Duration)
}

func NovyyZagruzchik() *Zagruzchik {
	return &Zagruzchik{
		// Таймаут задан, а не оставлен нулём. Клиент без таймаута висит на
		// молчащем сокете вечно, и служба вместе с ним.
		Klient:  &http.Client{Timeout: 30 * time.Second},
		Potolok: PotolokPoUmolchaniyu,
		Chasy:   nastoyashchieChasy{},
		Spat:    time.Sleep,
	}
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

	// Копия, а не правка чужого клиента: вызывающий отдал нам клиент, а не
	// разрешение менять его поведение у себя за спиной.
	klient := *z.Klient
	klient.CheckRedirect = zapretPonizheniya(zapros.URL.Scheme)

	otvet, err := klient.Do(zapros)
	if err != nil {
		if errors.Is(err, ErrPonizhenieTLS) {
			return Razbor{}, ErrPonizhenieTLS
		}
		return Razbor{}, fmt.Errorf("%w: %s", ErrPodpiskaNedostupna, bezAdresa(err, adres))
	}
	defer otvet.Body.Close()

	if otvet.StatusCode != http.StatusOK {
		// Только код. Тело чужое, и печатать его целиком значит однажды
		// напечатать в журнал то, что панель туда положила.
		return Razbor{}, fmt.Errorf("%w: код ответа %d", ErrPodpiskaNedostupna, otvet.StatusCode)
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

	// От длинных к коротким: иначе короткий кусок съест часть длинного и
	// оставит от него огрызок, по которому секрет всё ещё собирается.
	sort.Slice(kandidaty, func(i, j int) bool { return len(kandidaty[i]) > len(kandidaty[j]) })
	for _, k := range kandidaty {
		if len(k) > 3 {
			tekst = strings.ReplaceAll(tekst, k, "<ссылка>")
		}
	}
	return tekst
}

func bezAdresa(err error, adres string) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		err = ue.Err
	}
	tekst := err.Error()
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
		if err == nil || !errors.Is(err, ErrPodpiskaNedostupna) {
			return r, err
		}
		posledn = err
		if i < popytok-1 {
			z.Spat(pauza)
			pauza *= 2
		}
	}
	return Razbor{}, posledn
}

// Slit накладывает свежий список на прежний.
//
// Три правила, и каждое стоит за конкретной потерей. Ротация ключей сохраняет
// ОДНО поколение назад, потому что ошибочная публикация иначе затирает рабочие
// учётные данные навсегда, а сервер у проекта один. Сервер, добавленный руками,
// переживает обновление: он и есть то, к чему откатываются, когда проблема в
// самой подписке. Сервер, УШЕДШИЙ из подписки, исчезает, иначе список только
// растёт и selected-server-gone не срабатывает никогда.
func Slit(bylo, stalo []protokol.Server) []protokol.Server {
	prezhnie := make(map[string]protokol.Server, len(bylo))
	for _, s := range bylo {
		prezhnie[s.Id] = s
	}

	itog := make([]protokol.Server, 0, len(stalo)+len(bylo))
	for _, novyy := range stalo {
		staryy, est := prezhnie[novyy.Id]
		if est {
			if klyuchiRazlichny(staryy, novyy) {
				novyy.PrezhnieKlyuchi = &protokol.Klyuchi{
					Uuid: staryy.Uuid, PublicKey: staryy.PublicKey,
					ShortId: staryy.ShortId, Parol: staryy.Parol, Metod: staryy.Metod,
				}
			} else {
				// Обновление без ротации не имеет права стереть точку отката.
				novyy.PrezhnieKlyuchi = staryy.PrezhnieKlyuchi
			}
		}
		itog = append(itog, novyy)
	}

	// Ручные добавляются после: порядок подписки это порядок панели, и менять
	// его нам незачем.
	vzyaty := make(map[string]bool, len(stalo))
	for _, s := range stalo {
		vzyaty[s.Id] = true
	}
	for _, s := range bylo {
		if !s.IzPodpiski && !vzyaty[s.Id] {
			itog = append(itog, s)
		}
	}
	return itog
}

func klyuchiRazlichny(a, b protokol.Server) bool {
	return a.Uuid != b.Uuid || a.PublicKey != b.PublicKey ||
		a.ShortId != b.ShortId || a.Parol != b.Parol || a.Metod != b.Metod
}
