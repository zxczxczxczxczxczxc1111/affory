package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows/registry"
)

// Ярлыки и каталоги профилей проверяются на подставных путях: настоящие ведут
// в меню Пуск живой машины, и тест, который туда пишет, однажды оттуда сотрёт.
func podstavnyeSledy(t *testing.T, yarlyki string, profili map[string]string) {
	t.Helper()
	prezhnyayaPapka, prezhnieProfili := papkaYarlykov, profiliLyudey
	prezhneeUdalenie := udalitReestrovyySled
	// Файловая проверка не должна снимать регистрацию установленного клиента.
	udalitReestrovyySled = func(registry.Key, string) error { return nil }
	papkaYarlykov = func() (string, error) { return yarlyki, nil }
	profiliLyudey = func() (map[string]string, error) { return profili, nil }
	t.Cleanup(func() {
		papkaYarlykov, profiliLyudey = prezhnyayaPapka, prezhnieProfili
		udalitReestrovyySled = prezhneeUdalenie
	})
}

func TestSnyatSledyUbiraetYarlykiIKatalogiProfiley(t *testing.T) {
	koren := t.TempDir()

	yarlyki := filepath.Join(koren, "Start Menu", "Affory")
	if err := os.MkdirAll(yarlyki, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(yarlyki, "Affory.lnk"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	profil := filepath.Join(koren, "Users", "chelovek")
	sled := filepath.Join(profil, katalogVProfile)
	if err := os.MkdirAll(sled, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sled, "uvedomlenie.png"), []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Соседний каталог того же профиля трогать нельзя: удаляем свой след, а не
	// профиль человека.
	chuzhoy := filepath.Join(profil, "AppData", "Local", "Notepad")
	if err := os.MkdirAll(chuzhoy, 0o755); err != nil {
		t.Fatal(err)
	}

	podstavnyeSledy(t, yarlyki, map[string]string{"S-1-5-21-1-2-3-1001": profil})

	zhaloby := snyatSledy()
	for _, z := range zhaloby {
		t.Fatalf("след не убран: %s", z)
	}

	if _, err := os.Stat(yarlyki); !os.IsNotExist(err) {
		t.Fatal("папка ярлыков осталась")
	}
	if _, err := os.Stat(sled); !os.IsNotExist(err) {
		t.Fatal("каталог следа в профиле остался")
	}
	if _, err := os.Stat(chuzhoy); err != nil {
		t.Fatalf("снятие тронуло чужой каталог профиля: %v", err)
	}
}

// Служебные учётки не люди: чистить их профили незачем, а S-1-5-18 это профиль
// самой службы, и снести из него что-нибудь лишнее дороже любого следа.
func TestChelovecheskiySidOtseivaetSluzhebnye(t *testing.T) {
	lyudi := []string{"S-1-5-21-107355383-1496886291-1388135531-1000", "S-1-12-1-11-22-33-44"}
	ne := []string{"S-1-5-18", "S-1-5-19", "S-1-5-20", ".DEFAULT",
		"S-1-5-21-107355383-1496886291-1388135531-1000_Classes"}

	for _, s := range lyudi {
		if !chelovecheskiySid(s) {
			t.Fatalf("%s это человек, а отсеян", s)
		}
	}
	for _, s := range ne {
		if chelovecheskiySid(s) {
			t.Fatalf("%s не человек, а принят", s)
		}
	}
}

// Ветка с подключами: registry.DeleteKey отказывается от такой, и без своей
// рекурсии `Software\Affory` с его `Okno` не снялся бы никогда.
func TestUdalitKlyuchSPotomkamiSnimaetVetkuTselikom(t *testing.T) {
	koren := `Software\Affory proba udaleniya`
	k, _, err := registry.CreateKey(registry.CURRENT_USER, koren, registry.SET_VALUE)
	if err != nil {
		t.Skipf("реестр недоступен: %v", err)
	}
	k.Close()
	rebyonok, _, err := registry.CreateKey(registry.CURRENT_USER, koren+`\Okno`, registry.SET_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	if err := rebyonok.SetStringValue("skazali", "da"); err != nil {
		t.Fatal(err)
	}
	rebyonok.Close()
	t.Cleanup(func() { _ = udalitKlyuchSPotomkami(registry.CURRENT_USER, koren) })

	if err := udalitKlyuchSPotomkami(registry.CURRENT_USER, koren); err != nil {
		t.Fatalf("ветка не снята: %v", err)
	}
	if _, err := registry.OpenKey(registry.CURRENT_USER, koren, registry.QUERY_VALUE); err == nil {
		t.Fatal("ключ остался на месте")
	}
}

// Второй заход обязан молчать: снятие зовут и после установщика, который часть
// следов уже убрал, и повторно руками.
func TestSnyatieSledaIdempotentno(t *testing.T) {
	if err := udalitKlyuchSPotomkami(registry.CURRENT_USER, `Software\Affory net takogo klyucha`); err != nil {
		t.Fatalf("отсутствие ключа должно быть успехом: %v", err)
	}

	koren := t.TempDir()
	podstavnyeSledy(t, filepath.Join(koren, "net-papki"), map[string]string{})
	for _, z := range snyatSledy() {
		if strings.Contains(z, "ярлыки") {
			t.Fatalf("отсутствие папки ярлыков должно быть успехом: %s", z)
		}
	}
}
