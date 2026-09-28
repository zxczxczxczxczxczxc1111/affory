package genkonfig

import (
	"errors"
	"testing"
)

// Генератор кладёт теги наборов как []string (praviloNaborov, dnsPravila), а
// проверка до 28.09.2026 понимала только []any: висячий набор в настоящем
// конфиге проходил молча, и check 1.14.2 в правилах маршрута тоже молчит.
func TestVisyachiyNaborStrokamiLovitsya(t *testing.T) {
	k := map[string]any{
		"outbounds": []any{map[string]any{"type": "direct", "tag": TegPryamo}},
		"route": map[string]any{
			"rules": []any{map[string]any{"rule_set": []string{"net-takogo"}, "outbound": TegPryamo}},
		},
	}
	if err := sveritTegi(k); !errors.Is(err, ErrVisyachiyTeg) {
		t.Fatalf("висячий набор списком строк не пойман: %v", err)
	}
}

func TestVisyachiyNaborVDnsLovitsya(t *testing.T) {
	k := map[string]any{
		"outbounds": []any{map[string]any{"type": "direct", "tag": TegPryamo}},
		"dns": map[string]any{
			"servers": []any{map[string]any{"tag": TegMestnyy, "type": "local"}},
			"rules": []any{map[string]any{
				"type": "logical", "mode": "and",
				"rules":  []any{map[string]any{"rule_set": []string{"net-v-dns"}}},
				"server": TegMestnyy,
			}},
		},
	}
	if err := sveritTegi(k); !errors.Is(err, ErrVisyachiyTeg) {
		t.Fatalf("висячий набор во вложенном DNS-правиле не пойман: %v", err)
	}
}

func TestVisyachiyServerVoVlozhennomDnsLovitsya(t *testing.T) {
	k := map[string]any{
		"outbounds": []any{map[string]any{"type": "direct", "tag": TegPryamo}},
		"dns": map[string]any{
			"servers": []any{map[string]any{"tag": TegMestnyy, "type": "local"}},
			"rules": []any{map[string]any{
				"type": "logical", "mode": "or",
				"rules": []any{map[string]any{"domain": []string{"example.org"}, "server": "net-takogo"}},
			}},
		},
	}
	if err := sveritTegi(k); !errors.Is(err, ErrVisyachiyTeg) {
		t.Fatalf("висячий сервер во вложенном DNS-правиле не пойман: %v", err)
	}
}

// Пустой список на верхнем уровне check пропускает с кодом 0, а ядро считает
// такое условие выполненным ВСЕГДА (замер 28.09.2026, sing-box 1.14.2).
func TestPustoyeUsloviyeOtvergaetsya(t *testing.T) {
	sluchai := map[string]map[string]any{
		"наверху маршрута": {"route": map[string]any{"rules": []any{
			map[string]any{"domain_suffix": []string{}, "action": "reject"},
		}}},
		"во вложенном правиле маршрута": {"route": map[string]any{"rules": []any{
			map[string]any{"type": "logical", "mode": "and", "action": "reject", "rules": []any{
				map[string]any{"rule_set": []string{"reklama"}},
				map[string]any{"domain_suffix": []string{}, "invert": true},
			}},
		}}},
		"в DNS": {"dns": map[string]any{"rules": []any{
			map[string]any{"rule_set": []any{}, "action": "predefined", "rcode": "NXDOMAIN"},
		}}},
		"пустой список вложенных": {"route": map[string]any{"rules": []any{
			map[string]any{"type": "logical", "mode": "and", "rules": []any{}, "action": "reject"},
		}}},
	}
	for imya, k := range sluchai {
		if err := sveritUsloviya(k); !errors.Is(err, errPustoeUsloviye) {
			t.Errorf("%s: пустое условие не поймано: %v", imya, err)
		}
	}
	polnyy := map[string]any{"route": map[string]any{"rules": []any{
		map[string]any{"domain_suffix": []string{"example.org"}, "outbound": TegPryamo},
	}}}
	if err := sveritUsloviya(polnyy); err != nil {
		t.Fatalf("непустое условие отвергнуто: %v", err)
	}
}
