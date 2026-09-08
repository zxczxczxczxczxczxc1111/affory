package ssylki_test

import (
	"bufio"
	"encoding/json"
	"errors"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/genkonfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/ssylki"
)

// Корпус ФОРМ ссылок из живого сборника закрывает шов, названный в находке 33 и
// подтверждённый находкой 43: «ядро не приняло то, что собрал генератор» не
// проверялось никогда. Разбор и сборка покрыты по отдельности, а стык между
// ними до 02.09.2026 не проверял никто.
//
// Правило корпуса: у каждой формы ОПРЕДЕЛЁННЫЙ исход, третьего не дано.
// Либо разбор отвергает её с названной причиной, либо разбор принимает,
// генератор собирает, и ядро конфиг принимает. Форма, которую разбор принял, а
// ядро отвергло, это ровно находка 43.
//
// Числа проверяются ЯВНО. По «ошибок не было» нельзя отличить сделанное от
// несделанного: молча опустевший корпус тоже даёт зелёный.
const (
	formVKorpuse = 21
	// Было 4 до 06.09.2026. Стало 8: четыре формы корпуса это xhttp, и разбор
	// теперь отвергает их вместе с транспортом. Это ЦЕНА решения владельца, и
	// она обязана быть видна числом, а не остаться в чьей-то памяти.
	formOtvergnuto = 8
)

func TestKorpusFormSsylok(t *testing.T) {
	ssylkiKorpusa := prochitatKorpus(t)
	if len(ssylkiKorpusa) != formVKorpuse {
		t.Fatalf("форм в корпусе %d, ожидалось %d: корпус изменился молча",
			len(ssylkiKorpusa), formVKorpuse)
	}

	yadro := os.Getenv("AFFORY_SINGBOX")
	var otvergnuto, sobrano int
	for _, ss := range ssylkiKorpusa {
		imya := ss[strings.LastIndex(ss, "#")+1:]
		t.Run(imya, func(t *testing.T) {
			srv, err := ssylki.Razobrat(ss)
			if err != nil {
				// Отказ обязан быть НАШИМ и названным. Голая ошибка означала бы,
				// что форма провалилась куда-то мимо разбора.
				if !errors.Is(err, ssylki.ErrTransportNePodderzhan) &&
					!errors.Is(err, ssylki.ErrSsylkaKrivaya) {
					t.Fatalf("отказ без названной причины: %v", err)
				}
				otvergnuto++
				t.Logf("отвергнута разбором: %v", err)
				return
			}
			sobrano++

			telo, err := genkonfig.SingBox(vhodDlyaOdnogo(srv))
			if err != nil {
				t.Fatalf("разбор принял, а генератор отказал: %v", err)
			}
			var proverka struct {
				Outbounds []map[string]any `json:"outbounds"`
			}
			if err := json.Unmarshal(telo, &proverka); err != nil {
				t.Fatalf("собранный конфиг не разбирается: %v", err)
			}
			if yadro == "" {
				t.Skip("не задан AFFORY_SINGBOX: судить конфиг нечем")
			}
			put := filepath.Join(t.TempDir(), "konfig.json")
			if err := os.WriteFile(put, telo, 0o600); err != nil {
				t.Fatal(err)
			}
			if vyhod, err := exec.Command(yadro, "check", "-c", put).CombinedOutput(); err != nil {
				t.Fatalf("разбор принял, а ЯДРО отвергло. Это находка 43: %v\n%s", err, vyhod)
			}
		})
	}

	if otvergnuto != formOtvergnuto {
		t.Errorf("отвергнуто разбором %d форм, ожидалось %d. Если поддержка расширилась, "+
			"число правится вместе с причиной; если сузилась, это регресс",
			otvergnuto, formOtvergnuto)
	}
	t.Logf("форм %d, отвергнуто разбором %d, собрано и принято ядром %d",
		len(ssylkiKorpusa), otvergnuto, sobrano)
}

func prochitatKorpus(t *testing.T) []string {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "korpus-form.txt"))
	if err != nil {
		t.Fatalf("корпус не открылся: %v", err)
	}
	defer f.Close()
	var itog []string
	sk := bufio.NewScanner(f)
	for sk.Scan() {
		stroka := strings.TrimSpace(sk.Text())
		if stroka == "" || strings.HasPrefix(stroka, "#") {
			continue
		}
		itog = append(itog, stroka)
	}
	if err := sk.Err(); err != nil {
		t.Fatal(err)
	}
	return itog
}

// vhodDlyaOdnogo собирает минимальный вход генератора вокруг ОДНОГО сервера:
// корпус проверяет форму ссылки, а не устройство правил.
func vhodDlyaOdnogo(s protokol.Server) genkonfig.Vhod {
	adres := netip.MustParseAddr(s.Host)
	return genkonfig.Vhod{
		Server:         s,
		Servery:        []protokol.Server{s},
		Kandidaty:      []netip.Addr{adres},
		Resolver:       netip.MustParseAddr("192.168.1.1"),
		PutiProtsessov: []string{`C:\Program Files\Affory\affory-svc.exe`, `C:\Program Files\Affory\sing-box.exe`},
		ClashApi:       genkonfig.ClashApi{Adres: "127.0.0.1", Port: 19090, Sekret: "sekret"},
	}
}
