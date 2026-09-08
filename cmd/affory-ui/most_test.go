package main

import (
	"bytes"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

func TestSluzhbaUstanovlenaSprashivaetDispetcherSluzhb(t *testing.T) {
	// The bridge must tell "no service" apart from "service not answering"
	// without admin rights: SC_MANAGER_CONNECT plus SERVICE_QUERY_STATUS is
	// enough for any user. A name that cannot exist must come back false, not
	// as an error: an error here would turn every first run into a red screen.
	est, err := sluzhbaUstanovlena("Affory-takoy-sluzhby-net-" + t.Name())
	if err != nil {
		t.Fatalf("несуществующая служба дала ошибку вместо false: %v", err)
	}
	if est {
		t.Fatal("несуществующая служба найдена")
	}
	// A service every Windows has. Not AfforySvc: the test must pass on a
	// machine where Affory was never installed.
	est, err = sluzhbaUstanovlena("EventLog")
	if err != nil {
		t.Fatal(err)
	}
	if !est {
		t.Fatal("EventLog не найден: диспетчер служб не опрошен")
	}
}

// A refusal is an answer, not a dead pipe. kanal.Klient.Zvat hands a refusal
// back as (frame with oshibka, error), and until 02.09.2026 the bridge took
// every error for a transport failure: it closed the pipe, told the tray the
// service was silent, and the screen never saw a single refusal frame (the
// first-run poll of subscribeStats killed the pipe on every mount, and a
// concurrent status call died with "канал закрыт").
func TestOtkazNeObryvKanala(t *testing.T) {
	otkaz := protokol.Kadr{Tip: "otvet", Id: 1, Imya: "subscribeStats",
		Oshib: &protokol.Oshibka{Kod: "not-implemented", Tekst: "в волне 6"}}
	if obryvKanala(otkaz, errors.New("not-implemented: в волне 6")) {
		t.Fatal("отказ службы принят за обрыв канала: клиент будет закрыт зря")
	}
	if !obryvKanala(protokol.Kadr{}, errors.New("канал закрыт")) {
		t.Fatal("ошибка без кадра ответа это обрыв, а не отказ")
	}
	if obryvKanala(protokol.Kadr{Tip: "otvet", Id: 2, Imya: "status"}, nil) {
		t.Fatal("успешный ответ принят за обрыв")
	}
}

// Отмена диалога это пустой путь без ошибки, всё остальное это ошибка.
//
// До 03.09.2026 VybratArhiv превращал ЛЮБОЙ отказ диалога в пустую строку, а
// окно на пустую строку не делало ничего: кнопка «Выбрать архив» молча не
// работала, и отличить это от «человек передумал» было нельзя ни на экране,
// ни в журнале.
func TestOtvetDialogaOtlichaetOtmenuOtPolomki(t *testing.T) {
	put, err := otvetDialoga(`C:\vygruzka\sborka.zip`, nil)
	if err != nil || put != `C:\vygruzka\sborka.zip` {
		t.Fatalf("выбранный путь не прошёл: %q %v", put, err)
	}

	// Текст ровно тот, что отдаёт go-common-file-dialog внутри Wails.
	put, err = otvetDialoga("", errors.New("cancelled by user"))
	if err != nil {
		t.Fatalf("отмена человеком стала ошибкой: %v", err)
	}
	if put != "" {
		t.Fatalf("отмена вернула путь %q", put)
	}

	put, err = otvetDialoga("", errors.New("COM не поднялся"))
	if err == nil {
		t.Fatal("сорванный диалог выдан за отмену: кнопка снова молчит")
	}
	if put != "" {
		t.Fatalf("сорванный диалог вернул путь %q", put)
	}
}

// Профиль едет между службой и файлом через окно, и окно обязано не испортить
// его по дороге: служба отдаёт base64, на диск ложатся ИСХОДНЫЕ байты, обратно
// поднимается тот же base64. Пароля в этих двух функциях нет вовсе, и это не
// случайность: путь к файлу секретом не является, пароль является, и он уходит
// только телом команды по каналу.
func TestProfilEzditFaylomBezPoter(t *testing.T) {
	dvoichnoe := []byte{0x00, 0x41, 0xff, 0x10, 0x41, 0x46, 0x46}
	b64 := base64.StdEncoding.EncodeToString(dvoichnoe)
	put := filepath.Join(t.TempDir(), "profil.affory")

	if err := sohranitProfil(put, b64); err != nil {
		t.Fatalf("профиль не записан: %v", err)
	}
	naDiske, err := os.ReadFile(put)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(naDiske, dvoichnoe) {
		t.Fatalf("на диске %v, а ожидались исходные байты %v", naDiske, dvoichnoe)
	}

	nazad, err := prochitatProfil(put)
	if err != nil {
		t.Fatalf("профиль не прочитан: %v", err)
	}
	if nazad != b64 {
		t.Fatalf("обратно поднялось %q вместо %q", nazad, b64)
	}
}

// Непонятное на входе это отказ, а не файл из мусора: молча записанный мусор
// человек обнаружит только при попытке восстановиться на другой машине.
func TestSohranitProfilOtvergaetNeBase64(t *testing.T) {
	put := filepath.Join(t.TempDir(), "profil.affory")
	if err := sohranitProfil(put, "это точно не base64 %%%"); err == nil {
		t.Fatal("мусор записан как профиль")
	}
	if _, err := os.Stat(put); err == nil {
		t.Fatal("файл создан, хотя записывать было нечего")
	}
}

// Ф1 от 05.09.2026. Выход из трея снимает режим и ЖДЁТ ответа, а most.Zvat
// отдаёт отказ службы кадром с полем oshibka и БЕЗ ошибки Go: так задумано,
// экран читает отказ из кадра. Значит вызывающий, который смотрит только на
// err, принимает отказ за успех.
//
// Здесь цена этой ошибки ровно та, из-за которой всё чинилось: программа
// закрывается, доложив себе об успехе, а машина остаётся запертой.
func TestOtkazVOtveteNePrinimaetsyaZaUspeh(t *testing.T) {
	otkaz := `{"tip":"otvet","id":1,"imya":"setKillSwitch",` +
		`"oshibka":{"kod":"firewall-failed","tekst":"netsh не отработал"}}`
	err := otkazVOtvete(otkaz)
	if err == nil {
		t.Fatal("отказ службы прочитан как успех: программа закроется над запертой машиной")
	}
	if !strings.Contains(err.Error(), "netsh не отработал") {
		t.Errorf("в ошибке %q нет текста отказа: человеку покажут пустоту", err)
	}

	if err := otkazVOtvete(`{"tip":"otvet","id":2,"imya":"setKillSwitch"}`); err != nil {
		t.Errorf("успешный ответ прочитан как отказ: %v", err)
	}

	// Неразборчивый и пустой ответы это НЕ успех. Считать успехом то, что не
	// прочли, значит выйти по догадке.
	if otkazVOtvete("не json вовсе") == nil {
		t.Error("неразборчивый ответ принят за успех")
	}
	if otkazVOtvete("") == nil {
		t.Error("пустой ответ принят за успех")
	}
}
