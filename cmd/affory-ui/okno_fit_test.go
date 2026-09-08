package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"
	"testing"
)

func TestOknoFitsLogicalWorkArea(t *testing.T) {
	// The taskbar is not a storage location for missing controls.
	for _, area := range []application.Rect{{Width: 1920, Height: 1040}, {Width: 1097, Height: 617}, {Width: 800, Height: 560}} {
		got := vmestitOkno(oknoOpcii(), area)
		if got.Width > area.Width-24 || got.Height > area.Height-24 {
			t.Fatalf("window exceeds work area: %+v", area)
		}
		if got.MinWidth > got.Width || got.MinHeight > got.Height {
			t.Fatal("minimum size would undo screen fitting")
		}
	}
	large := vmestitOkno(oknoOpcii(), application.Rect{Width: 1920, Height: 1040})
	if large.Width != 1180 || large.Height != 860 {
		t.Fatal("roomy desktop lost its preferred window size")
	}
}
func TestOknoUnknownWorkAreaKeepsDefault(t *testing.T) {
	got := vmestitOkno(oknoOpcii(), application.Rect{})
	if got.Width != 1180 || got.Height != 860 {
		t.Fatal("missing display data shrank the window")
	}
}
