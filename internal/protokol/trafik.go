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
}

type PravilaTrafika struct {
	PoUmolchaniyu MarshrutTrafika       `json:"po_umolchaniyu"`
	Prilozheniya  []PraviloPrilozheniya `json:"prilozheniya"`
	Domeny        []PraviloDomena       `json:"domeny"`
	Servisy       []PraviloServisa      `json:"servisy"`
}
