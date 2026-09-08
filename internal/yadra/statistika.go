package yadra

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// Snimok это цифры под главным объектом экрана (§8.3 спеки).
//
// EstZaderzhka отдельно от числа намеренно: у только что поднятого ядра история
// пуста, и ноль вместо «не измерено» нарушил бы договор запасного пути, по
// которому экран не рисует ноль за неизмеренное.
type Snimok struct {
	Zaderzhka    time.Duration
	EstZaderzhka bool
	Otdano       uint64
	Prinyato     uint64
}

// Statistika собирает снимок из ДВУХ ответов clash_api, своих проб не делает.
//
// Задержка это последний замер urltest из истории исходящего: ядро меряет само,
// по своему расписанию, и просить его мерить ещё раз в секунду значило бы
// стучать в gstatic раз в секунду с каждой машины. Счётчики из /connections
// суммарные с момента подъёма ядра, а не посекундные из /traffic: на экране
// стоит «отдано» и «принято», а не скорость.
func Statistika(ctx context.Context, adres, sekret, teg string) (Snimok, error) {
	var sn Snimok

	telo, kod, err := sprositKlash(ctx, fmt.Sprintf("http://%s/proxies/%s", adres, url.PathEscape(teg)), sekret)
	if err != nil {
		return sn, err
	}
	switch {
	case kod == http.StatusUnauthorized || kod == http.StatusForbidden:
		return sn, sekretNePrinyat(adres, kod)
	case kod != http.StatusOK:
		return sn, fmt.Errorf("исходящий %s не описан ядром (код %d)", teg, kod)
	}
	var ish struct {
		Istoriya []struct {
			Zaderzhka int `json:"delay"`
		} `json:"history"`
	}
	if err := json.Unmarshal(telo, &ish); err != nil {
		return sn, fmt.Errorf("описание исходящего не разбирается: %w", err)
	}
	if n := len(ish.Istoriya); n > 0 {
		sn.Zaderzhka = time.Duration(ish.Istoriya[n-1].Zaderzhka) * time.Millisecond
		sn.EstZaderzhka = true
	}

	telo, kod, err = zaprosS(ctx, http.MethodGet, fmt.Sprintf("http://%s/connections", adres), sekret, nil, 4<<20)
	if err != nil {
		return sn, err
	}
	if kod != http.StatusOK {
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
