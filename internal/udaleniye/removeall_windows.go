package udaleniye

import (
	"errors"
	"os"
	"syscall"
)

// Katalog удаляет дерево без перехода по ссылкам. В Go 1.27 относительное
// открытие дочернего каталога AppData может возвращать ложный ELOOP.
// Обход открывает сам каталог обычным путём, а потом удаляет внутри os.Root.
func Katalog(path string) error {
	err := os.RemoveAll(path)
	if !errors.Is(err, syscall.ELOOP) {
		return err
	}
	return cherezKoren(path)
}

func cherezKoren(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
		return os.Remove(path)
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return err
	}
	defer root.Close()
	opened, err := root.Stat(".")
	if err != nil {
		return err
	}
	// При подмене каталога между Lstat и OpenRoot чужое дерево не трогаем.
	if !os.SameFile(info, opened) {
		return &os.PathError{Op: "removeall", Path: path, Err: syscall.EAGAIN}
	}
	dir, err := root.Open(".")
	if err != nil {
		return err
	}
	names, readErr := dir.Readdirnames(-1)
	closeErr := dir.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		return err
	}
	var failures []error
	for _, name := range names {
		if err := root.RemoveAll(name); err != nil {
			failures = append(failures, err)
		}
	}
	if err := errors.Join(failures...); err != nil {
		return err
	}
	if err := root.Close(); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
