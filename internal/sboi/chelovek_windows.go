//go:build windows

package sboi

import (
	"errors"

	"golang.org/x/sys/windows"
)

// korenSistemy называет ошибки Windows, которых нет среди общих ошибок os.
// Занятый файл и полный диск человек может починить сам, если ему сказать.
func korenSistemy(err error) string {
	switch {
	case errors.Is(err, windows.ERROR_SHARING_VIOLATION), errors.Is(err, windows.ERROR_LOCK_VIOLATION):
		return "файл занят другой программой"
	case errors.Is(err, windows.ERROR_DISK_FULL), errors.Is(err, windows.ERROR_HANDLE_DISK_FULL):
		return "на диске нет места"
	case errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST), errors.Is(err, windows.ERROR_SERVICE_NOT_ACTIVE):
		return "служба не запущена"
	}
	return ""
}
