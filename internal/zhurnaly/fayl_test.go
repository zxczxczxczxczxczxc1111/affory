package zhurnaly

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRotaciyaDerzhitTriFayla(t *testing.T) {
	// Ротация без предела хранения это не ротация, а переименование: диск
	// кончится ровно так же, только файлов будет много. Спека §5 п.6 говорит
	// «по 10 МБ, хранятся три файла», и три это ВСЕГО, вместе с текущим.
	d := t.TempDir()
	z, err := otkrytS(d, "proba.log", 20)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		if _, err := z.Write([]byte(fmt.Sprintf("stroka-%d\n", i))); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	fayly, err := filepath.Glob(filepath.Join(d, "proba.log*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(fayly) != 3 {
		t.Fatalf("файлов %d, ожидалось 3: %v", len(fayly), fayly)
	}
	var vse strings.Builder
	for _, f := range fayly {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		vse.Write(b)
	}
	if strings.Contains(vse.String(), "stroka-0") {
		t.Fatal("самая старая строка не вытеснена: хранится больше трёх файлов")
	}
	if !strings.Contains(vse.String(), "stroka-9") {
		t.Fatal("последняя строка потеряна")
	}
}

func TestZapisPerezhivaetPereotkrytie(t *testing.T) {
	// Служба перезапускается чаще, чем журнал переполняется. Открытие с
	// усечением стёрло бы ровно тот кусок, ради которого в журнал и лезут:
	// последние строки перед падением.
	d := t.TempDir()
	z, err := otkrytS(d, "p.log", 1000)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := z.Write([]byte("pervaya\n")); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	z2, err := otkrytS(d, "p.log", 1000)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := z2.Write([]byte("vtoraya\n")); err != nil {
		t.Fatal(err)
	}
	if err := z2.Close(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(d, "p.log"))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "pervaya\nvtoraya\n" {
		t.Fatalf("в файле %q", string(b))
	}
}

func TestZapisDlinneePredelaNeTeryaetsya(t *testing.T) {
	// Запись длиннее предела на ротацию не годится: провернуть файл ради неё
	// значит выбросить два предыдущих и всё равно превысить предел.
	d := t.TempDir()
	z, err := otkrytS(d, "p.log", 10)
	if err != nil {
		t.Fatal(err)
	}
	dlinnaya := strings.Repeat("x", 50) + "\n"
	for i := 0; i < 3; i++ {
		if _, err := z.Write([]byte(dlinnaya)); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(d, "p.log"))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != dlinnaya {
		t.Fatalf("последняя запись не легла целиком: %q", string(b))
	}
	fayly, err := filepath.Glob(filepath.Join(d, "p.log*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(fayly) > 3 {
		t.Fatalf("файлов %d, больше трёх", len(fayly))
	}
}

func TestBaytyNeMenyayutsya(t *testing.T) {
	// Спека: файлы UTF-8, перевод строки LF. Windows-привычка дописать CR
	// сломала бы разбор журнала теми же средствами, что и на сервере.
	d := t.TempDir()
	z, err := otkrytS(d, "p.log", 1000)
	if err != nil {
		t.Fatal(err)
	}
	obrazec := "строка с кириллицей\nи вторая\n"
	if _, err := z.Write([]byte(obrazec)); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(d, "p.log"))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != obrazec {
		t.Fatalf("байты изменились: %q", string(b))
	}
}

func TestPisatPosleZakrytiyaNePadaet(t *testing.T) {
	// Служба закрывает журнал при остановке, а горутина команды может дописать
	// строку после этого. Паника в этом месте валит остановку службы.
	d := t.TempDir()
	z, err := otkrytS(d, "p.log", 1000)
	if err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := z.Write([]byte("posle\n")); err == nil {
		t.Fatal("запись в закрытый журнал прошла молча")
	}
	if err := z.Close(); err != nil {
		t.Fatalf("повторное закрытие ругается: %v", err)
	}
}
