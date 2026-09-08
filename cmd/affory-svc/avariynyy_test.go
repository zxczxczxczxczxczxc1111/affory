package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/set"
)

// Находка 26. Аварийный файл клал только скрипт стенда, а настоящая установка
// нет. На реальной машине его не будет ровно тогда, когда он нужен: интернета
// нет, интерфейса нет, читать нечего.
func TestUstanovkaKladyotAvariynyyFayl(t *testing.T) {
	kat := t.TempDir()

	if err := polozhitAvariynyy(kat); err != nil {
		t.Fatalf("файл не положен: %v", err)
	}

	telo, err := os.ReadFile(filepath.Join(kat, imyaAvariynogo))
	if err != nil {
		t.Fatalf("файла нет рядом с программой: %v", err)
	}
	if len(telo) < 500 {
		t.Fatalf("файл подозрительно короткий, %d байт", len(telo))
	}
}

// Находка 29. Список имён в аварийном файле был написан руками и отставал.
//
// Проверено 02.09.2026: правило Affory-Allow-Dns-Tcp, заведённое в тот же день,
// в файл не попало. Человек без интернета набирает с листа то, что там
// написано, и одно недоснятое правило означает, что интернет не вернулся, а
// почему, не видно.
func TestAvariynyyFaylZnaetVseNashiPravila(t *testing.T) {
	kat := t.TempDir()
	if err := polozhitAvariynyy(kat); err != nil {
		t.Fatal(err)
	}
	telo, err := os.ReadFile(filepath.Join(kat, imyaAvariynogo))
	if err != nil {
		t.Fatal(err)
	}
	tekst := string(telo)

	for _, imya := range set.VseImenaPravil() {
		if !strings.Contains(tekst, imya) {
			t.Errorf("правила %s нет в аварийном файле: снять его человеку будет нечем", imya)
		}
	}
}

// Зеркало: файл не должен превращаться в свалку. Имя, которого в правилах нет,
// в нём взяться не может, иначе список перестанет быть списком.
func TestAvariynyyFaylNeVydumyvaetPravil(t *testing.T) {
	kat := t.TempDir()
	if err := polozhitAvariynyy(kat); err != nil {
		t.Fatal(err)
	}
	telo, err := os.ReadFile(filepath.Join(kat, imyaAvariynogo))
	if err != nil {
		t.Fatal(err)
	}
	nashi := map[string]bool{}
	for _, imya := range set.VseImenaPravil() {
		nashi[imya] = true
	}
	razdeliteli := func(r rune) bool {
		return r == ' ' || r == '\n' || r == '\r' || r == '(' || r == ')' || r == '='
	}
	for _, slovo := range strings.FieldsFunc(string(telo), razdeliteli) {
		if strings.HasPrefix(slovo, "Affory-") && !nashi[slovo] {
			t.Errorf("в файле есть %s, которого нет среди наших правил", slovo)
		}
	}
}
