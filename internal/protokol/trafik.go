package protokol

type MarshrutTrafika string

const (
	TrafikVPN    MarshrutTrafika = "vpn"
	TrafikPryamo MarshrutTrafika = "direct"
)

type PraviloPrilozheniya struct {
	Put      string          `json:"put"`
	Imya     string          `json:"imya"`
	Potomki  bool            `json:"potomki"`
	Marshrut MarshrutTrafika `json:"marshrut"`
}

type PraviloDomena struct {
	Domen    string          `json:"domen"`
	Marshrut MarshrutTrafika `json:"marshrut"`
}

type PraviloServisa struct {
	Id       string          `json:"id"`
	Marshrut MarshrutTrafika `json:"marshrut"`
	// Programmy это ПУТИ клиента сервиса, найденные окном среди запущенных
	// программ (D2, 22.09.2026). В каталоге лежат только имена файлов: у
	// Discord в пути номер сборки, у лаунчеров диск установки, и прибитый путь
	// дал бы правило на несуществующий файл.
	//
	// Путей несколько, когда запущены ветки выпуска (Discord и Discord PTB) или
	// две копии одной программы. Маршрут у них общий: он у карточки один.
	Programmy []string `json:"programmy,omitempty"`
}

type PravilaTrafika struct {
	PoUmolchaniyu MarshrutTrafika       `json:"po_umolchaniyu"`
	Prilozheniya  []PraviloPrilozheniya `json:"prilozheniya"`
	Domeny        []PraviloDomena       `json:"domeny"`
	Servisy       []PraviloServisa      `json:"servisy"`
}
