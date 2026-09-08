package main

import (
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Что показывать всплывашкой из трея, решает чистая функция от пары
// состояний (план «шесть удобств» §4). Правила: смена несущего под
// поднятым туннелем, срыв, восстановление. Подъём по кнопке человека не
// уведомление: он смотрит на окно.
func st(s protokol.Sostoyanie, nesushchiy, imya string, tekst string) protokol.StatusOtvet {
	o := protokol.StatusOtvet{Sostoyanie: s, NesushchiyId: nesushchiy, NesushchiyImya: imya}
	if tekst != "" {
		o.Oshib = &protokol.Oshibka{Kod: "x", Tekst: tekst}
	}
	return o
}

func TestUvedomleniyaPoPerehodam(t *testing.T) {
	sluchai := []struct {
		imya      string
		pred, nov protokol.StatusOtvet
		zagolovok string
	}{
		{"смена несущего под туннелем", st(protokol.SostPodnyat, "a", "vpn-pc-hy2", ""), st(protokol.SostPodnyat, "b", "vpn-pc-raw", ""), "несёт vpn-pc-raw"},
		{"первый несущий при подъёме молчит", st(protokol.SostPodnimaetsya, "", "", ""), st(protokol.SostPodnyat, "a", "vpn-pc-hy2", ""), ""},
		{"тот же несущий молчит", st(protokol.SostPodnyat, "a", "x", ""), st(protokol.SostPodnyat, "a", "x", ""), ""},
		{"срыв", st(protokol.SostPodnyat, "a", "x", ""), st(protokol.SostNeNeset, "", "", "туннель перестал нести трафик"), "туннель не несёт"},
		{"отказ подъёма", st(protokol.SostPodnimaetsya, "", "", ""), st(protokol.SostOtkaz, "", "", "серверов нет"), "подключиться не удалось"},
		{"восстановление", st(protokol.SostVosstanavl, "", "", ""), st(protokol.SostPodnyat, "a", "vpn-pc-hy2", ""), "туннель восстановлен"},
		{"отключение человеком молчит", st(protokol.SostPodnyat, "a", "x", ""), st(protokol.SostVyklyuchen, "", "", ""), ""},
		{"служба замолчала молчит", st(protokol.SostPodnyat, "a", "x", ""), st(protokol.SostSluzhbaMolchit, "", "", ""), ""},
		{"новая версия один раз", st(protokol.SostVyklyuchen, "", "", ""), sObnovleniem(st(protokol.SostVyklyuchen, "", "", ""), "0.6.3"), "есть обновление 0.6.3"},
		{"та же версия молчит", sObnovleniem(st(protokol.SostVyklyuchen, "", "", ""), "0.6.3"), sObnovleniem(st(protokol.SostVyklyuchen, "", "", ""), "0.6.3"), ""},
	}
	for _, c := range sluchai {
		u, est := chtoSoobshchit(c.pred, c.nov)
		if (c.zagolovok == "") != !est {
			t.Errorf("%s: уведомление есть=%v, ждали %q", c.imya, est, c.zagolovok)
			continue
		}
		if est && u.Zagolovok != c.zagolovok {
			t.Errorf("%s: заголовок %q, ждали %q", c.imya, u.Zagolovok, c.zagolovok)
		}
		if est && c.nov.Oshib != nil && u.Tekst != c.nov.Oshib.Tekst {
			t.Errorf("%s: текст %q, ждали причину %q", c.imya, u.Tekst, c.nov.Oshib.Tekst)
		}
	}
}

func sObnovleniem(o protokol.StatusOtvet, versiya string) protokol.StatusOtvet {
	o.Obnovlenie = &protokol.ObnovlenieOtvet{Versiya: versiya}
	return o
}
