package main

import (
	"image"
	"image/color"
	"image/draw"
	"testing"

	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"
)

// QR, собранный тем же пакетом, вклеенный в большой «экран» с полями: так
// выглядит снимок, где окно с QR занимает малую часть кадра.
func ekranSQr(t *testing.T, tekst string) image.Image {
	t.Helper()
	m, err := qrcode.NewQRCodeWriter().Encode(tekst, gozxing.BarcodeFormat_QR_CODE, 240, 240, nil)
	if err != nil {
		t.Fatal(err)
	}
	ekran := image.NewRGBA(image.Rect(0, 0, 1280, 720))
	draw.Draw(ekran, ekran.Bounds(), &image.Uniform{color.RGBA{40, 40, 48, 255}}, image.Point{}, draw.Src)
	for y := 0; y < m.GetHeight(); y++ {
		for x := 0; x < m.GetWidth(); x++ {
			c := color.RGBA{255, 255, 255, 255}
			if m.Get(x, y) {
				c = color.RGBA{0, 0, 0, 255}
			}
			ekran.Set(700+x, 300+y, c)
		}
	}
	return ekran
}

func TestRaspoznatQrNahoditSsylkuNaEkrane(t *testing.T) {
	ssylka := "vless://11111111-2222-3333-4444-555555555555@203.0.113.9:8443?type=tcp&security=reality&sni=www.example.com&fp=chrome&pbk=AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8&sid=01ab#Германия"
	got, err := raspoznatQr(ekranSQr(t, ssylka))
	if err != nil {
		t.Fatal(err)
	}
	if got != ssylka {
		t.Fatalf("прочитано %q", got)
	}
}

func TestRaspoznatQrBezQrOtkazyvaetSlovami(t *testing.T) {
	pusto := image.NewRGBA(image.Rect(0, 0, 640, 480))
	draw.Draw(pusto, pusto.Bounds(), &image.Uniform{color.RGBA{30, 30, 30, 255}}, image.Point{}, draw.Src)
	if _, err := raspoznatQr(pusto); err != errQrNeNayden {
		t.Fatalf("ждали «не найден», получили %v", err)
	}
}

// В QR приезжает и ключ, и подписка: панель выдаёт подписку картинкой. До
// 21.09.2026 сюда жёстко уходил addServer, и QR подписки отвергался словами
// про неизвестную схему при том, что человек всё сделал правильно.
func TestPodpiskaVQrOtlichaetsyaOtSsylkiNaServer(t *testing.T) {
	podpiski := []string{
		"https://panel.example/sub/abc",
		"http://panel.example/sub/abc",
		"HTTPS://PANEL.EXAMPLE/sub",
	}
	for _, a := range podpiski {
		if !podpiskaVQr(a) {
			t.Errorf("адрес подписки %q принят за ссылку на сервер", a)
		}
	}
	klyuchi := []string{
		"vless://11111111-2222-3333-4444-555555555555@203.0.113.9:8443?type=tcp",
		"hy2://parol@203.0.113.9:443",
		"ss://YWVzOnBhc3M@203.0.113.9:8388",
		"trojan://parol@203.0.113.9:443",
		"",
	}
	for _, k := range klyuchi {
		if podpiskaVQr(k) {
			t.Errorf("ссылка на сервер %q принята за подписку", k)
		}
	}
}
