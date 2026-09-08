package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Selecting a file must not execute it. The file picker is not an adventure game.
func (m *most) VybratPrilozhenie() (string, error) {
	if m.app == nil {
		return "", fmt.Errorf("окно выбора приложения недоступно")
	}
	dialog := m.app.Dialog.OpenFile().
		SetTitle("Выберите приложение для правила").
		AddFilter("Приложения Windows (*.exe)", "*.exe").
		CanChooseFiles(true).
		CanChooseDirectories(false)
	if m.okno != nil {
		dialog.AttachToWindow(m.okno)
	}
	path, err := dialog.PromptForSingleSelection()
	return prilozhenieIzDialoga(path, err)
}

func prilozhenieIzDialoga(path string, dialogErr error) (string, error) {
	path, err := otvetDialoga(path, dialogErr)
	if err != nil || path == "" {
		return path, err
	}
	if !filepath.IsAbs(path) || !strings.EqualFold(filepath.Ext(path), ".exe") {
		return "", fmt.Errorf("выберите файл приложения с расширением .exe")
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("выбранный файл недоступен: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("выберите файл приложения, а не папку")
	}
	return filepath.Clean(path), nil
}
