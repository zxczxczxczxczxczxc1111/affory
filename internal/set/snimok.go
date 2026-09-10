package set

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Снимок состояния сервера (задача 6.4). Пишется таймером на VPS раз в сутки
// и кладётся машиной оператора рядом с подпиской на прод как
// <адрес подписки>.sostoyanie.json (решено 03.09.2026, план
// волны 6, задача 6.0). Клиент выводит адрес из адреса подписки, поэтому новых
// секретов не появляется, а забирает его НАПРЯМУЮ: нужен он ровно тогда,
// когда туннель лежит.
type Snimok struct {
	Vremya           int64    `json:"vremya"`
	Trevogi          []string `json:"trevogi"`
	DneySertifikata  *int     `json:"dney_do_konca_sertifikata_maski"`
	DneySObnovleniya *int     `json:"dney_s_obnovleniya_xray"`
	XrayVersiya      string   `json:"xray_versiya"`
	// Hysteria2 вторым демоном (03.09.2026): сертификат LE на IP живёт 6 дней,
	// его срок это отдельная строка карточки, а не общая с маской.
	Hy2Aktiven         bool `json:"hy2_aktiven"`
	DneySertifikataHy2 *int `json:"dney_do_konca_sertifikata_hy2"`
}

// ErrSnimkaNet: на проде нет файла. Отличается от сетевой ошибки: сеть лежит
// у нас, а файла нет у оператора.
var ErrSnimkaNet = errors.New("снимка состояния рядом с подпиской нет")

// klientSnimka это один клиент на пакет. Прежде транспорт собирался на каждый
// вызов, а транспорт, собранный на вызов, уносит соединение в свой пул простоя
// и хоронит его там навсегда (см. тот же разбор у klientKlash в internal/yadra).
//
// DisableKeepAlives, а не пул: снимок берётся по расписанию подписки, то есть
// раз в час в лучшем случае. Переиспользовать тут нечего, а закрытое сразу
// соединение не может протечь даже теоретически.
var klientSnimka = &http.Client{
	Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true},
}

const (
	suffiksSnimka = ".sostoyanie.json"
	srokSnimka    = 15 * time.Second
	predelSnimka  = 64 << 10
)

// AdresSnimka выводит адрес снимка из адреса подписки. Подписка с запросом,
// якорем или без пути это не наш формат, и дописывать к ней хвост значило бы
// стучаться неизвестно куда.
func AdresSnimka(podpiska string) (string, error) {
	u, err := url.Parse(podpiska)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("адрес подписки не разбирается")
	}
	if u.RawQuery != "" || u.Fragment != "" || u.Path == "" || strings.HasSuffix(u.Path, "/") {
		return "", fmt.Errorf("адрес подписки не того вида, чтобы вывести из него адрес снимка")
	}
	return podpiska + suffiksSnimka, nil
}

// ZagruzitSnimok забирает снимок напрямую, без прокси, с теми же предохранителями,
// что у подписки: предел тела и разбор, отвергающий чужую страницу.
func ZagruzitSnimok(ctx context.Context, adres string) (Snimok, error) {
	do, otm := context.WithTimeout(ctx, srokSnimka)
	defer otm()
	z, err := http.NewRequestWithContext(do, http.MethodGet, adres, nil)
	if err != nil {
		return Snimok{}, fmt.Errorf("запрос снимка не собран: %w", err)
	}
	o, err := klientSnimka.Do(z)
	if err != nil {
		return Snimok{}, fmt.Errorf("снимок не получен: %w", err)
	}
	defer o.Body.Close()
	if o.StatusCode == http.StatusNotFound {
		return Snimok{}, ErrSnimkaNet
	}
	if o.StatusCode != http.StatusOK {
		return Snimok{}, fmt.Errorf("снимок: сервер ответил кодом %d", o.StatusCode)
	}
	telo, err := io.ReadAll(io.LimitReader(o.Body, predelSnimka))
	if err != nil {
		return Snimok{}, fmt.Errorf("снимок не дочитан: %w", err)
	}
	var s Snimok
	if err := json.Unmarshal(telo, &s); err != nil {
		return Snimok{}, fmt.Errorf("снимок не разбирается: %w", err)
	}
	if s.Vremya <= 0 {
		return Snimok{}, fmt.Errorf("в снимке нет времени, возраст судить не по чему")
	}
	if s.Trevogi == nil {
		s.Trevogi = []string{}
	}
	return s, nil
}
