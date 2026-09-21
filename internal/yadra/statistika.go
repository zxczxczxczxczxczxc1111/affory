package yadra

import (
	"context"
	"encoding/json"
	"fmt"
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

// Statistika собирает счётчики из /connections, своих проб не делает.
//
// Суммарные с момента подъёма ядра, а не посекундные из /traffic: на экране
// стоит «получено» и «отправлено», а не скорость.
func Statistika(ctx context.Context, adres, sekret string) (Snimok, error) {
	var sn Snimok

	telo, kod, err := zaprosS(ctx, http.MethodGet, fmt.Sprintf("http://%s/connections", adres), sekret, nil, 4<<20)
	if err != nil {
		return sn, err
	}
	switch {
	case kod == http.StatusUnauthorized || kod == http.StatusForbidden:
		return sn, sekretNePrinyat(adres, kod)
	case kod != http.StatusOK:
		return sn, fmt.Errorf("соединения ядра не отданы (код %d)", kod)
	}
	var sv struct {
		Prinyato uint64 `json:"downloadTotal"`
		Otdano   uint64 `json:"uploadTotal"`
	}
	if err := json.Unmarshal(telo, &sv); err != nil {
		return sn, fmt.Errorf("список соединений не разбирается: %w", err)
	}
	sn.Prinyato, sn.Otdano = sv.Prinyato, sv.Otdano
	return sn, nil
}
