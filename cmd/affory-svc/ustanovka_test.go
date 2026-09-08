package main

import (
	"errors"
	"testing"
)

// Снятие программы при включённом режиме «весь трафик» это единственный путь, с
// которого нет возврата: служба удалена, CLI удалён, политика Block осталась.
// Машина без сети и без инструмента, которым это чинить.
func TestSnyatiePriZapertoyMashineRaspechatyvaet(t *testing.T) {
	zaperta := true
	raspechatali := false

	prezhZapert, prezhVykl := zapertaLiMashina, raspechatatVes
	t.Cleanup(func() { zapertaLiMashina, raspechatatVes = prezhZapert, prezhVykl })

	zapertaLiMashina = func() (bool, error) { return zaperta, nil }
	raspechatatVes = func() error { raspechatali = true; zaperta = false; return nil }

	if err := raspechatatPeredSnyatiem(); err != nil {
		t.Fatal(err)
	}
	if !raspechatali {
		t.Fatal("машина осталась запертой: снятие удалит службу и чинить будет нечем")
	}
}

// Открытую машину трогать нельзя: человек мог сам поставить себе политику Block
// задолго до нас, и снятие нашей программы не повод её распечатывать.
func TestSnyatiePriOtkrytoyMashineNichegoNeTrogaet(t *testing.T) {
	prezhZapert, prezhVykl := zapertaLiMashina, raspechatatVes
	t.Cleanup(func() { zapertaLiMashina, raspechatatVes = prezhZapert, prezhVykl })

	zapertaLiMashina = func() (bool, error) { return false, nil }
	raspechatatVes = func() error {
		t.Fatal("тронули брандмауэр на открытой машине")
		return nil
	}

	if err := raspechatatPeredSnyatiem(); err != nil {
		t.Fatal(err)
	}
}

// Неудача распечатывания ОБЯЗАНА остановить снятие. Удалить службу после этого
// значит оставить человека с мёртвой сетью и без единой команды, которой можно
// её вернуть. Отказ восстановим: CLI ещё на месте, попробовать можно снова.
func TestNeudachaRaspechatyvaniyaOtmenyaetSnyatie(t *testing.T) {
	prezhZapert, prezhVykl := zapertaLiMashina, raspechatatVes
	t.Cleanup(func() { zapertaLiMashina, raspechatatVes = prezhZapert, prezhVykl })

	svoya := errors.New("netsh отказал")
	zapertaLiMashina = func() (bool, error) { return true, nil }
	raspechatatVes = func() error { return svoya }

	err := raspechatatPeredSnyatiem()
	if err == nil {
		t.Fatal("снятие продолжилось поверх запертой машины")
	}
	if !errors.Is(err, svoya) {
		t.Fatalf("причина потеряна по дороге: %v", err)
	}
}
