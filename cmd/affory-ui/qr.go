package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"strings"
	"time"

	"github.com/kbinani/screenshot"
	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// QR с экрана (план «шесть удобств» §3). Окно прячется, каждый экран
// снимается целиком, в снимке ищется QR. Найденная ссылка НЕ возвращается в
// окно: она уходит в службу тем же addServer, а окно узнаёт только имя
// добавленного сервера либо причину отказа. Ключ по экрану не гуляет.

var errQrNeNayden = errors.New("QR на экране не найден")

// raspoznatQr ищет один QR в картинке. Чистая функция: тест кормит её PNG,
// собранным тем же пакетом.
func raspoznatQr(img image.Image) (string, error) {
	bmp, err := gozxing.NewBinaryBitmapFromImage(img)
	if err != nil {
		return "", err
	}
	// TryHarder: на снимке экрана QR это малая часть кадра, и без этого флага
	// поиск сдаётся на первой же неудачной строке.
	r, err := qrcode.NewQRCodeReader().Decode(bmp, map[gozxing.DecodeHintType]interface{}{
		gozxing.DecodeHintType_TRY_HARDER: true,
	})
	if err != nil {
		return "", errQrNeNayden
	}
	return strings.TrimSpace(r.GetText()), nil
}

// snyatEkrany снимает каждый активный экран. Ошибка одного экрана не
// мешает остальным: QR обычно на главном.
func snyatEkrany() []image.Image {
	var kadry []image.Image
	for i := 0; i < screenshot.NumActiveDisplays(); i++ {
		img, err := screenshot.CaptureDisplay(i)
		if err != nil || img == nil {
			continue
		}
		kadry = append(kadry, img)
	}
	return kadry
}

// DobavitSEkrana: снимок, разбор, addServer. Возвращает имя добавленного
// сервера; отказ службы возвращается её текстом, чтобы окно показало его
// той же строкой, что и всё остальное.
func (m *most) DobavitSEkrana() (string, error) {
	// Окно уходит с экрана на время снимка: иначе в кадре его собственная
	// форма, а не чужое окно с QR. Возврат через defer при любом исходе.
	if m.okno != nil {
		m.okno.Hide()
		defer m.okno.Show()
		time.Sleep(250 * time.Millisecond)
	}
	var ssylka string
	for _, kadr := range snyatEkrany() {
		if t, err := raspoznatQr(kadr); err == nil {
			ssylka = t
			break
		}
	}
	if ssylka == "" {
		return "", errQrNeNayden
	}
	telo, _ := json.Marshal(map[string]string{"ssylka": ssylka})
	otvet, err := m.Zvat("addServer", string(telo))
	if err != nil {
		return "", err
	}
	var k protokol.Kadr
	if err := json.Unmarshal([]byte(otvet), &k); err != nil {
		return "", fmt.Errorf("ответ службы не разобран: %w", err)
	}
	if k.Oshib != nil {
		return "", errors.New(k.Oshib.Tekst)
	}
	var dobavlen struct {
		Server protokol.Server `json:"server"`
	}
	if err := json.Unmarshal(k.Telo, &dobavlen); err != nil || dobavlen.Server.Imya == "" {
		return "сервер", nil
	}
	return dobavlen.Server.Imya, nil
}
