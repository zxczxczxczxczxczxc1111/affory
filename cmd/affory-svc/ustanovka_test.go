package main

import (
	"errors"
	"fmt"
	"testing"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc/mgr"
)

// Отсутствие службы при снятии это не отказ (В3 аудита 1.6.1). Прежде любой
// отказ OpenService заворачивался в «служба не установлена» и ронял снятие
// насмерть, а отказ в доступе и отсутствие службы выглядели одинаково.
func TestSluzhbyNetUznayotTolkoOtsutstvie(t *testing.T) {
	for _, c := range []struct {
		err  error
		nety bool
	}{
		{windows.ERROR_SERVICE_DOES_NOT_EXIST, true},
		{fmt.Errorf("открытие: %w", windows.ERROR_SERVICE_DOES_NOT_EXIST), true},
		{windows.ERROR_ACCESS_DENIED, false},
		{windows.ERROR_SERVICE_MARKED_FOR_DELETE, false},
	} {
		if got := sluzhbyNet(c.err); got != c.nety {
			t.Errorf("%v: службы нет=%v, ждали %v", c.err, got, c.nety)
		}
	}
}

// Обновление службы на месте не имеет права портить то, чего не задаёт.
// UpdateConfig передаёт каждое поле как есть, и ноль в типе службы означает не
// «без изменений», а другой тип. Путь в кавычках: без них SCM читает
// `C:\Program` с аргументами.
func TestObnovlenieNastroekSluzhby(t *testing.T) {
	bylo := mgr.Config{
		ServiceType:    windows.SERVICE_WIN32_OWN_PROCESS,
		ErrorControl:   mgr.ErrorNormal,
		StartType:      mgr.StartManual,
		BinaryPathName: `"C:\Staroe\affory-svc.exe"`,
		DisplayName:    "что-то своё",
	}
	put := `D:\Новая папка\Affory\affory-svc.exe`
	stalo := obnovitNastroyki(bylo, put)
	if stalo.ServiceType != bylo.ServiceType || stalo.ErrorControl != bylo.ErrorControl {
		t.Errorf("тип или реакция на ошибку потеряны: %+v", stalo)
	}
	if stalo.StartType != mgr.StartAutomatic {
		t.Errorf("служба не автоматическая: %d", stalo.StartType)
	}
	if stalo.BinaryPathName != `"`+put+`"` {
		t.Errorf("путь службы %s, ждали в кавычках", stalo.BinaryPathName)
	}
	if stalo.DisplayName != "Affory" || stalo.ServiceStartName != "LocalSystem" {
		t.Errorf("имя или учётка не наши: %+v", stalo)
	}
}

// Снятие возвращает IPv6 всегда, а не только на запертой машине. Правило
// живёт отдельно от файла отката, и если служба уже лежала с поднятым
// туннелем, после удаления программы IPv6 оставался закрытым навсегда.
func TestSnyatieVozvrashchaetIPv6NaOtkrytoyMashine(t *testing.T) {
	prezhZapert, prezhVykl, prezhV6, prezhAvto := zapertaLiMashina, raspechatatVes, vernutIPv6, snyatAvtozapusk
	t.Cleanup(func() {
		zapertaLiMashina, raspechatatVes, vernutIPv6, snyatAvtozapusk = prezhZapert, prezhVykl, prezhV6, prezhAvto
	})
	zapertaLiMashina = func() (bool, error) { return false, nil }
	raspechatatVes = func() error {
		t.Fatal("тронули брандмауэр на открытой машине")
		return nil
	}
	vernuli, snyali := false, false
	vernutIPv6 = func() error { vernuli = true; return nil }
	snyatAvtozapusk = func() error { snyali = true; return nil }

	if err := snyatOstavsheesya(); err != nil {
		t.Fatal(err)
	}
	if !vernuli {
		t.Error("IPv6 не возвращён: правило переживёт удаление программы")
	}
	if !snyali {
		t.Error("автозапуск не снят: окно вернётся при входе и укажет на удалённый бинарь")
	}
}

// Неудача с IPv6 останавливает снятие так же, как неудача распечатывания:
// файлы остаются, и попробовать можно снова. Удалить программу и оставить
// человеку закрытый IPv6 без инструмента значит сделать наоборот.
func TestNeudachaIPv6OtmenyaetSnyatie(t *testing.T) {
	prezhZapert, prezhV6, prezhAvto := zapertaLiMashina, vernutIPv6, snyatAvtozapusk
	t.Cleanup(func() { zapertaLiMashina, vernutIPv6, snyatAvtozapusk = prezhZapert, prezhV6, prezhAvto })
	svoya := errors.New("netsh отказал")
	zapertaLiMashina = func() (bool, error) { return false, nil }
	vernutIPv6 = func() error { return svoya }
	snyatAvtozapusk = func() error { return nil }

	if err := snyatOstavsheesya(); !errors.Is(err, svoya) {
		t.Fatalf("снятие продолжилось поверх закрытого IPv6: %v", err)
	}
}

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
