package kodirovki

import (
	"bytes"
	"strings"
	"testing"
	"unicode/utf16"
)

// Кодовые страницы берутся явными числами, а не настройкой машины: у этой
// машины ANSI 1252 и консоль 437, то есть проверка «как у человека» на ней не
// состоится ни разу, если положиться на локаль.
const (
	cp1251 = 1251 // ANSI русской Windows: в ней читает трубу установщик
	cp866  = 866  // консоль русской Windows: в ней отвечает netsh
)

// Латиница и цифры проходят насквозь в любую сторону и на любой странице.
// Это половина всякой причины отказа: пути и ответы Windows приходят
// по-английски.
func TestLatinicaProhoditBaytVBayt(t *testing.T) {
	ishod := []byte(`open C:\Program Files\Affory: The system cannot find the file specified.`)
	gotovo, err := V(ishod, cp1251)
	if err != nil {
		t.Fatalf("перекодировка отказала: %v", err)
	}
	if !bytes.Equal(gotovo, ishod) {
		t.Errorf("латиница изменилась: %q против %q", gotovo, ishod)
	}
	if nazad := Iz(ishod, cp866); nazad != string(ishod) {
		t.Errorf("обратно латиница изменилась: %q", nazad)
	}
}

// Круг через страницу русской Windows возвращает исходный текст.
//
// Это и есть обещание установщику: он прочтёт то, что мы написали.
func TestKirillicaVozvrashchaetsyaPosleKruga(t *testing.T) {
	const ishod = "отпечатки не сняты: каталог программы не читается"
	gotovo, err := V([]byte(ishod), cp1251)
	if err != nil {
		t.Fatalf("перекодировка отказала: %v", err)
	}
	if strings.Contains(string(gotovo), "?") {
		t.Fatal("кириллица не уложилась в 1251: страница выбрана неверно")
	}
	if nazad := Iz(gotovo, cp1251); nazad != ishod {
		t.Errorf("обратно прочиталось %q вместо %q", nazad, ishod)
	}
}

// UTF-8 наружу не уходит: ровно его установщик и показывал вперемешку.
func TestKirillicaNeUezzhaetVUtf8(t *testing.T) {
	ishod := []byte("установка не удалась")
	gotovo, err := V(ishod, cp1251)
	if err != nil {
		t.Fatalf("перекодировка отказала: %v", err)
	}
	if bytes.Equal(gotovo, ishod) {
		t.Error("байты не изменились: перекодировки не было")
	}
}

// Ответ консоли русской Windows читается правильно.
//
// Байты зашиты: «Ни одно правило не соответствует указанным критериям.» в 866.
// Пока их читали как UTF-8, снятие несуществующего правила брандмауэра
// считалось отказом, и установка падала на подготовке (жалоба 21.09.2026).
func TestOtvetKonsoliChitaetsya(t *testing.T) {
	otvet866 := []byte{
		0x8d, 0xa8, 0x20, 0xae, 0xa4, 0xad, 0xae, 0x20, 0xaf, 0xe0, 0xa0, 0xa2, 0xa8, 0xab, 0xae, 0x20,
		0xad, 0xa5, 0x20, 0xe1, 0xae, 0xae, 0xe2, 0xa2, 0xa5, 0xe2, 0xe1, 0xe2, 0xa2, 0xe3, 0xa5, 0xe2,
		0x20, 0xe3, 0xaa, 0xa0, 0xa7, 0xa0, 0xad, 0xad, 0xeb, 0xac, 0x20, 0xaa, 0xe0, 0xa8, 0xe2, 0xa5,
		0xe0, 0xa8, 0xef, 0xac, 0x2e,
	}
	// Сырые байты как UTF-8 это мусор: судья доказывает себя.
	if strings.Contains(string(otvet866), "Ни одно правило") {
		t.Fatal("байты консоли прочитались как UTF-8: проверять нечего")
	}
	if got := Iz(otvet866, cp866); got != "Ни одно правило не соответствует указанным критериям." {
		t.Errorf("из 866 прочиталось %q", got)
	}
}

// Пустой вход не роняет ни одну сторону.
func TestPustoyVhodNeRonyaet(t *testing.T) {
	if b, err := V(nil, cp1251); err != nil || len(b) != 0 {
		t.Errorf("пустая перекодировка наружу: %q, %v", b, err)
	}
	if s := Iz(nil, cp866); s != "" {
		t.Errorf("пустая перекодировка внутрь дала %q", s)
	}
}

// Файл правил человек сохраняет чем попало, и все четыре ответа Windows
// обязаны читаться. Путь взят с кириллицей нарочно: без неё дефекта нет
// вовсе, а «Игры» и «Рабочий стол» в путях у людей повсеместно.
func TestFaylOtChelovekaChitaetsyaVLyubomVide(t *testing.T) {
	const ishod = `{"protsessy":["C:\Игры\game.exe"]}`

	utf16le := []byte{0xFF, 0xFE}
	for _, w := range utf16.Encode([]rune(ishod)) {
		utf16le = append(utf16le, byte(w), byte(w>>8))
	}
	utf16be := []byte{0xFE, 0xFF}
	for _, w := range utf16.Encode([]rune(ishod)) {
		utf16be = append(utf16be, byte(w>>8), byte(w))
	}
	// Образец в ANSI собирается страницей ЭТОЙ машины: читать его будет она же.
	// Прибитая 1251 сошлась бы только на русской Windows, а тест обязан значить
	// одно и то же везде.
	ansi, err := V([]byte(ishod), Ansi)
	if err != nil {
		t.Fatalf("образец в ANSI не собрался: %v", err)
	}

	sluchai := map[string][]byte{
		"UTF-8 без сигнатуры": []byte(ishod),
		"UTF-8 с сигнатурой":  append([]byte{0xEF, 0xBB, 0xBF}, ishod...),
		"UTF-16LE":            utf16le,
		"UTF-16BE":            utf16be,
	}
	for imya, telo := range sluchai {
		if got := string(FaylOtCheloveka(telo)); got != ishod {
			t.Errorf("%s прочитан как %q", imya, got)
		}
	}

	// ANSI проверяется отдельно: на машине с другой кодовой страницей
	// кириллица в него не укладывается вовсе, и сверять нечего.
	if !strings.Contains(string(ansi), "?") {
		if got := string(FaylOtCheloveka(ansi)); got != ishod {
			t.Errorf("ANSI прочитан как %q", got)
		}
	}
}

// Пустой и короткий файл не роняют чтение: обрывок сигнатуры вместо
// содержимого это обычное дело у файла, который человек создал и не заполнил.
//
// Длина при этом может и вырасти, и это правильно: одиночный байт ANSI
// становится двумя байтами UTF-8. Проверяется, что чтение доходит до конца и
// пустое остаётся пустым.
func TestKorotkiyFaylNeRonyaet(t *testing.T) {
	for _, telo := range [][]byte{nil, {}} {
		if got := FaylOtCheloveka(telo); len(got) != 0 {
			t.Errorf("из пустого файла получилось %q", got)
		}
	}
	for _, telo := range [][]byte{{0xFF}, {0xFF, 0xFE}, {0xEF, 0xBB}, {0xEF, 0xBB, 0xBF}} {
		FaylOtCheloveka(telo) // не должно ронять
	}
}
