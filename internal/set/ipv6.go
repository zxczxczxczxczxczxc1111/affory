package set

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/kodirovki"
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
func VernutIPv6() error { return SnyatPravilo(ImyaPravilaIPv6) }

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

// netsh отвечает по-разному на разных языках системы: "No rules match" в
// английской, «Ни одно правило» в русской. Опознание идёт по обеим, потому что
// язык гостя и язык рабочей машины это две разные настройки.
//
// Языков у Windows под сорок, и остальные тридцать восемь сюда не впишешь.
// Поэтому фраза это только быстрый путь, а решает SnyatPravilo ниже: оно
// смотрит на ИМЯ правила, которое не переводится ни на одном языке.
func netPravil(vyhod string) bool {
	n := strings.ToLower(vyhod)
	return strings.Contains(n, "no rules match") ||
		strings.Contains(n, "ни одно правило")
}

// SnyatPravilo убирает правило брандмауэра и считает его отсутствие успехом.
//
// Отсутствие правила netsh называет отказом: код возврата 1 и фраза на языке
// системы. Читать фразу это ставить работу продукта в зависимость от локали, и
// 21.09.2026 это уже стоило человеку установки: у него правил в тот момент не
// было, prepare-install упал на первом же снятии, а причина была написана
// кодовой страницей консоли, которую мы тогда не переводили.
//
// Поэтому при отказе спрашивается ФАКТ: стоит ли правило сейчас. Имя правила
// это наша строка из ASCII, она одинакова на любом языке.
func SnyatPravilo(imya string) error {
	vyhod, err := vypolnit([]string{"advfirewall", "firewall", "delete", "rule", "name=" + imya})
	if err == nil || netPravil(vyhod) {
		return nil
	}
	if est, _ := PraviloEst(imya); !est {
		return nil
	}
	return fmt.Errorf("правило %s не снято: %w", imya, err)
}

// PraviloEst отвечает, стоит ли правило с таким именем.
//
// Отказ самого показа считается «не знаем, но и снять не смогли»: наружу это
// уходит как false вместе с ошибкой, и вызывающий решает сам.
func PraviloEst(imya string) (bool, error) {
	vyhod, err := vypolnit([]string{"advfirewall", "firewall", "show", "rule", "name=" + imya})
	if netPravil(vyhod) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("не удалось спросить про правило %s: %w", imya, err)
	}
	return strings.Contains(vyhod, imya), nil
}

func vypolnitNetsh(argumenty []string) (string, error) {
	cmd := exec.Command("netsh", argumenty...)
	syrye, err := cmd.CombinedOutput()
	// netsh отвечает в кодовой странице КОНСОЛИ, а не в UTF-8. На русской
	// Windows это 866, и прочитанные как UTF-8 байты дают «ЌЁ ®¤­® Їа ўЁ«®»
	// вместо «Ни одно правило». Пока перевода не было, netPravil ниже не
	// совпадал НИКОГДА: снятие несуществующего правила считалось отказом, и
	// установка поверх прежней падала на подготовке у всех, у кого правил
	// брандмауэра в этот момент не осталось (жалоба 21.09.2026).
	vyhod := kodirovki.Iz(syrye, kodirovki.Konsoli)
	if err != nil {
		return vyhod, fmt.Errorf("netsh %s: %w: %s",
			strings.Join(argumenty, " "), err, strings.TrimSpace(vyhod))
	}
	return vyhod, nil
}
