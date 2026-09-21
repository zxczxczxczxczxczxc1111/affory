package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Жалоба 21.09.2026: человек обновился с 1.0.3 на 1.4.1, а в наборе осталось
// правило сервиса whatsapp, которого каталог с 16.09.2026 не знает. Продукт не
// собирал конфиг вовсе, то есть не подключался ни разу, и снять правило было
// негде: окно рисует список из каталога.

// polozhitSirotu кладёт в набор правило на сервис, которого в каталоге нет,
// рядом с живым правилом. Ровно то, что лежало у человека из жалобы.
func polozhitSirotu(t *testing.T, s *Sluzhba) {
	t.Helper()
	// Настоящий путь чтения: фикстура подменяет s.nabor своей картой в памяти и
	// приведение обходит, а проверяется здесь именно приведение.
	s.nabor = s.naborIzHranilishcha
	if err := s.pravitNabor(func(n *Nabor) error {
		n.Pravila.Trafik = &protokol.PravilaTrafika{
			PoUmolchaniyu: protokol.TrafikVPN,
			Servisy: []protokol.PraviloServisa{
				{Id: "whatsapp", Marshrut: protokol.TrafikPryamo},
				{Id: "youtube", Marshrut: protokol.TrafikVPN},
			},
		}
		return nil
	}); err != nil {
		t.Fatalf("набор не записан: %v", err)
	}
}

func TestOsirotevsheePraviloSnimaetsyaNaChtenii(t *testing.T) {
	s := podstavnaya(t, nil)
	polozhitSirotu(t, s)

	n, err := s.nabor()
	if err != nil {
		t.Fatalf("набор не читается: %v", err)
	}
	if n.Pravila.Trafik == nil {
		t.Fatal("правила пропали целиком")
	}
	if len(n.Pravila.Trafik.Servisy) != 1 || n.Pravila.Trafik.Servisy[0].Id != "youtube" {
		t.Fatalf("после приведения остались %+v, ждали только youtube", n.Pravila.Trafik.Servisy)
	}
}

func TestPrivestiPravilaIdempotentna(t *testing.T) {
	// Чтение идёт на каждую команду. Приведение, которое говорит о себе на
	// каждом чтении приведённого набора, это шум в журнале, а не починка.
	n := Nabor{Pravila: PravilaNabora{Trafik: &protokol.PravilaTrafika{
		PoUmolchaniyu: protokol.TrafikVPN,
		Servisy:       []protokol.PraviloServisa{{Id: "youtube", Marshrut: protokol.TrafikVPN}},
	}}}
	if p := n.PrivestiPravila(); len(p) != 0 {
		t.Fatalf("здоровый набор объявлен починенным: %v", p)
	}
	n.Pravila.Trafik.Servisy = append(n.Pravila.Trafik.Servisy,
		protokol.PraviloServisa{Id: "whatsapp", Marshrut: protokol.TrafikPryamo})
	pervyy := n.PrivestiPravila()
	if len(pervyy) != 1 || !strings.Contains(pervyy[0], "whatsapp") {
		t.Fatalf("починка не названа: %v", pervyy)
	}
	if vtoroy := n.PrivestiPravila(); len(vtoroy) != 0 {
		t.Fatalf("второй проход опять чинит: %v", vtoroy)
	}
}

func TestPodyomSOsirotevshimPravilomNePadaet(t *testing.T) {
	// Главное в жалобе: конфиг не собирался, и подключиться было нельзя.
	s := podstavnaya(t, nil)
	polozhitSirotu(t, s)
	if err := s.Connect(context.Background()); err != nil {
		t.Fatalf("подъём с осиротевшим правилом: %v", err)
	}
}

// Окно, открытое в момент обновления службы, держит СТАРЫЙ список и шлёт его
// целиком, включая правило на ушедший сервис. Отказ на весь список означал бы,
// что до перезапуска окна не сохранить ни одной правки.
func TestProveritTrafikTerpitOsirotevshiyServis(t *testing.T) {
	bylo := protokol.PravilaTrafika{PoUmolchaniyu: protokol.TrafikVPN,
		Servisy: []protokol.PraviloServisa{{Id: "whatsapp", Marshrut: protokol.TrafikPryamo}}}
	prislano := protokol.PravilaTrafika{PoUmolchaniyu: protokol.TrafikVPN,
		Servisy: []protokol.PraviloServisa{
			{Id: "whatsapp", Marshrut: protokol.TrafikPryamo},
			{Id: "telegram", Marshrut: protokol.TrafikPryamo},
		}}
	prinyato, err := proveritTrafik(prislano, bylo)
	if err != nil {
		t.Fatalf("отказ на списке с осиротевшим правилом: %v", err)
	}
	if len(prinyato.Servisy) != 1 || prinyato.Servisy[0].Id != "telegram" {
		t.Fatalf("приняты %+v, ждали только telegram", prinyato.Servisy)
	}
}

// А сервис, которого не было ни в каталоге, ни в наборе, это ошибка клиента, и
// она называется вслух: молча потерянное правило человек считает работающим.
func TestSetRulesOtvergaetVydumannyyServis(t *testing.T) {
	s := podstavnaya(t, nil)
	telo, _ := json.Marshal(map[string]any{"trafik": map[string]any{
		"po_umolchaniyu": "vpn",
		"servisy":        []map[string]any{{"id": "vydumka", "marshrut": "direct"}},
	}})
	o := s.Obrabotat(ctxAdmina(), protokol.Kadr{Id: 1, Imya: "setRules", Telo: telo})
	if o.Oshib == nil || o.Oshib.Kod != protokol.KodPraviloNegodno {
		t.Fatalf("выдуманный сервис принят или отказ не тем кодом: %+v", o.Oshib)
	}
	if !strings.Contains(o.Oshib.Tekst, "vydumka") {
		t.Fatalf("отказ не называет сервис: %s", o.Oshib.Tekst)
	}
}
