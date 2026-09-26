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
// окно: она уходит в службу командой addServers, а окно узнаёт только итог
// либо причину отказа. Ключ по экрану не гуляет.

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

// podpiskaVQr отличает адрес подписки от ссылки на сервер.
//
// Разбирать глубже незачем: ссылка на сервер это всегда своя схема
// (vless://, hy2:// и прочие), а подписка это http(s) и ничего больше. Ту же
// границу проводит окно у кнопок буфера (pohozheNaAdres в Servery.tsx).
func podpiskaVQr(s string) bool {
	n := strings.ToLower(s)
	return strings.HasPrefix(n, "http://") || strings.HasPrefix(n, "https://")
}

// DobavitSEkrana: снимок, разбор и та команда, которой соответствует код.
//
// В QR может лежать и то и другое: панель выдаёт подписку картинкой, чужие
// клиенты раздают отдельные ключи. Раньше сюда жёстко уходил addServer, и QR
// подписки отвергался словами про неизвестную схему, хотя человек всё сделал
// правильно.
//
// Возвращает ГОТОВУЮ строку исхода, а не имя: у подписки имени нет, а её адрес
// это секрет того же разряда, что ключ, и в окно он не попадает.
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
	// С 26.09.2026 в одном коде бывает пачка: выгрузка Affory и чужих клиентов
	// кладёт ключи по строке, и адрес подписки может стоять среди них.
	var adresa, klyuchi []string
	for _, stroka := range strings.Split(ssylka, "\n") {
		stroka = strings.TrimSpace(stroka)
		switch {
		case stroka == "":
		case podpiskaVQr(stroka):
			adresa = append(adresa, stroka)
		default:
			klyuchi = append(klyuchi, stroka)
		}
	}
	var itogi []string
	if len(klyuchi) > 0 {
		itog, err := m.pachkaSEkrana(strings.Join(klyuchi, "\n"))
		if err != nil {
			return "", err
		}
		itogi = append(itogi, itog)
	}
	for _, adres := range adresa {
		itog, err := m.podpiskaSEkrana(adres)
		if err != nil {
			// Ключи из того же кода уже добавлены: отказ подписки не имеет
			// права стереть их итог с экрана.
			if len(itogi) == 0 && len(adresa) == 1 {
				return "", err
			}
			itog = "подписка не добавлена: " + err.Error()
		}
		itogi = append(itogi, itog)
	}
	return strings.Join(itogi, "; "), nil
}

func (m *most) pachkaSEkrana(tekst string) (string, error) {
	telo, _ := json.Marshal(map[string]string{"tekst": tekst})
	k, err := m.komandaQr("addServers", telo)
	if err != nil {
		return "", err
	}
	var itog itogPachki
	if err := json.Unmarshal(k.Telo, &itog); err != nil {
		return "ключи добавлены", nil
	}
	return itog.stroka(), nil
}

// itogPachki это ответ addServers. Фраза та же, что собирает окно после
// вставки в поле (itogVstavki в Servery.tsx): один исход, одни слова.
type itogPachki struct {
	Dobavleno int `json:"dobavleno"`
	Obnovleno int `json:"obnovleno"`
	UzheBylo  int `json:"uzhe_bylo"`
	Otkazy    []struct {
		Stroka   int    `json:"stroka"`
		Prichina string `json:"prichina"`
	} `json:"otkazy"`
}

func (i itogPachki) stroka() string {
	chasti := []string{fmt.Sprintf("добавлено %d", i.Dobavleno)}
	if i.Obnovleno > 0 {
		chasti = append(chasti, fmt.Sprintf("переименовано %d", i.Obnovleno))
	}
	if i.UzheBylo > 0 {
		chasti = append(chasti, fmt.Sprintf("уже были %d", i.UzheBylo))
	}
	if len(i.Otkazy) > 0 {
		chasti = append(chasti, fmt.Sprintf("пропущено %d", len(i.Otkazy)))
		for _, o := range i.Otkazy {
			chasti = append(chasti, fmt.Sprintf("строка %d: %s", o.Stroka, o.Prichina))
		}
	}
	return strings.Join(chasti, ", ")
}

func (m *most) podpiskaSEkrana(adres string) (string, error) {
	telo, _ := json.Marshal(map[string]string{"adres": adres})
	k, err := m.komandaQr("addSubscription", telo)
	if err != nil {
		return "", err
	}
	// Запасная подписка ложится адресом и списка не тянет, активная приносит
	// серверы сразу. Человеку важна именно эта разница: после первой список на
	// экране не изменится, и молчание выглядело бы отказом.
	var dobavlena struct {
		Aktivnaya bool `json:"aktivnaya"`
		Serverov  int  `json:"serverov"`
	}
	if err := json.Unmarshal(k.Telo, &dobavlena); err != nil {
		return "подписка добавлена", nil
	}
	if !dobavlena.Aktivnaya {
		return "подписка добавлена про запас", nil
	}
	return fmt.Sprintf("подписка добавлена, серверов: %d", dobavlena.Serverov), nil
}

// komandaQr шлёт команду службе и разворачивает её кадр. Отказ службы
// возвращается её же текстом, чтобы окно показало его той строкой, что и всё
// остальное.
func (m *most) komandaQr(imya string, telo []byte) (protokol.Kadr, error) {
	var k protokol.Kadr
	otvet, err := m.Zvat(imya, string(telo))
	if err != nil {
		return k, err
	}
	if err := json.Unmarshal([]byte(otvet), &k); err != nil {
		return k, fmt.Errorf("ответ службы не разобран: %w", err)
	}
	if k.Oshib != nil {
		return k, errors.New(k.Oshib.Tekst)
	}
	return k, nil
}
