package main

import "testing"

func TestOknoBezRamkiIRazmerPoRaskladke(t *testing.T) {
	// Options are built by a plain function so they can be judged without
	// opening a window. A window opened inside a test is not a test anymore.
	o := oknoOpcii()
	// Полосу заголовка рисуем сами (§8.3). Нативная рамка поверх своей полосы
	// это две полосы, и заметно это только на живом окне.
	if !o.Frameless {
		t.Error("окно с нативной рамкой: полоса заголовка нарисуется дважды")
	}
	// Раскладка §8.3 не влезает в меньшее. Числа тут не вкус, а нижняя граница.
	if o.Width < 900 || o.Height < 600 {
		t.Errorf("окно %dx%d меньше раскладки §8.3", o.Width, o.Height)
	}
	if o.MinWidth < 900 || o.MinHeight < 600 {
		t.Errorf("окно можно сжать до %dx%d, раскладка развалится", o.MinWidth, o.MinHeight)
	}
}

// Иконка окна. Владелец 04.09.2026: в превью панели задач у Affory голая
// стандартная иконка, хотя на самой панели наша.
//
// Причина в библиотеке: Wails ищет иконку окна по ресурсному ID 3
// (webview_window_windows.go, NewIconFromResource(..., 3)), а go-winres кладёт
// нашу группу под ИМЕНЕМ "APP". Не найдя её, Wails берёт application.Options.Icon,
// а мы его не задавали, и окно остаётся с иконкой класса, то есть с той самой
// голой. Панель задач при этом показывает нашу, потому что читает ресурсы exe,
// а не окно. Поэтому тест смотрит на опции ПРИЛОЖЕНИЯ, а не окна.
func TestIkonkaPrilozheniyaZadanaNastoyashchimPNG(t *testing.T) {
	o := prilozhenieOpcii()
	if len(o.Icon) == 0 {
		t.Fatal("Options.Icon пуст: окно возьмёт иконку класса, в превью будет голая")
	}
	// Байты, а не путь: Wails делает HICON из содержимого, и пустой или
	// битый PNG отличается от отсутствующего только тем, что молчит.
	png := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	if len(o.Icon) < len(png) || string(o.Icon[:len(png)]) != string(png) {
		t.Error("Options.Icon не PNG: Wails не сделает из него иконку")
	}
}
