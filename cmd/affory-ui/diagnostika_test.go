package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// О6 аудита 1.6.1. Журналы собирает служба, файл пишет окно от имени человека
// по пути из диалога. Содержимое через страницу не проходит.

type zaprosPorcii struct {
	Id          string `json:"id"`
	Smeshchenie int    `json:"smeshchenie"`
}

// podstavitDiagnostiku подменяет диалог и службу. Служба отдаёт porcii по
// очереди; porcii[i] == "" значит отказ на этом шаге.
func podstavitDiagnostiku(t *testing.T, put string, porcii []string) (*[]zaprosPorcii, *string) {
	t.Helper()
	byloDialog, byloZvat := vybratKudaDiagnostiku, zvatSluzhbu
	t.Cleanup(func() { vybratKudaDiagnostiku, zvatSluzhbu = byloDialog, byloZvat })
	var predlozheno string
	vybratKudaDiagnostiku = func(_ *most, imya string) (string, error) {
		predlozheno = imya
		return put, nil
	}
	// Упавшая порция тоже входит в объявленный размер: иначе выгрузка честно
	// кончается до неё.
	vsego := 0
	for _, p := range porcii {
		vsego += max(len(p), 10)
	}
	var zaprosy []zaprosPorcii
	zvatSluzhbu = func(_ *most, imya, telo string) (string, error) {
		if imya != "exportDiagnostics" {
			t.Fatalf("окно позвало %s", imya)
		}
		var z zaprosPorcii
		if err := json.Unmarshal([]byte(telo), &z); err != nil {
			t.Fatal(err)
		}
		zaprosy = append(zaprosy, z)
		i := len(zaprosy) - 1
		if i >= len(porcii) || porcii[i] == "" {
			return `{"tip":"otvet","id":1,"imya":"exportDiagnostics","oshibka":{"kod":"diagnostics-stale","tekst":"выгрузка диагностики прервалась, сохрани её заново"}}`, nil
		}
		otv, _ := json.Marshal(map[string]any{
			"tip": "otvet", "id": 1, "imya": imya,
			"telo": map[string]any{"id": "x1", "vsego": vsego, "smeshchenie": z.Smeshchenie,
				"kusok": base64.StdEncoding.EncodeToString([]byte(porcii[i]))},
		})
		return string(otv), nil
	}
	return &zaprosy, &predlozheno
}

func TestDiagnostikaSkleivaetsyaVFayl(t *testing.T) {
	put := filepath.Join(t.TempDir(), "diag.txt")
	zaprosy, predlozheno := podstavitDiagnostiku(t, put, []string{"шапка\n", "хвост журнала\n"})

	itog, err := (&most{}).SohranitDiagnostiku()
	if err != nil {
		t.Fatal(err)
	}
	fayl, err := os.ReadFile(put)
	if err != nil {
		t.Fatal(err)
	}
	if string(fayl) != "шапка\nхвост журнала\n" {
		t.Fatalf("в файле %q", fayl)
	}
	if !regexp.MustCompile(`^affory-diagnostika-\d{4}-\d{2}-\d{2}-\d{6}\.txt$`).MatchString(*predlozheno) {
		t.Errorf("диалогу предложено имя %q", *predlozheno)
	}
	if !strings.Contains(itog, "diag.txt") {
		t.Errorf("итог %q не называет файл", itog)
	}
	zhdali := []zaprosPorcii{{Id: "", Smeshchenie: 0}, {Id: "x1", Smeshchenie: len("шапка\n")}}
	if fmt.Sprint(*zaprosy) != fmt.Sprint(zhdali) {
		t.Errorf("запросы %v, ждали %v", *zaprosy, zhdali)
	}
}

// Без каталога диалог открывался в рабочем каталоге процесса, то есть в
// Program Files, откуда ярлык запускает окно, и человек без прав на
// «Сохранить» первым делом получал отказ Windows (приёмка 1.7.0, 28.09.2026).
func TestDiagnostikaPredlagaetDokumenty(t *testing.T) {
	kat := katalogDokumentov()
	if kat == "" {
		t.Fatal("каталог для диагностики не выбран: диалог откроется в рабочем каталоге процесса")
	}
	if st, err := os.Stat(kat); err != nil || !st.IsDir() {
		t.Fatalf("каталога %q нет: %v", kat, err)
	}
	if pf := os.Getenv("ProgramFiles"); pf != "" && strings.HasPrefix(strings.ToLower(kat), strings.ToLower(pf)) {
		t.Fatalf("каталог %q внутри Program Files", kat)
	}
}

func TestDiagnostikaOtmenaDialogaNichegoNeDelaet(t *testing.T) {
	zaprosy, _ := podstavitDiagnostiku(t, "", []string{"шапка\n"})
	itog, err := (&most{}).SohranitDiagnostiku()
	if err != nil || itog != "" {
		t.Fatalf("отмена дала %q, %v", itog, err)
	}
	if len(*zaprosy) != 0 {
		t.Fatalf("после отмены служба получила %v", *zaprosy)
	}
}

// Отказ посреди выгрузки не оставляет ни полфайла, ни испорченного прежнего.
func TestDiagnostikaOtkazNePortitFayl(t *testing.T) {
	katalog := t.TempDir()
	put := filepath.Join(katalog, "diag.txt")
	if err := os.WriteFile(put, []byte("прежний"), 0o600); err != nil {
		t.Fatal(err)
	}
	podstavitDiagnostiku(t, put, []string{"шапка\n", ""})

	_, err := (&most{}).SohranitDiagnostiku()
	if err == nil || !strings.Contains(err.Error(), "прервалась") {
		t.Fatalf("ждали отказ службы, получили %v", err)
	}
	if fayl, _ := os.ReadFile(put); string(fayl) != "прежний" {
		t.Fatalf("прежний файл испорчен: %q", fayl)
	}
	vse, err := os.ReadDir(katalog)
	if err != nil {
		t.Fatal(err)
	}
	if len(vse) != 1 {
		t.Fatalf("в каталоге осталось лишнее: %v", vse)
	}
}
