package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/kanal"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Долг 0 волны 6. 02.09.2026 набор на хосте получил Vybran и ручной режим, то
// есть кто-то послал setServer, и разобраться было нечем: журнал писал только
// старт, ядро и подписку. Нужна строка на КАЖДУЮ команду: имя, кто прислал,
// чем кончилось.

func sluzhbaSZhurnalom(t *testing.T) (*Sluzhba, *bytes.Buffer) {
	t.Helper()
	s := NovayaSluzhba()
	var b bytes.Buffer
	s.zhurnalKomand = log.New(&b, "", 0)
	return s, &b
}

func TestZhurnalKomandNazyvaetKomanduProtsessIIshod(t *testing.T) {
	s, b := sluzhbaSZhurnalom(t)
	ctx := kanal.SDopuskom(context.Background(), kanal.Dopusk{
		Sid: "S-1-5-21-1", Pid: 4242, Protsess: `C:\Program Files\Affory\affory-cli.exe`})
	// Чужая версия протокола: отказ гарантирован, и журнал обязан назвать код.
	telo, _ := json.Marshal(map[string]int{"protocol": protokol.Versiya + 100})
	s.Obrabotat(ctx, protokol.Kadr{Tip: "cmd", Id: 7, Imya: "hello", Telo: telo})

	stroka := b.String()
	for _, nado := range []string{"hello", "4242", "affory-cli.exe", protokol.KodProtocolMismatch} {
		if !strings.Contains(stroka, nado) {
			t.Fatalf("в журнале нет %q: %q", nado, stroka)
		}
	}
}

func TestZhurnalKomandPishetUspeh(t *testing.T) {
	s, b := sluzhbaSZhurnalom(t)
	s.Obrabotat(context.Background(), protokol.Kadr{Tip: "cmd", Id: 1, Imya: "status"})
	if !strings.Contains(b.String(), "status") || !strings.Contains(b.String(), "ok") {
		t.Fatalf("успех не записан: %q", b.String())
	}
}

func TestZhurnalKomandNePishetTel(t *testing.T) {
	// В addServer и setSubscription лежат ключи. Одна печать тела «чтобы
	// посмотреть, что приходит» кладёт их в файл, который читают все админы
	// машины. Проверяется на секретной команде И на обычной: тела не место в
	// журнале команд вовсе, там достаточно имени и исхода.
	s, b := sluzhbaSZhurnalom(t)
	sekret := "tokenchik-sekretnyy"
	telo, _ := json.Marshal(map[string]string{"adres": "https://panel.example/sub/" + sekret})
	s.Obrabotat(context.Background(), protokol.Kadr{Tip: "cmd", Id: 2, Imya: "setSubscription", Telo: telo})
	telo2, _ := json.Marshal(map[string]int{"protocol": protokol.Versiya + 1, "hvost": 777001})
	s.Obrabotat(context.Background(), protokol.Kadr{Tip: "cmd", Id: 3, Imya: "hello", Telo: telo2})
	if strings.Contains(b.String(), sekret) || strings.Contains(b.String(), "777001") {
		t.Fatalf("тело команды попало в журнал: %q", b.String())
	}
	if !strings.Contains(b.String(), "setSubscription") {
		t.Fatalf("секретная команда не записана вовсе: %q", b.String())
	}
}

func TestBezZhurnalaKomandyRabotayut(t *testing.T) {
	// Тесты и стенд создают службу без журнала: nil обязан значить «молчим», а
	// не «падаем на первой команде».
	s := NovayaSluzhba()
	k := s.Obrabotat(context.Background(), protokol.Kadr{Tip: "cmd", Id: 1, Imya: "status"})
	if k.Oshib != nil {
		t.Fatalf("status без журнала отказал: %+v", k.Oshib)
	}
}
