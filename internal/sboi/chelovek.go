package sboi

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"unicode"
)

// Текст ошибки для окна (01.10.2026).
//
// До этого в окно уезжал err.Error() целиком, и человек читал «секреты не
// записаны: open C:\ProgramData\Affory\...: Access is denied.» или «подписка не
// загрузилась: dial tcp: lookup ...: getaddrinfow: This is usually a temporary
// error...». Разбор 01.10.2026 насчитал около восьмидесяти таких мест.
//
// Устройство: наша часть сообщения русская и идёт первой, чужая (Go, Windows,
// вывод ядра) английская и идёт хвостом после двоеточия. Хвост отрезается, а
// вместо него ставится причина по ТИПУ ошибки: «нет доступа», «файл занят
// другой программой». Полный текст остаётся в журнале службы, это дело
// вызывающего.

// ZapasnoyTekst идёт человеку, когда от ошибки не осталось ни одного нашего
// слова и тип ничего не подсказал.
const ZapasnoyTekst = "подробности в журнале"

// DlyaCheloveka отдаёт текст ошибки, который можно показать в окне. Для
// непустой ошибки строка никогда не пустая: поля, где пустота значит «всё
// хорошо», иначе молча превратили бы отказ в успех.
func DlyaCheloveka(err error) string {
	if err == nil {
		return ""
	}
	ves := err.Error()
	nashe := ObrezatTehniku(ves)
	koren := Koren(err)
	switch {
	case nashe == ves && nashe != "":
		// Обрезать было нечего: сообщение целиком наше и уже говорит, что
		// случилось. Причина по типу здесь была бы повтором.
		return nashe
	case nashe == "" && koren == "":
		return ZapasnoyTekst
	case nashe == "":
		return koren
	case koren == "" || strings.Contains(nashe, koren):
		return nashe
	}
	return nashe + ": " + koren
}

// ObrezatTehniku оставляет от текста ошибки нашу часть.
//
// Текст режется по «: » на звенья. Звено наше, если русских букв в нём не
// меньше латинских: «VPN перестал нести трафик» и «clash_api не отвечает»
// наши, «open C:\Users\Иван\x.json» и «dial tcp» нет, хотя в первом есть
// кириллица из имени папки. Ведущие чужие звенья пропускаются (код отказа в
// начале строки), а после первого нашего первое же чужое обрывает текст:
// за ним идёт вывод Go и Windows, и то, что лежит дальше, разобрать уже
// нельзя.
func ObrezatTehniku(tekst string) string {
	zvenya := strings.Split(tekst, ": ")
	var nashi []string
	for _, z := range zvenya {
		z = strings.TrimSpace(z)
		switch chey(z) {
		case nashe:
			nashi = append(nashi, z)
			continue
		case neytralno:
			// Одни адреса, пути и числа: «найти по имени: a.example,
			// b.example». После нашей фразы это её продолжение, в начале
			// строки сказать нечего.
			if len(nashi) > 0 {
				nashi = append(nashi, z)
				continue
			}
			continue
		}
		if len(nashi) > 0 {
			break
		}
	}
	return strings.TrimRight(strings.Join(nashi, ": "), " .:;,")
}

type prinadlezhnost int

const (
	chuzhoe prinadlezhnost = iota
	nashe
	neytralno
)

func chey(zveno string) prinadlezhnost {
	kir, lat := 0, 0
	for _, slovo := range strings.Fields(zveno) {
		// Пути, имена файлов и адреса не голосуют: «процессом C:\Program
		// Files\NekoBox\nekobox_core.exe» иначе перевесил бы русскую фразу, и
		// вместе с хвостом отрезалось бы имя мешающей программы.
		goloe := strings.Trim(slovo, `.,;:!?()«»"'`)
		if strings.ContainsAny(goloe, `\/.`) {
			continue
		}
		for _, r := range goloe {
			switch {
			case unicode.Is(unicode.Cyrillic, r):
				kir++
			case r < unicode.MaxASCII && unicode.IsLetter(r):
				lat++
			}
		}
	}
	switch {
	case kir > 0 && kir >= lat:
		return nashe
	case kir == 0 && lat == 0:
		return neytralno
	}
	return chuzhoe
}

// Koren называет причину по типу ошибки, без её текста. Пустая строка значит
// «тип ничего не говорит».
func Koren(err error) string {
	if err == nil {
		return ""
	}
	if vid := Klassifitsirovat(err); vid != Neyasno {
		return vid.Opisanie()
	}
	if t := korenSistemy(err); t != "" {
		return t
	}
	var sintaksis *json.SyntaxError
	var tip *json.UnmarshalTypeError
	var vyhod *exec.ExitError
	switch {
	case errors.Is(err, os.ErrPermission):
		return "нет доступа"
	case errors.Is(err, os.ErrNotExist):
		return "файл не найден"
	case errors.As(err, &sintaksis), errors.As(err, &tip):
		return "данные не разобрались"
	case errors.Is(err, io.ErrUnexpectedEOF), errors.Is(err, io.EOF):
		return "передача оборвалась"
	case errors.Is(err, zip.ErrFormat), errors.Is(err, zip.ErrChecksum), errors.Is(err, zip.ErrAlgorithm):
		return "архив повреждён"
	case errors.As(err, &vyhod):
		return "Windows не выполнила команду"
	}
	return ""
}
