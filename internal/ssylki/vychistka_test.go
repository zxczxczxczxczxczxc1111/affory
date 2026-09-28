package ssylki

import (
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// О6 аудита 1.6.1. Журналы уходят из машины файлом, и всё, по чему
// собирается доступ, должно остаться дома. Служба и так пишет адрес подписки
// отпечатком, это второй барьер на выходе.
func TestVychistkaZhurnalaUbiraetSekrety(t *testing.T) {
	servery := []protokol.Server{
		{Transport: "reality-tcp", Host: "203.0.113.9", Port: 8443,
			Uuid: "11111111-2222-3333-4444-555555555555", PublicKey: "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8", ShortId: "01ab23cd"},
		{Transport: "hy2", Host: "203.0.113.10", Port: 443, Parol: "sekretnyyparol", ObfsParol: "obfsparol123"},
		{Transport: "ws", Host: "203.0.113.11", Port: 443, Uuid: "66666666-7777-8888-9999-000000000000", Put: "/tajnyy-put"},
	}
	adresa := []string{"https://panel.example/sub/tokenAbc123?flag=1"}
	zhurnal := strings.Join([]string{
		"2026/09/28 12:00:01 ядро: outbound 11111111-2222-3333-4444-555555555555 отказал",
		"pbk=AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8 sid=01ab23cd",
		"hy2 auth sekretnyyparol obfs obfsparol123",
		"ws path /tajnyy-put на 203.0.113.11:443",
		"подписка https://panel.example/sub/tokenAbc123?flag=1 не ответила",
		"кусок пути /sub/tokenAbc123 отдельно",
		"вставлено vless://aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee@198.51.100.1:443?security=reality#X",
		"обновление https://user:pass@github.com/zxc/affory/releases/download/v1.7.0/a.zip",
		`правило C:\Users\xd\AppData\Local\Discord\app.exe`,
		`{"put":"C:\\Users\\Иван Петров\\Desktop\\game.exe"}`,
	}, "\n")

	chisto := Vychistit(zhurnal, servery, adresa)

	for _, sekret := range []string{
		"11111111-2222", "AAECAwQFBgcICQoLDA0", "01ab23cd", "sekretnyyparol", "obfsparol123",
		"/tajnyy-put", "tokenAbc123", "panel.example/sub", "aaaaaaaa-bbbb", "user:pass",
		"releases/download", `\xd\`, "Иван Петров",
	} {
		if strings.Contains(chisto, sekret) {
			t.Errorf("в вычищенном журнале остался %q:\n%s", sekret, chisto)
		}
	}
	// То, по чему разбирается сетевой дефект, остаётся: адреса узлов, узел
	// подписки и обновлений, путь к программе без имени человека.
	for _, nuzhno := range []string{
		"203.0.113.11:443", "https://github.com/", `C:\Users\<пользователь>\AppData\Local\Discord\app.exe`,
		`C:\\Users\\<пользователь>\\Desktop\\game.exe`, "ядро: outbound", "не ответила",
	} {
		if !strings.Contains(chisto, nuzhno) {
			t.Errorf("вычистка съела нужное %q:\n%s", nuzhno, chisto)
		}
	}
}

// Короткий секрет не заменяется по всему тексту: пароль «abc» превратил бы в
// кашу каждое слово, где встречаются эти три буквы, а доступа по нему всё
// равно не собрать без адреса.
func TestVychistkaNeTrogaetKorotkoe(t *testing.T) {
	servery := []protokol.Server{{Transport: "ss", Parol: "abc", Put: "/"}}
	tekst := "abcdef / abc"
	if got := Vychistit(tekst, servery, nil); got != tekst {
		t.Fatalf("короткое изменено: %q", got)
	}
}
