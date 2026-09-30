package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

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
	prezhniyAktivator := prochitatAktivator
	prochitatAktivator = func(string) (string, error) { return "", nil }
	t.Cleanup(func() {
		papkaYarlykov, profiliLyudey = prezhnyayaPapka, prezhnieProfili
		udalitReestrovyySled = prezhneeUdalenie
		prochitatAktivator = prezhniyAktivator
	})
}

// L12 аудита 1.8.0: данные WebView2, ключ COM-активации уведомлений и их
// картинка оставались после удаления.
func TestSnyatSledyUbiraetSledyOkna(t *testing.T) {
	koren := t.TempDir()
	profil := filepath.Join(koren, "Users", "chelovek")
	const sid = "S-1-5-21-1-2-3-1001"
	const guid = "{0f1e2d3c-4b5a-6978-8796-a5b4c3d2e1f0}"
	webview := filepath.Join(profil, katalogWebViewVProfile, "EBWebView")
	if err := os.MkdirAll(webview, 0o755); err != nil {
		t.Fatal(err)
	}
	temp := filepath.Join(profil, tempVProfile)
	if err := os.MkdirAll(temp, 0o755); err != nil {
		t.Fatal(err)
	}
	png := filepath.Join(temp, "Affory"+guid+".png")
	chuzhoyPng := filepath.Join(temp, "Drugoe"+guid+".png")
	for _, f := range []string{png, chuzhoyPng} {
		if err := os.WriteFile(f, []byte("png"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	podstavnyeSledy(t, filepath.Join(koren, "net-papki"), map[string]string{sid: profil})
	var snyaty []string
	udalitReestrovyySled = func(_ registry.Key, put string) error { snyaty = append(snyaty, put); return nil }
	prochitatAktivator = func(string) (string, error) { return guid, nil }

	for _, z := range snyatSledy() {
		t.Fatalf("след не убран: %s", z)
	}
	if _, err := os.Stat(filepath.Join(profil, katalogWebViewVProfile)); !os.IsNotExist(err) {
		t.Error("данные WebView2 остались")
	}
	if _, err := os.Stat(png); !os.IsNotExist(err) {
		t.Error("картинка уведомлений осталась")
	}
	if _, err := os.Stat(chuzhoyPng); err != nil {
		t.Errorf("снята чужая картинка: %v", err)
	}
	if !slices.Contains(snyaty, sid+`\Software\Classes\CLSID\`+guid) {
		t.Errorf("ключ COM-активации не снят, снимались %v", snyaty)
	}

	// Номер из куста человека, а снимает SYSTEM: мусор вместо номера не
	// должен увести удаление на чужой ключ.
	snyaty = nil
	prochitatAktivator = func(string) (string, error) { return `{x}\..\Microsoft`, nil }
	zhaloby := snyatSledy()
	for _, put := range snyaty {
		if strings.Contains(put, "CLSID") {
			t.Fatalf("по негодному номеру снимался ключ %s", put)
		}
	}
	if len(zhaloby) == 0 {
		t.Error("негодный номер уведомлений прошёл молча")
	}
}

// Приёмка 1.9.0 в госте 30.09.2026: данные WebView2 оставались после удаления,
// потому что процессы WebView2 ещё держали файлы, когда уборка до них дошла.
// Они умирают не вместе с окном, а следом, поэтому каталог снимается с
// повторами. Держатель здесь настоящий: открытый файл внутри каталога.
func TestZanyatyyKatalogWebViewSnimaetsyaPovtorom(t *testing.T) {
	profil := filepath.Join(t.TempDir(), "Users", "chelovek")
	webview := filepath.Join(profil, katalogWebViewVProfile, "EBWebView")
	if err := os.MkdirAll(webview, 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(webview, "Cookies"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	podstavnyeSledy(t, filepath.Join(t.TempDir(), "net-papki"), map[string]string{"S-1-5-21-1-2-3-1001": profil})
	prezhnee := zhdatSleda
	t.Cleanup(func() { zhdatSleda = prezhnee })
	pauz := 0
	zhdatSleda = func(time.Duration) {
		pauz++
		// Держатель уходит к первой паузе, как WebView2 следом за окном.
		_ = f.Close()
	}

	for _, z := range snyatSledy() {
		t.Fatalf("след не убран: %s", z)
	}
	if pauz == 0 {
		t.Fatal("держатель не помешал ни разу: тест ничего не проверил")
	}
	if _, err := os.Stat(filepath.Join(profil, katalogWebViewVProfile)); !os.IsNotExist(err) {
		t.Error("данные WebView2 остались")
	}
}

// Держатель, который не уходит вовсе, не держит удаление вечно: повторы
// конечны, и остаток называется жалобой.
func TestVechnoZanyatyyKatalogDayotZhalobu(t *testing.T) {
	profil := filepath.Join(t.TempDir(), "Users", "chelovek")
	webview := filepath.Join(profil, katalogWebViewVProfile, "EBWebView")
	if err := os.MkdirAll(webview, 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(webview, "Cookies"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	podstavnyeSledy(t, filepath.Join(t.TempDir(), "net-papki"), map[string]string{"S-1-5-21-1-2-3-1001": profil})
	prezhnee := zhdatSleda
	t.Cleanup(func() { zhdatSleda = prezhnee })
	var vsego time.Duration
	zhdatSleda = func(d time.Duration) { vsego += d }

	zhaloby := snyatSledy()
	if !slices.ContainsFunc(zhaloby, func(z string) bool { return strings.Contains(z, katalogWebViewVProfile) }) {
		t.Fatalf("занятый каталог прошёл молча: %v", zhaloby)
	}
	if vsego == 0 || vsego > 15*time.Second {
		t.Fatalf("ожидание занятого каталога %v", vsego)
	}
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
