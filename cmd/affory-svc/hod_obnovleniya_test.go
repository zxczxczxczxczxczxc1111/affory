package main

import (
	"encoding/json"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Обновление это минута, в которую окно раньше молчало.
//
// downloadUpdate качает два десятка мегабайт, сверяет хеш, распаковывает и
// только потом отвечает; всё это время интерфейс не показывал ничего, а затем
// служба намеренно останавливалась, статус пустел и окно подписывало версию
// словом «dev». Человек 13.09.2026 прочитал это как «меня перебросило на dev».
//
// Шаги идут событием, потому что ответ приходит один и в самом конце.
func TestObnovlenieShlyotHodShagami(t *testing.T) {
	s := sVersiey(t, "0.6.2")
	arhiv, hesh := arhivVPamyati(t)
	s.skachatFayl = vypuskVSeti("0.6.3", hesh, arhiv, nil)
	s.zapustitPodmenshchika = func(prog, novaya string) error { return nil }
	_, sob := s.Podpisatsya()

	sobrano := make(chan []protokol.Kadr, 1)
	go func() {
		var k []protokol.Kadr
		for kadr := range sob {
			if kadr.Imya == sobytieHodaObnovleniya {
				k = append(k, kadr)
			}
		}
		sobrano <- k
	}()

	o := s.Obrabotat(ctxAdmina(), protokol.Kadr{Tip: "cmd", Id: 1, Imya: "downloadUpdate"})
	if o.Oshib != nil {
		t.Fatalf("отказ: %+v", o.Oshib)
	}
	s.Otpisatsya(1)
	kadry := <-sobrano

	var shagi []string
	var sSkachano bool
	for _, k := range kadry {
		var h protokol.HodObnovleniya
		if err := json.Unmarshal(k.Telo, &h); err != nil {
			t.Fatalf("шаг не разобран: %v", err)
		}
		if len(shagi) == 0 || shagi[len(shagi)-1] != h.Shag {
			shagi = append(shagi, h.Shag)
		}
		if h.Shag == protokol.ShagSkachivanie && h.Vsego > 0 && h.Skachano > 0 {
			sSkachano = true
		}
		if h.Shag == protokol.ShagPodmena && h.Versiya != "0.6.3" {
			t.Fatalf("шаг подмены без версии выпуска: %+v", h)
		}
	}

	ozhid := []string{protokol.ShagSkachivanie, protokol.ShagSverka, protokol.ShagRaspakovka, protokol.ShagPodmena}
	if len(shagi) != len(ozhid) {
		t.Fatalf("шаги %v, ждали %v", shagi, ozhid)
	}
	for i := range ozhid {
		if shagi[i] != ozhid[i] {
			t.Fatalf("шаги %v, ждали %v", shagi, ozhid)
		}
	}
	if !sSkachano {
		t.Fatal("в шаге загрузки не было ни одной доли: полосе нечего показывать")
	}
}

// Отказ обязан гасить ход, иначе окно остаётся с полосой навсегда.
func TestOtkazObnovleniyaGasitHod(t *testing.T) {
	s := sVersiey(t, "0.6.2")
	arhiv, _ := arhivVPamyati(t)
	s.skachatFayl = vypuskVSeti("0.6.3", "0000000000000000000000000000000000000000000000000000000000000000", arhiv, nil)
	s.zapustitPodmenshchika = func(prog, novaya string) error { return nil }
	_, sob := s.Podpisatsya()

	posledniy := make(chan string, 1)
	go func() {
		shag := ""
		for kadr := range sob {
			if kadr.Imya == sobytieHodaObnovleniya {
				var h protokol.HodObnovleniya
				_ = json.Unmarshal(kadr.Telo, &h)
				shag = h.Shag
			}
		}
		posledniy <- shag
	}()

	if o := s.Obrabotat(ctxAdmina(), protokol.Kadr{Tip: "cmd", Id: 1, Imya: "downloadUpdate"}); o.Oshib == nil {
		t.Fatal("чужой хеш принят")
	}
	s.Otpisatsya(1)
	if shag := <-posledniy; shag != protokol.ShagOtkaz {
		t.Fatalf("последний шаг %q, а окно ждёт отказа, чтобы убрать полосу", shag)
	}
}
