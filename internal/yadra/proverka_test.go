package yadra

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Ядро печатает номер исходящего, и это единственная зацепка, по которой можно
// понять, КАКОЙ сервер оно не приняло. Разбор отделён от запуска ровно затем,
// чтобы его можно было проверить без живого ядра.
//
// Строки взяты из настоящих прогонов: 02.09.2026 на госте с чужой подпиской и
// 01.09.2026 при находке 33. Цветовые последовательности сохранены: ядро пишет
// их всегда, и регулярное выражение обязано их переживать.
func TestNomerIshodyashchegoIzVyhodaYadra(t *testing.T) {
	sluchai := []struct {
		imya   string
		vyhod  string
		nomer  int
		nashli bool
	}{
		{"чужая подписка 02.09",
			"\x1b[31mFATAL\x1b[0m[0000] initialize outbound[24]: invalid public_key", 24, true},
		{"находка 33",
			"FATAL[0000] create service: initialize outbound[1]: invalid public_key", 1, true},
		{"нулевой индекс тоже индекс",
			"FATAL initialize outbound[0]: invalid public_key", 0, true},
		{"отказ без номера остаётся отказом",
			"FATAL[0000] decode config at config.json: invalid character", 0, false},
	}
	for _, s := range sluchai {
		t.Run(s.imya, func(t *testing.T) {
			n, est := nomerIshodyashchego(s.vyhod)
			if est != s.nashli {
				t.Fatalf("признак номера %v, ожидался %v", est, s.nashli)
			}
			if est && n != s.nomer {
				t.Fatalf("номер %d, ожидался %d", n, s.nomer)
			}
		})
	}
}

// Живое ядро судит конфиг само. Без него проверять нечем, и притворяться, что
// проверено, хуже, чем пропустить.
func TestProveritNazyvaetNomerNegodnogoIshodyashchego(t *testing.T) {
	yadro := os.Getenv("AFFORY_SINGBOX")
	if yadro == "" {
		t.Skip("не задан AFFORY_SINGBOX")
	}
	// Второй исходящий негоден: reality с пустым ключом. Ровно то, что собирал
	// генератор для xhttp поверх обычного TLS до 02.09.2026.
	konfig := `{
      "log": {"level": "warn"},
      "outbounds": [
        {"type": "direct", "tag": "direct"},
        {"type": "vless", "tag": "srv-plohoy", "server": "203.0.113.9", "server_port": 443,
         "uuid": "b831381d-6324-4d53-ad4f-8cda48b30811",
         "tls": {"enabled": true, "server_name": "a.example",
                 "reality": {"enabled": true, "public_key": "", "short_id": ""}}}
      ]}`
	put := filepath.Join(t.TempDir(), "plohoy.json")
	if err := os.WriteFile(put, []byte(konfig), 0o600); err != nil {
		t.Fatal(err)
	}

	err := proveritKonfig(yadro, put)
	var o *OshibkaKonfiga
	if !errors.As(err, &o) {
		t.Fatalf("ожидался OshibkaKonfiga, получено %v", err)
	}
	if !o.EstNomer || o.Nomer != 1 {
		t.Fatalf("номер исходящего %d (есть=%v), ожидался 1: без него нечего исключать",
			o.Nomer, o.EstNomer)
	}
	if strings.Contains(o.Vyhod, "\x1b[") {
		t.Errorf("в выводе остались цветовые последовательности, в журнале это мусор: %q", o.Vyhod)
	}
}

func TestProveritPrinimaetGodnyyKonfig(t *testing.T) {
	yadro := os.Getenv("AFFORY_SINGBOX")
	if yadro == "" {
		t.Skip("не задан AFFORY_SINGBOX")
	}
	konfig := `{"log": {"level": "warn"}, "outbounds": [{"type": "direct", "tag": "direct"}]}`
	put := filepath.Join(t.TempDir(), "godnyy.json")
	if err := os.WriteFile(put, []byte(konfig), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := proveritKonfig(yadro, put); err != nil {
		t.Fatalf("годный конфиг отвергнут: %v", err)
	}
}
