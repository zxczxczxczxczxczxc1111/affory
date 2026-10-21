package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/sobytiya"
)

// О6 аудита 1.6.1. Журналы собирает служба, файл пишет окно от имени человека
// по пути из диалога. Содержимое через страницу не проходит. С 1.10.0 окно
// дописывает свой раздел, а без службы читает её журналы само.

type zaprosPorcii struct {
	Id          string `json:"id"`
	Smeshchenie int    `json:"smeshchenie"`
}

// okruzhenieVygruzki это подставные каталоги журналов службы и окна.
type okruzhenieVygruzki struct {
	sluzhby, okna  string
	zaprosySobytiy []string
}

// podstavitOkruzhenie подменяет каталоги журналов и журнал событий: тест не
// читает ни журналы живой службы, ни журнал событий машины.
func podstavitOkruzhenie(t *testing.T) *okruzhenieVygruzki {
	t.Helper()
	byloSluzhby, byloOkna, byloSobytiya := katalogZhurnalovSluzhby, katalogZhurnalaOkna, prochitatSobytiya
	t.Cleanup(func() {
		katalogZhurnalovSluzhby, katalogZhurnalaOkna, prochitatSobytiya = byloSluzhby, byloOkna, byloSobytiya
	})
	o := &okruzhenieVygruzki{sluzhby: t.TempDir(), okna: t.TempDir()}
	katalogZhurnalovSluzhby = o.sluzhby
	katalogZhurnalaOkna = func() string { return o.okna }
	prochitatSobytiya = func(zhurnal, _ string, _ int, filtr func(sobytiya.Zapis) bool) ([]sobytiya.Zapis, error) {
		o.zaprosySobytiy = append(o.zaprosySobytiy, zhurnal)
		var vse []sobytiya.Zapis
		for _, z := range []sobytiya.Zapis{
			{Vremya: time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC), Istochnik: "Service Control Manager", Kod: 7031,
				Dannye: []string{"Affory", "1"}},
			{Vremya: time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC), Istochnik: "Service Control Manager", Kod: 7031,
				Dannye: []string{"Чужая служба", "1"}},
			{Vremya: time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC), Istochnik: "Application Error", Kod: 1000,
				Dannye: []string{"sing-box.exe", "1.14.2"}},
		} {
			if filtr(z) {
				vse = append(vse, z)
			}
		}
		return vse, nil
	}
	return o
}

func (o *okruzhenieVygruzki) polozhit(t *testing.T, kat, imya, tekst string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(kat, imya), []byte(tekst), 0o600); err != nil {
		t.Fatal(err)
	}
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
	o := podstavitOkruzhenie(t)
	o.polozhit(t, o.okna, "okno.log", "2026/10/21 10:00:00 страница: отказ all-servers-down\n")
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
	tekst := string(fayl)
	if !strings.HasPrefix(tekst, "шапка\nхвост журнала\n\n===== окно\n") {
		t.Fatalf("часть службы или начало раздела окна не на месте:\n%s", tekst)
	}
	for _, nado := range []string{
		"версия ", "WebView2 ",
		"===== okno.log\n2026/10/21 10:00:00 страница: отказ all-servers-down\n",
		"===== padenie-okna.log\n(файла нет)\n",
		"-- System\n", "Service Control Manager 7031: Affory | 1\n",
		"-- Application\n", "Application Error 1000: sing-box.exe | 1.14.2\n",
	} {
		if !strings.Contains(tekst, nado) {
			t.Errorf("в выгрузке нет %q:\n%s", nado, tekst)
		}
	}
	if strings.Contains(tekst, "Чужая служба") {
		t.Errorf("в выгрузку попало событие чужой службы:\n%s", tekst)
	}
	if strings.Contains(tekst, "служба не ответила") {
		t.Errorf("служба ответила, а шапка говорит обратное:\n%s", tekst)
	}
	if !regexp.MustCompile(`^affory-zhurnal-\d{4}-\d{2}-\d{2}-\d{6}\.txt$`).MatchString(*predlozheno) {
		t.Errorf("диалогу предложено имя %q", *predlozheno)
	}
	if !strings.Contains(itog, "diag.txt") || strings.Contains(itog, "не ответила") {
		t.Errorf("итог %q", itog)
	}
	zhdali := []zaprosPorcii{{Id: "", Smeshchenie: 0}, {Id: "x1", Smeshchenie: len("шапка\n")}}
	if fmt.Sprint(*zaprosy) != fmt.Sprint(zhdali) {
		t.Errorf("запросы %v, ждали %v", *zaprosy, zhdali)
	}
	if fmt.Sprint(o.zaprosySobytiy) != "[System Application]" {
		t.Errorf("журналы событий %v", o.zaprosySobytiy)
	}
}

// Журнал сохраняют, чтобы сразу переслать: диалог открывается на рабочем
// столе (просьба владельца 21.10.2026). Без каталога он открывался в рабочем
// каталоге процесса, то есть в Program Files, и человек без прав получал
// отказ Windows (приёмка 1.7.0, 28.09.2026).
func TestDiagnostikaPredlagaetRabochiyStol(t *testing.T) {
	kat := katalogRabochegoStola()
	if kat == "" {
		t.Fatal("каталог для журнала не выбран: диалог откроется в рабочем каталоге процесса")
	}
	if st, err := os.Stat(kat); err != nil || !st.IsDir() {
		t.Fatalf("каталога %q нет: %v", kat, err)
	}
	if pf := os.Getenv("ProgramFiles"); pf != "" && strings.HasPrefix(strings.ToLower(kat), strings.ToLower(pf)) {
		t.Fatalf("каталог %q внутри Program Files", kat)
	}
}

func TestDiagnostikaOtmenaDialogaNichegoNeDelaet(t *testing.T) {
	podstavitOkruzhenie(t)
	zaprosy, _ := podstavitDiagnostiku(t, "", []string{"шапка\n"})
	itog, err := (&most{}).SohranitDiagnostiku()
	if err != nil || itog != "" {
		t.Fatalf("отмена дала %q, %v", itog, err)
	}
	if len(*zaprosy) != 0 {
		t.Fatalf("после отмены служба получила %v", *zaprosy)
	}
}

// Самый нужный журнал это журнал службы, которая не отвечает. Окно читает его
// с диска само, называет причину и не тащит в файл полвыгрузки службы.
func TestDiagnostikaBezSluzhbyChitaetZhurnalySama(t *testing.T) {
	o := podstavitOkruzhenie(t)
	o.polozhit(t, o.sluzhby, "sluzhba.log", "служба запущена\nключ vless://tayna@198.51.100.7:443?security=reality#x\n")
	o.polozhit(t, o.sluzhby, "padenie-sluzhby.log", "panic: обрыв\n")
	put := filepath.Join(t.TempDir(), "diag.txt")
	podstavitDiagnostiku(t, put, []string{"шапка службы\n", ""})

	itog, err := (&most{}).SohranitDiagnostiku()
	if err != nil {
		t.Fatal(err)
	}
	fayl, err := os.ReadFile(put)
	if err != nil {
		t.Fatal(err)
	}
	tekst := string(fayl)
	for _, nado := range []string{
		"служба не ответила: ", "прервалась",
		"===== sluzhba.log\nслужба запущена\nключ <ссылка>\n",
		"===== padenie-sluzhby.log\npanic: обрыв\n",
		"===== yadro.log\n(файла нет)\n",
		"===== окно\n",
	} {
		if !strings.Contains(tekst, nado) {
			t.Errorf("в выгрузке нет %q:\n%s", nado, tekst)
		}
	}
	for _, nelzya := range []string{"tayna", "198.51.100.7", "шапка службы"} {
		if strings.Contains(tekst, nelzya) {
			t.Errorf("в выгрузку попало %q:\n%s", nelzya, tekst)
		}
	}
	if !strings.Contains(itog, "Служба не ответила") {
		t.Errorf("итог %q не говорит, что служба молчала", itog)
	}
}

// Отказ записи не оставляет ни полфайла, ни испорченного прежнего: файл
// пишется рядом и подменяет целевой одним переименованием.
func TestDiagnostikaOtkazZapisiNePortitFayl(t *testing.T) {
	katalog := t.TempDir()
	// Целевой путь занят каталогом: переименование на него не проходит.
	put := filepath.Join(katalog, "diag.txt")
	if err := os.Mkdir(put, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(put, "vnutri"), []byte("прежний"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := zapisatTselikom(put, []byte("новое"))
	if err == nil || !strings.Contains(err.Error(), "не записан") {
		t.Fatalf("ждали отказ записи, получили %v", err)
	}
	if fayl, _ := os.ReadFile(filepath.Join(put, "vnutri")); string(fayl) != "прежний" {
		t.Fatalf("прежнее содержимое испорчено: %q", fayl)
	}
	vse, err := os.ReadDir(katalog)
	if err != nil {
		t.Fatal(err)
	}
	if len(vse) != 1 {
		t.Fatalf("в каталоге осталось лишнее: %v", vse)
	}
}

// Отказ журнала событий не роняет выгрузку: строка с причиной вместо записей.
func TestDiagnostikaOtkazZhurnalaSobytiy(t *testing.T) {
	podstavitOkruzhenie(t)
	prochitatSobytiya = func(string, string, int, func(sobytiya.Zapis) bool) ([]sobytiya.Zapis, error) {
		return nil, errors.New("доступ запрещён")
	}
	tekst := razdelOkna()
	if strings.Count(tekst, "(не прочитан: доступ запрещён)") != 2 {
		t.Fatalf("причина отказа не в каждом журнале:\n%s", tekst)
	}
}
