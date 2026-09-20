package sostoyanie

import "path/filepath"

// Two directories, two owners. Program files are read-only to the user; data is
// SYSTEM-only with inheritance broken, because it inherits from C:\ProgramData
// otherwise, and that hands every authenticated user a write bit on new files.
func KatalogProgrammy() string { return filepath.Join(`C:\Program Files`, "Affory") }

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
