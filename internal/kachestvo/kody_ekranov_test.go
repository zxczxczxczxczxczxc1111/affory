package kachestvo_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Третья сторона словаря: код, который ПОКАЗЫВАЕТ экран, но которого нет в
// словаре вовсе.
//
// Соседние ворота её не видят по построению. kody_test.go сверяет kody.go со
// спекой, otkazy_ui_test.go сверяет спеку с otkazy.ts, otpraviteli_test.go
// ищет отправителя у объявленного кода. Ни одни не смотрят, какой код экран
// передаёт в Otkaz и Neudacha.
//
// Цена дефекта: компонент неизвестный код переживает МОЛЧА. Он рисует
// собственный заголовок экрана, строку словаря не находит и пропускает, и
// экран выглядит правильным. Это худший вид дефекта словаря: сломано, а видно
// только тому, кто читает исходник.
const korenFronta = "../../cmd/affory-ui/frontend/src"

// Две формы, в которых код попадает на экран литералом: проп JSX и поле
// объекта состояния. Вторая важнее: `qr-s-ekrana` родился именно ею, а разбор
// одних пропов нашёл бы ноль литералов и позеленел бы на пустом месте.
var (
	rePropKoda  = regexp.MustCompile(`\bkod=\{?"([a-z0-9-]+)"`)
	rePolyaKoda = regexp.MustCompile(`\bkod:\s*"([a-z0-9-]+)"`)
)

func TestEkranyNePokazyvayutKodovVneSlovarya(t *testing.T) {
	ts, err := os.ReadFile(filepath.FromSlash(putOtkazovTS))
	if err != nil {
		t.Fatalf("otkazy.ts не прочитан: %v", err)
	}
	vSlovare := map[string]bool{}
	for _, m := range reZapisTS.FindAllStringSubmatch(string(ts), -1) {
		vSlovare[m[1]] = true
	}
	if len(vSlovare) == 0 {
		t.Fatal("в otkazy.ts не нашлось ни одной записи: сломан разбор словаря, а не экраны")
	}

	// Тестовые файлы НЕ читаются, и по той же причине, что у ворот отправителя:
	// фикстура это не экран. Тест имеет право нарисовать заведомо неизвестный
	// код, проверяя, что компонент его переживает, и ворота на такой фикстуре
	// краснели бы по неверной причине.
	nayden := map[string][]string{}
	vsego := 0
	oshibkaObhoda := filepath.WalkDir(filepath.FromSlash(korenFronta),
		func(put string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			imya := d.Name()
			if d.IsDir() || strings.Contains(imya, ".test.") {
				return nil
			}
			if !strings.HasSuffix(imya, ".ts") && !strings.HasSuffix(imya, ".tsx") {
				return nil
			}
			syroe, err := os.ReadFile(put)
			if err != nil {
				return err
			}
			for _, re := range []*regexp.Regexp{rePropKoda, rePolyaKoda} {
				for _, m := range re.FindAllStringSubmatch(string(syroe), -1) {
					vsego++
					nayden[m[1]] = append(nayden[m[1]], filepath.Base(put))
				}
			}
			return nil
		})
	if oshibkaObhoda != nil {
		t.Fatalf("исходники экранов не обошлись: %v", oshibkaObhoda)
	}

	// Якорь. Без него переименование пропа или переезд каталога превращают
	// ворота в зелёное ничто.
	if vsego == 0 {
		t.Fatal("ни одного литерала кода в экранах не найдено: сломан разбор, а не экраны чисты")
	}

	var chuzhie []string
	for kod, gde := range nayden {
		if vSlovare[kod] {
			continue
		}
		chuzhie = append(chuzhie, kod+" ("+strings.Join(gde, ", ")+")")
	}
	sort.Strings(chuzhie)
	for _, k := range chuzhie {
		t.Errorf("экран показывает код %s, которого нет в otkazy.ts: компонент переживёт его молча", k)
	}
	t.Logf("литералов кода в экранах %d, различных %d, записей в otkazy.ts %d",
		vsego, len(nayden), len(vSlovare))
}
