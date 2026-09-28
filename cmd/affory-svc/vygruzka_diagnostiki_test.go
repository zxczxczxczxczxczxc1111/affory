package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/puti"
)

// О6 аудита 1.6.1. Каталог данных открыт только системе и администраторам, и
// обычный пользователь журналы не получал вовсе: Проводник отвечал отказом.
// Служба отдаёт их по каналу вычищенными, файл пишет окно.

func zhurnalV(t *testing.T, s *Sluzhba, imya, tekst string) {
	t.Helper()
	katalog := filepath.Join(s.dirDannyh, puti.PodkatalogZhurnalov)
	if err := os.MkdirAll(katalog, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(katalog, imya), []byte(tekst), 0o600); err != nil {
		t.Fatal(err)
	}
}

type porciyaDiag struct {
	Id          string `json:"id"`
	Vsego       int    `json:"vsego"`
	Smeshchenie int    `json:"smeshchenie"`
	Kusok       string `json:"kusok"`
}

// vygruzitVsyo забирает выгрузку целиком, как окно, и проверяет каждый кадр
// на предел канала.
func vygruzitVsyo(t *testing.T, s *Sluzhba) (string, int) {
	t.Helper()
	var id string
	var sobrano []byte
	porciy := 0
	for {
		telo, _ := json.Marshal(map[string]any{"id": id, "smeshchenie": len(sobrano)})
		otv := s.Obrabotat(context.Background(), protokol.Kadr{Tip: "komanda", Id: 1, Imya: "exportDiagnostics", Telo: telo})
		if otv.Oshib != nil {
			t.Fatalf("отказ выгрузки: %+v", otv.Oshib)
		}
		kadr, err := json.Marshal(otv)
		if err != nil {
			t.Fatal(err)
		}
		if len(kadr) >= 1<<20 {
			t.Fatalf("кадр %d байт не пролезает в канал", len(kadr))
		}
		var p porciyaDiag
		if err := json.Unmarshal(otv.Telo, &p); err != nil {
			t.Fatal(err)
		}
		kusok, err := base64.StdEncoding.DecodeString(p.Kusok)
		if err != nil {
			t.Fatal(err)
		}
		if p.Smeshchenie != len(sobrano) {
			t.Fatalf("порция со смещения %d, а ждали %d", p.Smeshchenie, len(sobrano))
		}
		id = p.Id
		sobrano = append(sobrano, kusok...)
		porciy++
		if len(sobrano) >= p.Vsego || len(kusok) == 0 {
			if len(sobrano) != p.Vsego {
				t.Fatalf("собрано %d из %d", len(sobrano), p.Vsego)
			}
			return string(sobrano), porciy
		}
	}
}

func TestVygruzkaDiagnostikiSobiraetZhurnalyIVychishchaet(t *testing.T) {
	s := podstavnaya(t, nil)
	if err := s.zapisatNabor(Nabor{
		Servery: []protokol.Server{{Id: "a", Imya: "NL", Transport: "reality-tcp", Host: "203.0.113.9", Port: 443,
			Uuid: "11111111-2222-3333-4444-555555555555", PublicKey: "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8", ShortId: "01ab23cd"}},
		Podpiski:  []ZapisPodpiski{{Id: "p1", Adres: "https://panel.example/sub/tokenAbc123"}},
		Aktivnaya: "p1",
	}); err != nil {
		t.Fatal(err)
	}
	zhurnalV(t, s, "sluzhba.log", "запуск\nuuid 11111111-2222-3333-4444-555555555555 не принят\nподписка https://panel.example/sub/tokenAbc123 молчит\n")
	zhurnalV(t, s, "yadro.log.1", "старое перед поворотом\n")
	zhurnalV(t, s, "yadro.log", "новое после поворота\n")
	zhurnalV(t, s, "komandy.log", `команда от C:\Users\ivan\AppData\Local\Affory`+"\n")

	tekst, _ := vygruzitVsyo(t, s)

	for _, nuzhno := range []string{"служба:", "система:", "===== sluzhba.log", "===== yadro.log", "===== komandy.log", "не принят", "молчит"} {
		if !strings.Contains(tekst, nuzhno) {
			t.Errorf("в выгрузке нет %q:\n%s", nuzhno, tekst)
		}
	}
	staroe, novoe := strings.Index(tekst, "старое перед поворотом"), strings.Index(tekst, "новое после поворота")
	if staroe < 0 || novoe < 0 || staroe > novoe {
		t.Errorf("хвост ядра собран не по порядку: старое %d, новое %d", staroe, novoe)
	}
	if !strings.Contains(tekst, "===== obnovlenie.log") || !strings.Contains(tekst, "файла нет") {
		t.Errorf("отсутствующий журнал не назван:\n%s", tekst)
	}
	for _, sekret := range []string{"11111111-2222", "tokenAbc123", `\ivan\`} {
		if strings.Contains(tekst, sekret) {
			t.Errorf("в выгрузке остался %q", sekret)
		}
	}
	// Журнал соединений это история посещений: в диагностику он не входит.
	if strings.Contains(tekst, "soedineniya") {
		t.Error("в выгрузку попал журнал соединений")
	}
}

func TestVygruzkaDiagnostikiIdyotPorciyami(t *testing.T) {
	s := podstavnaya(t, nil)
	stroka := strings.Repeat("я", 60) + "\n"
	var b strings.Builder
	for b.Len() < 3<<20 {
		b.WriteString(stroka)
	}
	zhurnalV(t, s, "sluzhba.log", "ГОЛОВА\n"+b.String()+"ХВОСТ\n")

	tekst, porciy := vygruzitVsyo(t, s)

	if porciy < 2 {
		t.Fatalf("выгрузка больше порции ушла одним кадром")
	}
	if !strings.Contains(tekst, "ХВОСТ") || strings.Contains(tekst, "ГОЛОВА") {
		t.Fatal("из журнала взят не хвост")
	}
	nachalo := strings.Index(tekst, "===== sluzhba.log")
	razdel := tekst[nachalo:]
	razdel = razdel[strings.IndexByte(razdel, '\n')+1:]
	if len(razdel) > hvostZhurnala+1024 {
		t.Fatalf("хвост %d байт при пределе %d", len(razdel), hvostZhurnala)
	}
	// Обрезка по началу строки: с полстроки начинается битый символ.
	if !strings.HasPrefix(razdel, "(начало обрезано)\n"+strings.Repeat("я", 60)) {
		t.Fatalf("хвост начат не со строки: %q", razdel[:80])
	}
}

func TestVygruzkaDiagnostikiNaChuzhoyIdOtkaz(t *testing.T) {
	s := podstavnaya(t, nil)
	telo, _ := json.Marshal(map[string]any{"id": "chuzhoy", "smeshchenie": 5})
	otv := s.Obrabotat(context.Background(), protokol.Kadr{Tip: "komanda", Id: 1, Imya: "exportDiagnostics", Telo: telo})
	if otv.Oshib == nil || otv.Oshib.Kod != protokol.KodDiagnostikaUstarela {
		t.Fatalf("ждали отказ %s, получили %+v", protokol.KodDiagnostikaUstarela, otv.Oshib)
	}
}
