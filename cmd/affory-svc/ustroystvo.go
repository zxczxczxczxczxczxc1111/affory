package main

import (
	"fmt"
	"log"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/ssylki"
)

// ustroystvoMashiny собирает то, что панель подписки узнаёт о машине (П9
// аудита 1.6.1). Всё читается только на чтение. Недостающее поле просто не
// уходит в запрос: без номера машины панель с лимитом устройств ответит
// отказом, и человек увидит его своими словами.
func ustroystvoMashiny() ssylki.Ustroystvo {
	u := ssylki.Ustroystvo{Versiya: versiyaProgrammy, OS: "Windows"}
	var err error
	if u.Id, err = strokaIzReestra(`SOFTWARE\Microsoft\Cryptography`, "MachineGuid"); err != nil {
		log.Printf("номер машины для подписки не прочитан: %v", err)
	}
	v := windows.RtlGetVersion()
	u.VersiyaOS = fmt.Sprintf("%d.%d.%d", v.MajorVersion, v.MinorVersion, v.BuildNumber)
	// Модели может не быть у виртуальных машин и самосборов: тогда без неё.
	if m, err := strokaIzReestra(`HARDWARE\DESCRIPTION\System\BIOS`, "SystemProductName"); err == nil {
		u.Model = strings.TrimSpace(m)
	}
	return u
}

func strokaIzReestra(put, imya string) (string, error) {
	// Вид 64 явно, как и в versiya_v_reestre.go: иначе сборка под другую
	// разрядность читала бы не тот ключ.
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, put, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return "", err
	}
	defer k.Close()
	s, _, err := k.GetStringValue(imya)
	return s, err
}
