package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/kanal"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"golang.org/x/sys/windows"
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
	// О2 аудита 1.6.1: служба не ответила вовремя, но канал жив. Разрыв унёс
	// бы подписку на события и остальные команды в полёте.
	if obryvKanala(protokol.Kadr{}, fmt.Errorf("%w: status за 5s", kanal.ErrSrokOtveta)) {
		t.Fatal("вышедший срок принят за обрыв канала")
	}
}

// О3 аудита 1.6.1: служба другой версии. Окно получает кадр с кодом
// protocol-mismatch и рисует свой экран, а не сбой оболочки.
func TestRaznyeVersiiPriezzhayutKadrom(t *testing.T) {
	bylo := podklyuchitsya
	t.Cleanup(func() { podklyuchitsya = bylo })
	podklyuchitsya = func() (*kanal.Klient, error) {
		return nil, kanal.OtkazVersii{Tekst: "интерфейс говорит на версии 7, служба на 8"}
	}
	m := &most{}
	s, err := m.Zvat("hello", "")
	if err != nil {
		t.Fatalf("разные версии пришли ошибкой, а не кадром: %v", err)
	}
	var k protokol.Kadr
	if err := json.Unmarshal([]byte(s), &k); err != nil {
		t.Fatal(err)
	}
	if k.Oshib == nil || k.Oshib.Kod != protokol.KodProtocolMismatch || !strings.Contains(k.Oshib.Tekst, "версии 7") || k.Imya != "hello" {
		t.Fatalf("кадр %+v", k)
	}
	// Прочие отказы подключения остаются ошибкой.
	podklyuchitsya = func() (*kanal.Klient, error) { return nil, errors.New("канал не открылся") }
	if _, err := m.Zvat("status", ""); err == nil {
		t.Fatal("отказ подключения стал кадром")
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

// О7 аудита 1.6.1. Страница зовёт SohranitProfil и ProchitatProfil с любым
// путём, а значит, и чужой скрипт на ней мог бы писать и читать что угодно от
// имени человека. Путь принимается, только если его выбрали в диалоге этого окна.
func TestProfilTolkoPoPutiIzDialoga(t *testing.T) {
	m := &most{}
	b64 := base64.StdEncoding.EncodeToString([]byte("profil"))
	put := filepath.Join(t.TempDir(), "profil.affory")
	chuzhoy := filepath.Join(t.TempDir(), "chuzhoy.affory")

	if err := m.SohranitProfil(put, b64); err == nil {
		t.Fatal("записан путь, который не выбирали в диалоге")
	}
	if _, err := os.Stat(put); err == nil {
		t.Fatal("файл создан без выбора")
	}
	m.puti.zapomnit(&m.puti.sohranit, put)
	if err := m.SohranitProfil(chuzhoy, b64); err == nil {
		t.Fatal("записан путь мимо выбранного")
	}
	// Регистр букв у Windows путей не различает.
	if err := m.SohranitProfil(strings.ToUpper(put), b64); err != nil {
		t.Fatalf("выбранный путь не принят: %v", err)
	}

	if _, err := m.ProchitatProfil(put); err == nil {
		t.Fatal("прочитан путь, который не выбирали в диалоге открытия")
	}
	m.puti.zapomnit(&m.puti.prochitat, put)
	if s, err := m.ProchitatProfil(put); err != nil || s != b64 {
		t.Fatalf("выбранный путь не прочитан: %q, %v", s, err)
	}
}

// Выгрузка профиля открывала диалог в рабочем каталоге процесса, как
// диагностика до 1.7.0: ярлык запускает окно из Program Files, и человеку без
// прав Windows отказывала в записи (найдено на приёмке 1.7.0, 28.09.2026).
func TestProfilPredlagaetDokumenty(t *testing.T) {
	dokumenty, err := windows.KnownFolderPath(windows.FOLDERID_Documents, 0)
	if err != nil {
		t.Fatalf("папка «Документы» не найдена: %v", err)
	}
	prezhniy := pokazatSohranenieProfilya
	t.Cleanup(func() { pokazatSohranenieProfilya = prezhniy })
	var katalog string
	vybrano := filepath.Join(t.TempDir(), "profil.affory")
	pokazatSohranenieProfilya = func(_ *most, k string) (string, error) {
		katalog = k
		return vybrano, nil
	}

	m := &most{}
	put, err := m.VybratKudaSohranit()
	if err != nil || put != vybrano {
		t.Fatalf("диалог вернул %q, %v", put, err)
	}
	if !strings.EqualFold(katalog, dokumenty) {
		t.Fatalf("диалог профиля откроется в %q, а не в «Документах» %q", katalog, dokumenty)
	}
	if err := m.SohranitProfil(vybrano, base64.StdEncoding.EncodeToString([]byte("profil"))); err != nil {
		t.Fatalf("путь из диалога не запомнен: %v", err)
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
