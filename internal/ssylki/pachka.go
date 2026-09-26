package ssylki

import (
	"errors"
	"strings"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Вставка пачкой, 26.09.2026: человек кладёт в одно поле шесть ключей или сто,
// по строке или одним base64, как отдают подписки и чужие клиенты.
var (
	ErrPachkaVelika = errors.New("текст больше мегабайта")
	ErrPachkaPusta  = errors.New("в тексте нет ни одной ссылки")
	// Настройки чужого клиента целиком (JSON, Clash, WireGuard) по строкам
	// дали бы сотни отказов, из которых не понять главного: это не ссылки.
	ErrNeSsylki = errors.New("это файл настроек, а не ссылки")
)

// RazobratPachku разбирает вставленный текст в список серверов.
//
// Частичный успех это успех: пять ключей из шести добавляются, а шестой
// приезжает в Otkazy с номером строки и причиной. Отказ целиком только тогда,
// когда ни одной ссылки нет вовсе.
//
// Строки http(s) пропускаются молча: окно отправляет их подпиской.
func RazobratPachku(tekst string) (Razbor, error) {
	if int64(len(tekst)) > PotolokPoUmolchaniyu {
		return Razbor{}, ErrPachkaVelika
	}
	if pohozheNaNastroyki(tekst) {
		return Razbor{}, ErrNeSsylki
	}
	r := razobratStroki([]byte(tekst), false)
	// Сообщение панели во вставке это отказ строки, а не истекшая подписка:
	// подписки здесь нет, есть строка, которую человек скопировал.
	for _, u := range r.Uvedomleniya {
		r.Otkazy = append(r.Otkazy, OtkazStroki{Stroka: u.Stroka, Prichina: "вместо сервера сообщение: " + u.Tekst})
	}
	r.Uvedomleniya = nil
	if len(r.Servery) == 0 && len(r.Otkazy) == 0 {
		return r, ErrPachkaPusta
	}
	return r, nil
}

func pohozheNaNastroyki(tekst string) bool {
	t := strings.TrimSpace(strings.TrimPrefix(tekst, "\xef\xbb\xbf"))
	if strings.HasPrefix(t, "{") || strings.HasPrefix(t, "[Interface]") {
		return true
	}
	for _, stroka := range strings.Split(t, "\n") {
		if strings.TrimSpace(stroka) == "proxies:" {
			return true
		}
	}
	return false
}

// AdresPodpiski узнаёт строку, которая сама адрес подписки, а не сервер.
func AdresPodpiski(stroka string) bool {
	s := strings.ToLower(strings.TrimSpace(stroka))
	return strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "http://")
}

// OtkazEksporta это сервер, который в ссылку не перевёлся.
type OtkazEksporta struct {
	Imya     string `json:"imya"`
	Prichina string `json:"prichina"`
}

// SobratSpisok собирает ссылки по строке. Серверы, которые в ссылку не
// переводятся, возвращаются отдельно: выгрузка не имеет права терять их молча.
//
// Сюда обязан приходить сервер из набора, а не экранная копия: dlyaEkrana
// вычищает ключи, и ссылка из неё собралась бы без единой ошибки и не работала.
func SobratSpisok(servery []protokol.Server) (ssylki []string, propushcheny []OtkazEksporta) {
	for _, s := range servery {
		ss, err := Sobrat(s)
		if err != nil {
			prichina := "ссылка не собралась"
			if errors.Is(err, ErrTransportNePodderzhan) {
				prichina = "этот вид сервера в ссылку не переводится"
			}
			propushcheny = append(propushcheny, OtkazEksporta{Imya: s.Imya, Prichina: prichina})
			continue
		}
		ssylki = append(ssylki, ss)
	}
	return ssylki, propushcheny
}
