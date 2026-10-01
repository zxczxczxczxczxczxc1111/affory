package main

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/sboi"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/ssylki"
)

// Живой случай 01.10.2026: человек добавил подписку, имя её сервера у него
// дома не находится. Окно показало английский текст Windows и пообещало
// «работают серверы из прошлого обновления» при нуле серверов.
func TestOtkazPodpiskiGovoritSlovamiCheloveka(t *testing.T) {
	s := podstavnaya(t, nil)
	otkazImeni := ssylki.OtkazSVidom(sboi.DNS, fmt.Errorf(
		"%w: dial tcp: lookup panel.example: getaddrinfow: This is usually a temporary error during hostname resolution",
		ssylki.ErrPodpiskaNedostupna))
	zhivaya := false
	s.zagruzitPodpisku = func(context.Context, string) (ssylki.Razbor, error) {
		if zhivaya {
			return ssylki.Razbor{Servery: []protokol.Server{vtoroyServer()}}, nil
		}
		return ssylki.Razbor{}, otkazImeni
	}
	tehnika := []string{"dial tcp", "getaddrinfow", "This is usually", "lookup"}
	bezTehniki := func(gde, tekst string) {
		t.Helper()
		for _, slovo := range tehnika {
			if strings.Contains(tekst, slovo) {
				t.Errorf("%s несёт технический текст %q: %s", gde, slovo, tekst)
			}
		}
	}

	zadatPodpisku(t, s, "https://panel.example/sub/1")
	o := vypolnit(t, s, "refreshSubscription", nil)
	if o.Oshib == nil || o.Oshib.Kod != protokol.KodSubscriptionUnreach {
		t.Fatalf("отказ %+v, ожидался subscription-unreachable", o.Oshib)
	}
	bezTehniki("отказ", o.Oshib.Tekst)
	if !strings.Contains(o.Oshib.Tekst, "не нашёлся по имени") {
		t.Errorf("шаг не назван: %s", o.Oshib.Tekst)
	}
	if !strings.Contains(o.Oshib.Tekst, "серверов из неё пока нет") || strings.Contains(o.Oshib.Tekst, "сохранены") {
		t.Errorf("при нуле серверов обещаны прежние: %s", o.Oshib.Tekst)
	}

	// Строка подписки на экране: та же причина, без техники и без хвоста про
	// серверы, число стоит рядом отдельно.
	spisok := vypolnit(t, s, "listSubscriptions", nil)
	if spisok.Oshib != nil {
		t.Fatalf("список подписок: %+v", spisok.Oshib)
	}
	stroka := string(spisok.Telo)
	bezTehniki("строка подписки", stroka)
	if !strings.Contains(stroka, "не нашёлся по имени") {
		t.Errorf("в строке подписки нет причины: %s", stroka)
	}

	// Зеркало: подписка однажды загрузилась, потом сорвалась. Теперь серверы
	// из неё есть, и сказать надо именно это.
	zhivaya = true
	if o := vypolnit(t, s, "refreshSubscription", nil); o.Oshib != nil {
		t.Fatalf("живая подписка не обновилась: %+v", o.Oshib)
	}
	zhivaya = false
	o = vypolnit(t, s, "refreshSubscription", nil)
	if o.Oshib == nil || !strings.Contains(o.Oshib.Tekst, "прежние серверы из неё сохранены") {
		t.Fatalf("при живых серверах не сказано, что они сохранены: %+v", o.Oshib)
	}
}

// Каждый шаг загрузки называется своими словами, и ни один не отдаёт текст
// ошибки как есть. Тест держит связку шаг -> фраза, чтобы новая ветка в
// sboi не уехала молча в общее «подписка не загрузилась».
func TestPrichinaPodpiskiPoShagam(t *testing.T) {
	nedostupna := func(vid sboi.Vid) error {
		return ssylki.OtkazSVidom(vid, fmt.Errorf("%w: raw english text", ssylki.ErrPodpiskaNedostupna))
	}
	sluchai := []struct {
		imya   string
		err    error
		zhdyom string
	}{
		{"dns", nedostupna(sboi.DNS), "не нашёлся по имени"},
		{"tcp", nedostupna(sboi.TCP), "не отвечает на подключение"},
		{"tls", nedostupna(sboi.TLS), "защищённое соединение"},
		{"srok", nedostupna(sboi.Srok), "не ответил вовремя"},
		{"dostup", ssylki.OtkazOtveta(403, fmt.Errorf("%w: код ответа 403", ssylki.ErrPodpiskaNedostupna)), "не принял ссылку"},
		{"404", ssylki.OtkazOtveta(404, fmt.Errorf("%w: код ответа 404", ssylki.ErrPodpiskaNedostupna)), "подписки нет"},
		{"502", ssylki.OtkazOtveta(502, fmt.Errorf("%w: код ответа 502", ssylki.ErrPodpiskaNedostupna)), "на своей стороне (код 502)"},
		{"obe-dorogi", otkazObeihDorog{nedostupna(sboi.TCP)}, "через VPN тоже не вышло"},
		{"velika", fmt.Errorf("%w: больше 1048576 байт", ssylki.ErrPodpiskaVelika), "слишком большое"},
	}
	for _, sl := range sluchai {
		t.Run(sl.imya, func(t *testing.T) {
			p := prichinaPodpiski(sl.err)
			if !strings.Contains(p, sl.zhdyom) {
				t.Errorf("%q, ожидалось %q", p, sl.zhdyom)
			}
			if strings.Contains(p, "raw english") || strings.Contains(p, "1048576") {
				t.Errorf("технический текст уехал человеку: %q", p)
			}
		})
	}
}

// Слишком большое тело это ответ по существу, а не недоступность: раньше оно
// падало в subscription-unreachable и звало проверять сеть.
func TestBolshayaPodpiskaNeNedostupna(t *testing.T) {
	err := fmt.Errorf("%w: больше 1048576 байт", ssylki.ErrPodpiskaVelika)
	if o := otkazPodpiski(protokol.Kadr{}, ssylki.Razbor{}, err); o.Oshib == nil || o.Oshib.Kod != protokol.KodSubscriptionMalformed {
		t.Fatalf("отказ %+v, ожидался subscription-malformed", o.Oshib)
	}
}
