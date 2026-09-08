package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/set"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/yadra"
)

// Служба с НАСТОЯЩЕЙ пробой и подставным clash_api на петле.
//
// Шов zamerit здесь не подменяется намеренно: проверяется весь путь от кода
// HTTP до кода отказа на проводе. Подмена шва проверяла бы только вторую его
// половину и промолчала бы, если бы Zaderzhka перестала называть причину.
func sZhivoyProboy(t *testing.T, ruka http.HandlerFunc) (*Sluzhba, int) {
	t.Helper()
	srv := httptest.NewServer(ruka)
	t.Cleanup(srv.Close)
	_, port, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	nomer, err := strconv.Atoi(port)
	if err != nil {
		t.Fatal(err)
	}

	s := podstavnaya(t, nil)
	s.zamerit = yadra.Zaderzhka
	s.podnyatTunnel = func(ctx context.Context) (set.Adapter, error) {
		s.zapomnitKlash(nomer, "sekret-stenda")
		return set.Adapter{
			Indeks: 10, Imya: "tun0", Opisanie: "sing-tun Tunnel",
			Adresa: []netip.Addr{netip.MustParseAddr("172.19.0.1")},
		}, nil
	}
	return s, nomer
}

// Проба отвечает так же, как живое ядро на мёртвом исходящем: не-200.
func otvergaetProbu(w http.ResponseWriter, r *http.Request) {
	if strings.HasSuffix(r.URL.Path, "/delay") {
		http.Error(w, `{"message":"An error occurred in the delay test"}`,
			http.StatusServiceUnavailable)
		return
	}
	// PUT /proxies/<группа>: 204 это «команда принята». Переключение обязано
	// дойти до пробы, иначе тест судил бы не то место.
	w.WriteHeader(http.StatusNoContent)
}

// Подъём быстрее пятнадцати секунд: тест судит КОД отказа, а не терпение.
func korotkiyPodyom(t *testing.T) {
	t.Helper()
	srokBylo, popytokBylo := zhdatPodyoma, popytokPodyoma
	zhdatPodyoma, popytokPodyoma = 300*time.Millisecond, 1
	t.Cleanup(func() { zhdatPodyoma, popytokPodyoma = srokBylo, popytokBylo })
}

// Сервер отверг ключи, и человеку надо смотреть на подписку, а не на сеть.
//
// Прежде это схлопывалось в all-servers-down, то есть в готовый экран «проверь
// сеть» при исправной сети. Экран «сервер не принял ключ, проверь подписку» был
// написан и не показывался ни разу: кода server-auth-failed не слал никто.
func TestOtvergnutyyServerNeNazyvaetsyaObshcheyNedostupnostyu(t *testing.T) {
	korotkiyPodyom(t)
	s, _ := sZhivoyProboy(t, otvergaetProbu)

	o := s.Obrabotat(ctxAdmina(), protokol.Kadr{Tip: "cmd", Id: 1, Imya: "connect"})
	if o.Oshib == nil {
		t.Fatal("подъём объявлен удачным при отвергнутой пробе")
	}
	if kod := o.Oshib.Kod; kod != protokol.KodServerAuthFailed {
		t.Errorf("код отказа %q, ожидали server-auth-failed", kod)
	}
}

// Ограничение полосы: причина ДРУГАЯ, значит и код прежний. Без этого судьи
// починка назвала бы отвергнутым сервером любое молчание сети.
func TestMolchashchayaProbaOstayotsyaObshcheyNedostupnostyu(t *testing.T) {
	korotkiyPodyom(t)
	s := podstavnaya(t, errors.New("проба не дошла"))

	o := s.Obrabotat(ctxAdmina(), protokol.Kadr{Tip: "cmd", Id: 1, Imya: "connect"})
	if o.Oshib == nil {
		t.Fatal("подъём объявлен удачным при провалившейся пробе")
	}
	if kod := o.Oshib.Kod; kod != protokol.KodAllServersDown {
		t.Errorf("код отказа %q, ожидали all-servers-down", kod)
	}
}

// Второй потребитель той же ошибки: живое переключение. Ядро приняло PUT, а
// проба через новый выбор упёрлась в отвергнутое рукопожатие. Прежде это
// уезжало как switch-target-not-carrying, то есть «выбери другой сервер» там,
// где другой сервер не поможет: ключи-то из той же подписки.
func TestPereklyuchenieNaOtvergnutyyServerNazyvaetPrichinu(t *testing.T) {
	s, port := sZhivoyProboy(t, otvergaetProbu)
	// Ядро живо: без этого setServer только запишет выбор и в пробу не пойдёт.
	s.zapomnitKlash(port, "sekret-stenda")

	o := s.Obrabotat(ctxAdmina(), protokol.Kadr{
		Tip: "cmd", Id: 1, Imya: "setServer", Telo: []byte(`{"id":"nl"}`),
	})
	if o.Oshib == nil {
		t.Fatal("переключение объявлено удачным при отвергнутой пробе")
	}
	if kod := o.Oshib.Kod; kod != protokol.KodServerAuthFailed {
		t.Errorf("код отказа %q, ожидали server-auth-failed", kod)
	}
}

// Ответ команды и баннер состояния обязаны говорить ОДНО И ТО ЖЕ.
//
// Ответ отказа человек видит один раз, а состояние живёт на Главной до
// следующей команды. Разойдясь, они дают экран, где сверху «сервер не принял
// ключ», а снизу «ни один сервер не отвечает», и человек выбирает, чему верить.
func TestSostoyaniePosleOtvergnutoyProbyNazyvaetTuZhePrichinu(t *testing.T) {
	korotkiyPodyom(t)
	s, _ := sZhivoyProboy(t, otvergaetProbu)

	if err := s.Connect(context.Background()); err == nil {
		t.Fatal("подъём объявлен удачным при отвергнутой пробе")
	}
	st := s.Status()
	if st.Sostoyanie != protokol.SostNeNeset {
		t.Fatalf("состояние %s, ожидалось ne-neset", st.Sostoyanie)
	}
	if st.Oshib == nil || st.Oshib.Kod != protokol.KodServerAuthFailed {
		t.Errorf("код в состоянии %v, ожидали server-auth-failed", st.Oshib)
	}
}

// Ступень 1 kodPodklyucheniya судится отдельно от состояния НАМЕРЕННО.
//
// Состояние пишут все: наблюдатель, восстановление, пересборка правил. Ответ
// команды обязан описывать то, обо что споткнулась ЭТА команда, а не последнее
// случившееся в службе. Проверять ступень 1 через живой подъём нечем: там
// состояние и ошибка совпадают, и подмена одного другим прошла бы зелёной.
func TestKodPodklyucheniyaBerotPrichinuANeChuzhoeSostoyanie(t *testing.T) {
	prichina := fmt.Errorf("туннель не понёс трафик: %w", yadra.ErrServerOtvergKlyuchi)
	chuzhoe := &protokol.Oshibka{Kod: protokol.KodFirewallFailed, Tekst: "правила отстали"}
	if kod := kodPodklyucheniya(prichina, chuzhoe); kod != protokol.KodServerAuthFailed {
		t.Errorf("код %q, ожидали server-auth-failed: причина типизирована, а взято состояние", kod)
	}
}
