//go:build !windows

package sboi

// Клиент живёт только под Windows, но пакет собирается и на других системах:
// там ошибки занятого файла и полного диска приходят общими кодами os.
func korenSistemy(error) string { return "" }
