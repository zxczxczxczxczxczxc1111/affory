//go:build windows

package main

import "testing"

// Живую часть (найти окно, сделать HICON, послать WM_SETICON) тестом не
// покрыть: нужно настоящее окно. Она проверена прогоном 04.09.2026, Windows на
// WM_GETICON отвечает ненулевыми хэндлами для ICON_BIG и ICON_SMALL.
//
// Здесь проверяется решение, из-за которого ожидание могло бы крутиться десять
// секунд впустую: пустая картинка это не «ещё не готово», а «ставить нечего».
func TestPustayaKartinkaNeZastavlyaetZhdat(t *testing.T) {
	if !postavitMaluyuIkonku(nil) {
		t.Error("пустая картинка вернула «не готово»: опрос будет крутиться до потолка попыток")
	}
}

func TestIkonkaOknaVshitaVBinar(t *testing.T) {
	if len(ikonkaOkna) == 0 {
		t.Fatal("ikonkaOkna пуста: embed не сработал, окно останется с иконкой класса")
	}
}
