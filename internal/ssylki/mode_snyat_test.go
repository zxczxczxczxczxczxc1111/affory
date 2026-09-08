package ssylki_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/ssylki"
)

// `mode` это наследство xhttp, снятого 06.09.2026. Сам транспорт отбивается
// веткой типа выше, а вот параметр рядом с ЖИВЫМ транспортом чужие панели
// ставят и дальше, и ссылку из-за него отвергать нельзя.
//
// Проверяется не отсутствие поля в структуре: после снятия такую проверку
// нельзя даже написать, код просто не соберётся. Проверяется сериализованная
// запись, потому что именно она ложится на диск и уезжает в набор, и именно в
// ней мёртвый ключ жил бы вечно.
func TestModeNeDoezzhaetDoZapisi(t *testing.T) {
	srv, err := ssylki.Razobrat(vzyat(t, "vless-ws-mode.txt"))
	if err != nil {
		t.Fatalf("ссылка с mode= обязана разбираться, а не отвергаться: %v", err)
	}
	if srv.Transport != "ws" {
		t.Fatalf("транспорт %q, ожидался ws", srv.Transport)
	}

	telo, err := json.Marshal(srv)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(telo, []byte("rezhim_xhttp")) {
		t.Fatalf("в записи остался ключ снятого транспорта: %s", telo)
	}
}

// Страж совместимости, и он честно зелёный с самого начала: наборы, сохранённые
// прежними версиями, ключ `rezhim_xhttp` несут, а `encoding/json` неизвестные
// ключи молча пропускает. Его работа начнётся в тот день, когда кто-нибудь
// заведёт строгий декодер и уронит чтение всех старых наборов разом.
func TestStarayaZapisSKlyuchomXHTTPChitaetsya(t *testing.T) {
	staraya := []byte(`{
		"id": "nl-1",
		"imya": "NL",
		"transport": "ws",
		"host": "203.0.113.2",
		"port": 443,
		"uuid": "11111111-2222-3333-4444-555555555555",
		"rezhim_xhttp": "packet-up",
		"put": "/ws",
		"iz_podpiski": true
	}`)

	var srv protokol.Server
	if err := json.Unmarshal(staraya, &srv); err != nil {
		t.Fatalf("сохранённая запись перестала читаться: %v", err)
	}
	if srv.Transport != "ws" {
		t.Errorf("транспорт %q, ожидался ws", srv.Transport)
	}
	if srv.Put != "/ws" {
		t.Errorf("путь %q, ожидался /ws", srv.Put)
	}
	if srv.Port != 443 {
		t.Errorf("порт %d, ожидался 443", srv.Port)
	}
	if !srv.IzPodpiski {
		t.Error("пометка «из подписки» потерялась рядом со снятым ключом")
	}
}
