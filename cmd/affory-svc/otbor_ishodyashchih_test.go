package main

import (
	"encoding/json"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/yadra"
)

// Находка 43, класс тот же, что у находки 33: один негодный сервер в списке не
// даёт подняться туннелю ВООБЩЕ. Тогда закрыли частный случай (проверку ключа
// при разборе ссылки), и в записи честно сказали, чего не сделали: «ядро не
// приняло то, что собрал генератор» не проверялось никогда. Через сутки чужая
// подписка из 39 серверов принесла второй случай того же класса.
//
// Здесь проверяется КЛАСС: что бы ядро ни отвергло, названный им сервер
// исключается, а остальные поднимаются.
func TestOtvergnutyyYadromServerIsklyuchaetsyaAOstalnyePodnimayutsya(t *testing.T) {
	s := podstavnaya(t, nil)
	plohoy := protokol.Server{
		// Транспорт ИЗВЕСТНЫЙ намеренно: здесь проверяется отказ ЯДРА, а не наш
		// собственный пропуск неизвестного транспорта. Поставь сюда xhttp, и тест
		// зазеленеет по другой причине, то есть перестанет проверять находку 43.
		Id: "plohoy", Imya: "негодный", Transport: "ws",
		Host: "203.0.113.9", Port: 443, Uuid: "11111111-2222-3333-4444-555555555555", Put: "/ws",
	}
	s.nabor = func() (Nabor, error) {
		return Nabor{Servery: []protokol.Server{serverProby(), plohoy}, Vybran: "nl"}, nil
	}
	s.sobratAdresa = func() ([]netip.Addr, error) {
		return []netip.Addr{
			netip.MustParseAddr("192.0.2.225"),
			netip.MustParseAddr("203.0.113.9"),
		}, nil
	}

	// Ядро изображается швом: настоящее ядро в тесте недоступно, а поведение
	// проверяется НАШЕ, а не его. Отвергается ровно тот исходящий, у которого
	// тег негодного сервера, и ровно один раз: второй заход обязан пройти.
	zvali := 0
	s.proveritKonfig = func(put string) error {
		zvali++
		n, est := nomerPoTegu(t, put, "srv-plohoy")
		if !est {
			return nil
		}
		return &yadra.OshibkaKonfiga{
			Vyhod: "initialize outbound[" + strconv.Itoa(n) + "]: invalid public_key",
			Nomer: n, EstNomer: true,
		}
	}

	id, sob := s.Podpisatsya()
	defer s.Otpisatsya(id)

	put := filepath.Join(t.TempDir(), "sing-box.json")
	if _, _, err := s.sobratTunProverennyy(put); err != nil {
		t.Fatalf("подъём отказал целиком из-за ОДНОГО негодного сервера: %v", err)
	}
	if zvali < 2 {
		t.Fatalf("ядро спросили %d раз: без повторной проверки исключение ничем не подтверждено", zvali)
	}

	telo, err := os.ReadFile(put)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(telo), "srv-plohoy") {
		t.Error("отвергнутый сервер остался в конфиге: ядро отвергнет его снова")
	}
	if !strings.Contains(string(telo), "srv-nl") {
		t.Error("вместе с негодным ушёл рабочий сервер: это и есть находка 43")
	}

	// Молча выкинуть сервер значит оставить человека с подпиской, которая тихо
	// стала короче. Ровно то же правило, что у исключения по неразрешившемуся
	// имени.
	nashli := false
	for len(sob) > 0 {
		k := <-sob
		if k.Imya == "serversRejected" &&
			strings.Contains(string(k.Telo), protokol.KodServerRejectedByCore) {
			nashli = true
		}
	}
	if !nashli {
		t.Error("об исключённом сервере не сказали: подписка тихо стала короче")
	}
}

// Обратная сторона: если ядро отвергло ВЫБРАННЫЙ сервер, исключать его нельзя.
// Молча подняться на другом значило бы увести трафик не туда, куда просили, и
// человек об этом не узнал бы.
func TestOtvergnutyyVybrannyyServerEtoOtkazANePodmena(t *testing.T) {
	s := podstavnaya(t, nil)
	s.nabor = func() (Nabor, error) {
		return Nabor{Servery: []protokol.Server{serverProby(), vtoroyServer()}, Vybran: "nl"}, nil
	}
	s.sobratAdresa = func() ([]netip.Addr, error) {
		return []netip.Addr{
			netip.MustParseAddr("192.0.2.225"),
			netip.MustParseAddr("203.0.113.9"),
		}, nil
	}
	s.proveritKonfig = func(put string) error {
		n, est := nomerPoTegu(t, put, "srv-nl")
		if !est {
			return nil
		}
		return &yadra.OshibkaKonfiga{
			Vyhod: "initialize outbound[" + strconv.Itoa(n) + "]: invalid public_key",
			Nomer: n, EstNomer: true,
		}
	}

	put := filepath.Join(t.TempDir(), "sing-box.json")
	_, _, err := s.sobratTunProverennyy(put)
	if err == nil {
		t.Fatal("отказа не было: служба молча увела трафик на другой сервер")
	}
	if !strings.Contains(err.Error(), "invalid public_key") {
		t.Errorf("причина от ядра потеряна по дороге, осталось: %v", err)
	}
}

// Отказ БЕЗ номера исходящего исключать нечего, и притворяться, что исключили,
// нельзя: это привело бы к бесконечному кругу на неразбираемом конфиге.
func TestOtkazBezNomeraIshodyashchegoNeIsklyuchaetNikogo(t *testing.T) {
	s := podstavnaya(t, nil)
	zvali := 0
	s.proveritKonfig = func(string) error {
		zvali++
		return &yadra.OshibkaKonfiga{Vyhod: "decode config: invalid character"}
	}
	put := filepath.Join(t.TempDir(), "sing-box.json")
	if _, _, err := s.sobratTunProverennyy(put); err == nil {
		t.Fatal("неразбираемый конфиг принят за годный")
	}
	if zvali != 1 {
		t.Errorf("ядро спросили %d раз: без номера исключать некого, повторять бессмысленно", zvali)
	}
}

func nomerPoTegu(t *testing.T, put string, teg string) (int, bool) {
	t.Helper()
	telo, err := os.ReadFile(put)
	if err != nil {
		t.Fatal(err)
	}
	var k struct {
		Outbounds []struct {
			Tag string `json:"tag"`
		} `json:"outbounds"`
	}
	if err := json.Unmarshal(telo, &k); err != nil {
		t.Fatal(err)
	}
	for i, o := range k.Outbounds {
		if o.Tag == teg {
			return i, true
		}
	}
	return 0, false
}

// В авто выбранного сервера НЕТ, значит и защищать от исключения нечего. Пока
// vybrannyyId отвечает Servery[0], одна плохая ссылка в подписке роняет подъём
// целиком вместо исключения одного сервера: класс, закрытый коммитом 45eeb11,
// открывается заново ровно в том режиме, который становится умолчанием.
func TestVAvtoIsklyuchaetsyaLyuboyKandidat(t *testing.T) {
	s := podstavnaya(t, nil)
	n := Nabor{
		Servery: []protokol.Server{serverProby(), vtoroyServer()},
		Rezhim:  protokol.RezhimAvto,
	}
	s.nabor = func() (Nabor, error) { return n, nil }
	if id, err := s.vybrannyyId(); err != nil || id != "" {
		t.Fatalf("в авто выбранный сервер это %q (ошибка %v), а выбора нет вовсе", id, err)
	}
}
