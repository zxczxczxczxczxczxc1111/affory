package set

import (
	"errors"
	"fmt"
	"strings"

	"golang.org/x/sys/windows"
)

// ErrNeFayl: путь процесса обязан вести к файлу. Каталог, устройство и пустая
// строка это правило, которое не совпадёт ни с чем и будет выглядеть рабочим.
var ErrNeFayl = errors.New("путь процесса не ведёт к файлу")

// NormalizovatPut приводит путь к тому виду, в котором ядро видит путь
// запущенного процесса, и заодно проверяет, что файл существует.
//
// process_path is compared as a plain string by the core. `D:\` against `d:\`,
// an 8.3 short name, forward slashes, a mapped drive: all different strings,
// all miss the rule. The only spelling that is guaranteed to match is the one
// the kernel itself reports, so the file is opened and asked. Since 1.9 the
// core reports Win32 paths (drive letter, not HarddiskVolume), which is what
// VOLUME_NAME_DOS gives back, minus the \\?\ prefix.
func NormalizovatPut(put string) (string, error) {
	if strings.TrimSpace(put) == "" {
		return "", ErrNeFayl
	}
	imya, err := windows.UTF16PtrFromString(put)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrNeFayl, err)
	}
	// No access rights at all: opening for metadata does not need to read the
	// executable and must not fail on a file the user may not read. No
	// FILE_FLAG_BACKUP_SEMANTICS on purpose: without it a directory does not
	// open, which is the check we want.
	h, err := windows.CreateFile(imya, 0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrNeFayl, err)
	}
	defer windows.CloseHandle(h)

	bufer := make([]uint16, windows.MAX_LONG_PATH)
	// Flags 0 = FILE_NAME_NORMALIZED | VOLUME_NAME_DOS.
	n, err := windows.GetFinalPathNameByHandle(h, &bufer[0], uint32(len(bufer)), 0)
	if err != nil {
		return "", fmt.Errorf("окончательный путь не получен: %w", err)
	}
	if n >= uint32(len(bufer)) {
		return "", fmt.Errorf("окончательный путь длиннее %d", len(bufer))
	}
	return bezPrefiksa(windows.UTF16ToString(bufer[:n])), nil
}

// bezPrefiksa снимает \\?\ и \\?\UNC\: ядро отдаёт путь процесса без них, а
// строка сравнивается целиком.
func bezPrefiksa(p string) string {
	switch {
	case strings.HasPrefix(p, `\\?\UNC\`):
		return `\\` + strings.TrimPrefix(p, `\\?\UNC\`)
	case strings.HasPrefix(p, `\\?\`):
		return strings.TrimPrefix(p, `\\?\`)
	}
	return p
}
