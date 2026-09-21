package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"

	"golang.org/x/sys/windows"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/zhurnaly"
)

// Латиница и цифры проходят насквозь на любой кодовой странице. Это половина
// причины отказа: пути и ответы Windows приходят по-английски.
func TestLatinicaProhoditBaytVBayt(t *testing.T) {
	ishod := []byte(`open C:\Program Files\Affory: The system cannot find the file specified.`)
	gotovo, err := vAnsi(ishod)
	if err != nil {
		t.Fatalf("перекодировка отказала: %v", err)
	}
	if !bytes.Equal(gotovo, ishod) {
		t.Errorf("латиница изменилась: %q против %q", gotovo, ishod)
	}
}

// Кириллица наружу уходит НЕ в UTF-8, иначе установщик снова покажет
// «СѓСЃС‚Р°РЅРѕРІРєР°».
func TestKirillicaNeUezzhaetVUtf8(t *testing.T) {
	ishod := []byte("установка не удалась")
	gotovo, err := vAnsi(ishod)
	if err != nil {
		t.Fatalf("перекодировка отказала: %v", err)
	}
	if bytes.Equal(gotovo, ishod) {
		t.Error("байты не изменились: перекодировки не было")
	}
}

// Прочитанное обратно совпадает с исходным. Проверка идёт тем же способом,
// каким читает установщик: кодовой страницей машины.
func TestKirillicaChitaetsyaObratno(t *testing.T) {
	const ishod = "отпечатки не сняты: каталог программы не читается"
	gotovo, err := vAnsi([]byte(ishod))
	if err != nil {
		t.Fatalf("перекодировка отказала: %v", err)
	}
	if strings.Contains(string(gotovo), "?") {
		t.Skip("кодовая страница этой машины не знает кириллицы, читать обратно нечего")
	}
	if nazad := izKodirovkiMashiny(t, gotovo); nazad != ishod {
		t.Errorf("обратно прочиталось %q вместо %q", nazad, ishod)
	}
}

// Поток рапортует столько байт, сколько у него взяли.
//
// Перекодировка почти всегда меняет длину: кириллица из двух байт UTF-8
// становится одним. Честное число записанных байт здесь означало бы для log
// оборванную запись, и он ответил бы ошибкой на успешно напечатанную строку.
func TestPotokSchitaetBaytyVhoda(t *testing.T) {
	var kuda bytes.Buffer
	ishod := []byte("служба установлена\n")
	n, err := (vKodirovkeMashiny{&kuda}).Write(ishod)
	if err != nil {
		t.Fatalf("запись отказала: %v", err)
	}
	if n != len(ishod) {
		t.Errorf("записанными названы %d байт из %d", n, len(ishod))
	}
	if kuda.Len() == 0 {
		t.Error("наружу не ушло ничего")
	}
}

// Пустая запись никого не роняет: log зовёт Write и на пустой строке.
func TestPustayaZapisNeRonyaet(t *testing.T) {
	var kuda bytes.Buffer
	n, err := (vKodirovkeMashiny{&kuda}).Write(nil)
	if err != nil {
		t.Fatalf("пустая запись отказала: %v", err)
	}
	if n != 0 {
		t.Errorf("пустая запись насчитала %d байт", n)
	}
}

func izKodirovkiMashiny(t *testing.T, b []byte) string {
	t.Helper()
	n, err := windows.MultiByteToWideChar(cpAcp, 0, &b[0], int32(len(b)), nil, 0)
	if err != nil || n <= 0 {
		t.Fatalf("длина обратного перевода не посчиталась: %v", err)
	}
	shiroko := make([]uint16, n)
	if _, err := windows.MultiByteToWideChar(cpAcp, 0, &b[0], int32(len(b)), &shiroko[0], n); err != nil {
		t.Fatalf("обратный перевод отказал: %v", err)
	}
	return string(utf16.Decode(shiroko))
}

// Причина ложится рядом со своим бинарём в UTF-16LE и без BOM: ровно то, что
// умеет читать FileReadUTF16LE установщика.
func TestPrichinaLozhitsyaRyadomVUtf16(t *testing.T) {
	svoy, err := os.Executable()
	if err != nil {
		t.Fatalf("свой путь не читается: %v", err)
	}
	put := filepath.Join(filepath.Dir(svoy), imyaFaylaPrichiny)
	t.Cleanup(func() { os.Remove(put) })

	const prichina = "отпечатки не сняты: каталог программы не читается"
	polozhitPrichinu(prichina)

	b, err := os.ReadFile(put)
	if err != nil {
		t.Fatalf("файл причины не прочитан: %v", err)
	}
	if len(b) >= 2 && b[0] == 0xFF && b[1] == 0xFE {
		t.Error("в начале файла BOM: установщик покажет его первым символом строки")
	}
	if got := utf16LE(b); got != prichina {
		t.Errorf("в файле %q вместо %q", got, prichina)
	}
}

// Подкоманды падают через upast, а не через log.Fatalf.
//
// Разница не в стиле: log.Fatalf говорит причину только в трубу, а она живёт
// ровно до закрытия окна установщика. Человек с отказом на руках остаётся с
// кодом 1, который не отличает нехватку прав от занятого файла.
func TestPodkomandyPadayutCherezUpast(t *testing.T) {
	telo, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("main.go не прочитан: %v", err)
	}
	// Единственное разрешённое место это запуск от SCM в самом низу: там нет
	// ни установщика, ни человека у экрана, а есть журнал службы.
	if n := strings.Count(string(telo), "log.Fatalf"); n != 1 {
		t.Errorf("в main.go %d вызовов log.Fatalf, разрешён один (svc.Run)\n"+
			"подкоманды обязаны звать upast: он кладёт причину туда, где её увидит установщик", n)
	}
}

func utf16LE(b []byte) string {
	shiroko := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		shiroko = append(shiroko, uint16(b[i])|uint16(b[i+1])<<8)
	}
	return string(utf16.Decode(shiroko))
}

// Снятие с ключами стирает каталог данных, и свой же журнал ему не мешает.
//
// Журнал установки лежит ВНУТРИ этого каталога, а Windows не удаляет открытый
// файл: os.RemoveAll отвечает «файл используется другим процессом». Человек,
// только что согласившийся стереть ключи, получил бы отказ снятия. Найдено
// пробой до выкладки 1.4.1.
//
// Судья доказывает СЕБЯ первым шагом: пока журнал открыт, стирание обязано
// провалиться. Без этого шага он остался бы зелёным и на машине, где открытый
// файл удалению не мешает вовсе, то есть проверял бы погоду.
func TestSnyatieDannyhNeSpotykaetsyaOSvoyZhurnal(t *testing.T) {
	dannye := t.TempDir()
	zh, err := zhurnaly.Otkryt(filepath.Join(dannye, "log"), "ustanovka.log")
	if err != nil {
		t.Fatalf("журнал установки не открылся: %v", err)
	}
	if _, err := zh.Write([]byte("проба\n")); err != nil {
		t.Fatalf("в журнал не пишется: %v", err)
	}

	if err := snyatDannye(dannye, true); err == nil {
		t.Fatal("открытый журнал стиранию не помешал: судья ничего не проверяет")
	}

	zh.Close()
	if err := snyatDannye(dannye, true); err != nil {
		t.Fatalf("данные не удалены после закрытия журнала: %v", err)
	}
}

// Ветка снятия закрывает журнал ДО стирания данных, а не после.
//
// Порядок этих двух строк и есть вся починка: переставленные местами, они
// возвращают отказ снятия с ключами, и ловится это только живым прогоном в
// госте с ответом «да» на вопрос про ключи.
func TestSnyatieZakryvaetZhurnalDoStiraniya(t *testing.T) {
	telo, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("main.go не прочитан: %v", err)
	}
	zakrytie := strings.Index(string(telo), "zakrytZhurnalUstanovki()")
	stiranie := strings.Index(string(telo), "snyatDannye(")
	if zakrytie < 0 {
		t.Fatal("в main.go нет закрытия журнала установки")
	}
	if stiranie < 0 {
		t.Fatal("в main.go нет стирания данных")
	}
	if zakrytie > stiranie {
		t.Error("журнал установки закрывается ПОСЛЕ стирания данных: " +
			"открытый файл внутри каталога не даст его удалить")
	}
}
