// Package puti содержит расположение данных без доступа к состоянию службы.
package puti

const KatalogDannyh = `C:\ProgramData\Affory`
const PodkatalogZhurnalov = "log"

// Файлы падений и журнал окна (1.10.0). В файл падения Go пишет стек
// фатальной ошибки: консоли у службы и окна нет, и без файла стек пропадал.
const (
	PadenieSluzhby = "padenie-sluzhby.log"
	PadenieOkna    = "padenie-okna.log"
	ZhurnalOkna    = "okno.log"
)

// ZhurnalySluzhby это журналы службы в выгрузке диагностики, по порядку.
// Одним списком на службу и окно: окно читает их само, когда служба не
// отвечает. Журнал соединений сюда не входит, в нём каждый адрес, куда ходила
// машина. Имя подробного журнала сверяет тест службы с
// diagnostika.ImyaZhurnala.
var ZhurnalySluzhby = []string{
	"sluzhba.log", PadenieSluzhby, "yadro.log", "komandy.log", "obnovlenie.log", "ustanovka.log",
	"diagnostika.jsonl",
}
