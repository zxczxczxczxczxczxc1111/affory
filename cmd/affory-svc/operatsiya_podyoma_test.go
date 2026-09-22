package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/diagnostika"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Подъём пишет в подробный журнал одну строку об операции: чем кончился, где
// сорвался, сколько длился и к какому серверу относился (A7).
//
// До этого исход подъёма жил в текстовом журнале командных строк, а числа - в
// подробном, и связать их можно было только по времени. Разбор 22.09.2026
// на этом и встал.

// idServeraDlyaZhurnala намеренно узнаваемый: тест проверяет, что он НЕ
// доезжает до журнала открытым текстом.
const idServeraDlyaZhurnala = "nl-amsterdam-77"

func sluzhbaSZhurnalomOperatsiy(t *testing.T, zamerOtvet error) (*Sluzhba, *bytes.Buffer) {
	t.Helper()
	s := podstavnaya(t, zamerOtvet)
	var b bytes.Buffer
	s.zhurnalDiag = diagnostika.NovyyZhurnal(&b)
	s.zhurnalDiag.ZadatSborku("1.2.0+test123")
	srv := serverProby()
	srv.Id = idServeraDlyaZhurnala
	s.nabor = func() (Nabor, error) {
		return Nabor{Servery: []protokol.Server{srv}, Vybran: srv.Id}, nil
	}
	return s, &b
}

// operatsiiPodyoma отдаёт строки журнала, которые относятся к подключению.
func operatsiiPodyoma(t *testing.T, b *bytes.Buffer) []diagnostika.Srez {
	t.Helper()
	var out []diagnostika.Srez
	for _, stroka := range strings.Split(strings.TrimRight(b.String(), "\n"), "\n") {
		if stroka == "" {
			continue
		}
		var s diagnostika.Srez
		if err := json.Unmarshal([]byte(stroka), &s); err != nil {
			t.Fatalf("строка журнала не разбирается: %v (%s)", err, stroka)
		}
		if s.Sobytie == "операция podklyuchenie" {
			out = append(out, s)
		}
	}
	return out
}

func odnaOperatsiyaPodyoma(t *testing.T, b *bytes.Buffer) diagnostika.Srez {
	t.Helper()
	o := operatsiiPodyoma(t, b)
	if len(o) != 1 {
		t.Fatalf("строк об операции подключения %d, ждали одну: %s", len(o), b.String())
	}
	return o[0]
}

func TestUdachnyyPodyomPishetOperatsiyu(t *testing.T) {
	s, b := sluzhbaSZhurnalomOperatsiy(t, nil)
	if err := s.Connect(context.Background()); err != nil {
		t.Fatalf("подъём не прошёл: %v", err)
	}
	o := odnaOperatsiyaPodyoma(t, b)
	if o.OpItog != "ok" {
		t.Errorf("итог %q, ждали ok", o.OpItog)
	}
	if o.OpShag != "" {
		t.Errorf("у успеха назван шаг %q", o.OpShag)
	}
	if o.OpId == "" {
		t.Error("у операции нет номера: две попытки подряд в журнале не различить")
	}
	if o.Sborka != "1.2.0+test123" {
		t.Errorf("сборка %q", o.Sborka)
	}
	if o.OpUzel != diagnostika.Obezlichit(idServeraDlyaZhurnala) {
		t.Errorf("отпечаток сервера %q", o.OpUzel)
	}
	if strings.Contains(b.String(), idServeraDlyaZhurnala) {
		t.Errorf("идентификатор сервера доехал до журнала открытым: %s", b.String())
	}
}

func TestOtkazProbyPishetsyaShagomProby(t *testing.T) {
	staryy := zhdatPodyoma
	zhdatPodyoma = 300 * time.Millisecond
	defer func() { zhdatPodyoma = staryy }()

	s, b := sluzhbaSZhurnalomOperatsiy(t, errors.New("исходящий не отвечает (код 504)"))
	if err := s.Connect(context.Background()); err == nil {
		t.Fatal("не несущий сервер объявлен поднятым")
	}
	o := odnaOperatsiyaPodyoma(t, b)
	// Итог тот же код, что уходит человеку: журнал и экран обязаны называть
	// один отказ одинаково.
	if o.OpItog != protokol.KodAllServersDown {
		t.Errorf("итог %q, ждали %q", o.OpItog, protokol.KodAllServersDown)
	}
	if o.OpShag != "proba" {
		t.Errorf("шаг %q, ждали proba: сорвалось на пробе, а не на подъёме адаптера", o.OpShag)
	}
}

func TestOtkazYadraPishetsyaShagomYadra(t *testing.T) {
	s, b := sluzhbaSZhurnalomOperatsiy(t, nil)
	s.zhdatKlash = func(ctx context.Context, adres, sekret string) error {
		return errors.New("clash_api не отвечает за 5s")
	}
	if err := s.Connect(context.Background()); err == nil {
		t.Fatal("молчащее ядро не помешало объявить подъём")
	}
	o := odnaOperatsiyaPodyoma(t, b)
	if o.OpShag != "yadro" {
		t.Errorf("шаг %q, ждали yadro", o.OpShag)
	}
	if o.OpItog != protokol.KodYadroNeOtvechaet {
		t.Errorf("итог %q, ждали %q", o.OpItog, protokol.KodYadroNeOtvechaet)
	}
}

// Отмена это не отказ. Записать её кодом отказа значит потом считать нажатие
// «отключить» аварией подъёма и искать несуществующую поломку.
func TestOtmenaPodyomaNePishetsyaOtkazom(t *testing.T) {
	staryy := zhdatPodyoma
	zhdatPodyoma = 5 * time.Second
	defer func() { zhdatPodyoma = staryy }()

	s, b := sluzhbaSZhurnalomOperatsiy(t, errors.New("проба не прошла"))
	gotovo := make(chan error, 1)
	go func() { gotovo <- s.Connect(context.Background()) }()

	zhdatSostoyanie(t, s, protokol.SostPodnimaetsya, 3*time.Second)
	s.Otklyuchit()

	select {
	case <-gotovo:
	case <-time.After(3 * time.Second):
		t.Fatal("connect не вышел через 3 с после отключения")
	}
	o := odnaOperatsiyaPodyoma(t, b)
	if o.OpItog != "otmena" {
		t.Errorf("итог %q, ждали otmena", o.OpItog)
	}
	if o.OpShag != "otmena" {
		t.Errorf("шаг %q, ждали otmena", o.OpShag)
	}
}
