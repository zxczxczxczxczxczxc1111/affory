package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// These files are deliberately inert. Tests do not need executable wildlife.
func TestPrilozhenieIzDialoga(t *testing.T) {
	dir := t.TempDir()
	app := filepath.Join(dir, "Моё приложение.EXE")
	text := filepath.Join(dir, "notes.txt")
	folder := filepath.Join(dir, "folder.exe")
	for _, path := range []string{app, text} {
		if err := os.WriteFile(path, []byte("fixture, never execute"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(folder, 0700); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("dialog unavailable")
	for _, tc := range []struct {
		name      string
		path      string
		dialogErr error
		want      string
		wantErr   bool
	}{
		{"file with spaces and Cyrillic", app, nil, app, false},
		{"cancel without error", "", nil, "", false},
		{"cancel from Windows", "", errors.New("dialog cancelled by user"), "", false},
		{"dialog failure", "", failure, "", true},
		{"wrong extension", text, nil, "", true},
		{"directory with exe suffix", folder, nil, "", true},
		{"missing file", filepath.Join(dir, "gone.exe"), nil, "", true},
		{"relative path", "app.exe", nil, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := prilozhenieIzDialoga(tc.path, tc.dialogErr)
			if got != tc.want || (err != nil) != tc.wantErr {
				t.Fatalf("got (%q, %v), want (%q, error=%v)", got, err, tc.want, tc.wantErr)
			}
		})
	}
}

func TestVybratPrilozhenieWithoutApp(t *testing.T) {
	path, err := (&most{}).VybratPrilozhenie()
	if path != "" || err == nil {
		t.Fatalf("missing app: got (%q, %v)", path, err)
	}
}
