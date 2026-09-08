package main

import (
	"context"
	"errors"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/set"
)

// Снимок состояния сервера (задача 6.4). Адрес снимка выводится из адреса
// подписки и наружу не отдаётся: в ответе только поля снимка и возраст.
// Порог свежести сутки, как в §9.1 (health-snapshot-stale): снимок пишется
// раз в сутки, и возраст больше суток показывается серым, не блокируя карточку.

const porogSvezhestiSnimka = 24 * 60 * 60

func (s *Sluzhba) getServerHealth(ctx context.Context, k protokol.Kadr) protokol.Kadr {
	n, err := s.nabor()
	if err != nil {
		return otkaz(k.Id, k.Imya, protokol.KodHealthSnapshotMissing, err.Error())
	}
	if n.Podpiska == "" {
		return otkaz(k.Id, k.Imya, protokol.KodHealthSnapshotMissing, "подписка не задана, снимок брать негде")
	}
	adres, err := set.AdresSnimka(n.Podpiska)
	if err != nil {
		return otkaz(k.Id, k.Imya, protokol.KodHealthSnapshotMissing, err.Error())
	}
	sn, err := s.zagruzitSnimok(ctx, adres)
	if err != nil {
		if errors.Is(err, set.ErrSnimkaNet) {
			return otkaz(k.Id, k.Imya, protokol.KodHealthSnapshotMissing, "рядом с подпиской нет файла состояния")
		}
		// Текст ошибки сети не несёт адреса: он собран в set без URL.
		return otkaz(k.Id, k.Imya, protokol.KodHealthSnapshotMissing, err.Error())
	}
	vozrast := s.seychas().Unix() - sn.Vremya
	if vozrast < 0 {
		vozrast = 0
	}
	return otvet(k.Id, k.Imya, map[string]any{
		"vremya":                          sn.Vremya,
		"vozrast_s":                       vozrast,
		"ustarel":                         vozrast > porogSvezhestiSnimka,
		"trevogi":                         sn.Trevogi,
		"dney_do_konca_sertifikata_maski": sn.DneySertifikata,
		"dney_s_obnovleniya_xray":         sn.DneySObnovleniya,
		"hy2_aktiven":                     sn.Hy2Aktiven,
		"dney_do_konca_sertifikata_hy2":   sn.DneySertifikataHy2,
		"xray_versiya":                    sn.XrayVersiya,
	})
}
