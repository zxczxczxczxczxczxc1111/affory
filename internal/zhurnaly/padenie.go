package zhurnaly

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// OtkrytFaylPadeniya готовит файл для debug.SetCrashOutput (1.10.0): туда Go
// пишет стек фатальной ошибки и паники, которую никто не перехватил. Консоли
// у службы и окна нет, и до этого файла такое падение не оставляло ничего.
//
// Файл пишется только при падении, поэтому непустой файл при запуске значит,
// что прошлый запуск упал. Он уезжает в .1 (прежний .1 затирается), и
// вызывающий получает время падения, чтобы назвать его в своём журнале.
// Нулевое время значит «прошлый запуск не падал».
func OtkrytFaylPadeniya(katalog, imya string) (*os.File, time.Time, error) {
	if err := os.MkdirAll(katalog, 0o700); err != nil {
		return nil, time.Time{}, fmt.Errorf("каталог журналов недоступен: %w", err)
	}
	put := filepath.Join(katalog, imya)
	var proshloe time.Time
	st, err := os.Stat(put)
	switch {
	case err == nil && st.Size() > 0:
		proshloe = st.ModTime()
		if err := os.Rename(put, put+".1"); err != nil {
			// Не повернулся: новый стек допишется следом, и это лучше, чем
			// остаться без файла падений вовсе.
			proshloe = time.Time{}
		}
	case err != nil && !errors.Is(err, os.ErrNotExist):
		return nil, time.Time{}, fmt.Errorf("файл падений недоступен: %w", err)
	}
	f, err := os.OpenFile(put, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, proshloe, fmt.Errorf("файл падений не открылся: %w", err)
	}
	return f, proshloe, nil
}
