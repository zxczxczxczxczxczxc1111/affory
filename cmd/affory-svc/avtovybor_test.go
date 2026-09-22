package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/netip"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Область автовыбора (A5): человек распоряжается составом автомата, но не
// может оставить его пустым.

func naborIzTryoh() Nabor {
	return Nabor{Servery: []protokol.Server{
		{Id: "nl", Imya: "Нидерланды", Transport: "ws", Host: "192.0.2.1", Port: 443},
		{Id: "de", Imya: "Германия", Transport: "ws", Host: "192.0.2.2", Port: 443},
		{Id: "fi", Imya: "Финляндия", Transport: "ws", Host: "192.0.2.3", Port: 443},
	}}
}

func TestUbrannyyIzAvtoNeSchitaetsyaKandidatom(t *testing.T) {
	n := naborIzTryoh()
	if err := n.ZadatUchastieVAvto("de", false); err != nil {
		t.Fatalf("сервер не убрался: %v", err)
	}
	if n.UchastvuetVAvto("de") {
		t.Error("убранный сервер всё ещё числится в области")
	}
	if got := len(n.AvtoKandidaty()); got != 2 {
		t.Errorf("кандидатов %d, ждали два", got)
	}
	// Повторная уборка это не ошибка: человек мог нажать дважды, а список
	// исключений не должен расти дубликатами.
	if err := n.ZadatUchastieVAvto("de", false); err != nil {
		t.Fatalf("повторная уборка ответила ошибкой: %v", err)
	}
	if got := len(n.VneAvto); got != 1 {
		t.Errorf("исключений %d после двух уборок одного сервера", got)
	}
	if err := n.ZadatUchastieVAvto("de", true); err != nil {
		t.Fatalf("сервер не вернулся: %v", err)
	}
	if !n.UchastvuetVAvto("de") || len(n.AvtoKandidaty()) != 3 {
		t.Error("возврат в область не сработал")
	}
}

func TestPoslednegoIzAvtoUbratNelzya(t *testing.T) {
	// Пустая группа urltest это конфиг, который ядро отвергает целиком: человек
	// нажал бы на переключатель и остался без VPN. Отказ здесь честнее тихого
	// согласия.
	n := naborIzTryoh()
	for _, id := range []string{"nl", "de"} {
		if err := n.ZadatUchastieVAvto(id, false); err != nil {
			t.Fatalf("%s не убрался: %v", id, err)
		}
	}
	err := n.ZadatUchastieVAvto("fi", false)
	if !errors.Is(err, errPoslednyyVAvto) {
		t.Fatalf("последний убрался с ошибкой %v", err)
	}
	if got := len(n.AvtoKandidaty()); got != 1 {
		t.Fatalf("кандидатов %d, ждали одного", got)
	}
}

func TestNesushchestvuyushchiyServerNeUbiraetsya(t *testing.T) {
	n := naborIzTryoh()
	if err := n.ZadatUchastieVAvto("kotorogo-net", false); !errors.Is(err, errServerNeNayden) {
		t.Fatalf("ошибка %v, ждали «сервер не найден»", err)
	}
	if len(n.VneAvto) != 0 {
		t.Fatal("в исключениях появился сервер, которого в наборе нет")
	}
}

func TestIsklyucheniyaChistyatsyaVsledZaServerami(t *testing.T) {
	// Подписка меняет состав раз в 12 часов. Без чистки список исключений рос
	// бы вечно, а через полгода одна из мёртвых строк совпала бы с
	// идентификатором нового сервера и молча убрала бы его из автомата.
	n := naborIzTryoh()
	n.VneAvto = []string{"de", "kotorogo-net"}
	n.Servery = n.Servery[:2] // fi ушёл из подписки
	n.PrivestiAvtovybor()
	if len(n.VneAvto) != 1 || n.VneAvto[0] != "de" {
		t.Fatalf("исключения после чистки: %v", n.VneAvto)
	}
	// Идемпотентность: чистка идёт на КАЖДОМ чтении набора.
	n.PrivestiAvtovybor()
	if len(n.VneAvto) != 1 {
		t.Fatalf("вторая чистка изменила список: %v", n.VneAvto)
	}
}

func TestKomandaSetAutoMemberOtdayotSpisokSPometkoy(t *testing.T) {
	s := podstavnaya(t, nil)
	if err := s.pravitNabor(func(n *Nabor) error {
		n.Servery = naborIzTryoh().Servery
		return nil
	}); err != nil {
		t.Fatalf("набор не записан: %v", err)
	}

	telo, _ := json.Marshal(map[string]any{"id": "de", "uchastvuet": false})
	k := s.Obrabotat(context.Background(), protokol.Kadr{Imya: "setAutoMember", Telo: telo})
	if k.Oshib != nil {
		t.Fatalf("команда ответила отказом %q: %s", k.Oshib.Kod, k.Oshib.Tekst)
	}
	// Ответ это список серверов, а не статус: изменилось участие сервера, и
	// обновить экран надо именно списком.
	var otvet struct {
		Servery []protokol.Server `json:"servery"`
	}
	b, _ := json.Marshal(k.Telo)
	if err := json.Unmarshal(b, &otvet); err != nil {
		t.Fatalf("ответ не разбирается: %v", err)
	}
	if len(otvet.Servery) != 3 {
		t.Fatalf("серверов в ответе %d", len(otvet.Servery))
	}
	for _, srv := range otvet.Servery {
		if srv.VneAvto != (srv.Id == "de") {
			t.Errorf("у %s пометка vne_avto = %v", srv.Id, srv.VneAvto)
		}
	}
	// Решение пережило команду и лежит в наборе, а не только в ответе.
	n, err := s.nabor()
	if err != nil {
		t.Fatalf("набор не прочитан: %v", err)
	}
	if n.UchastvuetVAvto("de") {
		t.Error("набор не запомнил, что сервер убран из авто")
	}
}

func TestKomandaSetAutoMemberOtkazyvaetNaPoslednem(t *testing.T) {
	s := podstavnaya(t, nil)
	if err := s.pravitNabor(func(n *Nabor) error {
		n.Servery = naborIzTryoh().Servery[:1]
		return nil
	}); err != nil {
		t.Fatalf("набор не записан: %v", err)
	}
	telo, _ := json.Marshal(map[string]any{"id": "nl", "uchastvuet": false})
	k := s.Obrabotat(context.Background(), protokol.Kadr{Imya: "setAutoMember", Telo: telo})
	if k.Oshib == nil {
		t.Fatal("единственный сервер убрался из автовыбора")
	}
	if k.Oshib.Kod != protokol.KodTeloNegodno {
		t.Errorf("код отказа %q", k.Oshib.Kod)
	}
}

// podstavnayaSPolnymNaborom это служба с тремя серверами и с адресами
// кандидатов: без них генератор отвергает конфиг целиком (sveritKandidatov).
func podstavnayaSPolnymNaborom(t *testing.T) *Sluzhba {
	t.Helper()
	s := podstavnaya(t, nil)
	servery := naborIzTryoh().Servery
	if err := s.pravitNabor(func(n *Nabor) error {
		n.Servery = servery
		n.Vybran = servery[0].Id
		return nil
	}); err != nil {
		t.Fatalf("набор не записан: %v", err)
	}
	s.sobratAdresa = func() ([]netip.Addr, error) {
		adresa := make([]netip.Addr, 0, len(servery))
		for _, srv := range servery {
			adresa = append(adresa, netip.MustParseAddr(srv.Host))
		}
		return adresa, nil
	}
	return s
}

func TestKonfigSobiraetsyaBezUbrannogoIzAvto(t *testing.T) {
	// Сквозной путь до конфига ядра: набор -> sobratTun -> группа urltest.
	// Без него связь «команда что-то записала» и «ядро это увидело» держится
	// на честном слове.
	s := podstavnayaSPolnymNaborom(t)
	if err := s.pravitNabor(func(n *Nabor) error {
		return n.ZadatUchastieVAvto("de", false)
	}); err != nil {
		t.Fatalf("сервер не убрался: %v", err)
	}
	telo, _, _, err := s.sobratTun(nil, false)
	if err != nil {
		t.Fatalf("конфиг не собрался: %v", err)
	}
	var konfig struct {
		Outbounds []struct {
			Tag       string   `json:"tag"`
			Type      string   `json:"type"`
			Outbounds []string `json:"outbounds"`
		} `json:"outbounds"`
	}
	if err := json.Unmarshal(telo, &konfig); err != nil {
		t.Fatalf("конфиг не разбирается: %v", err)
	}
	var vAvto, vSelektore []string
	for _, o := range konfig.Outbounds {
		switch o.Type {
		case "urltest":
			vAvto = o.Outbounds
		case "selector":
			vSelektore = o.Outbounds
		}
	}
	for _, teg := range vAvto {
		if teg == "srv-de" {
			t.Fatal("убранный сервер остался кандидатом urltest")
		}
	}
	nashli := false
	for _, teg := range vSelektore {
		if teg == "srv-de" {
			nashli = true
		}
	}
	if !nashli {
		t.Fatal("убранный сервер пропал из селектора: выбрать его руками стало нечем")
	}
}
