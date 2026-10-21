package zhurnaly

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"

	"golang.org/x/sys/windows"
)

// Хвосты журналов для выгрузки диагностики. Живут здесь, а не в службе, с
// 1.10.0: окно читает журналы службы само, когда служба не отвечает, а
// выгрузка без службы нужна ровно тогда, когда служба и сломалась.

// DopisatRazdel дописывает в выгрузку раздел «===== imya» с хвостом файла.
// Отказ чтения это строка в разделе, а не отказ выгрузки: «файла нет» тоже
// сведения.
func DopisatRazdel(b *bytes.Buffer, imya, put string, predel int64) {
	fmt.Fprintf(b, "\n===== %s\n", imya)
	hvost, obrezan, err := HvostSPovorotom(put, predel)
	switch {
	case errors.Is(err, os.ErrNotExist):
		b.WriteString("(файла нет)\n")
		return
	case err != nil:
		fmt.Fprintf(b, "(не прочитан: %v)\n", err)
		return
	}
	if obrezan {
		b.WriteString("(начало обрезано)\n")
	}
	b.Write(hvost)
	if len(hvost) > 0 && hvost[len(hvost)-1] != '\n' {
		b.WriteByte('\n')
	}
}

// HvostSPovorotom отдаёт последние predel байт журнала. Сразу после поворота
// текущий файл короткий, а события перед поворотом лежат в .1: недостающее
// добирается оттуда. Хвост начинается с начала строки.
func HvostSPovorotom(put string, predel int64) ([]byte, bool, error) {
	tekushchiy, obrezanTekushchiy, err := hvostFayla(put, predel)
	if err != nil {
		return nil, false, err
	}
	if obrezanTekushchiy || int64(len(tekushchiy)) >= predel {
		return tekushchiy, true, nil
	}
	proshlyy, obrezanProshlyy, err := hvostFayla(put+".1", predel-int64(len(tekushchiy)))
	if errors.Is(err, os.ErrNotExist) {
		return tekushchiy, false, nil
	}
	if err != nil {
		// Прошлый файл не прочитался: текущий всё равно ценен.
		return tekushchiy, true, nil
	}
	return append(proshlyy, tekushchiy...), obrezanProshlyy, nil
}

// hvostFayla читает конец файла. Файл открыт на чтение с общим доступом на
// удаление: журнал в это время могут повернуть, и переименование не должно
// упереться в наш дескриптор.
func hvostFayla(put string, predel int64) ([]byte, bool, error) {
	f, err := OtkrytObshchim(put)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, false, err
	}
	nachalo := max(st.Size()-predel, 0)
	if _, err := f.Seek(nachalo, io.SeekStart); err != nil {
		return nil, false, err
	}
	dannye, err := io.ReadAll(io.LimitReader(f, predel))
	if err != nil {
		return nil, false, err
	}
	if nachalo == 0 {
		return dannye, false, nil
	}
	// Начали с середины: обрывок строки (и, возможно, символа) отбрасывается.
	if i := bytes.IndexByte(dannye, '\n'); i >= 0 {
		dannye = dannye[i+1:]
	} else {
		dannye = nil
	}
	return dannye, true, nil
}

// OtkrytObshchim открывает файл на чтение, не мешая писателю повернуть его.
func OtkrytObshchim(put string) (*os.File, error) {
	imya, err := windows.UTF16PtrFromString(put)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(imya, windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND) {
			return nil, fmt.Errorf("%s: %w", put, os.ErrNotExist)
		}
		return nil, fmt.Errorf("%s: %w", put, err)
	}
	return os.NewFile(uintptr(h), put), nil
}
