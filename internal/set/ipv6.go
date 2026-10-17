package set

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/sys/windows"

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

func komandaPokaza() []string {
	return []string{"advfirewall", "firewall", "show", "rule", "name=" + ImyaPravilaIPv6}
}

// ZakrytIPv6 закрывает исходящий IPv6 на время туннеля.
//
// Прежде здесь всегда заводилось правило netsh, и его отказ ронял подключение.
// У сотрудника с 04.10.2026 служба брандмауэра тринадцать раз отвечала на
// заведение «Неустранимая ошибка брандмауэра Защитника Windows (0x2)», пачками
// до перезагрузки, и VPN не поднимался вовсе. При этом IPv6 в те минуты уже был
// закрыт: ядро ставит свой фильтр WFP (sing-tun, tun_windows.go, фильтр «block
// ipv6»: при strict_route и туннеле без адреса IPv6 режется весь исходящий
// IPv6, кроме самого ядра). Фильтры ядра идут прямо в BFE, мимо службы MpsSvc,
// её настроек и языка системы, то есть ломаются ровно там, где не поднимется и
// туннель.
//
// Поэтому сначала спрашивается факт, и правило netsh остаётся запасным путём на
// случай, когда ядро IPv6 не закрыло, а у машины IPv6 есть. Отказ и тогда
// роняет подключение: туннель с открытым IPv6 течёт, а течь молча хуже, чем
// громко не подключиться.
func ZakrytIPv6() error {
	if probaFiltra() {
		return nil
	}
	if est, err := estIPv6NaMashine(); err == nil && !est {
		return nil
	}
	// Фильтр ядро ставит сразу после адаптера, а подъём спрашивает, как только
	// адаптер появился. Первый ответ «открыт» поэтому может быть просто ранним.
	srok := time.Now().Add(srokFiltraYadra)
	for time.Now().Before(srok) {
		time.Sleep(shagFiltraYadra)
		if probaFiltra() {
			return nil
		}
	}
	log.Printf("исходящий IPv6 не закрыт ядром, завожу правило %s", ImyaPravilaIPv6)
	if err := GlushitIPv6(); err != nil {
		return fmt.Errorf("исходящий IPv6 открыт, ядро его не закрыло, а %w", err)
	}
	praviloZavedeno.Store(true)
	return nil
}

// OtkrytIPv6 снимает то, что поставил ZakrytIPv6. Фильтр ядра уходит вместе с
// ядром сам, снимать нужно только своё правило netsh.
//
// Без правила netsh не зовётся вовсе: на машине со сломанной службой
// брандмауэра каждое отключение иначе стоило бы трёх отказов netsh. Правило,
// оставшееся от прежнего процесса или прежней версии, снимает уборка при старте
// службы (snyatSirotuIPv6), а не это место.
func OtkrytIPv6() error {
	if !praviloZavedeno.Load() {
		return nil
	}
	if err := VernutIPv6(); err != nil {
		return err
	}
	praviloZavedeno.Store(false)
	return nil
}

// IPv6Zakryt отвечает проверке утечек, может ли сейчас что-то уйти по IPv6
// мимо туннеля. Спрашивается факт, а не наше правило: им IPv6 теперь
// закрывается только в запасном случае.
func IPv6Zakryt() (bool, error) {
	if probaFiltra() {
		return true, nil
	}
	est, err := estIPv6NaMashine()
	if err != nil {
		return false, err
	}
	return !est, nil
}

// praviloZavedeno помнит, заводил ли ЭТОТ процесс правило netsh.
var praviloZavedeno atomic.Bool

var (
	srokFiltraYadra = 3 * time.Second
	shagFiltraYadra = 200 * time.Millisecond
	srokProbyFiltra = 300 * time.Millisecond
)

// adresProbyFiltra из диапазона IPv6 для документации (2001:db8::/32): он
// никому не принадлежит, и без фильтра пакет уходит в никуда, а не к чужому
// серверу.
const adresProbyFiltra = "[2001:db8::1]:443"

// probaFiltra отвечает, может ли исходящий IPv6 этой службы уйти в интернет.
// Шов: тесты подставляют ответ.
var probaFiltra = func() bool {
	d := net.Dialer{Timeout: srokProbyFiltra}
	c, err := d.Dial("tcp6", adresProbyFiltra)
	if err == nil {
		// Соединиться с адресом для документации нельзя, но если вышло, IPv6
		// точно не закрыт.
		if err := c.Close(); err != nil {
			log.Printf("проба IPv6: соединение не закрылось: %v", err)
		}
		return false
	}
	return zakrytPoOtkazu(err)
}

// zakrytPoOtkazu разбирает отказ пробы. Оба «закрытых» отказа Windows отдаёт
// сразу, ещё до отправки пакета:
//
//   - WSAEACCES: исходящий режет фильтр WFP, в туннеле это фильтр ядра;
//   - WSAENETUNREACH: пути в интернет по IPv6 у машины нет вовсе. Так бывает у
//     адаптера с одними частными fd00:: от роутера без шлюза IPv6, и уйти по
//     нему мимо туннеля нечему. Без этого случая такая машина шла бы в netsh,
//     то есть ровно туда, где у сотрудника падало (жалоба 17.10.2026).
//
// Любой другой исход значит «не закрыт» или «не знаю», и это одинаково повод
// закрывать самим.
func zakrytPoOtkazu(err error) bool {
	return errors.Is(err, windows.WSAEACCES) || errors.Is(err, windows.WSAENETUNREACH)
}

// estIPv6NaMashine отвечает, есть ли у какого-нибудь адаптера адрес IPv6 шире
// ссылочного. Без него уйти по IPv6 нечем: fe80:: живёт только в своём
// сегменте. Частные fd00:: считаются: роутер с подменой адресов выпускает их
// наружу так же, как домашние IPv4. Шов: тесты подставляют ответ.
var estIPv6NaMashine = func() (bool, error) {
	adaptery, err := net.Interfaces()
	if err != nil {
		return false, fmt.Errorf("адаптеры не прочитаны: %w", err)
	}
	for _, a := range adaptery {
		if a.Flags&net.FlagUp == 0 || a.Flags&net.FlagLoopback != 0 {
			continue
		}
		adresa, err := a.Addrs()
		if err != nil {
			return false, fmt.Errorf("адреса адаптера %s не прочитаны: %w", a.Name, err)
		}
		for _, ad := range adresa {
			p, ok := ad.(*net.IPNet)
			if !ok || p.IP.To4() != nil {
				continue
			}
			if p.IP.IsGlobalUnicast() {
				return true, nil
			}
		}
	}
	return false, nil
}

// GlushitIPv6 заводит правило netsh. Это запасной путь ZakrytIPv6.
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

// srokNetsh ограничивает один вызов netsh (Н9 аудита 1.6.1). Без срока
// зависший netsh держал подъём туннеля и снятие защиты без конца.
var srokNetsh = 15 * time.Second

// programmaNetsh это шов: тест подставляет зависающую программу.
var programmaNetsh = putNetsh

func vypolnitNetsh(argumenty []string) (string, error) {
	ctx, otmena := context.WithTimeout(context.Background(), srokNetsh)
	defer otmena()
	cmd := exec.CommandContext(ctx, programmaNetsh(), argumenty...)
	// Убитый по сроку процесс мог оставить наследника с нашими трубами вывода:
	// ждать их после убийства дольше пары секунд незачем.
	cmd.WaitDelay = 2 * time.Second
	syrye, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return "", fmt.Errorf("netsh %s не ответил за %v", strings.Join(argumenty, " "), srokNetsh)
	}
	// netsh отвечает в кодовой странице КОНСОЛИ, а не в UTF-8. На русской
	// Windows это 866, и прочитанные как UTF-8 байты дают «ЌЁ ®¤­® Їа ўЁ«®»
	// вместо «Ни одно правило». Пока перевода не было, netPravil ниже не
	// совпадал НИКОГДА: снятие несуществующего правила считалось отказом, и
	// установка поверх прежней падала на подготовке у всех, у кого правил
	// брандмауэра в этот момент не осталось (жалоба 21.09.2026).
	//
	// Но и 866 не везде: на Windows 10.0.26200 у сотрудника netsh ответил в
	// UTF-8, и прочитанный как 866 ответ превратился в «╨Э╨╡╤Г...» (жалоба
	// 17.10.2026). Поэтому сначала проверяется UTF-8.
	vyhod := kodirovki.IzVyvoda(syrye, kodirovki.Konsoli)
	if err != nil {
		return vyhod, fmt.Errorf("netsh %s: %w: %s",
			strings.Join(argumenty, " "), err, strings.TrimSpace(vyhod))
	}
	return vyhod, nil
}

// putNetsh отдаёт netsh из System32. Служба зовёт его от SYSTEM, и поиск по
// PATH нашёл бы первый попавшийся `netsh.exe` из каталога, стоящего в PATH
// раньше системного. Голое имя остаётся только на случай, когда системный
// каталог не назван.
// putSvchost это хост системных служб, в нём живёт служба DNS. Запасной путь
// записан так же, как во встроенных правилах Windows: брандмауэр сам
// раскрывает переменную.
func putSvchost() string {
	sistemnyy, err := windows.GetSystemDirectory()
	if err != nil {
		return `%SystemRoot%\System32\svchost.exe`
	}
	return filepath.Join(sistemnyy, "svchost.exe")
}

func putNetsh() string {
	sistemnyy, err := windows.GetSystemDirectory()
	if err != nil {
		return "netsh"
	}
	return filepath.Join(sistemnyy, "netsh.exe")
}
