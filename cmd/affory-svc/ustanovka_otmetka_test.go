package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/sostoyanie"
)

// L11 аудита 1.8.0: обновление установщиком опускало туннель насовсем,
// хотя самообновление поднимало его обратно.
func TestOtmetkaPodnyatogoTunnelyaDoezzhaetDoInstall(t *testing.T) {
	katalog := t.TempDir()
	seychas := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	podnyat := func() (sostoyanie.SostoyanieFayla, error) {
		return sostoyanie.SostoyanieFayla{Sostoyanie: protokol.SostPodnyat}, nil
	}

	otmetitPodnyatyyTunnel(katalog, podnyat, seychas)
	if !zabratOtmetkuPodnyat(katalog, seychas.Add(time.Minute)) {
		t.Fatal("туннель был поднят, а install об этом не узнал")
	}
	if _, err := os.Stat(filepath.Join(katalog, imyaOtmetkiPodnyat)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("отметка осталась и сработает на следующей установке")
	}
	if zabratOtmetkuPodnyat(katalog, seychas) {
		t.Fatal("отметки нет, а install поднимает туннель")
	}
}

func TestOtmetkaNeKladyotsyaBezTunnelya(t *testing.T) {
	katalog := t.TempDir()
	seychas := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	for _, prochitat := range []func() (sostoyanie.SostoyanieFayla, error){
		func() (sostoyanie.SostoyanieFayla, error) {
			return sostoyanie.SostoyanieFayla{Sostoyanie: protokol.SostVyklyuchen}, nil
		},
		func() (sostoyanie.SostoyanieFayla, error) {
			return sostoyanie.SostoyanieFayla{}, errors.New("тест: файла нет")
		},
	} {
		otmetitPodnyatyyTunnel(katalog, prochitat, seychas)
		if zabratOtmetkuPodnyat(katalog, seychas) {
			t.Fatal("туннель не был поднят, а install его поднимет")
		}
	}
}

// Отметка оборванной установки не должна поднять туннель через неделю.
func TestStarayaOtmetkaNeSrabatyvaet(t *testing.T) {
	katalog := t.TempDir()
	seychas := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	otmetitPodnyatyyTunnel(katalog, func() (sostoyanie.SostoyanieFayla, error) {
		return sostoyanie.SostoyanieFayla{Sostoyanie: protokol.SostPodnyat}, nil
	}, seychas)
	if zabratOtmetkuPodnyat(katalog, seychas.Add(srokOtmetkiPodnyat+time.Minute)) {
		t.Fatal("старая отметка подняла туннель")
	}
	if _, err := os.Stat(filepath.Join(katalog, imyaOtmetkiPodnyat)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("старая отметка не убрана")
	}
}
