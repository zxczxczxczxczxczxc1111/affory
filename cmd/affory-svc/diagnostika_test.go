package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/diagnostika"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Подключение, а не только устройство. Зелёный набор в internal/diagnostika
// доказывает, что срез собирается; здесь доказывается, что его КТО-ТО зовёт и
// что выключенная настройка молчит.

func TestVyklyuchennayaDiagnostikaNichegoNePishet(t *testing.T) {
	var b bytes.Buffer
	s := &Sluzhba{
		periodDiagnostiki: time.Millisecond,
		zhurnalDiag:       diagnostika.NovyyZhurnal(&b),
		istochnikiDiag: diagnostika.Istochniki{
			Seychas:      time.Now,
			Deskriptorov: func(int) (int, error) { return 1, nil },
			Porty:        func() (int, int, error) { return 1, 2, nil },
			Runtime:      func() (int, uint64) { return 1, 1 },
		},
	}
	s.snimok.Diagnostika = false

	ctx, otmena := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer otmena()
	s.sobiratDiagnostiku(ctx)

	if b.Len() != 0 {
		t.Fatalf("настройка выключена, а в журнал написано: %q", b.String())
	}
}

func TestVklyuchennayaDiagnostikaPishetSrezy(t *testing.T) {
	var b bytes.Buffer
	s := &Sluzhba{
		periodDiagnostiki: time.Millisecond,
		zhurnalDiag:       diagnostika.NovyyZhurnal(&b),
		istochnikiDiag: diagnostika.Istochniki{
			Seychas:      time.Now,
			PidYadra:     func() int { return 0 },
			Deskriptorov: func(int) (int, error) { return 3155, nil },
			Porty:        func() (int, int, error) { return 16380, 16384, nil },
			Runtime:      func() (int, uint64) { return 900, 60 << 20 },
		},
	}
	s.snimok.Diagnostika = true

	ctx, otmena := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer otmena()
	s.sobiratDiagnostiku(ctx)

	stroki := strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
	if len(stroki) < 2 {
		t.Fatalf("срезов %d, ждали хотя бы два: %q", len(stroki), b.String())
	}
	var srez diagnostika.Srez
	if err := json.Unmarshal([]byte(stroki[0]), &srez); err != nil {
		t.Fatalf("срез не разбирается: %v", err)
	}
	if srez.DeskriptorovSluzhby != 3155 || srez.PortovZanyato != 16380 {
		t.Fatalf("в журнал попало не то: %+v", srez)
	}
}

// Байты пишутся ДЕЛЬТОЙ за секунду, а не накопленным итогом с подъёма ядра.
// Разница двух больших чисел глазами не читается, а вопрос ровно про провал в
// одну секунду.
func TestBaytyPishutsyaDeltoyZaSekundu(t *testing.T) {
	var b bytes.Buffer
	nakoplenoVverh := uint64(1000)
	nakoplenoVniz := uint64(5000)
	s := &Sluzhba{
		periodDiagnostiki: time.Millisecond,
		zhurnalDiag:       diagnostika.NovyyZhurnal(&b),
	}
	s.snimok.Diagnostika = true
	s.istochnikiDiag = diagnostika.Istochniki{
		Seychas:      time.Now,
		PidYadra:     func() int { return 0 },
		Deskriptorov: func(int) (int, error) { return 10, nil },
		Porty:        func() (int, int, error) { return 1, 2, nil },
		Runtime:      func() (int, uint64) { return 1, 1 },
		Yadro: func() (diagnostika.Yadro, error) {
			nakoplenoVverh += 100
			nakoplenoVniz += 700
			return s.deltaBayt(nakoplenoVverh, nakoplenoVniz, 3, time.Millisecond), nil
		},
	}

	ctx, otmena := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer otmena()
	s.sobiratDiagnostiku(ctx)

	stroki := strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
	if len(stroki) < 3 {
		t.Fatalf("срезов мало для проверки дельты: %q", b.String())
	}
	// Первый срез не судим: до него накопленного не с чем сравнивать.
	var vtoroy diagnostika.Srez
	if err := json.Unmarshal([]byte(stroki[1]), &vtoroy); err != nil {
		t.Fatalf("срез не разбирается: %v", err)
	}
	if vtoroy.BaytVverh != 100 || vtoroy.BaytVniz != 700 {
		t.Fatalf("дельта посчитана неверно: вверх %d, вниз %d", vtoroy.BaytVverh, vtoroy.BaytVniz)
	}
}

// Настройка живёт на диске и переживает перезапуск: включать подробный журнал
// заново после каждого падения службы значит не поймать ни одного падения.
func TestNastroykaDiagnostikiPopadaetVStatus(t *testing.T) {
	s := &Sluzhba{}
	s.snimok.Diagnostika = true
	st := s.Status()
	if !st.Diagnostika {
		t.Fatalf("настройка включена, а статус молчит: %+v", st)
	}
}

var _ = protokol.StatusOtvet{}
