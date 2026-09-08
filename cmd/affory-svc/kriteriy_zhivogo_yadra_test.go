package main

import (
	"os"
	"strings"
	"testing"
)

// The criterion is spelled out twice in the code itself
// (komandy_serverov.go, profil.go) and was violated in six places.
// This gate reads the sources and fails on any new violation.
//
// «Ядро живо» это ПУСТОЙ адрес clash_api, а не состояние. Разница не
// теоретическая: opustit обнуляет порт, а состояния otkaz и ne-neset держатся,
// пока крутится восстановление. Судящий по состоянию отвечает «сначала
// отключись» тому, кто уже отключён, и отказывается смотреть на утечки ровно
// в подъёме, где утечка вероятнее всего.

// Ворота грубые НАМЕРЕННО: строка про состояние в этих файлах либо ошибка,
// либо требует метки и объяснения, дописанного руками.
const metkaSostoyaniya = "sostoyanie-a-ne-yadro"

var faylyKriteriya = []string{
	"killswitch.go", "utechki.go", "statistika.go",
	"zhurnal_soedineniy.go", "komandy_pravil.go",
}

// suditPoSostoyaniyu отвечает, СРАВНИВАЕТ ли строка состояние службы.
//
// Упоминание константы само по себе не приговор: postavit ставит состояние, и
// это не суждение о живом ядре. Приговор это сравнение.
func suditPoSostoyaniyu(stroka string) bool {
	kod := stroka
	if i := strings.Index(kod, "//"); i >= 0 {
		kod = kod[:i]
	}
	if !strings.Contains(kod, "protokol.SostPodnyat") &&
		!strings.Contains(kod, "protokol.SostVyklyuchen") {
		return false
	}
	return strings.Contains(kod, "==") || strings.Contains(kod, "!=")
}

func TestNiktoNeSuditOZhivomYadrePoSostoyaniyu(t *testing.T) {
	for _, f := range faylyKriteriya {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("%s не читается: %v", f, err)
		}
		for i, stroka := range strings.Split(string(b), "\n") {
			if !suditPoSostoyaniyu(stroka) || strings.Contains(stroka, metkaSostoyaniya) {
				continue
			}
			t.Errorf("%s:%d судит о живом ядре по состоянию: %s", f, i+1, strings.TrimSpace(stroka))
		}
	}
}

// Ворота, которые ничего не умеют найти, зелены навсегда и потому не
// существуют. Прибор проверяется на образцах, а не на вере.
func TestVorotaKriteriyaLovyatObrazcy(t *testing.T) {
	krasnye := []string{
		"	if sost != protokol.SostPodnyat {",
		"		Podnyat:       s.sost == protokol.SostPodnyat,",
		"	if !vkl || sost != protokol.SostPodnyat || len(tun.Adresa) == 0 {",
		"	return otvet(k.Id, k.Imya, teloPravil(p, s.Status().Sostoyanie != protokol.SostVyklyuchen))",
	}
	for _, s := range krasnye {
		if !suditPoSostoyaniyu(s) {
			t.Errorf("ворота пропустили суждение по состоянию: %s", strings.TrimSpace(s))
		}
	}
	zelenye := []string{
		`	if adres, _ := s.dostupKKlash(); adres == "" {`,
		"	// Критерий «ядро живо» это не protokol.SostPodnyat, а адрес clash_api",
		"	s.postavit(protokol.SostPodnyat, nil)",
		`	return fmt.Errorf("режим включается только поверх живого туннеля, сейчас %s", sost)`,
	}
	for _, s := range zelenye {
		if suditPoSostoyaniyu(s) {
			t.Errorf("ворота сработали на строке, которая ничего не судит: %s", strings.TrimSpace(s))
		}
	}
}
