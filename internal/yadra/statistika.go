package yadra

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// Snimok это счётчики под главным объектом экрана (§8.3 спеки).
//
// Задержки здесь НЕТ, и это решение 21.09.2026. Раньше она бралась отсюда:
// последний замер urltest, то есть дозвон через сервер вместе с рукопожатием,
// TLS с целью и запрос - три-пять кругов до другой страны одним числом. Рядом
// с пингом из игры или Discord такая цифра втрое больше при исправной связи, и
// человек читает её как беду. Круг меряется своим запросом через локальный
// прокси: set.Otklik.
type Snimok struct {
	Otdano   uint64
	Prinyato uint64
}

// potolokSoedineniy ограничивает чтение /connections. Список идёт потоком и
// в память целиком не ложится, так что потолок только страховка от чужого
// ядра: 64 МиБ это больше сотни тысяч соединений.
const potolokSoedineniy = 64 << 20

// Statistika собирает счётчики из /connections, своих проб не делает.
//
// Суммарные с момента подъёма ядра, а не посекундные из /traffic: на экране
// стоит «получено» и «отправлено», а не скорость.
//
// Тело читается потоком (L9 аудита 1.8.0). Прежде оно бралось целиком до
// 4 МиБ, и при большем списке соединений обрезанный JSON не разбирался:
// цифры на экране вставали ровно тогда, когда трафика больше всего.
func Statistika(ctx context.Context, adres, sekret string) (Snimok, error) {
	var sn Snimok
	do, otm := context.WithTimeout(ctx, srokZaprosa)
	defer otm()
	z, err := http.NewRequestWithContext(do, http.MethodGet, fmt.Sprintf("http://%s/connections", adres), nil)
	if err != nil {
		return sn, fmt.Errorf("запрос к clash_api не собран: %w", err)
	}
	z.Header.Set("Authorization", "Bearer "+sekret)
	o, err := klientKlash.Do(z)
	if err != nil {
		return sn, fmt.Errorf("clash_api не отвечает: %w", err)
	}
	defer o.Body.Close()
	switch {
	case o.StatusCode == http.StatusUnauthorized || o.StatusCode == http.StatusForbidden:
		return sn, sekretNePrinyat(adres, o.StatusCode)
	case o.StatusCode != http.StatusOK:
		return sn, fmt.Errorf("соединения ядра не отданы (код %d)", o.StatusCode)
	}
	sn, err = schetchikiIz(io.LimitReader(o.Body, potolokSoedineniy))
	if err != nil {
		return Snimok{}, fmt.Errorf("список соединений не разбирается: %w", err)
	}
	return sn, nil
}

// schetchikiIz достаёт downloadTotal и uploadTotal из объекта верхнего
// уровня. Остальные значения, список соединений тоже, пропускаются по
// лексемам и в память не ложатся. Порядок полей не важен, а когда оба
// счётчика прочитаны, дочитывать незачем. Недостающий счётчик это ноль,
// как было при разборе целиком.
func schetchikiIz(r io.Reader) (Snimok, error) {
	var sn Snimok
	d := json.NewDecoder(r)
	t, err := d.Token()
	if err != nil {
		return sn, err
	}
	if ogr, ok := t.(json.Delim); !ok || ogr != '{' {
		return sn, errors.New("ответ не объект")
	}
	var estPrinyato, estOtdano bool
	for d.More() {
		t, err := d.Token()
		if err != nil {
			return sn, err
		}
		switch t {
		case "downloadTotal":
			if err := d.Decode(&sn.Prinyato); err != nil {
				return sn, err
			}
			estPrinyato = true
		case "uploadTotal":
			if err := d.Decode(&sn.Otdano); err != nil {
				return sn, err
			}
			estOtdano = true
		default:
			if err := propustitZnachenie(d); err != nil {
				return sn, err
			}
		}
		if estPrinyato && estOtdano {
			return sn, nil
		}
	}
	return sn, nil
}

// propustitZnachenie проходит одно значение целиком, сколь угодно
// вложенное, не собирая его.
func propustitZnachenie(d *json.Decoder) error {
	glubina := 0
	for {
		t, err := d.Token()
		if err != nil {
			return err
		}
		if ogr, ok := t.(json.Delim); ok {
			switch ogr {
			case '{', '[':
				glubina++
			case '}', ']':
				glubina--
			}
		}
		if glubina == 0 {
			return nil
		}
	}
}
