package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/set"
)

// Задача 6.3: checkExitIp и checkLeaks.

func sluzhbaDlyaProverok(t *testing.T, podnyat bool) *Sluzhba {
	t.Helper()
	s := podstavnaya(t, nil)
	s.sprositVyhod = func(ctx context.Context, endpoint string, port int) (string, error) {
		if port > 0 {
			return "192.0.2.10", nil
		}
		return "198.51.100.1", nil
	}
	s.ipv6Zaglushen = func() (bool, error) { return true, nil }
	if podnyat {
		// Доступ к clash_api, а не одно только состояние. Проверки теперь
		// спрашивают «живо ли ядро» по адресу clash_api, и фикстура, ставившая
		// podnyat при нулевом порту управления, изображала невозможное:
		// настоящий подъём записывает доступ до старта ядра.
		s.zapomnitKlash(52715, "sekret-stenda")
		s.mu.Lock()
		s.sost = protokol.SostPodnyat
		s.portProksiNash = 1080
		s.mu.Unlock()
	}
	return s
}

func TestCheckExitIpCherezProksiIZapominaetsya(t *testing.T) {
	s := sluzhbaDlyaProverok(t, true)
	o := s.Obrabotat(context.Background(), protokol.Kadr{Tip: "cmd", Id: 1, Imya: "checkExitIp"})
	if o.Oshib != nil {
		t.Fatalf("отказ: %+v", o.Oshib)
	}
	var telo struct {
		Adres  string `json:"adres"`
		Cherez string `json:"cherez"`
	}
	if err := json.Unmarshal(o.Telo, &telo); err != nil {
		t.Fatal(err)
	}
	if telo.Adres != "192.0.2.10" || telo.Cherez != "tunnel" {
		t.Fatalf("ответ %+v", telo)
	}
	s.mu.Lock()
	zapomnen := s.adresVyhoda
	s.mu.Unlock()
	if zapomnen != "192.0.2.10" {
		t.Fatalf("адрес выхода не запомнен для статистики: %q", zapomnen)
	}
}

func TestCheckExitIpBezTunnelyaMeryaetNapryamuyu(t *testing.T) {
	s := sluzhbaDlyaProverok(t, false)
	o := s.Obrabotat(context.Background(), protokol.Kadr{Tip: "cmd", Id: 1, Imya: "checkExitIp"})
	if o.Oshib != nil {
		t.Fatalf("отказ: %+v", o.Oshib)
	}
	if !strings.Contains(string(o.Telo), `"napryamuyu"`) || !strings.Contains(string(o.Telo), "198.51.100.1") {
		t.Fatalf("ответ %s", o.Telo)
	}
}

func TestCheckExitIpBezProksiPriTunneleOtkazyvaet(t *testing.T) {
	// Напрямую при поднятом туннеле мерить нельзя: свои процессы идут мимо
	// туннеля по правилу петли, и домашний адрес был бы замером не того.
	s := sluzhbaDlyaProverok(t, true)
	s.mu.Lock()
	s.portProksiNash = 0
	s.mu.Unlock()
	o := s.Obrabotat(context.Background(), protokol.Kadr{Tip: "cmd", Id: 1, Imya: "checkExitIp"})
	if o.Oshib == nil || o.Oshib.Kod != protokol.KodVyhodNeIzmeren {
		t.Fatalf("ожидался отказ %s, получено %+v", protokol.KodVyhodNeIzmeren, o)
	}
}

func TestCheckLeaksOtdayotPunktyIAdres(t *testing.T) {
	s := sluzhbaDlyaProverok(t, true)
	o := s.Obrabotat(context.Background(), protokol.Kadr{Tip: "cmd", Id: 1, Imya: "checkLeaks"})
	if o.Oshib != nil {
		t.Fatalf("отказ: %+v", o.Oshib)
	}
	var r set.RezultatProverki
	if err := json.Unmarshal(o.Telo, &r); err != nil {
		t.Fatal(err)
	}
	if len(r.Punkty) < 4 || r.AdresVyhoda != "192.0.2.10" {
		t.Fatalf("результат %+v", r)
	}
	nevidim := 0
	for _, p := range r.Punkty {
		if p.Itog == set.ItogNeVidim {
			nevidim++
		}
	}
	if nevidim == 0 {
		t.Fatal("проверка не назвала, чего она не видит")
	}
}

func TestPodyomObnovlyaetAdresVyhoda(t *testing.T) {
	// По нажатию И при смене сервера (§5): подъём и смена несущего дёргают
	// запрос сами, таймера нет.
	s := sluzhbaDlyaProverok(t, false)
	s.zapomnitKlash(52715, "sekret-stenda")
	s.mu.Lock()
	s.portProksiNash = 1080
	s.mu.Unlock()
	s.postavit(protokol.SostPodnyat, nil)
	srok := time.Now().Add(2 * time.Second)
	for time.Now().Before(srok) {
		s.mu.Lock()
		a := s.adresVyhoda
		s.mu.Unlock()
		if a == "192.0.2.10" {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("после подъёма адрес выхода не обновился")
}

// Проверка утечек обязана смотреть ВО ВРЕМЯ подъёма.
//
// Прежде вход проверки судил по состоянию и в podnimaetsya отвечал «туннель не
// поднят». Это ровно тот момент, где утечка вероятнее всего: адаптер уже
// поднят, правило IPv6 уже заведено, а трафик ещё может идти мимо. Критерий
// теперь один на весь проект: живо ли ядро, то есть есть ли адрес clash_api.
func TestProverkaUtechekSmotritVoVremyaPodyoma(t *testing.T) {
	s := sluzhbaDlyaProverok(t, true)
	s.postavit(protokol.SostPodnimaetsya, nil)

	if v := s.vhodProverki(false); !v.Podnyat {
		t.Fatal("вход проверки считает туннель опущенным при живом ядре")
	}
	o := s.Obrabotat(context.Background(), protokol.Kadr{Tip: "cmd", Id: 1, Imya: "checkExitIp"})
	if o.Oshib != nil {
		t.Fatalf("отказ: %+v", o.Oshib)
	}
	var telo struct {
		Adres  string `json:"adres"`
		Cherez string `json:"cherez"`
	}
	if err := json.Unmarshal(o.Telo, &telo); err != nil {
		t.Fatal(err)
	}
	if telo.Cherez != "tunnel" {
		t.Fatalf("адрес выхода спрошен %q: в подъёме спрашивать надо через туннель", telo.Cherez)
	}

	// Зеркало: ядро опущено, порт управления обнулён, и проверка обязана
	// вернуться к прямому запросу, даже если состояние ещё говорит podnyat.
	s.opustit()
	s.mu.Lock()
	s.sost = protokol.SostPodnyat
	s.mu.Unlock()
	if v := s.vhodProverki(false); v.Podnyat {
		t.Fatal("вход проверки поверил состоянию: ядра нет, спрашивать через него нечем")
	}
}

// Сразу после переподъёма ядро рвёт первые запросы через прокси. Адрес выхода
// обязан дойти до экрана со следующей попытки, а не ждать кнопки.
func TestAdresVyhodaPovtoryaetsyaPosleObryva(t *testing.T) {
	bylo := pauzaAdresaVyhoda
	pauzaAdresaVyhoda = time.Millisecond
	t.Cleanup(func() { pauzaAdresaVyhoda = bylo })
	s := sluzhbaDlyaProverok(t, true)
	zvonkov := 0
	s.sprositVyhod = func(context.Context, string, int) (string, error) {
		zvonkov++
		if zvonkov < 3 {
			return "", errors.New("wsarecv: An existing connection was forcibly closed by the remote host.")
		}
		return "192.0.2.10", nil
	}
	s.obnovitAdresVyhoda()
	s.mu.Lock()
	adres := s.adresVyhoda
	s.mu.Unlock()
	if adres != "192.0.2.10" || zvonkov != 3 {
		t.Fatalf("адрес %q после %d попыток", adres, zvonkov)
	}
}

// Туннель опустили между попытками: спрашивать дальше через него нечем.
func TestAdresVyhodaNeSprashivaetsyaPosleOpuskaniya(t *testing.T) {
	bylo := pauzaAdresaVyhoda
	pauzaAdresaVyhoda = time.Millisecond
	t.Cleanup(func() { pauzaAdresaVyhoda = bylo })
	s := sluzhbaDlyaProverok(t, true)
	zvonkov := 0
	s.sprositVyhod = func(context.Context, string, int) (string, error) {
		zvonkov++
		s.opustit()
		return "", errors.New("сеть пропала")
	}
	s.obnovitAdresVyhoda()
	if zvonkov != 1 {
		t.Fatalf("после опускания туннеля спросили ещё %d раз", zvonkov-1)
	}
}
