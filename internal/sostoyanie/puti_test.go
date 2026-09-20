package sostoyanie

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Подмена каталога данных существует ради тестов и обязана оставаться их
// привилегией.
//
// Судья читает исходники, а не полагается на договорённость: вызов, случайно
// попавший в продуктовый код, уводит службу с пути, под который выставлены
// права ACL, и делает это молча. Заметить такое чтением диффа можно ровно один
// раз из десяти.
func TestPodmenaKatalogaZovyotsyaTolkoIzTestov(t *testing.T) {
	koren, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("корень модуля не вычислен: %v", err)
	}

	var naydeno []string
	err = filepath.WalkDir(koren, func(put string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Чужие деревья не наши: там свой код и свои правила.
			if d.Name() == "node_modules" || d.Name() == ".git" || d.Name() == "dist" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		telo, err := os.ReadFile(put)
		if err != nil {
			return err
		}
		// Своё объявление не считается: оно и есть предмет охраны.
		if put == filepath.Join(koren, "internal", "sostoyanie", "puti.go") {
			return nil
		}
		if strings.Contains(string(telo), "PodmenitKatalogDannyh") {
			otn, _ := filepath.Rel(koren, put)
			naydeno = append(naydeno, otn)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("обход дерева не удался: %v", err)
	}

	if len(naydeno) > 0 {
		t.Fatalf("подмена каталога данных зовётся из продуктового кода: %v\n"+
			"это уводит службу с пути, под который выставлены права", naydeno)
	}
}

// Подмена и её снятие работают, и снятая подмена возвращает ПОСТОЯННЫЙ путь.
// Без второй половины судья выше зелёный при сломанной подмене.
func TestPodmenaKatalogaDeystvuetIShimaetsya(t *testing.T) {
	prezhniy := korenDannyh
	t.Cleanup(func() { korenDannyh = prezhniy })

	PodmenitKatalogDannyh(`C:\gde-to-eshchyo`)
	if KatalogDannyh() != `C:\gde-to-eshchyo` {
		t.Errorf("подмена не подействовала: %s", KatalogDannyh())
	}
	if KatalogZhurnalov() != filepath.Join(`C:\gde-to-eshchyo`, "log") {
		t.Errorf("журналы не поехали за каталогом: %s", KatalogZhurnalov())
	}

	PodmenitKatalogDannyh("")
	if KatalogDannyh() != filepath.Join(`C:\ProgramData`, "Affory") {
		t.Errorf("снятая подмена не вернула постоянный путь: %s", KatalogDannyh())
	}
}
