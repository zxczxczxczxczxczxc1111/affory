package yadra

import (
	"bytes"
	"errors"
	"log"
	"strings"
	"sync"
	"time"
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
	imya  string
	ost   []byte
	strok int
	skazl bool
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
var ErrDrayverNeVstal = errors.New("драйвер адаптера не установился")

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
