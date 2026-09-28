package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Подменщик ложится в каталог данных и каждый раз в новый каталог.
//
// До 1.6.2 он писался в `%TEMP%\affory-podmena`. У SYSTEM на старых Windows 10
// это `C:\Windows\Temp`, где обычный пользователь может завести каталог заранее
// и подменить копию между записью и запуском: подменщик запускается от SYSTEM.
func TestPodmenshchikKladyotsyaVKatalogDannyh(t *testing.T) {
	dannye := t.TempDir()
	pervyy, err := prigotovitPodmenshchika(dannye)
	if err != nil {
		t.Fatalf("первый подменщик не приготовлен: %v", err)
	}
	vtoroy, err := prigotovitPodmenshchika(dannye)
	if err != nil {
		t.Fatalf("второй подменщик не приготовлен: %v", err)
	}
	for _, exe := range []string{pervyy, vtoroy} {
		kat := filepath.Dir(exe)
		if !strings.EqualFold(filepath.Dir(kat), dannye) || !strings.HasPrefix(filepath.Base(kat), prefiksPodmeny) {
			t.Errorf("подменщик лежит в %s, а не в своём каталоге внутри %s", exe, dannye)
		}
		if _, err := os.Stat(exe); err != nil {
			t.Errorf("копии службы нет: %v", err)
		}
	}
	if strings.EqualFold(filepath.Dir(pervyy), filepath.Dir(vtoroy)) {
		t.Errorf("два подменщика в одном каталоге %s: заведённый заранее каталог был бы принят",
			filepath.Dir(pervyy))
	}
}

// Уборка снимает только каталоги подменщика и не трогает остальные данные.
func TestUborkaPodmenyTrogaetTolkoEyo(t *testing.T) {
	dannye := t.TempDir()
	zapisat := func(chasti ...string) {
		put := filepath.Join(append([]string{dannye}, chasti...)...)
		if err := os.MkdirAll(filepath.Dir(put), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(put, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	zapisat(prefiksPodmeny+"111", "affory-svc.exe")
	if err := os.Mkdir(filepath.Join(dannye, prefiksPodmeny+"222"), 0o700); err != nil {
		t.Fatal(err)
	}
	zapisat("obnovleniya", "affory-1.6.1.zip")
	zapisat("log", "sluzhba.log")
	zapisat("sekrety.bin")
	// Файл с тем же началом имени не наш: подменщик заводит только каталоги.
	zapisat(prefiksPodmeny + "fayl")

	if zhaloby := ubratKatalogiPodmeny(dannye); len(zhaloby) != 0 {
		t.Errorf("уборка пожаловалась: %v", zhaloby)
	}
	for _, imya := range []string{prefiksPodmeny + "111", prefiksPodmeny + "222"} {
		if _, err := os.Stat(filepath.Join(dannye, imya)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("каталог подменщика %s не убран: %v", imya, err)
		}
	}
	for _, put := range []string{
		filepath.Join("obnovleniya", "affory-1.6.1.zip"),
		filepath.Join("log", "sluzhba.log"),
		"sekrety.bin",
		prefiksPodmeny + "fayl",
	} {
		if _, err := os.Stat(filepath.Join(dannye, put)); err != nil {
			t.Errorf("уборка подмены задела %s: %v", put, err)
		}
	}
}

// Без каталога данных уборке нечего делать, и это не отказ.
func TestUborkaPodmenyBezKatalogaDannyh(t *testing.T) {
	if zhaloby := ubratKatalogiPodmeny(filepath.Join(t.TempDir(), "net")); len(zhaloby) != 0 {
		t.Errorf("отсутствие каталога данных названо отказом: %v", zhaloby)
	}
}
