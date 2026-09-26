package main

import (
	"testing"

	"golang.org/x/sys/windows/registry"
)

// Ключ под HKCU\Software\Affory-test стоит на месте ключа деинсталляции из HKLM:
// тот же вид значения, тот же путь кода, без прав администратора, и после
// теста он удаляется. Настоящую запись живой машины тест не трогает.
const testovyyKlyuchUdaleniya = `Software\Affory-test\Uninstall\Affory`

func podstavnoyKlyuchUdaleniya(t *testing.T, zavesti bool) {
	t.Helper()
	_ = registry.DeleteKey(registry.CURRENT_USER, testovyyKlyuchUdaleniya)
	if zavesti {
		k, _, err := registry.CreateKey(registry.CURRENT_USER, testovyyKlyuchUdaleniya, registry.ALL_ACCESS)
		if err != nil {
			t.Fatalf("тестовый ключ не создан: %v", err)
		}
		if err := k.SetStringValue("DisplayVersion", "1.4.2"); err != nil {
			t.Fatal(err)
		}
		k.Close()
	}
	prezhniy := klyuchUdaleniyaDlyaZapisi
	klyuchUdaleniyaDlyaZapisi = func() (registry.Key, error) {
		return registry.OpenKey(registry.CURRENT_USER, testovyyKlyuchUdaleniya, registry.SET_VALUE)
	}
	t.Cleanup(func() {
		klyuchUdaleniyaDlyaZapisi = prezhniy
		_ = registry.DeleteKey(registry.CURRENT_USER, testovyyKlyuchUdaleniya)
		_ = registry.DeleteKey(registry.CURRENT_USER, `Software\Affory-test\Uninstall`)
		_ = registry.DeleteKey(registry.CURRENT_USER, `Software\Affory-test`)
	})
}

func versiyaIzPodstavnogo(t *testing.T) (string, bool) {
	t.Helper()
	k, err := registry.OpenKey(registry.CURRENT_USER, testovyyKlyuchUdaleniya, registry.QUERY_VALUE)
	if err != nil {
		return "", false
	}
	defer k.Close()
	v, _, err := k.GetStringValue("DisplayVersion")
	if err != nil {
		t.Fatalf("DisplayVersion не читается: %v", err)
	}
	return v, true
}

// Ровно дефект живой машины: установщик 1.4.2 оставил свой номер, install
// версии 1.5.0 обязан его переписать.
func TestInstallPerepisyvaetNomerVReestre(t *testing.T) {
	podstavnoyKlyuchUdaleniya(t, true)
	if err := zapisatVersiyuVReestr("1.5.0"); err != nil {
		t.Fatal(err)
	}
	if v, _ := versiyaIzPodstavnogo(t); v != "1.5.0" {
		t.Fatalf("DisplayVersion %q, а стоит 1.5.0", v)
	}
	// Откат это install прежнего бинаря: номер обязан вернуться тем же путём.
	if err := zapisatVersiyuVReestr("1.4.2"); err != nil {
		t.Fatal(err)
	}
	if v, _ := versiyaIzPodstavnogo(t); v != "1.4.2" {
		t.Fatalf("после отката DisplayVersion %q, а не 1.4.2", v)
	}
}

// Программа без установщика в списке программ не числится, и install не
// должна заводить ей запись без деинсталлятора.
func TestInstallNeZavodytZapisBezUstanovshchika(t *testing.T) {
	podstavnoyKlyuchUdaleniya(t, false)
	if err := zapisatVersiyuVReestr("1.5.0"); err != nil {
		t.Fatalf("отсутствие ключа сочтено отказом: %v", err)
	}
	if _, est := versiyaIzPodstavnogo(t); est {
		t.Fatal("install завела запись деинсталляции сама")
	}
}

func TestSborkaDevNomerNePishet(t *testing.T) {
	podstavnoyKlyuchUdaleniya(t, true)
	if err := zapisatVersiyuVReestr("dev"); err != nil {
		t.Fatal(err)
	}
	if v, _ := versiyaIzPodstavnogo(t); v != "1.4.2" {
		t.Fatalf("сборка dev переписала номер на %q", v)
	}
}
