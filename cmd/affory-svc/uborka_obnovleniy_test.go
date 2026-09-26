package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestUborkaUbiraetTolkoArhivyStarsheRabotayushchey(t *testing.T) {
	kat := t.TempDir()
	fayly := map[string]int{
		"affory-0.6.3.zip":          150, // заглушка с живой машины, тоже старше
		"affory-0.6.3.zip.sha256":   83,
		"affory-1.4.2.zip":          1000,
		"affory-1.4.2.zip.sha256":   83,
		"affory-1.5.0.zip":          2000, // работающая версия
		"affory-1.5.0.zip.sha256":   83,
		"affory-1.5.1.zip":          3000, // след неудавшейся попытки
		"affory-1.5.1.zip.sha256":   83,
		"zametka.txt":               5, // чужой файл
		"affory-latest.zip":         5, // похоже, но не наше имя
		"affory-1.4.2.zip.chast":    5,
		"Affory-1.0.0-setup.exe":    5,
		"affory-1.0.0.zip.sha256.x": 5,
	}
	for imya, n := range fayly {
		if err := os.WriteFile(filepath.Join(kat, imya), make([]byte, n), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Каталог с именем архива это не архив, и удалять его нельзя.
	if err := os.Mkdir(filepath.Join(kat, "affory-1.0.0.zip"), 0o755); err != nil {
		t.Fatal(err)
	}

	ubrano, bayt, zhaloby := ubratStaryeArhivy(kat, "1.5.0")
	if len(zhaloby) != 0 {
		t.Fatalf("жалобы уборки: %v", zhaloby)
	}
	slices.Sort(ubrano)
	hotim := []string{"affory-0.6.3.zip", "affory-0.6.3.zip.sha256", "affory-1.4.2.zip", "affory-1.4.2.zip.sha256"}
	if !slices.Equal(ubrano, hotim) {
		t.Fatalf("убрано %v, а должно %v", ubrano, hotim)
	}
	if bayt != 150+83+1000+83 {
		t.Fatalf("насчитано %d байт", bayt)
	}

	ostalos, err := os.ReadDir(kat)
	if err != nil {
		t.Fatal(err)
	}
	var imena []string
	for _, z := range ostalos {
		imena = append(imena, z.Name())
	}
	for _, nado := range []string{"affory-1.5.0.zip", "affory-1.5.0.zip.sha256", "affory-1.5.1.zip",
		"affory-1.5.1.zip.sha256", "zametka.txt", "affory-latest.zip", "affory-1.4.2.zip.chast",
		"Affory-1.0.0-setup.exe", "affory-1.0.0.zip.sha256.x", "affory-1.0.0.zip"} {
		if !slices.Contains(imena, nado) {
			t.Errorf("%s убран, а должен был остаться", nado)
		}
	}
}

// Сборка dev не знает своего места в ряду версий и не трогает ничего.
func TestUborkaSborkiDevNichegoNeTrogaet(t *testing.T) {
	kat := t.TempDir()
	put := filepath.Join(kat, "affory-0.1.0.zip")
	if err := os.WriteFile(put, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if ubrano, _, zhaloby := ubratStaryeArhivy(kat, "dev"); len(ubrano) != 0 || len(zhaloby) != 0 {
		t.Fatalf("dev убрала %v, жалобы %v", ubrano, zhaloby)
	}
	if _, err := os.Stat(put); err != nil {
		t.Fatalf("архив пропал: %v", err)
	}
}

// Каталога нет у того, кто ни разу не обновлялся из окна. Это не отказ.
func TestUborkaBezKatalogaMolchit(t *testing.T) {
	ubrano, _, zhaloby := ubratStaryeArhivy(filepath.Join(t.TempDir(), "net"), "1.5.0")
	if len(ubrano) != 0 || len(zhaloby) != 0 {
		t.Fatalf("убрано %v, жалобы %v", ubrano, zhaloby)
	}
}
