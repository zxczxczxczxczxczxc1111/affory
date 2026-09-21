package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"unicode/utf16"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/kodirovki"
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
// Возвращается поток для обычной печати и закрывалка журнала; жалобы log
// уходят в оба места.
//
// Закрывалка не украшение. Журнал держит файл ВНУТРИ каталога данных, а
// снятие с ключами этот каталог стирает целиком, и Windows не удаляет открытый
// файл: `os.RemoveAll` отвечает «файл используется другим процессом», снятие
// падает на стирании данных, а человек, только что согласившийся стереть
// ключи, видит отказ. Проверено пробой до выкладки 1.4.1.
func nastroitVyvodPodkomandy() (io.Writer, func()) {
	konsol := io.Writer(vKodirovkeMashiny{os.Stderr})
	oshibki := konsol
	zakryt := func() {}
	// Каталог журналов лежит внутри каталога данных, и права ему ставит ровно
	// один вызов. Завести каталог здесь, а не просто открыть в нём файл: иначе
	// первая же установка на чистой машине создала бы его наследованием от
	// C:\ProgramData, то есть с правом записи для всех.
	//
	// Вызов идемпотентен и повторяется внутри установки: он не столько создаёт
	// каталог, сколько приводит его права к нужным.
	if err := sostoyanie.ZavestiKatalogDannyh(); err == nil {
		if zh, err := zhurnaly.Otkryt(sostoyanie.KatalogZhurnalov(), "ustanovka.log"); err == nil {
			oshibki = io.MultiWriter(konsol, zh)
			zakryt = func() {
				log.SetOutput(konsol)
				zh.Close()
			}
		}
	}
	log.SetOutput(oshibki)
	return vKodirovkeMashiny{os.Stdout}, zakryt
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

// vKodirovkeMashiny оборачивает поток так, чтобы наружу уходили байты в
// кодовой странице машины, а не в UTF-8.
type vKodirovkeMashiny struct{ w io.Writer }

func (v vKodirovkeMashiny) Write(b []byte) (int, error) {
	gotovo, err := kodirovki.V(b, kodirovki.Ansi)
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
