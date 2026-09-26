package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image/png"
	"strings"
	"testing"
)

// Код, нарисованный для выгрузки, обязан читаться тем же распознавателем, что
// читает QR с экрана: иначе выгрузка с одного ПК не добавится на другом.
func TestKodyQrChitayutsyaObratno(t *testing.T) {
	var stroki []string
	for i := 0; i < 6; i++ {
		stroki = append(stroki, fmt.Sprintf("hy2://parol-%02d-abcdefabcdefabcdef@203.0.113.20:%d?sni=a.example&obfs=salamander&obfs-password=0123456789abcdef01234567#k%d", i, 11000+i, i))
	}
	tekst := strings.Join(stroki, "\n")
	kody, err := (&most{}).KodyQr(tekst)
	if err != nil {
		t.Fatal(err)
	}
	if len(kody) != 1 {
		t.Fatalf("шесть ключей дали %d кодов, ждали один", len(kody))
	}
	if got := prochitatKod(t, kody[0]); got != tekst {
		t.Fatalf("прочитано не то:\n%s", got)
	}
}

// Длинная выгрузка делится по строкам, и ни одна ссылка не режется.
func TestKodyQrDelyatsyaPoStrokam(t *testing.T) {
	var stroki []string
	for i := 0; i < 30; i++ {
		stroki = append(stroki, fmt.Sprintf("anytls://parol-%02d-0123456789abcdef0123456789@203.0.113.20:%d?sni=a.example#k%d", i, 20000+i, i))
	}
	kody, err := (&most{}).KodyQr(strings.Join(stroki, "\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(kody) < 2 {
		t.Fatalf("30 ключей дали %d кодов, ждали деление", len(kody))
	}
	var sobrano []string
	for _, k := range kody {
		chast := prochitatKod(t, k)
		if len(chast) > potolokKoda {
			t.Errorf("код несёт %d байт при потолке %d", len(chast), potolokKoda)
		}
		sobrano = append(sobrano, strings.Split(chast, "\n")...)
	}
	if strings.Join(sobrano, "\n") != strings.Join(stroki, "\n") {
		t.Fatal("после деления строки не сложились обратно")
	}
}

func TestKodyQrPustoyTekst(t *testing.T) {
	if _, err := (&most{}).KodyQr(" \n "); err == nil {
		t.Fatal("пустой текст дал код")
	}
}

func prochitatKod(t *testing.T, dataURI string) string {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(dataURI, "data:image/png;base64,"))
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	s, err := raspoznatQr(img)
	if err != nil {
		t.Fatalf("код не прочитался: %v", err)
	}
	return s
}

func TestItogPachkiStroka(t *testing.T) {
	i := itogPachki{Dobavleno: 5, UzheBylo: 1}
	i.Otkazy = append(i.Otkazy, struct {
		Stroka   int    `json:"stroka"`
		Prichina string `json:"prichina"`
	}{Stroka: 3, Prichina: "ссылка не разобрана"})
	if got := i.stroka(); got != "добавлено 5, уже были 1, пропущено 1, строка 3: ссылка не разобрана" {
		t.Fatalf("итог %q", got)
	}
}
