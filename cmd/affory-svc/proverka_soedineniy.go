package main

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/yadra"
)

type snimokSoedineniy struct {
	Yadro       bool               `json:"yadro"`
	Vremya      time.Time          `json:"vremya"`
	Ogranichen  bool               `json:"ogranichen"`
	Soedineniya []yadra.Soedinenie `json:"soedineniya"`
}

// Разовый снимок не включает запись истории и не создаёт сетевых запросов
// к проверяемому домену. Отсутствие соединения не означает отказ маршрута.
func (s *Sluzhba) listConnections(ctx context.Context, k protokol.Kadr) protokol.Kadr {
	var q struct {
		Domen string `json:"domen"`
		Put   string `json:"put"`
	}
	if err := json.Unmarshal(k.Telo, &q); err != nil || len(q.Domen) > 253 || len(q.Put) > 32768 {
		return otkaz(k.Id, k.Imya, protokol.KodProtocolMismatch, "Некорректный фильтр соединений")
	}
	q.Domen = strings.Trim(strings.ToLower(strings.TrimSpace(q.Domen)), ".")
	q.Put = strings.TrimSpace(q.Put)
	adres, secret := s.dostupKKlash()
	result := snimokSoedineniy{Vremya: time.Now(), Soedineniya: []yadra.Soedinenie{}}
	if adres == "" {
		return otvet(k.Id, k.Imya, result)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	rows, err := s.soedineniyaYadra(ctx, adres, secret)
	if err != nil {
		return otkaz(k.Id, k.Imya, protokol.KodYadroNeOtvechaet, "Не удалось проверить соединения: "+err.Error())
	}
	current, currentSecret := s.dostupKKlash()
	if current != adres || currentSecret != secret {
		return otkaz(k.Id, k.Imya, protokol.KodYadroNeOtvechaet, "Подключение изменилось во время проверки. Проверь соединения ещё раз")
	}
	result.Yadro = true
	for _, c := range rows {
		host := strings.TrimSuffix(strings.ToLower(c.Host), ".")
		if q.Domen != "" && host != q.Domen && !strings.HasSuffix(host, "."+q.Domen) {
			continue
		}
		if q.Put != "" && !strings.EqualFold(c.Protsess, q.Put) {
			continue
		}
		if len(result.Soedineniya) == 200 {
			result.Ogranichen = true
			break
		}
		result.Soedineniya = append(result.Soedineniya, c)
	}
	return otvet(k.Id, k.Imya, result)
}
