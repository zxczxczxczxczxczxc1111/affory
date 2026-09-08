package main

import (
	"embed"
	"log"
	"os"
	"slices"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// flagTrey is what the Run key passes at logon: the program comes up in the
// tray and the window stays closed (spec, "Автозапуск"). Spelled here and in
// affory-svc/avtozapusk.go; the two binaries share no package.
const flagTrey = "--trey"

// The embed directive is the one line a rewrite of this file loses first.
// The build still passes, the window opens empty, and nothing says why.
//
//go:embed all:frontend/dist
var assets embed.FS

// Иконка ОКНА, отдельно от иконки exe и от четырёх иконок трея.
//
// Wails ищет иконку окна по ресурсному ID 3, а go-winres кладёт нашу группу
// под именем "APP", поэтому поиск не находит ничего и окно остаётся с иконкой
// класса: в превью панели задач она голая стандартная. Эти байты и есть второй
// путь, который библиотека пробует следом.
//
//go:embed ikonki/sfera_256.png
var ikonkaOkna []byte

// prilozhenieOpcii собраны отдельно от main ровно чтобы их можно было
// проверить тестом: в main они были бы литералом внутри вызова.
func prilozhenieOpcii(sluzhby ...application.Service) application.Options {
	return application.Options{
		Name:        "Affory",
		Description: "Клиент VPN",
		Icon:        ikonkaOkna,
		Services:    sluzhby,
		Assets: application.AssetOptions{
			// AssetFileServerFS, not the FS itself: Assets is an AssetOptions
			// with an http.Handler inside, and the compiler will not tell you
			// which of the two you meant.
			Handler: application.AssetFileServerFS(assets),
		},
	}
}

func main() {
	m := &most{}
	uvedomleniya := notifications.New()
	// Toast-уведомления Windows: служба Wails при старте регистрирует
	// COM-активацию в HKCU (Software\Classes\CLSID\<guid>), это её цена.
	app := application.New(prilozhenieOpcii(
		application.NewService(m),
		application.NewService(uvedomleniya),
	))
	// The service needs the app to emit events; the app needs the service to
	// exist. Wire it after both are alive rather than pretend there is no cycle.
	m.app = app

	opcii := oknoOpcii()
	vTrey := slices.Contains(os.Args[1:], flagTrey)
	opcii.Hidden = vTrey
	okno := app.Window.NewWithOptions(opcii)
	m.okno = okno
	// Fit once after Wails has a real monitor. Reloads must not wrestle the user.
	var fitOnce sync.Once
	okno.OnWindowEvent(events.Common.WindowRuntimeReady, func(_ *application.WindowEvent) {
		fitOnce.Do(func() {
			screen, err := okno.GetScreen()
			if err != nil {
				log.Printf("window work area: %v", err)
				return
			}
			if screen == nil {
				return
			}
			fitted := vmestitOkno(opcii, screen.WorkArea)
			if fitted.Width == opcii.Width && fitted.Height == opcii.Height {
				return
			}
			okno.SetMinSize(fitted.MinWidth, fitted.MinHeight)
			okno.SetSize(fitted.Width, fitted.Height)
			okno.Center()
		})
	})
	// Closing the window hides it: the tray is the program, the window is a
	// view of it. Quitting is the tray menu's last line, nothing else.
	// Маленькая иконка ставится ПОСЛЕ появления окна: до этого HWND не
	// существует. Ожиданием, а не хуком: WindowShow при первом показе не
	// приходит вовсе.
	PostavitMaluyuIkonkuKogdaOkno(ikonkaOkna)
	okno.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		okno.Hide()
		e.Cancel()
	})

	// Снятие режима из трея ходит той же командой, что и переключатель на
	// экране настроек, и ЖДЁТ ответа: выход, начатый до снятия замка, оставил бы
	// машину запертой без единой программы, которой это чинить.
	trey := novyyTrey(app, okno, m.zvatFonovo, m.SnyatRezhim)
	uved := novyyUvedomlyatel(uvedomleniya)
	m.naStatus = func(st protokol.StatusOtvet) {
		trey.Obnovit(st)
		uved.Prinyat(st)
	}
	go trey.Nablyudat(m)

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
