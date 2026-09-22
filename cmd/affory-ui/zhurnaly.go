package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/puti"
	"golang.org/x/sys/windows"
)

// Путь выбирает оболочка, а не JavaScript. Проводник запускается в сеансе
// пользователя без командной строки shell и без изменения прав папки.
func (m *most) OtkrytPapkuZhurnalov() error {
	return otkrytPapkuZhurnalov(filepath.Join(puti.KatalogDannyh, puti.PodkatalogZhurnalov), func(put string) error {
		folder, err := windows.UTF16PtrFromString(put)
		if err != nil {
			return err
		}
		verb, err := windows.UTF16PtrFromString("open")
		if err != nil {
			return err
		}
		return windows.ShellExecute(0, verb, folder, nil, nil, windows.SW_SHOWNORMAL)
	})
}

func otkrytPapkuZhurnalov(put string, otkryt func(string) error) error {
	info, err := os.Stat(put)
	if os.IsNotExist(err) {
		return fmt.Errorf("папка журналов ещё не создана: %s", put)
	}
	if err != nil {
		return fmt.Errorf("папка журналов недоступна: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("вместо папки журналов найден файл: %s", put)
	}
	if err := otkryt(put); err != nil {
		return fmt.Errorf("не удалось открыть папку журналов: %w", err)
	}
	return nil
}
