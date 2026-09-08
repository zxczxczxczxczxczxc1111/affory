//go:build windows

package main

import (
	"os"
	"syscall"
	"time"

	"github.com/wailsapp/wails/v3/pkg/w32"
)

// Маленькая иконка окна. Wails ставит только большую
// (webview_window_windows.go: SendMessage(WM_SETICON, ICON_BIG, ...)), а слева
// от заголовка в превью панели задач Windows рисует МАЛЕНЬКУЮ. Без неё берётся
// иконка класса окна, зарегистрированного с IDI_APPLICATION, то есть голая
// стандартная. Владелец увидел ровно это 04.09.2026.
//
// Здесь только вторая половина работы: большую закрывает application.Options.Icon
// в main.go.
const (
	wmSetIcon = 0x0080
	iconSmall = 0
)

// svoyoOkno возвращает окно ЭТОГО процесса. Поиск по заголовку не годится:
// «Affory» может оказаться заголовком чужого окна, и тогда иконку получит оно.
func svoyoOkno() w32.HWND {
	moy := os.Getpid()
	var nashe w32.HWND
	obratnyy := syscall.NewCallback(func(hwnd w32.HWND, _ uintptr) uintptr {
		if _, pid := w32.GetWindowThreadProcessId(hwnd); pid == moy && w32.IsWindowVisible(hwnd) {
			nashe = hwnd
			return 0 // нашли, перечисление можно прекращать
		}
		return 1
	})
	w32.EnumWindows(obratnyy, 0)
	return nashe
}

// PostavitMaluyuIkonkuKogdaOkno ждёт появления окна и ставит иконку.
//
// Ожидание, а не событие: WindowShow при ПЕРВОМ показе не приходит (проверено
// 04.09.2026, ICON_SMALL оставался нулём), а до появления окна HWND не
// существует и ставить иконку некуда. Опрос переживает любую смену событий в
// библиотеке, чего про хук сказать нельзя.
func PostavitMaluyuIkonkuKogdaOkno(kartinka []byte) {
	go func() {
		for i := 0; i < 50; i++ {
			if postavitMaluyuIkonku(kartinka) {
				return
			}
			time.Sleep(200 * time.Millisecond)
		}
	}()
}

// postavitMaluyuIkonku молчит при любой неудаче намеренно: иконка это украшение
// окна, и падать из-за неё, когда туннель работает, было бы хуже дефекта.
// Возвращает, получилось ли: по этому признаку опрос выше прекращается.
func postavitMaluyuIkonku(kartinka []byte) bool {
	if len(kartinka) == 0 {
		return true // ставить нечего, ждать нет смысла
	}
	hwnd := svoyoOkno()
	if hwnd == 0 {
		return false
	}
	ikonka, err := w32.CreateSmallHIconFromImage(kartinka)
	if err != nil || ikonka == 0 {
		return false
	}
	w32.SendMessage(hwnd, wmSetIcon, iconSmall, uintptr(ikonka))
	return true
}
