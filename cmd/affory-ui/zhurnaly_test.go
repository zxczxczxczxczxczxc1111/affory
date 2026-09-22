package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPapkaZhurnalovOtkryvayetsyaBezShellArgumentov(t *testing.T) {
	path := filepath.Join(t.TempDir(), "логи с пробелом")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	called := 0
	if err := otkrytPapkuZhurnalov(path, func(got string) error {
		called++
		if got != path {
			t.Fatalf("wrong folder: %q", got)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if called != 1 {
		t.Fatal("folder was not opened exactly once")
	}
}

func TestPapkaZhurnalovOtdayotOshibkiINeZapuskayetFayl(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "log")
	if err := os.WriteFile(file, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{file, filepath.Join(dir, "missing")} {
		if err := otkrytPapkuZhurnalov(path, func(string) error { t.Fatal("invalid folder passed to shell"); return nil }); err == nil {
			t.Fatal("missing error")
		}
	}
	want := errors.New("shell unavailable")
	if err := otkrytPapkuZhurnalov(dir, func(string) error { return want }); !errors.Is(err, want) {
		t.Fatalf("shell failure hidden: %v", err)
	}
}
