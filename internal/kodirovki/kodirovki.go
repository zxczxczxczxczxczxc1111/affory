// Пакет kodirovki переводит текст между UTF-8 и кодовыми страницами Windows.
//
// Нужен в двух местах, и оба уже стоили разбора по жалобе человека.
//
// Наружу: вывод подкоманд читает установщик, а он декодирует трубу кодовой
// страницей ANSI машины. Наш UTF-8 превращался в «СѓСЃС‚Р°РЅРѕРІРєР°», то есть
// причина отказа установки терялась целиком.
//
// Внутрь: netsh отвечает в кодовой странице КОНСОЛИ (на русской Windows это
// 866), а мы читали её байты как UTF-8 и искали в них русскую фразу «ни одно
// правило». Совпадения не было никогда, поэтому отсутствие правила считалось
// отказом, и установка поверх прежней падала на подготовке у всех, у кого
// правил брандмауэра в этот момент не осталось.
package kodirovki

import (
	"errors"
	"unicode/utf16"
	"unicode/utf8"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	// Ansi это CP_ACP: кодовая страница ANSI машины, ею читает установщик.
	Ansi uint32 = 0
	// Konsoli это CP_OEMCP: кодовая страница консоли, в ней отвечает netsh.
	Konsoli uint32 = 1
)

// WideCharToMultiByte НЕТ в golang.org/x/sys/windows v0.47.0 (в нём только
// обратная MultiByteToWideChar), объявлено руками — так же, как
// GetExtendedTcpTable в internal/yadra/port.go.
var (
	kernel32                = windows.NewLazySystemDLL("kernel32.dll")
	procWideCharToMultiByte = kernel32.NewProc("WideCharToMultiByte")
)

var errPusto = errors.New("перекодировать нечего")

// V переводит UTF-8 в заданную кодовую страницу.
func V(b []byte, cp uint32) ([]byte, error) {
	if len(b) == 0 {
		return b, nil
	}
	shiroko := utf16.Encode([]rune(string(b)))
	if len(shiroko) == 0 {
		return nil, errPusto
	}
	nuzhno := wideCharToMultiByte(cp, shiroko, nil)
	if nuzhno <= 0 {
		return nil, errors.New("длина в кодовой странице не посчиталась")
	}
	gotovo := make([]byte, nuzhno)
	if wideCharToMultiByte(cp, shiroko, gotovo) <= 0 {
		return nil, errors.New("перекодировка в кодовую страницу отказала")
	}
	return gotovo, nil
}

// Iz переводит заданную кодовую страницу в UTF-8.
//
// Отказ молчаливый: возвращаются исходные байты строкой. Текст, который не
// удалось перевести, полезнее пустого — в нём хотя бы видны латиница и цифры.
func Iz(b []byte, cp uint32) string {
	if len(b) == 0 {
		return ""
	}
	n, err := windows.MultiByteToWideChar(cp, 0, &b[0], int32(len(b)), nil, 0)
	if err != nil || n <= 0 {
		return string(b)
	}
	shiroko := make([]uint16, n)
	if _, err := windows.MultiByteToWideChar(cp, 0, &b[0], int32(len(b)), &shiroko[0], n); err != nil {
		return string(b)
	}
	return string(utf16.Decode(shiroko))
}

// wideCharToMultiByte с пустым приёмником отвечает, сколько байт нужно.
func wideCharToMultiByte(cp uint32, shiroko []uint16, kuda []byte) int {
	var p *byte
	if len(kuda) > 0 {
		p = &kuda[0]
	}
	r, _, _ := procWideCharToMultiByte.Call(
		uintptr(cp),
		0, // без флагов: подстановка знака вопроса для непредставимых нас устраивает
		uintptr(unsafe.Pointer(&shiroko[0])),
		uintptr(len(shiroko)),
		uintptr(unsafe.Pointer(p)),
		uintptr(len(kuda)),
		0, 0,
	)
	return int(r)
}

// FaylOtCheloveka приводит к UTF-8 файл, который писал человек.
//
// Блокнот, `Set-Content` в PowerShell 5.1, редакторы «для программистов» и
// «Сохранить как» из проводника дают четыре разных ответа на вопрос, в чём
// сохранить текст. Человеку, который просто набрал пути своих программ, нечем
// об этом догадаться: он получает «invalid character» и читает это как
// сломанную программу, а не как сломанный файл.
//
// Разбираются три случая: UTF-16 в обе стороны по сигнатуре, UTF-8 с
// сигнатурой и без, и всё остальное как ANSI машины. Последнее это догадка,
// но единственная возможная: у ANSI нет признака, а кириллица в нём для
// разбора JSON выглядит как испорченные байты.
func FaylOtCheloveka(b []byte) []byte {
	switch {
	case len(b) >= 2 && b[0] == 0xFF && b[1] == 0xFE:
		return []byte(izUtf16(b[2:], false))
	case len(b) >= 2 && b[0] == 0xFE && b[1] == 0xFF:
		return []byte(izUtf16(b[2:], true))
	case len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF:
		return b[3:]
	case utf8.Valid(b):
		return b
	default:
		return []byte(Iz(b, Ansi))
	}
}

func izUtf16(b []byte, starshiyPervym bool) string {
	shiroko := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		if starshiyPervym {
			shiroko = append(shiroko, uint16(b[i])<<8|uint16(b[i+1]))
		} else {
			shiroko = append(shiroko, uint16(b[i])|uint16(b[i+1])<<8)
		}
	}
	return string(utf16.Decode(shiroko))
}
