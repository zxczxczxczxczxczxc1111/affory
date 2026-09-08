package main

import (
	_ "embed"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Task 4.8, spec §8.4. The icon answers "alive or not", the text answers
// "what exactly": connected vs connecting differ by a warm tint that is the
// first channel to go on a coloured taskbar, so the tooltip and the first
// menu line carry the state in words. Four icons for seven states.

//go:embed ikonki/connected_32.png
var ikonkaConnected []byte

//go:embed ikonki/connecting_32.png
var ikonkaConnecting []byte

//go:embed ikonki/disconnected_32.png
var ikonkaDisconnected []byte

//go:embed ikonki/error_32.png
var ikonkaError []byte

var ikonki = map[string][]byte{
	"connected":    ikonkaConnected,
	"connecting":   ikonkaConnecting,
	"disconnected": ikonkaDisconnected,
	"error":        ikonkaError,
}

func ikonkaSostoyaniya(s protokol.Sostoyanie) string {
	switch s {
	case protokol.SostPodnyat:
		return "connected"
	case protokol.SostPodnimaetsya, protokol.SostVosstanavl:
		return "connecting"
	case protokol.SostVyklyuchen:
		return "disconnected"
	default:
		// otkaz, ne-neset, and a silent service: something the human should
		// look at, which is what the red tint is for.
		return "error"
	}
}

// Same words as podpisi.ts; TestPodpisiTreyaSovpadayutSEkranom keeps them so.
var podpisiTreya = map[protokol.Sostoyanie]string{
	protokol.SostSluzhbaMolchit: "служба не отвечает",
	protokol.SostVyklyuchen:     "выключено",
	protokol.SostPodnimaetsya:   "подключается",
	protokol.SostPodnyat:        "подключено",
	protokol.SostNeNeset:        "подключено, трафик не идёт",
	protokol.SostVosstanavl:     "восстанавливается",
	protokol.SostOtkaz:          "отказ",
}

func podpisTreya(s protokol.Sostoyanie) string {
	if p, est := podpisiTreya[s]; est {
		return p
	}
	return string(s)
}

// deystvieTreya is the one action the tray menu offers per state, mirroring
// glavnoeDeystvie in podpisi.ts. Transient states get none.
func deystvieTreya(s protokol.Sostoyanie) (podpis, komanda string) {
	switch s {
	case protokol.SostVyklyuchen:
		return "Подключить", "connect"
	case protokol.SostPodnyat, protokol.SostNeNeset:
		return "Отключить", "disconnect"
	case protokol.SostOtkaz:
		return "Повторить", "connect"
	default:
		return "", ""
	}
}

// punktVyhoda отвечает, что написать в последней строке меню и что сделать по
// нажатию.
//
// Крестик окна прячет программу в трей, значит «Выход» это уже не «свернуть», а
// «я закончил». Закончить при включённом режиме «весь трафик» нельзя: замок
// остаётся на машине. Ф1 от 05.09.2026.
//
// Подпись обязана называть снятие защиты. Строка «Выход», снимающая режим молча,
// хуже отсутствия двери: человек нажал одно, а получил другое.
func punktVyhoda(zamok bool) (podpis string, snyatRezhim bool) {
	if zamok {
		return "Выйти и снять защиту", true
	}
	return "Выход", false
}

// Trey owns the system tray icon and the "hide instead of close" contract:
// the window is a view, the tray is the program (spec, "Автозапуск").
type Trey struct {
	app       *application.App
	okno      *application.WebviewWindow
	sistemnyy *application.SystemTray
	menu      *application.Menu
	// zvat runs a service command in the background; the tray never waits
	// for an answer, the answer arrives as a state event like any other.
	zvat func(komanda string)
	// snyatRezhim ЖДЁТ ответа, в отличие от zvat: уходить, не дождавшись
	// снятия замка, значит уходить с запертой машины.
	snyatRezhim func() error

	mu               sync.Mutex
	sost             protokol.Sostoyanie
	zamok            bool
	punktSostoyaniya *application.MenuItem
	punktDeystviya   *application.MenuItem
	punktVyhoda      *application.MenuItem
}

func novyyTrey(app *application.App, okno *application.WebviewWindow, zvat func(komanda string),
	snyatRezhim func() error) *Trey {
	t := &Trey{app: app, okno: okno, zvat: zvat, snyatRezhim: snyatRezhim,
		sost: protokol.SostSluzhbaMolchit}
	t.sistemnyy = app.SystemTray.New()
	t.menu = app.NewMenu()
	// First line is the state, disabled on purpose: it is a label, not a
	// button, and a menu whose first row does nothing on click is the
	// convention every tray on Windows already taught the human.
	t.punktSostoyaniya = t.menu.Add(podpisTreya(t.sost))
	t.punktSostoyaniya.SetEnabled(false)
	t.menu.AddSeparator()
	t.menu.Add("Открыть окно").OnClick(func(*application.Context) { t.Pokazat() })
	t.punktDeystviya = t.menu.Add("Подключить")
	t.punktDeystviya.OnClick(func(*application.Context) {
		t.mu.Lock()
		_, komanda := deystvieTreya(t.sost)
		t.mu.Unlock()
		if komanda != "" {
			go t.zvat(komanda)
		}
	})
	t.menu.AddSeparator()
	t.punktVyhoda = t.menu.Add("Выход")
	t.punktVyhoda.OnClick(func(*application.Context) { t.vyyti() })
	t.sistemnyy.SetMenu(t.menu)
	t.sistemnyy.OnClick(t.Pokazat)
	t.sistemnyy.OnRightClick(t.sistemnyy.OpenMenu)
	t.Obnovit(protokol.StatusOtvet{Sostoyanie: t.sost})
	return t
}

// vyyti снимает режим, если он включён, и только потом закрывает программу.
//
// Неудача снятия ОТМЕНЯЕТ выход и открывает окно. Уйти молча значило бы
// оставить человека ровно в том положении, из-за которого это и чинилось:
// машина заперта, программы нет, объяснения нет. Окно покажет и режим, и
// причину отказа: экраны для них уже написаны.
func (t *Trey) vyyti() {
	t.mu.Lock()
	zamok := t.zamok
	t.mu.Unlock()
	_, snyat := punktVyhoda(zamok)
	if snyat {
		if t.snyatRezhim == nil {
			t.Pokazat()
			return
		}
		// В отдельной горутине: обработчик меню крутится на главном потоке, а
		// команда ходит по каналу и ждёт ответа.
		go func() {
			if err := t.snyatRezhim(); err != nil {
				t.Pokazat()
				return
			}
			t.app.Quit()
		}()
		return
	}
	t.app.Quit()
}

// Pokazat brings the window up from the tray.
func (t *Trey) Pokazat() {
	t.okno.Show()
	// Wails v3.0.0-beta.16: Show() on a window created Hidden and never run
	// only RUNS it (still hidden) and returns; the actual show is the second
	// call. Seen live on 02.09.2026: tray click, menu item, nothing on screen.
	// IsVisible is false in exactly that case, and a second Show is harmless
	// for a window that is already up.
	if !t.okno.IsVisible() {
		t.okno.Show()
	}
	t.okno.Focus()
}

// Obnovit repaints icon, tooltip and menu for a state. Safe from any
// goroutine: the tray methods marshal onto the main thread themselves.
func (t *Trey) Obnovit(st protokol.StatusOtvet) {
	s := st.Sostoyanie
	t.mu.Lock()
	t.sost = s
	t.zamok = st.KillSwitch
	t.mu.Unlock()
	t.sistemnyy.SetIcon(ikonki[ikonkaSostoyaniya(s)])
	t.sistemnyy.SetTooltip("Affory: " + podpisTreya(s))
	t.punktSostoyaniya.SetLabel(podpisTreya(s))
	podpis, _ := deystvieTreya(s)
	if podpis == "" {
		t.punktDeystviya.SetLabel("...").SetEnabled(false)
	} else {
		t.punktDeystviya.SetLabel(podpis).SetEnabled(true)
	}
	podpisVyhoda, _ := punktVyhoda(st.KillSwitch)
	t.punktVyhoda.SetLabel(podpisVyhoda)
	t.menu.Update()
	// Wails v3.0.0-beta.16, Windows: Update() refreshes the Menu object, but
	// the tray keeps the popup it built at SetMenu time, so the human saw
	// "выключено / Подключить" over a raised tunnel (owner, 03.09.2026).
	// Handing the same menu back to the tray rebuilds the popup.
	t.sistemnyy.SetMenu(t.menu)
}

// Nablyudat keeps the tray truthful while no window is open: the bridge only
// dials the service when somebody calls it, and at logon nobody does. Ask
// for status once, then again every few seconds while the pipe is down.
// While the pipe is up, state events repaint the tray and this loop idles.
func (t *Trey) Nablyudat(m *most) {
	// A beat before the first ask: the tray registers with the shell inside
	// app.Run, and a status answer that lands before NIM_ADD is repainted
	// onto an icon the shell has not seen yet (a warning, not a loss, the
	// next event repaints; seen live on 02.09.2026).
	time.Sleep(time.Second)
	for {
		if !m.podklyuchen() {
			go m.zvatFonovo("status")
		}
		time.Sleep(5 * time.Second)
	}
}
