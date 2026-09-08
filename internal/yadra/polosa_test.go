package yadra_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/yadra"
)

// TestPolosaSchitaetSkachannoe: замер отдаёт положительное число байт в
// секунду, когда мишень отдаёт байты.
func TestPolosaSchitaetSkachannoe(t *testing.T) {
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

	itog, err := yadra.ZamerPolosy(context.Background(), yadra.VhodPolosy{
		Adres: m.URL, Potokov: 2, Srok: 700 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("замер не состоялся: %v", err)
	}
	if itog.Mbit <= 0 {
		t.Fatalf("полоса неположительная: %v", itog.Mbit)
	}
	if itog.Bayt == 0 {
		t.Fatal("байт не насчитано, а число выдано")
	}
}

// TestPolosaNegodnyyVhodEtoOtkazDoSeti: проверка входа стоит ДО первого
// запроса. Замер с нулём потоков или пустым адресом обязан отказать сразу, а
// не «померить» ноль и выдать его за полосу канала.
func TestPolosaNegodnyyVhodEtoOtkazDoSeti(t *testing.T) {
	plohie := []yadra.VhodPolosy{
		{Adres: "", Potokov: 1, Srok: time.Second},
		{Adres: "ftp://example.org/f", Potokov: 1, Srok: time.Second},
		{Adres: "http://example.org/f", Potokov: 0, Srok: time.Second},
		{Adres: "http://example.org/f", Potokov: 99, Srok: time.Second},
		{Adres: "http://example.org/f", Potokov: 1, Srok: 0},
		{Adres: "http://example.org/f", Potokov: 1, Srok: time.Hour},
	}
	for _, v := range plohie {
		if _, err := yadra.ZamerPolosy(context.Background(), v); err == nil {
			t.Fatalf("негодный вход принят: %+v", v)
		}
	}
}

// TestPolosaMyortvayaMishenEtoOtkazANeNol: ноль, выданный за измерение, стал бы
// объявленной полосой hysteria2, а объявленный ноль это Brutal с нулевой
// оценкой канала.
func TestPolosaMyortvayaMishenEtoOtkazANeNol(t *testing.T) {
	m := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer m.Close()

	if _, err := yadra.ZamerPolosy(context.Background(), yadra.VhodPolosy{
		Adres: m.URL, Potokov: 2, Srok: 500 * time.Millisecond,
	}); err == nil {
		t.Fatal("мишень, не отдавшая ни байта, принята за измерение")
	}
}

// TestPolosaUvazhaetOtmenu: отменённый замер возвращается сразу, а не досиживает
// свой срок.
func TestPolosaUvazhaetOtmenu(t *testing.T) {
	kusok := strings.Repeat("x", 32*1024)
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

	ctx, otmena := context.WithCancel(context.Background())
	go func() {
		time.Sleep(200 * time.Millisecond)
		otmena()
	}()
	nachalo := time.Now()
	if _, err := yadra.ZamerPolosy(ctx, yadra.VhodPolosy{
		Adres: m.URL, Potokov: 2, Srok: 20 * time.Second,
	}); err == nil {
		t.Fatal("отменённый замер вернул успех")
	}
	if proshlo := time.Since(nachalo); proshlo > 5*time.Second {
		t.Fatalf("отмена не замечена, ждали %v", proshlo)
	}
}

// Отдача. Второе направление того же замера, и оно НЕ симметрично первому.
//
// Скачивание меряется повторными GET, отдача повторными POST с телом. Разница
// не косметическая: мишень скачивания отдаёт байты всякому, а мишень заливки их
// принимает, и таких в интернете куда меньше. Значит отказ «не принимает
// заливку» это штатный ответ, который обязан отличаться от нуля.

// TestOtdachaSchitaetOtdannoe: замер отдаёт положительное число, когда мишень
// принимает байты.
func TestOtdachaSchitaetOtdannoe(t *testing.T) {
	var prinyato int64
	m := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n, _ := io.Copy(io.Discard, r.Body)
		atomic.AddInt64(&prinyato, n)
		w.WriteHeader(http.StatusOK)
	}))
	defer m.Close()

	itog, err := yadra.ZamerOtdachi(context.Background(), yadra.VhodPolosy{
		Adres: m.URL, Potokov: 2, Srok: 700 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("замер отдачи не состоялся: %v", err)
	}
	if itog.Mbit <= 0 || itog.Bayt == 0 {
		t.Fatalf("отдача неположительная: %v Мбит, %d байт", itog.Mbit, itog.Bayt)
	}
	if atomic.LoadInt64(&prinyato) == 0 {
		t.Fatal("мишень не приняла ни байта, а число выдано")
	}
}

// TestOtdachaMishenNePrinimaetZalivku: 405 это ОТКАЗ, а не ноль Мбит.
//
// Обычная мишень скачивания (файл на CDN) на POST отвечает 405 или 404. Если
// такой ответ посчитать нулём, ноль уедет в объявление hysteria2, а объявленный
// ноль это Brutal с нулевой оценкой канала.
func TestOtdachaMishenNePrinimaetZalivku(t *testing.T) {
	for _, kod := range []int{http.StatusMethodNotAllowed, http.StatusNotFound, http.StatusForbidden} {
		m := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			io.Copy(io.Discard, r.Body)
			w.WriteHeader(kod)
		}))
		_, err := yadra.ZamerOtdachi(context.Background(), yadra.VhodPolosy{
			Adres: m.URL, Potokov: 1, Srok: 500 * time.Millisecond,
		})
		m.Close()
		if err == nil {
			t.Fatalf("мишень с кодом %d принята за измерение отдачи", kod)
		}
	}
}

// TestOtdachaPorciyaImeetDlinu: тело шлётся порциями с известной длиной, а не
// бесконечным chunked.
//
// Мишень, требующая Content-Length, отвергает chunked целиком, и проверено это
// на живой мишени: speed.cloudflare.com/__up принял мегабайт именно с длиной.
func TestOtdachaPorciyaImeetDlinu(t *testing.T) {
	var bezDliny, chunked atomic.Bool
	var zaprosov atomic.Int64
	m := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		zaprosov.Add(1)
		if r.ContentLength <= 0 {
			bezDliny.Store(true)
		}
		if len(r.TransferEncoding) > 0 {
			chunked.Store(true)
		}
		io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer m.Close()

	if _, err := yadra.ZamerOtdachi(context.Background(), yadra.VhodPolosy{
		Adres: m.URL, Potokov: 1, Srok: 600 * time.Millisecond,
	}); err != nil {
		t.Fatalf("замер отдачи не состоялся: %v", err)
	}
	// Ноль запросов это НЕ «нарушений не найдено». Без этой строки тест
	// зеленеет на замере, который не отправил ничего вовсе.
	if zaprosov.Load() == 0 {
		t.Fatal("мишень не увидела ни одного запроса")
	}
	if bezDliny.Load() {
		t.Fatal("порция ушла без Content-Length")
	}
	if chunked.Load() {
		t.Fatal("порция ушла chunked")
	}
}

// TestOtdachaUvazhaetOtmenu: отмена это отказ ВСЕГДА, даже когда байты уже
// насчитаны. Число за неполный срок ушло бы в объявление как полоса канала.
func TestOtdachaUvazhaetOtmenu(t *testing.T) {
	m := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer m.Close()

	ctx, otmena := context.WithCancel(context.Background())
	go func() {
		time.Sleep(200 * time.Millisecond)
		otmena()
	}()
	nachalo := time.Now()
	if _, err := yadra.ZamerOtdachi(ctx, yadra.VhodPolosy{
		Adres: m.URL, Potokov: 2, Srok: 20 * time.Second,
	}); err == nil {
		t.Fatal("отменённый замер отдачи вернул успех")
	}
	if proshlo := time.Since(nachalo); proshlo > 5*time.Second {
		t.Fatalf("отмена не замечена, ждали %v", proshlo)
	}
}

// TestOtdachaIdyotCherezProksi: путь замера тот же, что у трафика человека.
//
// Замер отдачи мимо туннеля мерил бы канал до провайдера, а объявляется полоса
// ТУННЕЛЯ. Ошибка тихая: число получается больше настоящего, то есть ровно то
// завышение, которое ломает Brutal.
func TestOtdachaIdyotCherezProksi(t *testing.T) {
	var cherezProksi atomic.Bool
	proksi := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Прокси видит АБСОЛЮТНЫЙ URI: так отличается проход через него от
		// прямого запроса на тот же порт.
		if r.RequestURI != "" && strings.HasPrefix(r.RequestURI, "http://") {
			cherezProksi.Store(true)
		}
		io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer proksi.Close()

	if _, err := yadra.ZamerOtdachi(context.Background(), yadra.VhodPolosy{
		Adres:   "http://mishen.nedostupna.invalid/up",
		Proksi:  strings.TrimPrefix(proksi.URL, "http://"),
		Potokov: 1, Srok: 500 * time.Millisecond,
	}); err != nil {
		t.Fatalf("замер через прокси не состоялся: %v", err)
	}
	if !cherezProksi.Load() {
		t.Fatal("замер отдачи пошёл мимо прокси")
	}
}
