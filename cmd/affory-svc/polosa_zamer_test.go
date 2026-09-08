package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Команда measureBandwidth. Задача 8, часть В.
//
// Смысл в том, чтобы число для объявления hysteria2 бралось ИЗМЕРЕНИЕМ, а не
// полем ввода: Brutal шлёт ровно с объявленной скоростью, и цену завышения
// платит канал человека, который своей полосы не знает.

func TestPolosaBezMisheniEtoOtkazSKodom(t *testing.T) {
	// Мишень задаётся явно и умолчания не имеет: клиент не ходит самовольно на
	// чужой хост и не тратит трафик без спроса.
	s := podstavnaya(t, nil)
	k := s.Obrabotat(context.Background(), protokol.Kadr{
		Tip: "komanda", Id: 1, Imya: "measureBandwidth",
		Telo: json.RawMessage(`{}`),
	})
	if k.Oshib == nil {
		t.Fatal("замер без мишени принят")
	}
	if k.Oshib.Kod != protokol.KodPolosaNeIzmerena {
		t.Fatalf("код отказа %q, а ожидался %q", k.Oshib.Kod, protokol.KodPolosaNeIzmerena)
	}
}

func TestPolosaMeryaetIsovetuetSZapasomVniz(t *testing.T) {
	kusok := strings.Repeat("x", 64*1024)
	m := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for {
			if _, err := w.Write([]byte(kusok)); err != nil {
				return
			}
			select {
			case <-r.Context().Done():
				return
			default:
			}
		}
	}))
	defer m.Close()

	s := podstavnaya(t, nil)
	k := s.Obrabotat(context.Background(), protokol.Kadr{
		Tip: "komanda", Id: 1, Imya: "measureBandwidth",
		Telo: json.RawMessage(`{"adres":"` + m.URL + `","potokov":2,"sekund":1}`),
	})
	if k.Oshib != nil {
		t.Fatalf("отказ на живой мишени: %v", k.Oshib)
	}
	var o struct {
		MbitVniz  float64 `json:"mbit_vniz"`
		SovetVniz int     `json:"sovet_vniz"`

		CherezTunnel bool `json:"cherez_tunnel"`
	}
	if err := json.Unmarshal(k.Telo, &o); err != nil {
		t.Fatalf("ответ не разбирается: %v", err)
	}
	if o.MbitVniz <= 0 {
		t.Fatalf("полоса неположительная: %v", o.MbitVniz)
	}
	// Совет строго меньше замера: заниженное объявление работает
	// ограничителем и не ломает ничего, завышенное рвёт соединение.
	if float64(o.SovetVniz) >= o.MbitVniz {
		t.Fatalf("совет %d не ниже замера %.1f: запас вниз потерян", o.SovetVniz, o.MbitVniz)
	}
	if o.SovetVniz < 1 {
		t.Fatalf("советуется %d: объявленный ноль это Brutal с нулевой оценкой канала", o.SovetVniz)
	}
	// Прокси не поднят, значит замер честно говорит «мимо туннеля», а не
	// выдаёт канал за туннель.
	if o.CherezTunnel {
		t.Fatal("замер объявлен туннельным, хотя прокси не поднят")
	}
}

func TestPolosaMyortvayaMishenNeStanovitsyaNulyom(t *testing.T) {
	m := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer m.Close()

	s := podstavnaya(t, nil)
	k := s.Obrabotat(context.Background(), protokol.Kadr{
		Tip: "komanda", Id: 1, Imya: "measureBandwidth",
		Telo: json.RawMessage(`{"adres":"` + m.URL + `","potokov":1,"sekund":1}`),
	})
	if k.Oshib == nil {
		t.Fatal("мишень, не отдавшая ни байта, принята за измерение")
	}
}

// Отдача. Мишень заливки отдельная: файл на CDN отдаёт байты всякому, а
// принимает их редкий сервер, и адреса у направлений разные (проверено:
// speed.cloudflare.com отдаёт по __down, принимает по __up).

// mishenOtdachi принимает заливку и отвечает заданным кодом.
func mishenOtdachi(kod int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		w.WriteHeader(kod)
	}))
}

// mishenSkachivaniya отдаёт байты, пока её слушают.
func mishenSkachivaniya() *httptest.Server {
	kusok := strings.Repeat("x", 64*1024)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for {
			if _, err := w.Write([]byte(kusok)); err != nil {
				return
			}
			select {
			case <-r.Context().Done():
				return
			default:
			}
		}
	}))
}

type otvetZamera struct {
	MbitVniz   float64 `json:"mbit_vniz"`
	SovetVniz  int     `json:"sovet_vniz"`
	MbitVverh  float64 `json:"mbit_vverh"`
	SovetVverh int     `json:"sovet_vverh"`
	OtkazVverh string  `json:"otkaz_vverh"`
}

func TestPolosaMeryaetObaNapravleniya(t *testing.T) {
	vniz := mishenSkachivaniya()
	defer vniz.Close()
	vverh := mishenOtdachi(http.StatusOK)
	defer vverh.Close()

	s := podstavnaya(t, nil)
	k := s.Obrabotat(context.Background(), protokol.Kadr{
		Tip: "komanda", Id: 1, Imya: "measureBandwidth",
		Telo: json.RawMessage(`{"adres":"` + vniz.URL + `","adres_vverh":"` + vverh.URL + `","potokov":2,"sekund":1}`),
	})
	if k.Oshib != nil {
		t.Fatalf("отказ на живых мишенях: %v", k.Oshib)
	}
	var o otvetZamera
	if err := json.Unmarshal(k.Telo, &o); err != nil {
		t.Fatalf("ответ не разбирается: %v", err)
	}
	if o.MbitVniz <= 0 || o.MbitVverh <= 0 {
		t.Fatalf("измерено не в обе стороны: вниз %.1f, вверх %.1f", o.MbitVniz, o.MbitVverh)
	}
	if o.OtkazVverh != "" {
		t.Fatalf("живая мишень отдачи объявлена отказом: %s", o.OtkazVverh)
	}
	// Запас вниз обязателен в ОБЕ стороны: Brutal наказывает за завышение
	// любой из них, а не только приёма.
	if float64(o.SovetVverh) >= o.MbitVverh || o.SovetVverh < 1 {
		t.Fatalf("совет по отдаче %d при замере %.1f: запас вниз потерян", o.SovetVverh, o.MbitVverh)
	}
}

func TestPolosaBezMisheniVverhMeryaetTolkoVniz(t *testing.T) {
	// Мишень отдачи не задана это НЕ поломка: у человека может не быть сервера,
	// принимающего заливку. Замер приёма обязан состояться и сказать про вторую
	// сторону «не измерено», а не отказать целиком.
	vniz := mishenSkachivaniya()
	defer vniz.Close()

	s := podstavnaya(t, nil)
	k := s.Obrabotat(context.Background(), protokol.Kadr{
		Tip: "komanda", Id: 1, Imya: "measureBandwidth",
		Telo: json.RawMessage(`{"adres":"` + vniz.URL + `","potokov":2,"sekund":1}`),
	})
	if k.Oshib != nil {
		t.Fatalf("отказ при незаданной мишени отдачи: %v", k.Oshib)
	}
	var o otvetZamera
	if err := json.Unmarshal(k.Telo, &o); err != nil {
		t.Fatalf("ответ не разбирается: %v", err)
	}
	if o.MbitVniz <= 0 {
		t.Fatalf("приём не измерен: %.1f", o.MbitVniz)
	}
	if o.MbitVverh != 0 || o.SovetVverh != 0 {
		t.Fatalf("отдача выдана числом без мишени: %.1f, совет %d", o.MbitVverh, o.SovetVverh)
	}
	if o.OtkazVverh == "" {
		t.Fatal("отдача не измерена и молчит: пустое место читается как ноль")
	}
}

func TestPolosaOtkazZalivkiNeLomaetPriyom(t *testing.T) {
	// Обычная мишень скачивания на POST отвечает 405. Это отказ ОДНОЙ стороны,
	// и он не имеет права уносить с собой измеренный приём.
	vniz := mishenSkachivaniya()
	defer vniz.Close()
	vverh := mishenOtdachi(http.StatusMethodNotAllowed)
	defer vverh.Close()

	s := podstavnaya(t, nil)
	k := s.Obrabotat(context.Background(), protokol.Kadr{
		Tip: "komanda", Id: 1, Imya: "measureBandwidth",
		Telo: json.RawMessage(`{"adres":"` + vniz.URL + `","adres_vverh":"` + vverh.URL + `","potokov":1,"sekund":1}`),
	})
	if k.Oshib != nil {
		t.Fatalf("отказ заливки унёс весь замер: %v", k.Oshib)
	}
	var o otvetZamera
	if err := json.Unmarshal(k.Telo, &o); err != nil {
		t.Fatalf("ответ не разбирается: %v", err)
	}
	if o.MbitVniz <= 0 {
		t.Fatalf("приём потерян вместе с отказом отдачи: %.1f", o.MbitVniz)
	}
	if o.MbitVverh != 0 {
		t.Fatalf("отвергнутая заливка стала числом: %.1f", o.MbitVverh)
	}
	if !strings.Contains(o.OtkazVverh, "405") {
		t.Fatalf("причина отказа отдачи не названа: %q", o.OtkazVverh)
	}
}
