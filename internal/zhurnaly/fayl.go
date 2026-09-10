// Пакет zhurnaly это файл журнала, который не съедает диск.
//
// Журналы у нас файлы, а не журнал событий Windows: файл читается одной
// командой, а журнал событий надо разбирать, и он ломает кодировку (§13 спеки).
// Плата за файл ровно одна: он растёт вечно, если этим не заняться. Поэтому
// ротация живёт ЗДЕСЬ, у самого файла, а не в каждом, кто пишет.
package zhurnaly

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Из §5 п.6 спеки: ротация по 10 МБ, хранятся три файла. Три это ВСЕГО, вместе
// с текущим: `x.log`, `x.log.1`, `x.log.2`, то есть не больше 30 МБ на
// компонент.
const (
	Predel       int64 = 10 << 20
	HranimFaylov       = 3
)

// ErrZakryt отличает «журнал закрыт» от «диск отвалился». Первое нормальная
// гонка при остановке службы, второе новость.
var ErrZakryt = errors.New("журнал закрыт")

type Fayl struct {
	put    string
	predel int64

	mu     sync.Mutex
	f      *os.File
	razmer int64
}

// Otkryt открывает журнал компонента в каталоге журналов.
func Otkryt(katalog, imya string) (*Fayl, error) { return otkrytS(katalog, imya, Predel) }

// Предел параметром только ради тестов: гонять настоящие 10 МБ на каждый
// прогон значит тест, который никто не станет ждать.
func otkrytS(katalog, imya string, predel int64) (*Fayl, error) {
	if err := os.MkdirAll(katalog, 0o700); err != nil {
		return nil, fmt.Errorf("каталог журналов %s: %w", katalog, err)
	}
	z := &Fayl{put: filepath.Join(katalog, imya), predel: predel}
	if err := z.otkryt(); err != nil {
		return nil, err
	}
	return z, nil
}

// Открытие ДОПИСЫВАЮЩЕЕ. Служба перезапускается чаще, чем журнал переполняется,
// и усечение при старте стёрло бы ровно тот кусок, ради которого в журнал лезут:
// последние строки перед падением.
func (z *Fayl) otkryt() error {
	f, err := os.OpenFile(z.put, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("журнал %s: %w", z.put, err)
	}
	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return fmt.Errorf("размер журнала %s: %w", z.put, err)
	}
	z.f, z.razmer = f, st.Size()
	return nil
}

// Write пишет байты как есть: ни BOM, ни CR. Спека требует UTF-8 и LF, а
// «windows-привычка» дописать возврат каретки сломала бы разбор журнала теми же
// средствами, что и на сервере.
func (z *Fayl) Write(p []byte) (int, error) {
	z.mu.Lock()
	defer z.mu.Unlock()
	if z.f == nil {
		return 0, ErrZakryt
	}
	// Ротация ДО записи, а не после: иначе файл сначала перерастает предел, и
	// «10 МБ» превращается в «10 МБ плюс сколько дали». Пустой файл не
	// проворачивается никогда: запись, которая одна длиннее предела, иначе
	// выбросила бы два предыдущих файла и всё равно превысила предел.
	if z.razmer > 0 && z.razmer+int64(len(p)) > z.predel {
		if err := z.provernut(); err != nil {
			return 0, err
		}
	}
	n, err := z.f.Write(p)
	z.razmer += int64(n)
	return n, err
}

// provernut сдвигает файлы от старшего к младшему. Порядок обязателен: сдвиг
// от младшего затёр бы старший ещё до того, как тот уедет дальше.
func (z *Fayl) provernut() error {
	_ = z.f.Close()
	z.f = nil
	starshiy := HranimFaylov - 1
	_ = os.Remove(fmt.Sprintf("%s.%d", z.put, starshiy))
	for i := starshiy - 1; i >= 1; i-- {
		_ = os.Rename(fmt.Sprintf("%s.%d", z.put, i), fmt.Sprintf("%s.%d", z.put, i+1))
	}
	// Ошибка переименования журнал НЕ останавливает: файл могли удалить руками
	// или держать открытым просмотрщиком, и терять из-за этого всю дальнейшую
	// запись значит слепнуть в самый интересный момент.
	_ = os.Rename(z.put, z.put+".1")
	z.razmer = 0
	return z.otkryt()
}

func (z *Fayl) Close() error {
	z.mu.Lock()
	defer z.mu.Unlock()
	if z.f == nil {
		return nil
	}
	err := z.f.Close()
	z.f = nil
	return err
}

// Ochistit опустошает журнал, оставляя его рабочим.
//
// Кнопка «Очистить журнал» в интерфейсе обещает стереть журналы, и оставить
// один нетронутым значит соврать.
//
// Через ПЕРЕОТКРЫТИЕ, а не Truncate: файл открыт с O_APPEND, и Windows на
// усечение такого дескриптора отвечает «Access is denied». Проверено прогоном
// 10.09.2026, первая версия падала ровно здесь. Удаление тоже не годится:
// открытый файл Windows удалить не даст.
func (z *Fayl) Ochistit() error {
	z.mu.Lock()
	defer z.mu.Unlock()
	if z.f == nil {
		return ErrZakryt
	}
	if err := z.f.Close(); err != nil {
		return fmt.Errorf("журнал %s не закрывается: %w", z.put, err)
	}
	f, err := os.OpenFile(z.put, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		// Дескриптора больше нет, и притворяться, что журнал жив, нельзя:
		// следующая запись должна честно сказать «закрыт».
		z.f = nil
		return fmt.Errorf("журнал %s не переоткрывается: %w", z.put, err)
	}
	z.f, z.razmer = f, 0
	return nil
}
