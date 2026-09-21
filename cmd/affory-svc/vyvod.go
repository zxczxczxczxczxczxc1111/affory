package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/sostoyanie"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/zhurnaly"
)

// nastroitVyvodPodkomandy направляет вывод подкоманды туда, где его прочитают:
// в трубу установщика и в журнал установки.
//
// Журнал нужен потому, что труба живёт ровно столько, сколько открыто окно
// установщика. Человек, нажавший «ОК» на отказе, до 21.09.2026 не мог назвать
// причину вообще ничем: сообщение звало его в sluzhba.log, а тот заводится
// только при старте службы от SCM, то есть при неудачной установке его нет.
//
// Возвращается поток для обычной печати; жалобы log уходят в оба места.
func nastroitVyvodPodkomandy() io.Writer {
	oshibki := io.Writer(vKodirovkeMashiny{os.Stderr})
	// Каталог журналов лежит внутри каталога данных, и права ему ставит ровно
	// один вызов. Завести каталог здесь, а не просто открыть в нём файл: иначе
	// первая же установка на чистой машине создала бы его наследованием от
	// C:\ProgramData, то есть с правом записи для всех.
	//
	// Вызов идемпотентен и повторяется внутри установки: он не столько создаёт
	// каталог, сколько приводит его права к нужным.
	if err := sostoyanie.ZavestiKatalogDannyh(); err == nil {
		if zh, err := zhurnaly.Otkryt(sostoyanie.KatalogZhurnalov(), "ustanovka.log"); err == nil {
			oshibki = io.MultiWriter(oshibki, zh)
		}
	}
	log.SetOutput(oshibki)
	return vKodirovkeMashiny{os.Stdout}
}

// imyaFaylaPrichiny это файл, из которого установщик берёт текст для окна
// отказа. Лежит рядом со своим бинарём: при установке это каталог установки,
// при подготовке — временный каталог установщика, и оба пути установщику
// известны.
//
// Файл нужен потому, что трубы мало. Установщик декодирует её кодовой
// страницей ANSI машины, и на нерусской Windows (у этой машины 1252) кириллица
// превращается в вопросы ещё до того, как её кто-то прочтёт. Здесь кодировка
// UTF-16LE, родная для Windows: она знает наш текст целиком независимо от
// локали, а NSIS читает её FileReadUTF16LE.
//
// BOM нарочно нет: FileReadUTF16LE отдал бы его первым символом строки.
const imyaFaylaPrichiny = "ustanovka-prichina.txt"

// upast говорит причину всем, кто способен её показать, и уходит с кодом 1.
//
// Заменяет log.Fatalf в подкомандах: тот печатал причину в трубу, и она
// пропадала вместе с окном установщика. Человеку оставался код 1, который не
// отличает нехватку прав от занятого файла.
func upast(format string, a ...any) {
	tekst := fmt.Sprintf(format, a...)
	log.Print(tekst)
	polozhitPrichinu(tekst)
	os.Exit(1)
}

// polozhitPrichinu пишет текст рядом со своим бинарём.
//
// Отказ молчаливый: причина уже названа в трубу и в журнал, и жаловаться на
// то, что не удалось пожаловаться, значит подменить настоящую причину своей.
func polozhitPrichinu(tekst string) {
	put, err := os.Executable()
	if err != nil {
		return
	}
	shiroko := utf16.Encode([]rune(tekst))
	bayty := make([]byte, 0, len(shiroko)*2)
	for _, w := range shiroko {
		bayty = append(bayty, byte(w), byte(w>>8))
	}
	_ = os.WriteFile(filepath.Join(filepath.Dir(put), imyaFaylaPrichiny), bayty, 0o600)
}

// Вывод подкоманд читает не человек напрямую, а установщик: NSIS забирает его
// из трубы и декодирует кодовой страницей ANSI своей машины. Go пишет UTF-8,
// и получается то, что человек увидел 21.09.2026 вместо причины отказа:
//
//	СѓСЃС‚Р°РЅРѕРІРєР° РЅРµ СѓРґР°Р»Р°СЃСЊ: РѕС‚РїРµС‡Р°С‚РєРё РЅРµ СЃРЅСЏС‚С‹
//
// Причина была названа полностью и точно, и всё равно потеряна. Переложить
// это на установщик нечем: nsExec умеет ANSI и OEM, но не UTF-8. Поэтому
// перекодируем на своей стороне, в кодовую страницу той машины, где идёт
// установка.
//
// WideCharToMultiByte НЕТ в golang.org/x/sys/windows v0.47.0 (в нём только
// обратная MultiByteToWideChar), объявлено руками — так же, как
// GetExtendedTcpTable в internal/yadra/port.go.
var (
	kernel32                = windows.NewLazySystemDLL("kernel32.dll")
	procWideCharToMultiByte = kernel32.NewProc("WideCharToMultiByte")
)

// cpAcp это CP_ACP: кодовая страница ANSI текущей машины. Не постоянная 1251:
// на русской Windows это она и есть, а на любой другой наш текст всё равно
// должен уехать в то, что установщик собирается читать.
const cpAcp = 0

// vKodirovkeMashiny оборачивает поток так, чтобы наружу уходили байты в
// кодовой странице машины, а не в UTF-8.
type vKodirovkeMashiny struct{ w io.Writer }

func (v vKodirovkeMashiny) Write(b []byte) (int, error) {
	gotovo, err := vAnsi(b)
	if err != nil {
		// Нечитаемая строка полезнее пропавшей: пусть уезжает как есть.
		return v.w.Write(b)
	}
	if _, err := v.w.Write(gotovo); err != nil {
		return 0, err
	}
	// Наружу рапортуется длина ВХОДА, а не выхода. io.Writer обязан вернуть
	// столько, сколько у него взяли: перекодировка почти всегда меняет длину,
	// и честное число байт записи заставило бы log считать её оборванной.
	return len(b), nil
}

var errPustoePerekodirovanie = errors.New("перекодировать нечего")

// vAnsi переводит UTF-8 в кодовую страницу машины.
func vAnsi(b []byte) ([]byte, error) {
	if len(b) == 0 {
		return b, nil
	}
	shiroko := utf16.Encode([]rune(string(b)))
	if len(shiroko) == 0 {
		return nil, errPustoePerekodirovanie
	}
	nuzhno := wideCharToMultiByte(shiroko, nil)
	if nuzhno <= 0 {
		return nil, errors.New("длина в кодовой странице машины не посчиталась")
	}
	gotovo := make([]byte, nuzhno)
	if wideCharToMultiByte(shiroko, gotovo) <= 0 {
		return nil, errors.New("перекодировка в кодовую страницу машины отказала")
	}
	return gotovo, nil
}

// wideCharToMultiByte с пустым приёмником отвечает, сколько байт нужно.
func wideCharToMultiByte(shiroko []uint16, kuda []byte) int {
	var p *byte
	if len(kuda) > 0 {
		p = &kuda[0]
	}
	r, _, _ := procWideCharToMultiByte.Call(
		uintptr(cpAcp),
		0, // без флагов: подстановка знака вопроса для непредставимых нас устраивает
		uintptr(unsafe.Pointer(&shiroko[0])),
		uintptr(len(shiroko)),
		uintptr(unsafe.Pointer(p)),
		uintptr(len(kuda)),
		0, 0,
	)
	return int(r)
}
