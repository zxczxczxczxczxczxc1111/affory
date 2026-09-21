package sostoyanie

import (
	"os"
	"path/filepath"
)

// Two directories, two owners. Program files are read-only to the user; data is
// SYSTEM-only with inheritance broken, because it inherits from C:\ProgramData
// otherwise, and that hands every authenticated user a write bit on new files.

// katalogProgrammy пуст у службы и заполнен у подменщика: см. ниже.
var katalogProgrammy string

// KatalogProgrammy отдаёт каталог, где лежат наши файлы: ядро, окно, служба.
//
// Это каталог СВОЕГО бинаря, а не постоянный путь. До 21.09.2026 здесь стояла
// константа `C:\Program Files\Affory`, а установщик при этом спрашивал каталог
// и слушался ответа. Человек, поставивший программу на другой диск, получал
// отказ на первом же шаге установки («каталог программы не читается»), причём
// файлы к тому моменту уже лежали там, где он просил. По той же константе
// ищется ядро, окно для автозапуска и хвосты обновления, то есть установка
// мимо `C:\Program Files` не работала целиком, просто падала раньше всего
// остального.
//
// Свой путь знают все, кому этот каталог нужен: службу запускает SCM по
// её ImagePath, подкоманды запускает установщик из каталога установки.
// Исключение ровно одно, и оно подменяет путь явно (PodmenitKatalogProgrammy).
func KatalogProgrammy() string {
	if katalogProgrammy != "" {
		return katalogProgrammy
	}
	put, err := os.Executable()
	if err != nil {
		// Последняя опора. GetModuleFileName не отказывает на живой машине, но
		// пустая строка здесь увела бы службу в корень диска молча, а прежний
		// постоянный путь верен для всех, кто согласился с каталогом по
		// умолчанию, то есть для подавляющего большинства.
		return filepath.Join(`C:\Program Files`, "Affory")
	}
	return filepath.Dir(put)
}

// PodmenitKatalogProgrammy называет каталог программы явно.
//
// Зовётся ОДНИМ местом: подменщиком обновления. Он копия службы, работающая из
// временного каталога, и свой путь ему врёт — каталог программы он получает
// аргументом. Без этой строки любой будущий вызов KatalogProgrammy внутри
// подменщика тихо указал бы в `%TEMP%`.
func PodmenitKatalogProgrammy(put string) { katalogProgrammy = put }

// korenDannyh пуст в проде и означает `C:\ProgramData\Affory`.
//
// Переменная нужна ровно одному: прогону тестов. Служба собирается из тех же
// пакетов, что и её тесты, и конструктор службы подключает НАСТОЯЩЕЕ хранилище
// секретов по этому пути. Фикстура, забывшая подменить шов записи, пишет тем
// самым в блоб живой машины — и 20.09.2026 это стоило четырёх разборов подряд:
// `TestZhurnalKomandNePishetTel` слал `setSubscription`, и тестовая подписка
// `panel.example` появлялась у владельца после каждой сборки выпуска. Внутри
// продукта это выглядело как запись, возникшая без единой команды.
//
// Подменять путь в каждой фикстуре — сорок мест, где можно забыть. Подменять
// один раз в TestMain — ноль таких мест.
var korenDannyh string

// KatalogDannyh отдаёт каталог данных: в проде постоянный, под тестами
// подменённый.
func KatalogDannyh() string {
	if korenDannyh != "" {
		return korenDannyh
	}
	return filepath.Join(`C:\ProgramData`, "Affory")
}

// PodmenitKatalogDannyh уводит каталог данных в сторону на весь процесс.
//
// Зовётся ТОЛЬКО из TestMain тестовых пакетов, и это стережёт
// TestNiktoNePodmenyaetKatalogVProde. В проде вызова нет ни одного: служба
// обязана работать с тем путём, под который выставлены права.
func PodmenitKatalogDannyh(put string) { korenDannyh = put }

// Журналы компонентов лежат в подкаталоге данных, по файлу на компонент (§5
// п.6 спеки). Права наследуются от каталога данных через (OI)(CI): читают
// SYSTEM и админы, больше никто.
func KatalogZhurnalov() string { return filepath.Join(KatalogDannyh(), "log") }
