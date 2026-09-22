package diagnostika

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"runtime/debug"
	"sync"
	"time"
)

// Контекст сетевой операции (A7, 22.09.2026).
//
// Разбор журнала живой машины 22.09.2026 упёрся не в отсутствие строк, а в
// невозможность связать их между собой. По записям нельзя было ответить на
// четыре вопроса сразу: какая попытка это была, сколько она длилась, к какому
// поколению запроса относилась и какая сборка её делала. Длительность падения
// считалась скриптом по двум разным строкам, а совпадение операций угадывалось
// по времени.
//
// Отсюда состав: у каждой операции свой короткий идентификатор, своя
// длительность, своё поколение и отпечаток сборки. Адрес подписки и
// идентификатор сервера в журнал не идут НИКОГДА: адрес подписки это пропуск, а
// не имя, - поэтому от них остаётся обезличенный отпечаток, по которому две
// строки сходятся между собой и не сходятся ни с чем снаружи.

// Operatsiya это одна сетевая операция службы с её исходом.
type Operatsiya struct {
	// Vid: «podpiska», «podklyuchenie», «proba». Короткое слово, не фраза:
	// по нему группируют строки.
	Vid string
	// Pokolenie того счётчика, которым операция себя проверяет. Ноль значит
	// «операция без поколения», а не «первое».
	Pokolenie  uint64
	Dlitelnost time.Duration
	// Itog: «ok» или код отказа §9.1. Пусто не бывает: строка операции без
	// исхода не отвечает ни на один вопрос.
	Itog string
	// Shag коротко называет, ГДЕ сорвалось. У сетевой загрузки это sboi.Vid
	// («dns», «tls», «srok»), у подъёма туннеля стадия подъёма («tunnel»,
	// «yadro», «vybor», «proba»). Пусто у успеха.
	Shag string
	// Istochnik и Uzel уже ОБЕЗЛИЧЕНЫ вызывающим через Obezlichit.
	Istochnik string
	Uzel      string
}

// NomerOperatsii даёт короткий случайный идентификатор.
//
// Случайный, а не счётчик: счётчик начинается заново при каждом запуске
// службы, и две строки из разных запусков совпали бы номерами.
func NomerOperatsii() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Провал генератора не повод терять строку журнала целиком: имя
		// «неизвестно» честнее, чем отсутствие записи об операции.
		return "neizvestno"
	}
	return hex.EncodeToString(b[:])
}

// Obezlichit превращает секрет в устойчивый отпечаток.
//
// Восемь шестнадцатеричных знаков от SHA-256. Этого хватает, чтобы связать
// строки одной подписки между собой, и не хватает, чтобы узнать адрес: обратно
// не разворачивается, а перебор по словарю упирается в то, что в журнале нет
// ни одной подсказки, что именно хешировали.
func Obezlichit(secret string) string {
	if secret == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:4])
}

var (
	razSborka sync.Once
	sborka    string
)

// Sborka это версия программы вместе с отпечатком дерева, из которого она
// собрана.
//
// Ревизия берётся у самого бинаря (debug.ReadBuildInfo), а не подставляется
// сборочным скриптом: скрипт пришлось бы менять, а ревизия уже лежит внутри
// любого бинаря, собранного из репозитория. При сборке без git остаётся одна
// версия, и это честно: выдуманный отпечаток хуже отсутствующего.
func Sborka(versiya string) string {
	razSborka.Do(func() {
		sborka = versiya
		info, ok := debug.ReadBuildInfo()
		if !ok {
			return
		}
		var reviziya string
		izmeneno := false
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				if len(s.Value) >= 7 {
					reviziya = s.Value[:7]
				}
			case "vcs.modified":
				izmeneno = s.Value == "true"
			}
		}
		sborka = sobratSborku(versiya, reviziya, izmeneno)
	})
	return sborka
}

// sobratSborku складывает строку сборки. Отдельно от чтения build info: под
// `go test` ревизии нет вовсе, и проверить все три случая на настоящем чтении
// физически нечем.
func sobratSborku(versiya, reviziya string, izmeneno bool) string {
	if reviziya == "" {
		return versiya
	}
	s := versiya + "+" + reviziya
	// Сборка из ПРАВЛЕНОГО дерева называет не тот код, что работает. Без
	// пометки журнал отладочной сборки указывал бы на коммит, в котором
	// искомой строки нет вовсе.
	if izmeneno {
		s += "-izmeneno"
	}
	return s
}

// SobytieOperatsii пишет строку об операции вне очереди тиков.
func (z *Zhurnal) SobytieOperatsii(o Operatsiya) error {
	if z == nil || z.kuda == nil {
		return nil
	}
	return z.Pisat(Srez{
		Vremya:         z.seychas(),
		Sobytie:        "операция " + o.Vid,
		OpId:           NomerOperatsii(),
		OpPokolenie:    o.Pokolenie,
		OpDlitelnostMs: float64(o.Dlitelnost) / float64(time.Millisecond),
		OpItog:         o.Itog,
		OpShag:         o.Shag,
		OpIstochnik:    o.Istochnik,
		OpUzel:         o.Uzel,
		Sborka:         z.sborkaSnimok(),
	})
}

// sborkaSnimok читает отпечаток ПОД замком: операции пишутся из фоновых
// горутин, а задаётся отпечаток на открытии журнала. Чтение мимо замка это
// гонка, которую детектор поймал бы только при неудачном совпадении.
func (z *Zhurnal) sborkaSnimok() string {
	z.mu.Lock()
	defer z.mu.Unlock()
	return z.sborka
}

// ZadatSborku запоминает отпечаток сборки для строк операций. Отдельным
// вызовом, а не доводом конструктора: журнал заводится раньше, чем служба
// добирается до своей версии.
func (z *Zhurnal) ZadatSborku(s string) {
	if z == nil {
		return
	}
	z.mu.Lock()
	defer z.mu.Unlock()
	z.sborka = s
}
