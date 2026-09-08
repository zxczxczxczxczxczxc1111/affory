package set

import (
	"fmt"
	"os/exec"
	"strings"
)

// Правило ставится при подъёме туннеля и снимается при опускании, НЕЗАВИСИМО от
// режима. Редакция 5 вешала блокировку на kill-switch, а тот по умолчанию
// выключен, то есть в режиме, где клиент живёт всё время, IPv6 не глушился
// ничем.
const ImyaPravilaIPv6 = "Affory-IPv6-Block-Out"

// Семейство адресов выражается ТОЛЬКО адресом. У netsh advfirewall нет
// селектора семейства вовсе (проверено справкой утилиты), поэтому написанное в
// редакции 3 `protocol=any localip=any remoteip=any` означало не "IPv6", а
// "всё исходящее". Замерено в госте 01.09.2026: до правила интернет есть, после
// нет, после снятия снова есть. Блокирующее правило бьёт разрешающие, машина
// запирается наглухо.
//
// `::/0` netsh не принимает: "One or more of the address prefixes is invalid".
// Пространство покрывается двумя половинами. Проверены и приняты также `::/1`,
// `2000::/3` и диапазон целиком; выбрана эта форма как самая явная.
const setiIPv6 = "::/1,8000::/1"

// Шов. Тесты читают команду и подменяют исполнение: заводить настоящее правило
// брандмауэра ради проверки его текста было бы обменом надёжности на ничто.
var vypolnit = vypolnitNetsh

func komandaSozdaniya() []string {
	return []string{
		"advfirewall", "firewall", "add", "rule",
		"name=" + ImyaPravilaIPv6,
		"dir=out", "action=block", "profile=any",
		"remoteip=" + setiIPv6,
	}
}

// Снятие по имени. Имя И ЕСТЬ опознание: по нему правило ищут проверка
// осиротевшего, аварийный файл и человек в панике.
func komandaSnyatiya() []string {
	return []string{"advfirewall", "firewall", "delete", "rule", "name=" + ImyaPravilaIPv6}
}

func komandaPokaza() []string {
	return []string{"advfirewall", "firewall", "show", "rule", "name=" + ImyaPravilaIPv6}
}

func GlushitIPv6() error {
	// Сначала снять, потом поставить. netsh не обновляет правило по имени, он
	// заводит ВТОРОЕ с тем же именем, и снятие потом убирает только одно.
	_ = VernutIPv6()
	if _, err := vypolnit(komandaSozdaniya()); err != nil {
		return fmt.Errorf("правило %s не заведено: %w", ImyaPravilaIPv6, err)
	}
	return nil
}

// VernutIPv6 идемпотентна: снятие несуществующего правила это успех, а не
// ошибка. Иначе уборка после сбоя падала бы ровно там, где она нужнее всего.
func VernutIPv6() error {
	vyhod, err := vypolnit(komandaSnyatiya())
	if err == nil || netPravil(vyhod) {
		return nil
	}
	return fmt.Errorf("правило %s не снято: %w", ImyaPravilaIPv6, err)
}

// PravilaIPv6Est отвечает проверке утечек (6.3): стоит ли правило сейчас.
func PravilaIPv6Est() (bool, error) { return pravilaIPv6Est() }

// pravilaIPv6Est нужна проверке осиротевшего в задаче 2.6.
func pravilaIPv6Est() (bool, error) {
	vyhod, err := vypolnit(komandaPokaza())
	if netPravil(vyhod) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("не удалось спросить про правило: %w", err)
	}
	return strings.Contains(vyhod, ImyaPravilaIPv6), nil
}

// netsh отвечает по-разному на разных языках системы, но обе фразы содержат
// код "No rules match" в английской и "Ни одно правило" в русской локали.
// Опознание идёт по обеим, потому что язык гостя и язык машины владельца это
// две разные настройки, и совпадать они не обязаны.
func netPravil(vyhod string) bool {
	n := strings.ToLower(vyhod)
	return strings.Contains(n, "no rules match") ||
		strings.Contains(n, "ни одно правило")
}

func vypolnitNetsh(argumenty []string) (string, error) {
	cmd := exec.Command("netsh", argumenty...)
	vyhod, err := cmd.CombinedOutput()
	if err != nil {
		return string(vyhod), fmt.Errorf("netsh %s: %w: %s",
			strings.Join(argumenty, " "), err, strings.TrimSpace(string(vyhod)))
	}
	return string(vyhod), nil
}
