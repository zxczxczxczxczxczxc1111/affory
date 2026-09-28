package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kbinani/screenshot"
	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/ssylki"
)

// QR с экрана (план «шесть удобств» §3). Окно прячется, каждый экран
// снимается целиком, в снимке ищется QR. Найденная ссылка НЕ возвращается в
// окно: окно получает сводку без секретов, а после подтверждения ссылка уходит
// в службу командой addServers. Ключ по экрану не гуляет.

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

// Шов для тестов: снимок экрана.
var prochitatQrSEkrana = (*most).prochitatEkran

// prochitatEkran снимает экраны и отдаёт текст первого найденного QR.
func (m *most) prochitatEkran() (string, error) {
	// Окно уходит с экрана на время снимка: иначе в кадре его собственная
	// форма, а не чужое окно с QR. Возврат через defer при любом исходе.
	if m.okno != nil {
		m.okno.Hide()
		defer m.okno.Show()
		time.Sleep(250 * time.Millisecond)
	}
	for _, kadr := range snyatEkrany() {
		if t, err := raspoznatQr(kadr); err == nil {
			return t, nil
		}
	}
	return "", errQrNeNayden
}

// naydennoeQr это то, что уйдёт в службу после подтверждения. Ровно то, что
// описано в сводке: негодные строки подписок сюда не попадают.
type naydennoeQr struct {
	klyuchi string
	adresa  []string
}

// qrNaPodtverzhdenii держит находку между двумя шагами. В окно она не
// уходит: там только сводка.
type qrNaPodtverzhdenii struct {
	mu      sync.Mutex
	naydeno *naydennoeQr
}

func (q *qrNaPodtverzhdenii) polozhit(n naydennoeQr) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.naydeno = &n
}

// vzyat отдаёт находку и забывает её: одна находка на одно подтверждение.
func (q *qrNaPodtverzhdenii) vzyat() *naydennoeQr {
	q.mu.Lock()
	defer q.mu.Unlock()
	n := q.naydeno
	q.naydeno = nil
	return n
}

// svodkaQr это всё, что окно узнаёт о находке. Ключи без паролей и UUID,
// подписки только узлом: путь адреса и есть её секрет.
type svodkaQr struct {
	Klyuchi  []klyuchVQr `json:"klyuchi"`
	Podpiski []string    `json:"podpiski"`
	Negodnyh int         `json:"negodnyh"`
}

type klyuchVQr struct {
	Imya      string `json:"imya"`
	Transport string `json:"transport"`
	Adres     string `json:"adres"`
}

var errQrNeChego = errors.New("в QR нет ни ключей, ни адреса подписки")

// NaytiQrNaEkrane: снимок и разбор, но НЕ добавление (О8 аудита 1.6.1).
//
// Раньше найденное сразу уходило в службу, и любая страница с QR на экране
// подсовывала человеку свой сервер или свою подписку. Теперь окно показывает
// сводку, а добавляет DobavitNaydennoeQr по нажатию.
//
// В QR может лежать и то и другое: панель выдаёт подписку картинкой, чужие
// клиенты раздают отдельные ключи, выгрузка Affory кладёт ключи по строке, и
// адрес подписки может стоять среди них.
func (m *most) NaytiQrNaEkrane() (svodkaQr, error) {
	// Прежняя находка не доживает до новой попытки, чем бы та ни кончилась.
	m.qr.vzyat()
	tekst, err := prochitatQrSEkrana(m)
	if err != nil {
		return svodkaQr{}, err
	}
	svodka := svodkaQr{Klyuchi: []klyuchVQr{}, Podpiski: []string{}}
	var naydeno naydennoeQr
	var klyuchi []string
	for _, stroka := range strings.Split(tekst, "\n") {
		stroka = strings.TrimSpace(stroka)
		switch {
		case stroka == "":
		case podpiskaVQr(stroka):
			if u, err := url.Parse(stroka); err == nil && u.Hostname() != "" {
				naydeno.adresa = append(naydeno.adresa, stroka)
				svodka.Podpiski = append(svodka.Podpiski, u.Hostname())
			} else {
				svodka.Negodnyh++
			}
		default:
			klyuchi = append(klyuchi, stroka)
		}
	}
	if len(klyuchi) > 0 {
		naydeno.klyuchi = strings.Join(klyuchi, "\n")
		r, err := ssylki.RazobratPachku(naydeno.klyuchi)
		if err == nil && len(r.Servery) == 0 && len(r.Otkazy) > 0 {
			err = fmt.Errorf("ключ в QR не разобрался: %s", r.Otkazy[0].Prichina)
			if !strings.Contains(naydeno.klyuchi, "://") {
				err = errQrNeChego
			}
		}
		if errors.Is(err, ssylki.ErrPachkaPusta) {
			err = errQrNeChego
		}
		if err != nil {
			if len(naydeno.adresa) == 0 {
				return svodkaQr{}, err
			}
			// Подписка рядом годная: ключи не добавятся, но сводка их считает.
			naydeno.klyuchi = ""
			svodka.Negodnyh += len(klyuchi)
		} else {
			for _, s := range r.Servery {
				svodka.Klyuchi = append(svodka.Klyuchi, klyuchVQr{
					Imya: s.Imya, Transport: s.Transport,
					Adres: net.JoinHostPort(s.Host, strconv.Itoa(s.Port)),
				})
			}
			svodka.Negodnyh += len(r.Otkazy)
		}
	}
	if len(svodka.Klyuchi) == 0 && len(naydeno.adresa) == 0 {
		return svodkaQr{}, errQrNeChego
	}
	m.qr.polozhit(naydeno)
	return svodka, nil
}

// ZabytQr: человек нажал «Отмена», находка не добавляется.
func (m *most) ZabytQr() { m.qr.vzyat() }

// DobavitNaydennoeQr отправляет в службу ровно то, что было в сводке.
//
// Возвращает ГОТОВУЮ строку исхода, а не имя: у подписки имени нет, а её адрес
// это секрет того же разряда, что ключ, и в окно он не попадает.
func (m *most) DobavitNaydennoeQr() (string, error) {
	naydeno := m.qr.vzyat()
	if naydeno == nil {
		return "", errors.New("найденного QR больше нет, сними экран заново")
	}
	adresa := naydeno.adresa
	var itogi []string
	if naydeno.klyuchi != "" {
		itog, err := m.pachkaSEkrana(naydeno.klyuchi)
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
	k, err := m.komandaSluzhbe("addServers", telo)
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
	k, err := m.komandaSluzhbe("addSubscription", telo)
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
