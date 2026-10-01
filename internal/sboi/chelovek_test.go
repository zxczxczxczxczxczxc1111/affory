//go:build windows

package sboi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

// Тексты взяты из разбора 01.10.2026: ровно то, что человек видел в окне.
func TestObrezatTehniku(t *testing.T) {
	sluchai := []struct{ vhod, zhdyom string }{
		{"подписка не загрузилась: dial tcp: lookup vpn.example.net: getaddrinfow: This is usually a temporary error during hostname resolution (имя сервера не разрешилось)",
			"подписка не загрузилась"},
		{`секреты не записаны: open C:\ProgramData\Affory\sekrety.bin: Access is denied.`, "секреты не записаны"},
		// Кириллица из имени папки не делает звено нашим.
		{`секреты не записаны: open C:\Users\Иван\AppData\x.json: Access is denied.`, "секреты не записаны"},
		{"драйвер VPN не установился: ERROR[0001] start inbound/tun: Access is denied.: TUN-адаптер не появился: affory: context deadline exceeded",
			"драйвер VPN не установился"},
		{`clash_api не отвечает: Get "http://127.0.0.1:53123/proxies/vybor": dial tcp 127.0.0.1:53123: connectex: refused`,
			"clash_api не отвечает"},
		// Код в начале строки пропускается, наш текст за ним остаётся.
		{`foreign-proxy-hijack: порт 9090 занят процессом C:\x\nekobox_core.exe, а не ядром sing-box.exe`,
			`порт 9090 занят процессом C:\x\nekobox_core.exe, а не ядром sing-box.exe`},
		// Длинный путь не перевешивает русскую фразу.
		{`порт 9090 занят процессом C:\Program Files\NekoBox Portable Edition\nekobox_core.exe, а не ядром sing-box.exe`,
			`порт 9090 занят процессом C:\Program Files\NekoBox Portable Edition\nekobox_core.exe, а не ядром sing-box.exe`},
		{"VPN не понёс трафик за 2 попытки по 15s", "VPN не понёс трафик за 2 попытки по 15s"},
		{"полоса не измерена: мишень не отдала байт, ответив 403", "полоса не измерена: мишень не отдала байт, ответив 403"},
		// Список адресов после нашей фразы остаётся: по нему видно, какие
		// серверы пропущены.
		{"имя не разрешилось: a.example.com, b.example.net", "имя не разрешилось: a.example.com, b.example.net"},
		{"context deadline exceeded", ""},
		{"", ""},
	}
	for _, sl := range sluchai {
		if got := ObrezatTehniku(sl.vhod); got != sl.zhdyom {
			t.Errorf("ObrezatTehniku(%q)\n  = %q\n  ждали %q", sl.vhod, got, sl.zhdyom)
		}
	}
}

func TestDlyaChelovekaStavitPrichinuPoTipu(t *testing.T) {
	zanyat := &os.PathError{Op: "open", Path: `C:\ProgramData\Affory\x`, Err: windows.ERROR_SHARING_VIOLATION}
	sluchai := []struct {
		imya   string
		err    error
		zhdyom string
	}{
		{"доступ", fmt.Errorf("секреты не записаны: %w", &os.PathError{Op: "open", Path: `C:\x`, Err: windows.ERROR_ACCESS_DENIED}),
			"секреты не записаны: нет доступа"},
		{"занят", fmt.Errorf("файл состояния не записан: %w", zanyat), "файл состояния не записан: файл занят другой программой"},
		{"диск", fmt.Errorf("архив не записан: %w", &os.PathError{Op: "write", Path: `C:\x`, Err: windows.ERROR_DISK_FULL}),
			"архив не записан: на диске нет места"},
		{"имя", fmt.Errorf("описание выпуска не получено: %w", &net.DNSError{Err: "no such host", Name: "api.github.com"}),
			"описание выпуска не получено: имя сервера не разрешилось"},
		{"срок голый", context.DeadlineExceeded, "ответа не дождались"},
		{"json", fmt.Errorf("описание выпуска не разобрано: %w", &json.SyntaxError{}), "описание выпуска не разобрано: данные не разобрались"},
		// Сообщение целиком наше: причина по типу была бы повтором.
		{"наше целиком", errors.New("сервер занят подключением, сначала отключись"), "сервер занят подключением, сначала отключись"},
		{"ничего не осталось", errors.New("unexpected thing"), ZapasnoyTekst},
	}
	for _, sl := range sluchai {
		t.Run(sl.imya, func(t *testing.T) {
			if got := DlyaCheloveka(sl.err); got != sl.zhdyom {
				t.Errorf("%q, ждали %q", got, sl.zhdyom)
			}
		})
	}
	if DlyaCheloveka(nil) != "" {
		t.Error("пустая ошибка дала текст")
	}
}

// Ни одна латинская фраза из Windows не должна доехать до человека, какой бы
// ни была обёртка.
func TestDlyaChelovekaBezAngliyskogo(t *testing.T) {
	for _, err := range []error{
		fmt.Errorf("правило не принято: %w", &os.PathError{Op: "CreateFile", Path: `C:\x.exe`, Err: windows.ERROR_FILE_NOT_FOUND}),
		fmt.Errorf("журнал соединений не стёрт: %w", &os.PathError{Op: "remove", Path: `C:\log`, Err: windows.ERROR_SHARING_VIOLATION}),
		errors.New("The RPC server is unavailable."),
	} {
		got := DlyaCheloveka(err)
		for _, slovo := range []string{"The ", "is ", "CreateFile", "remove", "RPC"} {
			if strings.Contains(got, slovo) {
				t.Errorf("%q несёт %q", got, slovo)
			}
		}
	}
}
