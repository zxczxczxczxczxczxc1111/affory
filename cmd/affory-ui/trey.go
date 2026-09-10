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

	// Швы ради теста: живой трей требует запущенного приложения Wails, и без
	// них ни одно из требований ниже не проверить прогоном.
	naGlavnom func(func())
	risovat   func(vidTreya)

	// Три поля ниже трогаются ТОЛЬКО на главном потоке, поэтому без мьютекса, и
	// это не экономия, а условие правильности. Проверять «открыто ли меню» из
	// чужой горутины бессмысленно: между проверкой и доставкой вызова на
	// главный поток меню успевает открыться. Решение принимается там же, где
	// исполняется.
	narisovano   *vidTreya
	otlozhennyy  *vidTreya
	menyuOtkryto bool
}

// vidTreya это ВСЁ, что видно в трее, одним сравнимым значением.
//
// Сравнимым намеренно: пока состояние не изменилось, трогать трей нельзя, а
// «не изменилось» должно решаться одним ==, а не сверкой шести полей руками.
type vidTreya struct {
	ikonka        string
	podskazka     string
	sostoyanie    string
	deystvie      string
	deystvieZhivo bool
	vyhod         string
}

// vidDlya это чистая функция состояния. Ни одного вызова в трей, ровно чтобы
// решение «что должно быть на экране» проверялось отдельно от рисования.
func vidDlya(st protokol.StatusOtvet) vidTreya {
	podpis := podpisTreya(st.Sostoyanie)
	deystvie, _ := deystvieTreya(st.Sostoyanie)
	v := vidTreya{
		ikonka:        ikonkaSostoyaniya(st.Sostoyanie),
		podskazka:     "Affory: " + podpis,
		sostoyanie:    podpis,
		deystvie:      deystvie,
		deystvieZhivo: deystvie != "",
	}
	if !v.deystvieZhivo {
		// Переходное состояние: действия нет, но строка обязана остаться на
		// месте, иначе меню прыгает высотой на каждом подъёме.
		v.deystvie = "..."
	}
	v.vyhod, _ = punktVyhoda(st.KillSwitch)
	return v
}

func novyyTrey(app *application.App, okno *application.WebviewWindow, zvat func(komanda string),
	snyatRezhim func() error) *Trey {
	t := &Trey{app: app, okno: okno, zvat: zvat, snyatRezhim: snyatRezhim,
		sost: protokol.SostSluzhbaMolchit}
	// InvokeAsync, а не InvokeSync: ждать главного потока фоновой горутине
	// незачем, а при открытом меню ожидание длилось бы столько, сколько человек
	// держит меню на экране.
	t.naGlavnom = application.InvokeAsync
	t.risovat = t.risovatZhivo
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
	// Обрамление показа меню, а не голый OpenMenu. Обработчик Wails зовёт
	// прямо из wndProc, то есть уже на главном потоке, а OpenMenu блокирует до
	// закрытия меню: TrackPopupMenuEx возвращается, когда человек выбрал или
	// передумал. Значит между двумя строками ниже меню и правда на экране, и
	// пересобирать его в это время нельзя.
	t.sistemnyy.OnRightClick(func() {
		t.MenyuOtkrylos()
		t.sistemnyy.OpenMenu()
		t.MenyuZakrylos()
	})
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

// Obnovit перерисовывает трей под состояние. Зовётся из любой горутины.
//
// Само рисование уезжает на главный поток целиком, а не по вызову: SetLabel и
// SetEnabled в Wails v3.0.0-beta.16 идут БЕЗ InvokeSync, то есть правят живой
// HMENU из вызвавшей горутины. Раньше это сходило с рук, потому что рядом стоял
// вызов пострашнее.
func (t *Trey) Obnovit(st protokol.StatusOtvet) {
	v := vidDlya(st)
	t.mu.Lock()
	t.sost = st.Sostoyanie
	t.zamok = st.KillSwitch
	t.mu.Unlock()
	t.naGlavnom(func() { t.narisovatNaGlavnom(v) })
}

// narisovatNaGlavnom зовётся ТОЛЬКО на главном потоке, и на этом держится всё
// остальное.
//
// Два отказа рисовать, оба обязательные:
//
// Первый: состояние не изменилось. Экран спрашивает status раз в пять секунд, и
// до 10.09.2026 каждый ответ приводил к SetMenu. Пересобирать меню, в котором не
// поменялась ни буква, значит платить полную цену пересборки за ничто.
//
// Второй: меню открыто. SetMenu в Wails это DestroyMenu плюс постройка заново
// (windowsSystemTray.updateMenu), а открытое меню трея это TrackPopupMenuEx со
// своим модальным циклом сообщений, который наш вызов честно подхватит и
// исполнит. Меню уничтожается под курсором, и человек видит белый прямоугольник
// вместо списка. Отложенное применяется после закрытия.
func (t *Trey) narisovatNaGlavnom(v vidTreya) {
	if t.narisovano != nil && *t.narisovano == v {
		return
	}
	if t.menyuOtkryto {
		// Копится ПОСЛЕДНЕЕ, а не очередь: промежуточных состояний за время,
		// пока меню открыто, уже никто не увидит, а лишняя пересборка стоит
		// ровно столько же, сколько нужная.
		otlozhit := v
		t.otlozhennyy = &otlozhit
		return
	}
	t.primenit(v)
}

func (t *Trey) primenit(v vidTreya) {
	t.otlozhennyy = nil
	primeneno := v
	t.narisovano = &primeneno
	t.risovat(v)
}

// MenyuOtkrylos и MenyuZakrylos обрамляют показ меню. Оба зовутся с главного
// потока: обработчик правого клика Wails вызывает прямо из wndProc, а
// OpenMenu блокирует, пока меню на экране.
func (t *Trey) MenyuOtkrylos() { t.menyuOtkryto = true }

func (t *Trey) MenyuZakrylos() {
	t.menyuOtkryto = false
	if t.otlozhennyy != nil {
		t.primenit(*t.otlozhennyy)
	}
}

// risovatZhivo это единственное место, которое говорит с системным треем.
//
// Уже на главном потоке, поэтому InvokeSync внутри SetIcon, SetTooltip и
// SetMenu не идёт через PostMessage вовсе: он видит свой поток и зовёт напрямую.
// Именно этого и не хватало, пока рисование шло из фоновой горутины.
//
// Menu.Update() здесь НЕТ намеренно. Для трея он бесполезен целиком: строит
// второй HMENU через windowsMenu, которого трей не показывает никогда. Трей
// обновляется исключительно через SetMenu, и прежний комментарий про это в
// соседней строке был прав ровно наполовину.
func (t *Trey) risovatZhivo(v vidTreya) {
	t.sistemnyy.SetIcon(ikonki[v.ikonka])
	t.sistemnyy.SetTooltip(v.podskazka)
	t.punktSostoyaniya.SetLabel(v.sostoyanie)
	t.punktDeystviya.SetLabel(v.deystvie).SetEnabled(v.deystvieZhivo)
	t.punktVyhoda.SetLabel(v.vyhod)
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
