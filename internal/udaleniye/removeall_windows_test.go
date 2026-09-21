package udaleniye

import (
	"errors"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"testing"
)

func TestKatalogAppData(t *testing.T) {
	for _, env := range []string{"LOCALAPPDATA", "APPDATA"} {
		t.Run(env, func(t *testing.T) {
			parent := os.Getenv(env)
			if parent == "" {
				t.Skipf("%s не задан", env)
			}
			dir, err := os.MkdirTemp(parent, "Affory removeall test ")
			if err != nil {
				t.Fatal(err)
			}
			nested := filepath.Join(dir, "nested")
			file := filepath.Join(nested, "cache")
			// Только собственная фикстура; уборка не зависит от RemoveAll.
			t.Cleanup(func() {
				for _, path := range []string{file, nested, dir} {
					if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
						t.Error(err)
					}
				}
			})
			if err := os.Mkdir(nested, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(file, []byte("cache"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := Katalog(dir); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Lstat(dir); !os.IsNotExist(err) {
				t.Fatalf("каталог остался: %v", err)
			}
			if err := Katalog(dir); err != nil {
				t.Fatalf("повторное удаление: %v", err)
			}
		})
	}
}

func TestObhodNePerehoditPoSsylkam(t *testing.T) {
	for _, atRoot := range []bool{false, true} {
		t.Run(map[bool]string{false: "vnutri", true: "koren"}[atRoot], func(t *testing.T) {
			base := t.TempDir()
			outside := filepath.Join(base, "outside")
			if err := os.Mkdir(outside, 0700); err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(outside, "keep")
			if err := os.WriteFile(file, []byte("keep"), 0600); err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(base, "remove")
			link := dir
			if !atRoot {
				if err := os.Mkdir(dir, 0700); err != nil {
					t.Fatal(err)
				}
				link = filepath.Join(dir, "link")
			}
			if err := os.Symlink(outside, link); err != nil {
				t.Fatal(err)
			}
			if err := cherezKoren(dir); err != nil {
				t.Fatal(err)
			}
			if data, err := os.ReadFile(file); err != nil || string(data) != "keep" {
				t.Fatalf("цель ссылки повреждена: %q, %v", data, err)
			}
			if _, err := os.Lstat(dir); !os.IsNotExist(err) {
				t.Fatalf("каталог остался: %v", err)
			}
		})
	}
}

func TestObhodVozvrashchaetOshibkuZanyatogoFayla(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "locked")
	if err := os.WriteFile(file, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	name, err := windows.UTF16PtrFromString(file)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(handle)
	if err := cherezKoren(dir); !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
		t.Fatalf("ожидался отказ занятого файла, получено: %v", err)
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatal(err)
	}
}
