package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// Установщик падал на первом же File с «Невозможно записать», потому что ждал
// освобождения ровно секунду и ни разу не повторял.

func TestPustoyKatalogSchitaetsyaSvobodnym(t *testing.T) {
	// Чистая установка: файлов ещё нет вовсе, и ждать нечего.
	if err := zhdatSvobodnyhFaylov(t.TempDir(), time.Second); err != nil {
		t.Fatalf("пустой каталог объявлен занятым: %v", err)
	}
}

func TestZakrytyeFaylySvobodny(t *testing.T) {
	dir := t.TempDir()
	for _, imya := range faylyUstanovki {
		if err := os.WriteFile(filepath.Join(dir, imya), []byte("MZ"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := zhdatSvobodnyhFaylov(dir, time.Second); err != nil {
		t.Fatalf("закрытые файлы объявлены занятыми: %v", err)
	}
}

func TestZanyatyyFaylNazyvaetsyaISrokSoblyudaetsya(t *testing.T) {
	dir := t.TempDir()
	put := filepath.Join(dir, "affory-ui.exe")
	if err := os.WriteFile(put, []byte("MZ"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Файл, открытый БЕЗ права на совместное использование: ровно так его
	// держит запущенный образ и антивирус со сканированием при закрытии.
	u16, err := windows.UTF16PtrFromString(put)
	if err != nil {
		t.Fatal(err)
	}
	h, err := windows.CreateFile(u16, windows.GENERIC_READ, 0, nil,
		windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Skipf("файл не удалось занять: %v", err)
	}
	defer windows.CloseHandle(h)

	nachalo := time.Now()
	err = zhdatSvobodnyhFaylov(dir, 700*time.Millisecond)
	if err == nil {
		t.Fatal("занятый файл объявлен свободным")
	}
	if !strings.Contains(err.Error(), "affory-ui.exe") {
		t.Fatalf("отказ не называет файл: %v", err)
	}
	// Ждали, а не сдались сразу: прежний Sleep 1000 не давал ни одной попытки.
	if proshlo := time.Since(nachalo); proshlo < 500*time.Millisecond {
		t.Fatalf("сдались через %v, не отработав срок", proshlo)
	}
}

func TestPometkaNaUdalenieUznayotsyaTolkoSvoyaOshibka(t *testing.T) {
	// Ожиданием лечится РОВНО этот код. Повторять «нет прав» тридцать секунд
	// значит растянуть отказ и ничего не починить.
	if !pometkaNaUdalenie(windows.ERROR_SERVICE_MARKED_FOR_DELETE) {
		t.Fatal("своя ошибка не узнана")
	}
	for _, chuzhaya := range []error{
		windows.ERROR_ACCESS_DENIED,
		windows.ERROR_SERVICE_EXISTS,
		errors.New("что-то своё"),
	} {
		if pometkaNaUdalenie(chuzhaya) {
			t.Fatalf("%v принята за пометку на удаление", chuzhaya)
		}
	}
}
