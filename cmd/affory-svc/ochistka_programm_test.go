package main

import (
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Чистка старых правил приложений (D2, 22.09.2026).

func naborSPrilozheniyami(apps ...protokol.PraviloPrilozheniya) *Nabor {
	return &Nabor{Pravila: PravilaNabora{Trafik: &protokol.PravilaTrafika{
		PoUmolchaniyu: protokol.TrafikVPN, Prilozheniya: apps}}}
}

func TestPraviloNaProgrammuServisaSnimaetsya(t *testing.T) {
	n := naborSPrilozheniyami(protokol.PraviloPrilozheniya{
		Put: `C:\Program Files (x86)\Steam\steam.exe`, Imya: "steam.exe", Potomki: true, Marshrut: protokol.TrafikPryamo})

	otchyot := n.snyatPravilaStavshieServisami()

	// Иначе один Steam остался бы в двух списках сразу - ровно то, из-за чего
	// списки и слили.
	if len(n.Pravila.Trafik.Prilozheniya) != 0 {
		t.Fatalf("правило осталось: %+v", n.Pravila.Trafik.Prilozheniya)
	}
	if len(otchyot) != 1 || !strings.Contains(otchyot[0], "steam") {
		t.Errorf("отчёт молчит о снятии: %v", otchyot)
	}
}

func TestChistkaNeSozdayotKartochkuServisa(t *testing.T) {
	// Снятие ничего не выдумывает: охват запускаемых программ у карточки
	// включён всегда, и молчаливо расширить маршрут на игры нельзя.
	n := naborSPrilozheniyami(protokol.PraviloPrilozheniya{
		Put: `C:\S\steam.exe`, Potomki: false, Marshrut: protokol.TrafikPryamo})

	n.snyatPravilaStavshieServisami()

	if len(n.Pravila.Trafik.Servisy) != 0 {
		t.Fatalf("маршрут выставлен за человека: %+v", n.Pravila.Trafik.Servisy)
	}
}

func TestSvoyuProgrammuChistkaNeTrogaet(t *testing.T) {
	// Игра, браузер, рабочий клиент: карточки сервиса для них нет, и снятое
	// правило нечем заменить.
	n := naborSPrilozheniyami(protokol.PraviloPrilozheniya{
		Put: `C:\Moyo\igra.exe`, Potomki: true, Marshrut: protokol.TrafikVPN})

	if otchyot := n.snyatPravilaStavshieServisami(); len(otchyot) != 0 {
		t.Fatalf("отчёт о том, чего не было: %v", otchyot)
	}
	if len(n.Pravila.Trafik.Prilozheniya) != 1 {
		t.Fatal("снято правило на программу вне каталога")
	}
}

func TestVetkiVypuskaSnimayutsyaTozhe(t *testing.T) {
	// Discord PTB это тот же Discord, и карточка знает оба имени.
	n := naborSPrilozheniyami(
		protokol.PraviloPrilozheniya{Put: `C:\D\Discord.exe`, Potomki: true, Marshrut: protokol.TrafikVPN},
		protokol.PraviloPrilozheniya{Put: `C:\D\DiscordPTB.exe`, Potomki: true, Marshrut: protokol.TrafikPryamo},
		protokol.PraviloPrilozheniya{Put: `C:\T\Telegram Desktop.exe`, Potomki: true, Marshrut: protokol.TrafikVPN})

	otchyot := n.snyatPravilaStavshieServisami()

	if len(n.Pravila.Trafik.Prilozheniya) != 0 {
		t.Fatalf("ветка выпуска или прежнее имя клиента не узнаны: %+v", n.Pravila.Trafik.Prilozheniya)
	}
	if len(otchyot) != 3 {
		t.Errorf("отчёт не по строке на правило: %v", otchyot)
	}
}

func TestChistkaIdyotTolkoOdinRaz(t *testing.T) {
	// На каждом чтении чистка снимала бы и то правило, которое человек завёл
	// руками уже после обновления.
	n := naborSPrilozheniyami(protokol.PraviloPrilozheniya{
		Put: `C:\S\steam.exe`, Potomki: true, Marshrut: protokol.TrafikPryamo})
	n.snyatPravilaStavshieServisami()
	if !n.Pravila.StaryeProgrammySnyaty {
		t.Fatal("флаг чистки не поднят: на диск уедет набор, который почистят заново")
	}

	n.Pravila.Trafik.Prilozheniya = []protokol.PraviloPrilozheniya{{
		Put: `C:\D\Discord.exe`, Potomki: true, Marshrut: protokol.TrafikVPN}}
	otchyot := n.snyatPravilaStavshieServisami()

	if len(otchyot) != 0 {
		t.Errorf("второй проход что-то сделал: %v", otchyot)
	}
	if len(n.Pravila.Trafik.Prilozheniya) != 1 {
		t.Fatal("правило, заведённое человеком после чистки, снято")
	}
}

func TestChistkaNaChteniiNaboraSluchaetsya(t *testing.T) {
	// Приведение живёт на чтении набора, а не в командах: иначе всякая новая
	// дверь к набору это шанс забыть его позвать.
	s := podstavnaya(t, nil)
	// Свой набор на диске, а не заглушка чтения: приведение живёт в чтении
	// хранилища, и проверять его надо там же.
	s.nabor = s.naborIzHranilishcha
	if err := s.pravitNabor(func(n *Nabor) error {
		n.Pravila.Trafik = &protokol.PravilaTrafika{PoUmolchaniyu: protokol.TrafikVPN,
			Prilozheniya: []protokol.PraviloPrilozheniya{{
				Put: `C:\S\steam.exe`, Imya: "steam.exe", Potomki: true, Marshrut: protokol.TrafikPryamo}}}
		// Набор прошлой версии флага не знает.
		n.Pravila.StaryeProgrammySnyaty = false
		return nil
	}); err != nil {
		t.Fatalf("набор не записан: %v", err)
	}

	n, err := s.nabor()
	if err != nil {
		t.Fatalf("набор не прочитан: %v", err)
	}
	if len(n.Pravila.Trafik.Prilozheniya) != 0 {
		t.Fatalf("на чтении чистка не случилась: %+v", n.Pravila.Trafik)
	}
}
