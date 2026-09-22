package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Подготовка кандидата до остановки (A6).
//
// Прежде перезапуск ради новых правил начинался с Disconnect, и человек с
// набором, которого ядро не принимает, оставался без VPN.

// sluzhbaPodnyataya поднимает туннель подставной службы и отдаёт её готовой к
// правке правил.
func sluzhbaPodnyataya(t *testing.T) *Sluzhba {
	t.Helper()
	s := podstavnaya(t, nil)
	srv := serverProby()
	s.nabor = func() (Nabor, error) {
		return Nabor{Servery: []protokol.Server{srv}, Vybran: srv.Id}, nil
	}
	s.sobratAdresa = func() ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr(srv.Host)}, nil
	}
	putKandidataFayl = func() string { return filepath.Join(t.TempDir(), "sing-box.kandidat.json") }
	t.Cleanup(func() { putKandidataFayl = putKandidata })
	if err := s.Connect(context.Background()); err != nil {
		t.Fatalf("подъём не прошёл: %v", err)
	}
	if got := s.Status().Sostoyanie; got != protokol.SostPodnyat {
		t.Fatalf("состояние %s, ожидалось podnyat", got)
	}
	return s
}

func TestNegodnyyKandidatNeRonyaetRabocheePodklyuchenie(t *testing.T) {
	s := sluzhbaPodnyataya(t)
	// Ядро отвергает ЛЮБОЙ конфиг: так выглядит набор правил, который оно не
	// понимает. Важно, что отказ приходит на проверке, а не на подъёме.
	s.proveritKonfig = func(string) error { return errors.New("initialize rule[3]: invalid domain") }

	err := s.perepodklyuchit(context.Background())
	if !errors.Is(err, errKandidatNegoden) {
		t.Fatalf("ошибка %v, ждали отказ кандидата", err)
	}
	// Главное: туннель не тронут.
	if got := s.Status().Sostoyanie; got != protokol.SostPodnyat {
		t.Fatalf("состояние %s: рабочее подключение упало из-за негодного кандидата", got)
	}
}

func TestKandidatProveryaetsyaDoOstanovki(t *testing.T) {
	// Порядок, а не только исход: проверка обязана случиться, ПОКА старое ядро
	// живо. Иначе «проверили и уронили» и «уронили и проверили» неотличимы по
	// зелёным тестам.
	s := sluzhbaPodnyataya(t)
	var yadroZhilo bool
	s.proveritKonfig = func(string) error {
		s.mu.Lock()
		yadroZhilo = s.portClash != 0
		s.mu.Unlock()
		return errors.New("не принят")
	}
	_ = s.perepodklyuchit(context.Background())
	if !yadroZhilo {
		t.Fatal("кандидата проверяли уже после остановки: старого ядра в этот момент не было")
	}
}

func TestGodnyyKandidatPropuskaetPerepodklyuchenie(t *testing.T) {
	s := sluzhbaPodnyataya(t)
	s.proveritKonfig = func(string) error { return nil }
	if err := s.perepodklyuchit(context.Background()); err != nil {
		t.Fatalf("годный кандидат не дал переподключиться: %v", err)
	}
	if got := s.Status().Sostoyanie; got != protokol.SostPodnyat {
		t.Fatalf("состояние после переподключения %s", got)
	}
}

func TestProverkaKandidataNichegoNeZapominaet(t *testing.T) {
	// Сухая сборка не смеет трогать память службы: порт прокси нужен сторожу
	// системного прокси и экрану, отпечаток правил отвечает на вопрос «ждёт ли
	// набор подъёма», а список серверов ядра говорит, что оно несёт.
	s := sluzhbaPodnyataya(t)
	s.proveritKonfig = func(string) error { return nil }
	s.mu.Lock()
	portBylo, otpechatokBylo := s.portProksiNash, s.pravilaKonfiga
	s.mu.Unlock()

	if err := s.proveritKandidata(); err != nil {
		t.Fatalf("проверка кандидата: %v", err)
	}
	s.mu.Lock()
	portStalo, otpechatokStalo := s.portProksiNash, s.pravilaKonfiga
	s.mu.Unlock()
	if portStalo != portBylo {
		t.Errorf("порт прокси после проверки %d, был %d", portStalo, portBylo)
	}
	if otpechatokStalo != otpechatokBylo {
		t.Error("отпечаток правил переписан проверкой: pravilaOzhidayut начнёт врать")
	}
}

func TestFaylKandidataOtdelnyyIUbiraetsyaZaSoboy(t *testing.T) {
	s := sluzhbaPodnyataya(t)
	var proveryali string
	s.proveritKonfig = func(put string) error {
		proveryali = put
		return nil
	}
	if err := s.proveritKandidata(); err != nil {
		t.Fatalf("проверка кандидата: %v", err)
	}
	if proveryali == "" {
		t.Fatal("ядру не дали ни одного файла")
	}
	if proveryali == putKonfigaTun() {
		t.Fatal("кандидат записан поверх рабочего конфига: по нему судят живое ядро и восстановление")
	}
	if _, err := os.Stat(proveryali); !os.IsNotExist(err) {
		t.Errorf("файл кандидата остался на диске: %v", err)
	}
}

func TestOtkazKandidataVozvrashchaetPrezhniePravila(t *testing.T) {
	// Сохранённые правила, которых ядро не принимает, это отложенная потеря
	// VPN: следующий подъём собрался бы с ними уже без участия человека.
	s := sluzhbaPodnyataya(t)
	// Свой набор на диске, а не заглушка чтения: откат пишет НАБОР, и проверять
	// его надо там же, где он лежит.
	s.nabor = s.naborIzHranilishcha
	if err := s.pravitNabor(func(n *Nabor) error {
		n.Servery = []protokol.Server{serverProby()}
		n.Vybran = serverProby().Id
		n.Pravila = PravilaNabora{Domeny: []string{"staryy.example"}}
		return nil
	}); err != nil {
		t.Fatalf("набор не записан: %v", err)
	}
	s.proveritKonfig = func(string) error { return errors.New("initialize rule[1]: invalid domain") }

	telo, _ := json.Marshal(map[string]any{
		"domeny": []string{"novyy.example"},
		"trafik": map[string]any{"po_umolchaniyu": "vpn", "domeny": []map[string]any{{"domen": "novyy.example", "marshrut": "vpn"}}},
	})
	k := s.Obrabotat(context.Background(), protokol.Kadr{Imya: "setRules", Telo: telo})
	if k.Oshib == nil {
		t.Fatal("негодные правила применены молча")
	}
	if k.Oshib.Kod != protokol.KodPravilaNePrinyaty {
		t.Errorf("код отказа %q", k.Oshib.Kod)
	}
	// Человеку сказано главное: VPN цел.
	if !strings.Contains(k.Oshib.Tekst, "прежним правилам") {
		t.Errorf("текст отказа не говорит, что VPN работает по прежним правилам: %q", k.Oshib.Tekst)
	}
	n, err := s.nabor()
	if err != nil {
		t.Fatalf("набор не прочитан: %v", err)
	}
	if len(n.Pravila.Domeny) != 1 || n.Pravila.Domeny[0] != "staryy.example" {
		t.Fatalf("правила после отказа: %v", n.Pravila.Domeny)
	}
	if got := s.Status().Sostoyanie; got != protokol.SostPodnyat {
		t.Fatalf("состояние %s: подключение упало из-за негодных правил", got)
	}
}
