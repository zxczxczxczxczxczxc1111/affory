package hranenie

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/sostoyanie"
)

// ImyaSekretov публично ради тестов и ради стенда: живая проверка обязана уметь
// подложить чужой блоб, а для этого знать имя.
const ImyaSekretov = "sekrety.dat"

// Пять попыток и удвоение паузы от секунды это около пятнадцати секунд.
//
// Потолок нужен обязательно. Цикл повторов без предела не проваливает старт
// службы, а ВЕШАЕТ его, и «висит» это единственный исход, под который в §9.1
// нет экрана вовсе.
const PopytokRasshifrovki = 5

var ErrSekretyNechitaemy = errors.New("секреты не расшифровываются на этой машине")

// Shifrovshchik это DPAPI МАШИННЫЙ, и это решение, а не умолчание.
//
// Служба стартует до входа пользователя в систему, и пользовательского
// контекста в этот момент физически нет. Цена принята осознанно и записана в
// спеке: администратор этой машины расшифрует блоб. Защита здесь от кражи
// файла, а не от локального администратора, которым мы и так являемся.
//
// Дополнительной энтропии нет намеренно. Она лежала бы в нашем же исполняемом
// файле рядом с блобом, то есть добавляла бы работу нам и ноль работы тому, кто
// унёс каталог целиком.
type Shifrovshchik struct {
	dir string

	// Швы. Настоящий DPAPI в тесте недостижим по временным отказам: LSASS у нас
	// всегда готов, а весь смысл повторов именно в том, что бывает наоборот.
	zashifrovat  func([]byte) ([]byte, error)
	rasshifrovat func([]byte) ([]byte, error)

	Chasy func() time.Time
	Spat  func(time.Duration)
}

func Novyy() *Shifrovshchik { return NovyyV(sostoyanie.KatalogDannyh()) }

func NovyyV(dir string) *Shifrovshchik {
	return &Shifrovshchik{
		dir: dir, zashifrovat: dpapiZashifrovat, rasshifrovat: dpapiRasshifrovat,
		Chasy: time.Now, Spat: time.Sleep,
	}
}

func (s *Shifrovshchik) put() string { return filepath.Join(s.dir, ImyaSekretov) }

// Sohranit шифрует и кладёт на диск атомарно.
//
// Атомарность здесь важнее, чем у файла состояния. Недописанный блоб не
// расшифруется НИКОГДА, то есть выглядит ровно как мёртвый ключ, а мёртвый ключ
// мы хороним. Оборванная запись иначе стоила бы человеку всех его серверов.
func (s *Shifrovshchik) Sohranit(telo []byte) error {
	blob, err := s.zashifrovat(telo)
	if err != nil {
		return fmt.Errorf("секреты не шифруются: %w", err)
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("каталог данных недоступен: %w", err)
	}
	vremen := s.put() + ".tmp"
	if err := os.WriteFile(vremen, blob, 0o600); err != nil {
		return fmt.Errorf("секреты не записаны: %w", err)
	}
	if err := s.pereimenovat(vremen, s.put()); err != nil {
		_ = os.Remove(vremen)
		return fmt.Errorf("секреты не переименованы: %w", err)
	}
	return nil
}

// PopytokPereimenovaniya bounds the wait for a busy target: with the pause
// doubling from 50 ms this is about three seconds, longer than any scanner
// holds a 1 KB file and shorter than a human starts wondering.
const PopytokPereimenovaniya = 7

// pereimenovat is os.Rename that outlives a busy target. On Windows a file
// held open by anybody (Defender scanning the fresh write, our own reader a
// moment earlier) refuses to be replaced with ERROR_SHARING_VIOLATION or
// ERROR_ACCESS_DENIED, and the guest run of 02.09.2026 lost a subscription
// refresh to exactly that. Anything else is a real error and returns at once.
func (s *Shifrovshchik) pereimenovat(ot, k string) error {
	pauza := 50 * time.Millisecond
	var posledn error
	for i := 0; i < PopytokPereimenovaniya; i++ {
		posledn = os.Rename(ot, k)
		if posledn == nil || !zanyat(posledn) {
			return posledn
		}
		if i < PopytokPereimenovaniya-1 {
			s.Spat(pauza)
			pauza *= 2
		}
	}
	return posledn
}

func zanyat(err error) bool {
	var errno windows.Errno
	if !errors.As(err, &errno) {
		return false
	}
	return errno == windows.ERROR_SHARING_VIOLATION || errno == windows.ERROR_ACCESS_DENIED
}

// Zagruzit читает блоб, повторяя при отказе.
//
// Отсутствие файла это НЕ ошибка: первый запуск, серверов ещё не добавляли.
// Показать на этом secrets-unreadable значило бы напугать человека тем, что всё
// идёт по плану.
func (s *Shifrovshchik) Zagruzit() ([]byte, error) {
	blob, err := os.ReadFile(s.put())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("секреты не читаются: %w", err)
	}

	// Повторы, а не приговор с первой попытки. CryptUnprotectData это вызов в
	// LSASS, и до входа пользователя он отказывает по причинам, не имеющим
	// отношения к целости ключа. Похоронить блоб на первом же отказе значит
	// превратить временную занятость системы в окончательный вердикт.
	pauza := time.Second
	var posledn error
	for i := 0; i < PopytokRasshifrovki; i++ {
		telo, err := s.rasshifrovat(blob)
		if err == nil {
			return telo, nil
		}
		posledn = err
		if i < PopytokRasshifrovki-1 {
			s.Spat(pauza)
			pauza *= 2
		}
	}

	// Попытки кончились. Теперь это приговор, и блоб убирается с дороги, иначе
	// следующий запуск упрётся в него же и так до конца времён.
	imya, err := s.pohoronit()
	if err != nil {
		return nil, fmt.Errorf("%w: %v (и убрать блоб не удалось: %v)",
			ErrSekretyNechitaemy, posledn, err)
	}
	return nil, fmt.Errorf("%w: %v (блоб отложен как %s)", ErrSekretyNechitaemy, posledn, imya)
}

// pohoronit переименовывает мёртвый блоб.
//
// В суффиксе дата И ВРЕМЯ. С одной датой второй отказ за те же сутки перетирает
// первый, переживший блоб оказывается вторым, и по журналу этого не видно.
func (s *Shifrovshchik) pohoronit() (string, error) {
	imya := fmt.Sprintf("%s.mertvyy-%s", ImyaSekretov, s.Chasy().Format("2006-01-02-150405"))
	novyy := filepath.Join(s.dir, imya)
	// Секунды всё-таки конечны, а два отказа подряд бывают быстрее секунды.
	// Затирать уже отложенный блоб нельзя по той же причине, по какой заведено
	// время в имени.
	for i := 1; ; i++ {
		if _, err := os.Stat(novyy); os.IsNotExist(err) {
			break
		}
		novyy = filepath.Join(s.dir, fmt.Sprintf("%s-%d", imya, i))
	}
	if err := os.Rename(s.put(), novyy); err != nil {
		return "", err
	}
	return filepath.Base(novyy), nil
}

// dpapiZashifrovat шифрует с флагом CRYPTPROTECT_LOCAL_MACHINE.
//
// Без флага блоб привязывается к учётной записи, и служба из-под SYSTEM
// прочитать его не сможет вовсе. Это и есть контроль к живой проверке в госте:
// тот же блоб без флага обязан дать secrets-unreadable.
func dpapiZashifrovat(telo []byte) ([]byte, error) {
	return dpapi(telo, true)
}

func dpapiRasshifrovat(blob []byte) ([]byte, error) {
	return dpapi(blob, false)
}

func dpapi(telo []byte, shifrovat bool) ([]byte, error) {
	// Пустой срез нельзя отдавать как указатель на первый элемент: его нет.
	if len(telo) == 0 {
		return nil, errors.New("пустое тело")
	}
	vhod := windows.DataBlob{Size: uint32(len(telo)), Data: &telo[0]}
	var vyhod windows.DataBlob

	var err error
	if shifrovat {
		err = windows.CryptProtectData(&vhod, nil, nil, 0, nil,
			windows.CRYPTPROTECT_LOCAL_MACHINE|windows.CRYPTPROTECT_UI_FORBIDDEN, &vyhod)
	} else {
		err = windows.CryptUnprotectData(&vhod, nil, nil, 0, nil,
			windows.CRYPTPROTECT_UI_FORBIDDEN, &vyhod)
	}
	if err != nil {
		return nil, err
	}
	// LocalFree обязателен: DPAPI выделяет буфер сам, и без освобождения служба,
	// живущая месяцами, течёт на каждом чтении секретов.
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(vyhod.Data)))

	// Копия, а не срез поверх чужой памяти: буфер освобождается прямо здесь
	// отложенным вызовом, и срез пережил бы его на любой срок.
	nazad := make([]byte, vyhod.Size)
	copy(nazad, unsafe.Slice(vyhod.Data, vyhod.Size))
	return nazad, nil
}
