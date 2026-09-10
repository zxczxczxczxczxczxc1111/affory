package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Решено 10.09.2026. Жалоба звучала как «affory-svc.exe не
// закрывается при выходе через трей», но служба это служба Windows и переживать
// окно обязана: на ней держатся автозапуск при входе и возврат после внезапной
// смерти (05.09.2026). Настоящий дефект был рядом: после выхода туннель
// оставался поднятым, весь трафик продолжал идти через него, значка в трее не
// было, и объяснения тоже.
//
// Принято: выход ОПУСКАЕТ туннель и оставляет службу.

func TestPunktVyhodaNazyvaetOtklyuchenie(t *testing.T) {
	podpis, _, opustit := punktVyhoda(protokol.SostPodnyat, false)
	if !opustit {
		t.Fatal("туннель поднят, а выход собрался уйти, не опустив его")
	}
	if !strings.Contains(strings.ToLower(podpis), "отключ") {
		t.Fatalf("подпись %q не называет отключение: человек нажмёт одно, получит другое", podpis)
	}
}

func TestPunktVyhodaNeObeshchaetLishnego(t *testing.T) {
	podpis, snyat, opustit := punktVyhoda(protokol.SostVyklyuchen, false)
	if snyat || opustit {
		t.Fatalf("туннель опущен и замка нет, а выход собрался снимать (%v) или опускать (%v)", snyat, opustit)
	}
	if podpis != "Выход" {
		t.Fatalf("подпись %q, ожидалось «Выход»", podpis)
	}
}

// Замок сильнее: он остаётся на машине и без нас, поэтому подпись обязана
// называть именно его. Опускание при этом всё равно происходит.
func TestPunktVyhodaSZamkomNazyvaetZashchitu(t *testing.T) {
	podpis, snyat, opustit := punktVyhoda(protokol.SostPodnyat, true)
	if !snyat || !opustit {
		t.Fatalf("замок и туннель на месте, а выход снимает (%v) и опускает (%v)", snyat, opustit)
	}
	if !strings.Contains(strings.ToLower(podpis), "защит") {
		t.Fatalf("подпись %q не называет снятие защиты", podpis)
	}
}

// Служба молчит, значит команду отправлять некуда, и обещать опускание нельзя.
func TestPunktVyhodaPriMolchashcheySluzhbe(t *testing.T) {
	podpis, _, opustit := punktVyhoda(protokol.SostSluzhbaMolchit, false)
	if opustit {
		t.Fatal("служба молчит, а выход собрался слать ей disconnect")
	}
	if podpis != "Выход" {
		t.Fatalf("подпись %q, ожидалось «Выход»", podpis)
	}
}

type stendVyhoda struct {
	t          *Trey
	snyato     int
	opushcheno int
	pokazano   int
	zakryto    int
}

func novyyStendVyhoda(sost protokol.Sostoyanie, zamok bool, snyatErr, opustitErr error) *stendVyhoda {
	s := &stendVyhoda{}
	s.t = &Trey{
		sost:  sost,
		zamok: zamok,
		snyatRezhim: func() error {
			s.snyato++
			return snyatErr
		},
		otklyuchit: func() error {
			s.opushcheno++
			return opustitErr
		},
		pokazat: func() { s.pokazano++ },
		zakryt:  func() { s.zakryto++ },
	}
	return s
}

func TestVyhodOpuskaetTunnelPeredZakrytiem(t *testing.T) {
	s := novyyStendVyhoda(protokol.SostPodnyat, false, nil, nil)
	s.t.vyytiSinhronno()
	if s.opushcheno != 1 {
		t.Fatalf("туннель опущен %d раз, ожидался один", s.opushcheno)
	}
	if s.zakryto != 1 {
		t.Fatalf("программа закрыта %d раз, ожидался один", s.zakryto)
	}
}

// Порядок обязателен: сначала замок, потом туннель. Опустить туннель под
// замком значит оставить человека без сети на всё время, пока он соображает,
// что произошло.
func TestVyhodSnimaetZamokDoOpuskaniya(t *testing.T) {
	var poryadok []string
	s := &stendVyhoda{}
	s.t = &Trey{
		sost:        protokol.SostPodnyat,
		zamok:       true,
		snyatRezhim: func() error { poryadok = append(poryadok, "zamok"); return nil },
		otklyuchit:  func() error { poryadok = append(poryadok, "tunnel"); return nil },
		pokazat:     func() {},
		zakryt:      func() { poryadok = append(poryadok, "zakryt") },
	}
	s.t.vyytiSinhronno()
	if len(poryadok) != 3 || poryadok[0] != "zamok" || poryadok[1] != "tunnel" || poryadok[2] != "zakryt" {
		t.Fatalf("порядок выхода %v, ожидался замок, туннель, закрытие", poryadok)
	}
}

// Неудача ОТМЕНЯЕТ выход и открывает окно, ровно как это уже сделано для замка.
// Уйти молча значило бы оставить человека с поднятым туннелем, без значка и без
// объяснения, то есть ровно в том положении, из-за которого это и чинится.
func TestVyhodOtmenyaetsyaKogdaTunnelNeOpustilsya(t *testing.T) {
	s := novyyStendVyhoda(protokol.SostPodnyat, false, nil, errors.New("служба отказала"))
	s.t.vyytiSinhronno()
	if s.zakryto != 0 {
		t.Fatal("туннель не опустился, а программа закрылась: человек остался с поднятым туннелем без значка")
	}
	if s.pokazano != 1 {
		t.Fatalf("окно показано %d раз: причину отказа человеку показать негде", s.pokazano)
	}
}

// Замок не снялся, значит до туннеля дело не доходит вовсе.
func TestVyhodNeTrogaetTunnelKogdaZamokNeSnyalsya(t *testing.T) {
	s := novyyStendVyhoda(protokol.SostPodnyat, true, errors.New("отказ"), nil)
	s.t.vyytiSinhronno()
	if s.opushcheno != 0 {
		t.Fatal("замок не снят, а туннель уже опускается: машина остаётся запертой и без сети")
	}
	if s.zakryto != 0 {
		t.Fatal("замок не снят, а программа закрылась")
	}
}
