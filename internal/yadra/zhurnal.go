package yadra

import (
	"bytes"
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/sboi"
)

// Вывод ядра в журнал службы.
//
// До 02.09.2026 exec.Command запускался без Stdout и Stderr, то есть всё, что
// говорило ядро, выбрасывалось. Цена выяснилась на находке 43: туннель не
// поднимался, служба отвечала «TUN-адаптер не появился», а настоящую причину
// («initialize outbound[24]: invalid public_key») пришлось доставать запуском
// ядра руками в госте. В поле такой возможности нет вовсе.
//
// Уровень журнала самого ядра у нас `warn`, поэтому поток редкий: это не
// подробный лог, а именно жалобы.
const predelStrokYadra = 200

// Окно, за которое считается предел.
//
// Прежде предел считался за ВЕСЬ запуск ядра, и после двухсотой строки журнал
// молчал до перезапуска. Ядро работает сутками. По журналу живой машины
// 10.09.2026: пять раз «дальше молчим», и шторм отказов сокетов у ядра виден
// только до порога, а сколько их было на самом деле, узнать неоткуда.
//
// Минута выбрана как срок, за который человек успевает заметить неполадку и
// снять журнал: заливка файла по-прежнему невозможна (потолок 200 строк в
// минуту, то есть 12 тысяч в час), а жалобы следующего часа уже слышны.
const oknoStrokYadra = time.Minute

// Zhurnal это куда уходит вывод ядра. По умолчанию журнал процесса, служба
// подставляет свой файл `log\yadro.log`: жалобы ядра и жизнь службы читаются
// по отдельности, а не вперемешку.
var Zhurnal = log.Default()

type zhurnalYadra struct {
	imya string
	// Путь конфига, с которым поднято ядро: по нему жалобы на выходы
	// отделяются от жалоб соседнего ядра (см. ZhalobaVyhoda).
	konfig string
	ost    []byte
	strok  int
	skazl  bool
	// Начало текущего окна и сколько строк за него проглочено.
	nachalo    time.Time
	proglochen int
	// Шов времени: тест не имеет права спать минуту, чтобы проверить окно.
	seychas func() time.Time
}

func (z *zhurnalYadra) teper() time.Time {
	if z.seychas != nil {
		return z.seychas()
	}
	return time.Now()
}

// Write собирает ЦЕЛЫЕ строки. Поток приходит кусками, и граница куска не
// совпадает с границей строки: наивная запись рвала бы сообщения посередине,
// причём тем чаще, чем длиннее сообщение, то есть на самых интересных.
func (z *zhurnalYadra) Write(p []byte) (int, error) {
	z.ost = append(z.ost, p...)
	for {
		i := bytes.IndexByte(z.ost, '\n')
		if i < 0 {
			return len(p), nil
		}
		stroka := bezTsveta(string(bytes.TrimRight(z.ost[:i], "\r")))
		z.ost = z.ost[i+1:]
		if stroka == "" {
			continue
		}
		teper := z.teper()
		if z.nachalo.IsZero() {
			z.nachalo = teper
		}
		// Окно кончилось: слушаем снова и говорим, скольких не услышали.
		// Число обязательно, иначе по журналу нельзя отличить «ядро замолчало»
		// от «мы перестали слушать».
		if teper.Sub(z.nachalo) >= oknoStrokYadra {
			if z.proglochen > 0 {
				Zhurnal.Printf("ядро %s: проглочено строк за окно: %d", z.imya, z.proglochen)
			}
			z.nachalo, z.strok, z.skazl, z.proglochen = teper, 0, false, 0
		}
		if z.strok >= predelStrokYadra {
			z.proglochen++
			if !z.skazl {
				z.skazl = true
				Zhurnal.Printf("ядро %s: строк больше %d за %v, дальше молчим до конца окна",
					z.imya, predelStrokYadra, oknoStrokYadra)
			}
			continue
		}
		z.strok++
		// Запоминаем ДО предела строк? Нет, после: за пределом мы уже молчим,
		// и обвинять драйвер строкой, которой нет в журнале, нечестно.
		zapomnitZhalobu(stroka)
		zapomnitZhalobuVyhoda(z.konfig, stroka, teper)
		Zhurnal.Printf("ядро %s: %s", z.imya, stroka)
	}
}

// Жалоба ядра на драйвер TUN-адаптера, и это ЕДИНСТВЕННЫЙ честный признак
// невставшего драйвера, который у нас есть.
//
// Признак из плана полосы («рядом с ядром нет wintun.dll») к этому продукту не
// применим: файла рядом с ядром нет и не было, wintun.dll лежит ВНУТРИ
// sing-box.exe (спека §14.1 и §9.1), а установщик кладёт четыре exe и три txt.
// Проверка «файла нет» отвечала бы «нет драйвера» на любой отказ подъёма, то
// есть была бы той самой догадкой, против которой полоса и написана.
//
// Судья поэтому ядро: оно единственное знает, обо что споткнулось. Занятое имя
// адаптера даёт тот же таймаут ожидания, но другую жалобу, и по ней случаи
// расходятся.
var zhaloby struct {
	mu      sync.Mutex
	drayver string
}

// Слова, по которым жалоба опознаётся как «драйвер». Их два, и оба это ИМЯ
// драйвера, а не пересказ ошибки: текст сообщений пишет ядро, и он поменяется,
// а имя драйвера в нём останется.
//
// Уровень журнала ядра у нас warn, поэтому поток это жалобы, а не хроника: имя
// драйвера в такой строке означает, что с ним что-то не так.
var slovaDrayvera = []string{"wintun", "tun device"}

// ErrDrayverNeVstal значит: ядро пожаловалось на драйвер TUN-адаптера, и
// таймаут ожидания вызван им, а не занятым именем адаптера.
var ErrDrayverNeVstal = errors.New("драйвер VPN не установился")

// ZhalobaNaDrayver отдаёт последнюю жалобу ядра на драйвер TUN-адаптера.
// Пустая строка означает «ядро про драйвер не жаловалось», а не «драйвер цел».
func ZhalobaNaDrayver() string {
	zhaloby.mu.Lock()
	defer zhaloby.mu.Unlock()
	return zhaloby.drayver
}

// ZabytZhalobyYadra стирает запомненное. Зовётся подъёмом ПЕРЕД стартом ядра:
// без этого жалоба прошлого подъёма обвиняла бы драйвер в отказе, к которому он
// отношения не имеет.
func ZabytZhalobyYadra() {
	zhaloby.mu.Lock()
	defer zhaloby.mu.Unlock()
	zhaloby.drayver = ""
}

func zapomnitZhalobu(stroka string) {
	nizhniy := strings.ToLower(stroka)
	for _, slovo := range slovaDrayvera {
		if strings.Contains(nizhniy, slovo) {
			zhaloby.mu.Lock()
			zhaloby.drayver = stroka
			zhaloby.mu.Unlock()
			return
		}
	}
}

// Жалобы ядра на выходы (02.10.2026).
//
// Причину, по которой ядро не дозвонилось до сервера, кроме журнала не знает
// никто. Clash API на неудачный замер отвечает 503 и фразой «An error occurred
// in the delay test», а настоящую ошибку выбрасывает
// (experimental/clashapi/proxies.go). Замер через вход ядра видит обрыв: на
// CONNECT ядро сначала отвечает «200 Connection established» и только потом
// звонит серверу (sing protocol/http/handshake.go). Остаётся строка журнала,
// которую ядро пишет на каждый неудавшийся звонок (route/conn.go):
//
//	connection: open connection to <куда> using outbound/<тип>[<тег>]: <ошибка>
//
// Тег у временного ядра пинга это сервер (srv-<id>), у основного группа
// vybor: она сервер не называет, и привязывать её к серверу дело того, кто
// знает выбор группы.
//
// Хранится последняя УЗНАННАЯ жалоба на тег. Нераспознанные строки её не
// затирают намеренно: у живого ядра отказы UDP идут сотнями вперемешку с
// отказами сертификата, и «последняя строка» почти всегда оказывалась бы
// шумом. Устаревание решает вызывающий сроком posle.

// ZhalobaYadra это узнанная причина отказа выхода и когда ядро о ней сказало.
type ZhalobaYadra struct {
	Prichina sboi.PrichinaYadra
	Vremya   time.Time
}

// По пути конфига: основное ядро, ядро пинга и ядро проверки сервера
// одноимённы (sing-box.exe), а конфиги у них разные. Каждый Zapustit
// начинает жалобы своего конфига с нуля, то есть жалобы живут ровно один
// запуск ядра.
var zhalobyVyhodov struct {
	mu sync.Mutex
	po map[string]map[string]ZhalobaYadra
}

// Отказ звонка и отказ UDP-сессии. С пробелом впереди и со словом
// «connection:» намеренно: та же фраза приходит и внутри «connection download
// closed: remote error: open connection to ...», и там это отказ ЧУЖОГО
// выхода, на стороне сервера, а тег в ней серверный.
var metkiOtkazaVyhoda = []string{
	" connection: open connection to ",
	" connection: listen packet connection using ",
}

const metkaVyhoda = "using outbound/"

// razobratOtkazVyhoda достаёт тег выхода и текст ошибки из строки ядра.
func razobratOtkazVyhoda(stroka string) (teg, oshibka string, ok bool) {
	nachalo := -1
	for _, m := range metkiOtkazaVyhoda {
		if i := strings.Index(stroka, m); i >= 0 {
			nachalo = i + len(m)
			break
		}
	}
	if nachalo < 0 {
		return "", "", false
	}
	ost := stroka[nachalo:]
	i := strings.Index(ost, metkaVyhoda)
	if i < 0 {
		return "", "", false
	}
	ost = ost[i+len(metkaVyhoda):]
	l := strings.IndexByte(ost, '[')
	r := strings.Index(ost, "]: ")
	if l < 0 || r <= l+1 {
		return "", "", false
	}
	return ost[l+1 : r], ost[r+len("]: "):], true
}

func zapomnitZhalobuVyhoda(konfig, stroka string, kogda time.Time) {
	teg, oshibka, ok := razobratOtkazVyhoda(stroka)
	if !ok {
		return
	}
	prichina := sboi.PoStrokeYadra(oshibka)
	if prichina == sboi.NeNazvana {
		return
	}
	zhalobyVyhodov.mu.Lock()
	defer zhalobyVyhodov.mu.Unlock()
	if zhalobyVyhodov.po == nil {
		zhalobyVyhodov.po = map[string]map[string]ZhalobaYadra{}
	}
	poTegam := zhalobyVyhodov.po[konfig]
	if poTegam == nil {
		poTegam = map[string]ZhalobaYadra{}
		zhalobyVyhodov.po[konfig] = poTegam
	}
	poTegam[teg] = ZhalobaYadra{Prichina: prichina, Vremya: kogda}
}

// zabytZhalobyVyhodov зовётся запуском ядра: жалобы прошлого запуска с тем же
// конфигом говорят о ядре, которого уже нет.
func zabytZhalobyVyhodov(konfig string) {
	zhalobyVyhodov.mu.Lock()
	defer zhalobyVyhodov.mu.Unlock()
	delete(zhalobyVyhodov.po, konfig)
}

// ZhalobaVyhoda отдаёт последнюю узнанную жалобу ядра с конфигом konfig на
// выход teg, если она не старше posle. Второе значение false значит «ядро
// ничего узнаваемого не сказало», и тогда причину не называют вовсе.
func ZhalobaVyhoda(konfig, teg string, posle time.Time) (ZhalobaYadra, bool) {
	zhalobyVyhodov.mu.Lock()
	defer zhalobyVyhodov.mu.Unlock()
	z, est := zhalobyVyhodov.po[konfig][teg]
	if !est || z.Vremya.Before(posle) {
		return ZhalobaYadra{}, false
	}
	return z, true
}
