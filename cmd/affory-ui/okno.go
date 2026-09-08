package main

import "github.com/wailsapp/wails/v3/pkg/application"

// oknoOpcii builds the window options in a plain function so a test can judge
// them without opening a window. The window manager gets numbers, not a séance.
func oknoOpcii() application.WebviewWindowOptions {
	return application.WebviewWindowOptions{
		Name:  "glavnoe",
		Title: "Affory",
		// Frameless because the title bar is ours (§8.3). A native frame on
		// top of a drawn one gives two bars, and only a live window shows it.
		Frameless: true,
		Width:     1180,
		Height:    860,
		MinWidth:  900,
		MinHeight: 600,
		// Black before the first paint, so the webview does not flash white
		// on a dark theme. The token file repeats the value; this one runs
		// earlier than any CSS ever loads.
		BackgroundColour: application.NewRGB(0, 0, 0),
		URL:              "/",
		Windows: application.WindowsWindow{
			// Keep the OS shadow and resize edges even without a frame.
			DisableFramelessWindowDecorations: false,
		},
	}
}

// WorkArea is in logical pixels already; multiplying DPI here would breed giants.
func vmestitOkno(o application.WebviewWindowOptions, area application.Rect) application.WebviewWindowOptions {
	if area.Width <= 0 || area.Height <= 0 {
		return o
	}
	width, height := max(1, area.Width-24), max(1, area.Height-24)
	o.Width, o.Height = min(o.Width, width), min(o.Height, height)
	o.MinWidth, o.MinHeight = min(o.MinWidth, o.Width), min(o.MinHeight, o.Height)
	return o
}
