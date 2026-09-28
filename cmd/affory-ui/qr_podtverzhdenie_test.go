package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// О8 аудита 1.6.1. «QR с экрана» добавлял всё, что нашёл, без вопроса. Любая
// страница с QR на экране (реклама, чужой сайт, картинка в чате) подсовывала
// человеку свой сервер или свою подписку, и узнавал он об этом по итогу.
// Теперь находка и добавление это два шага, и между ними человек видит, что
// нашлось.

type vyzovSluzhby struct {
	imya string
	telo map[string]string
}

// podstavitQr подменяет снимок экрана и службу. Служба отвечает успехом на
// обе команды и запоминает, что у неё просили.
func podstavitQr(t *testing.T, naEkrane string) *[]vyzovSluzhby {
	t.Helper()
	byloChtenie, byloZvat := prochitatQrSEkrana, zvatSluzhbu
	t.Cleanup(func() { prochitatQrSEkrana, zvatSluzhbu = byloChtenie, byloZvat })
	prochitatQrSEkrana = func(*most) (string, error) { return naEkrane, nil }
	var vyzovy []vyzovSluzhby
	zvatSluzhbu = func(_ *most, imya, telo string) (string, error) {
		v := vyzovSluzhby{imya: imya}
		if err := json.Unmarshal([]byte(telo), &v.telo); err != nil {
			t.Fatalf("тело %s не JSON: %v", imya, err)
		}
		vyzovy = append(vyzovy, v)
		otvet := `{"tip":"otvet","id":1,"imya":"addServers","telo":{"dobavleno":2}}`
		if imya == "addSubscription" {
			otvet = `{"tip":"otvet","id":1,"imya":"addSubscription","telo":{"aktivnaya":true,"serverov":6}}`
		}
		return otvet, nil
	}
	return &vyzovy
}

const (
	klyuchVless = "vless://11111111-2222-3333-4444-555555555555@203.0.113.9:8443?type=tcp&security=reality&sni=www.example.com&fp=chrome&pbk=AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8&sid=01ab#Германия"
	klyuchHy2   = "hy2://sekretnyyparol@203.0.113.10:443?sni=example.com#Нидерланды"
	adresPodp   = "https://panel.example/sub/tokenAbc123"
)

func TestNaydennoeQrPokazyvaetSvodkuBezSekretov(t *testing.T) {
	podstavitQr(t, klyuchVless+"\n"+klyuchHy2+"\n"+adresPodp)
	svodka, err := (&most{}).NaytiQrNaEkrane()
	if err != nil {
		t.Fatal(err)
	}
	if len(svodka.Klyuchi) != 2 {
		t.Fatalf("ключей в сводке %d, ждали 2: %+v", len(svodka.Klyuchi), svodka)
	}
	pervyy := svodka.Klyuchi[0]
	if pervyy.Imya != "Германия" || pervyy.Transport != "reality-tcp" || pervyy.Adres != "203.0.113.9:8443" {
		t.Fatalf("первый ключ в сводке %+v", pervyy)
	}
	if svodka.Klyuchi[1].Transport != "hy2" || svodka.Klyuchi[1].Adres != "203.0.113.10:443" {
		t.Fatalf("второй ключ в сводке %+v", svodka.Klyuchi[1])
	}
	if len(svodka.Podpiski) != 1 || svodka.Podpiski[0] != "panel.example" {
		t.Fatalf("подписки в сводке %v, ждали только узел", svodka.Podpiski)
	}
	tekst, err := json.Marshal(svodka)
	if err != nil {
		t.Fatal(err)
	}
	for _, sekret := range []string{"11111111-2222", "sekretnyyparol", "tokenAbc123", "/sub"} {
		if strings.Contains(string(tekst), sekret) {
			t.Errorf("в окно ушёл секрет %q: %s", sekret, tekst)
		}
	}
}

func TestQrNeDobavlyaetsyaBezPodtverzhdeniya(t *testing.T) {
	vyzovy := podstavitQr(t, klyuchVless+"\n"+adresPodp)
	m := &most{}
	if _, err := m.NaytiQrNaEkrane(); err != nil {
		t.Fatal(err)
	}
	if len(*vyzovy) != 0 {
		t.Fatalf("до подтверждения служба уже получила %+v", *vyzovy)
	}

	itog, err := m.DobavitNaydennoeQr()
	if err != nil {
		t.Fatal(err)
	}
	if len(*vyzovy) != 2 {
		t.Fatalf("после подтверждения вызовов %d, ждали 2: %+v", len(*vyzovy), *vyzovy)
	}
	if v := (*vyzovy)[0]; v.imya != "addServers" || v.telo["tekst"] != klyuchVless {
		t.Fatalf("ключи ушли не так: %+v", v)
	}
	if v := (*vyzovy)[1]; v.imya != "addSubscription" || v.telo["adres"] != adresPodp {
		t.Fatalf("подписка ушла не так: %+v", v)
	}
	if !strings.Contains(itog, "добавлено 2") || !strings.Contains(itog, "серверов: 6") {
		t.Fatalf("итог %q", itog)
	}

	// Находка одна на одно подтверждение: второе нажатие ничего не добавляет.
	if _, err := m.DobavitNaydennoeQr(); err == nil {
		t.Fatal("то же найденное добавлено второй раз")
	}
	if len(*vyzovy) != 2 {
		t.Fatalf("второе нажатие дошло до службы: %+v", *vyzovy)
	}
}

func TestOtmenaQrZabyvaetNaydennoe(t *testing.T) {
	vyzovy := podstavitQr(t, klyuchVless)
	m := &most{}
	if _, err := m.NaytiQrNaEkrane(); err != nil {
		t.Fatal(err)
	}
	m.ZabytQr()
	if _, err := m.DobavitNaydennoeQr(); err == nil {
		t.Fatal("после отмены найденное всё равно добавилось")
	}
	if len(*vyzovy) != 0 {
		t.Fatalf("после отмены служба получила %+v", *vyzovy)
	}
}

// Без подтверждения нечего и показывать, если годного нет: отказ словами,
// и прежняя находка не доживает до следующего нажатия.
func TestQrBezGodnogoOtkazyvaet(t *testing.T) {
	sluchai := map[string]string{
		"просто текст":             "ни ключей, ни адреса подписки",
		`{"outbounds":[]}`:         "файл настроек",
		"vless://bez-adresa-vovse": "не разобрался",
	}
	for naEkrane, zhdyom := range sluchai {
		vyzovy := podstavitQr(t, naEkrane)
		m := &most{}
		m.qr.polozhit(naydennoeQr{klyuchi: klyuchVless})
		_, err := m.NaytiQrNaEkrane()
		if err == nil || !strings.Contains(err.Error(), zhdyom) {
			t.Errorf("%q: ждали отказ про %q, получили %v", naEkrane, zhdyom, err)
		}
		if _, err := m.DobavitNaydennoeQr(); err == nil {
			t.Errorf("%q: после отказа добавилась прежняя находка", naEkrane)
		}
		if len(*vyzovy) != 0 {
			t.Errorf("%q: служба получила %+v", naEkrane, *vyzovy)
		}
	}
}

// Битая строка рядом с годными не отменяет годные, но сводка её считает:
// человек должен знать, что в коде было больше, чем добавится.
func TestNegodnyeStrokiSchitayutsya(t *testing.T) {
	podstavitQr(t, klyuchVless+"\nvless://bez-adresa-vovse\nhttps://")
	svodka, err := (&most{}).NaytiQrNaEkrane()
	if err != nil {
		t.Fatal(err)
	}
	if len(svodka.Klyuchi) != 1 || len(svodka.Podpiski) != 0 || svodka.Negodnyh != 2 {
		t.Fatalf("сводка %+v, ждали один ключ и две негодные строки", svodka)
	}
}
