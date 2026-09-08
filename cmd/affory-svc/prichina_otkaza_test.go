package main

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/set"
)

// Разовый all-servers-down 03.09.2026 остался неразобранным по двум причинам,
// и обе здесь закрываются проверками.
//
// Первая: диспетчер переписывал ЛЮБУЮ ошибку Connect в all-servers-down. Код,
// который служба честно поставила в состояние (ключи не читаются, ядро не
// ответило, брандмауэр не дался), до человека не доезжал вовсе.
//
// Вторая: отказной путь Connect не писал в журнал ни строки. Пустой журнал в
// момент отказа был не загадкой, а гарантией кода.

// zhurnalProgona перехватывает журнал на время одного теста.
func zhurnalProgona(t *testing.T) *bytes.Buffer {
	t.Helper()
	var b bytes.Buffer
	pred := log.Writer()
	prefiks := log.Prefix()
	flagi := log.Flags()
	log.SetOutput(&b)
	t.Cleanup(func() { log.SetOutput(pred); log.SetPrefix(prefiks); log.SetFlags(flagi) })
	return &b
}

func TestOtkazConnectaNeSlivaetsyaVAllServersDown(t *testing.T) {
	s := podstavnaya(t, nil)
	s.nabor = func() (Nabor, error) { return Nabor{}, errors.New("secrets.dat не расшифрован") }

	o := s.Obrabotat(context.Background(), protokol.Kadr{Tip: "cmd", Id: 1, Imya: "connect"})
	if o.Oshib == nil {
		t.Fatal("connect с нечитаемыми ключами прошёл успехом")
	}
	if o.Oshib.Kod != protokol.KodSecretsUnreadable {
		t.Fatalf("код %q, ждали %q: настоящая причина потеряна на границе протокола",
			o.Oshib.Kod, protokol.KodSecretsUnreadable)
	}
}

func TestNegotovoeYadroNeVydayotsyaZaSlomannyyTun(t *testing.T) {
	s := podstavnaya(t, nil)
	s.zhdatKlash = func(ctx context.Context, adres, sekret string) error {
		return errors.New("clash_api не отвечает за 5s")
	}
	o := s.Obrabotat(context.Background(), protokol.Kadr{Tip: "cmd", Id: 1, Imya: "connect"})
	if o.Oshib == nil {
		t.Fatal("неготовое ядро прошло успехом")
	}
	// «TUN не создался» это неверный диагноз: адаптер как раз создан, молчит ядро.
	if o.Oshib.Kod != protokol.KodYadroNeOtvechaet {
		t.Fatalf("код %q, ждали %q", o.Oshib.Kod, protokol.KodYadroNeOtvechaet)
	}
}

func TestKazhdayaProbaPopadaetVZhurnalSPrichinoy(t *testing.T) {
	zh := zhurnalProgona(t)
	s := podstavnaya(t, nil)
	staryy := zhdatPodyoma
	zhdatPodyoma = 700 * time.Millisecond
	defer func() { zhdatPodyoma = staryy }()
	s.zhdatKlash = func(ctx context.Context, adres, sekret string) error { return nil }
	prob := 0
	s.zamerit = func(ctx context.Context, adres, sekret, teg string) (time.Duration, error) {
		prob++
		return 0, errors.New("исходящий vybor не отвечает (код 503)")
	}
	if err := s.Connect(context.Background()); err == nil {
		t.Fatal("мёртвая проба дала успех")
	}
	tekst := zh.String()
	// Каждая проба своей строкой: две пробы за 15 с и полсотни это разные
	// диагнозы, и различить их можно только по числу строк.
	if n := strings.Count(tekst, "проба "); n < prob {
		t.Fatalf("проб было %d, строк в журнале %d:\n%s", prob, n, tekst)
	}
	if !strings.Contains(tekst, "код 503") {
		t.Fatalf("причина пробы не попала в журнал:\n%s", tekst)
	}
	// Итог обязан быть в журнале сам по себе: раньше на этом месте не было
	// ни одной строки, и разбирать отказ было нечем.
	if !strings.Contains(tekst, protokol.KodAllServersDown) {
		t.Fatalf("вердикт отказа не записан:\n%s", tekst)
	}
	// Исход КАЖДОЙ попытки, включая последнюю: прежде последняя молчала.
	if n := strings.Count(tekst, "попытка подъёма"); n != popytokPodyoma {
		t.Fatalf("строк про попытки %d, попыток %d:\n%s", n, popytokPodyoma, tekst)
	}
}

func TestOtkazPodyomaTunnelyaPopadaetVZhurnal(t *testing.T) {
	zh := zhurnalProgona(t)
	s := podstavnaya(t, nil)
	s.podnyatTunnel = func(ctx context.Context) (set.Adapter, error) {
		return set.Adapter{}, errors.New("адаптер tun0 не появился за 20s")
	}
	if err := s.Connect(context.Background()); err == nil {
		t.Fatal("несозданный туннель дал успех")
	}
	if !strings.Contains(zh.String(), "не появился") {
		t.Fatalf("причина отказа туннеля не записана:\n%s", zh.String())
	}
}
