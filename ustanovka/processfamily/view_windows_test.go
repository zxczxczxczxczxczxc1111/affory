package afforyprocess

import (
	"errors"
	"golang.org/x/sys/windows"
	"os"
	"testing"
)

func TestViewsFilterWindowsSession(t *testing.T) {
	w, err := NewWatcher(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	var session uint32
	if err := windows.ProcessIdToSessionId(uint32(os.Getpid()), &session); err != nil {
		t.Fatal(err)
	}
	views, err := w.Views(session)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, v := range views {
		if v.Session != session {
			t.Fatal("another session included")
		}
		found = found || v.PID == uint32(os.Getpid())
	}
	if !found {
		t.Fatal("self missing")
	}
	other, err := w.Views(0xfffffffe)
	if err != nil {
		t.Fatal(err)
	}
	if len(other) != 0 {
		t.Fatal("session filtering ignored")
	}
	w.Close()
	if _, err := w.Views(session); !errors.Is(err, ErrSharedUnavailable) {
		t.Fatalf("stopped tracker returned a snapshot: %v", err)
	}
}
