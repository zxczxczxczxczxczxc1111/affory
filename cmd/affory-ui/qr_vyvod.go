package main

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/png"
	"strings"

	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"
	"github.com/makiuchi-d/gozxing/qrcode/decoder"
)

// QR для выгрузки серверов (26.09.2026): перенести ключи на телефон или
// другой ПК, наведя камеру на экран.

// potolokKoda это байт на один код. Шесть наших ключей это ~930 байт, то есть
// один код средней плотности. Код на пределе стандарта (2953 байта, 177
// модулей) телефон с монитора читает плохо, поэтому дальше текст делится по
// строкам на несколько кодов, а не набивается в один.
const potolokKoda = 1400

// pikseleyNaKod это сторона картинки, к которой тянется код. Окно ужимает её
// под свою ширину само, а запас нужен, чтобы модули не мылились.
const pikseleyNaKod = 720

var errNePomeshchaetsya = errors.New("одна ссылка не помещается в QR, скопируй текстом")

// KodyQr рисует текст одним или несколькими QR и отдаёт PNG в виде data URI.
func (m *most) KodyQr(tekst string) ([]string, error) {
	chasti := razlozhitNaKody(tekst, potolokKoda)
	if len(chasti) == 0 {
		return nil, errors.New("выгружать нечего")
	}
	kody := make([]string, 0, len(chasti))
	for _, chast := range chasti {
		b, err := narisovatQr(chast)
		if err != nil {
			return nil, err
		}
		kody = append(kody, "data:image/png;base64,"+base64.StdEncoding.EncodeToString(b))
	}
	return kody, nil
}

// razlozhitNaKody делит текст по строкам на куски не длиннее potolok. Строка
// длиннее потолка идёт отдельным куском: резать ссылку посередине нельзя,
// половина ключа не разберётся ни одним клиентом.
func razlozhitNaKody(tekst string, potolok int) []string {
	var chasti []string
	var tek strings.Builder
	for _, stroka := range strings.Split(tekst, "\n") {
		stroka = strings.TrimSpace(stroka)
		if stroka == "" {
			continue
		}
		if tek.Len() > 0 && tek.Len()+1+len(stroka) > potolok {
			chasti = append(chasti, tek.String())
			tek.Reset()
		}
		if tek.Len() > 0 {
			tek.WriteByte('\n')
		}
		tek.WriteString(stroka)
	}
	if tek.Len() > 0 {
		chasti = append(chasti, tek.String())
	}
	return chasti
}

// narisovatQr кодирует уровнем M, пока влезает, иначе L: M переживает блик и
// муар экрана, L только выручает длинную ссылку.
func narisovatQr(tekst string) ([]byte, error) {
	var matrica *gozxing.BitMatrix
	var err error
	for _, uroven := range []decoder.ErrorCorrectionLevel{decoder.ErrorCorrectionLevel_M, decoder.ErrorCorrectionLevel_L} {
		matrica, err = qrcode.NewQRCodeWriter().Encode(tekst, gozxing.BarcodeFormat_QR_CODE, 0, 0,
			map[gozxing.EncodeHintType]interface{}{
				gozxing.EncodeHintType_ERROR_CORRECTION: uroven,
				gozxing.EncodeHintType_MARGIN:           4,
			})
		if err == nil {
			break
		}
	}
	if err != nil {
		return nil, errNePomeshchaetsya
	}
	// Ширина 0 даёт один пиксель на модуль, масштаб целый: дробный размывает
	// границы модулей, и камера путает соседние.
	storona := matrica.GetWidth()
	masshtab := pikseleyNaKod / storona
	if masshtab < 2 {
		masshtab = 2
	}
	img := image.NewGray(image.Rect(0, 0, storona*masshtab, storona*masshtab))
	for y := 0; y < storona*masshtab; y++ {
		for x := 0; x < storona*masshtab; x++ {
			c := color.Gray{Y: 255}
			if matrica.Get(x/masshtab, y/masshtab) {
				c = color.Gray{Y: 0}
			}
			img.SetGray(x, y, c)
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
