package ssylki_test

import (
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/ssylki"
)

// Ссылки БЕЗ TLS существуют и встречаются в живых сборниках: в корпусе
// igareck/vpn-configs-for-russia их шесть из 145, на портах 80 и 2200, с
// `security=none` и `security=false`.
//
// До 02.09.2026 модель этого не выражала вовсе: разбор вычислял `security`
// локально и хранил только `pbk`, поэтому генератор для ws и grpc включал TLS
// БЕЗУСЛОВНО. Это молчаливая версия находки 43: ядро такой конфиг принимает,
// `check` его не отвергает, а соединения не будет никогда.
func TestSsylkaBezTlsSohranyaetEtoVModeli(t *testing.T) {
	sluchai := []struct {
		imya   string
		ssylka string
		bezTLS bool
	}{
		{"ws с security=none",
			"vless://11111111-2222-3333-4444-555555555555@203.0.113.20:2200?encryption=none&type=ws&security=none&path=%2Fv1&host=example.org#uzel", true},
		{"ws с security=false",
			"vless://11111111-2222-3333-4444-555555555555@203.0.113.21:80?security=false&type=ws&headerType=none&host=example.org&path=%2F#uzel", true},
		{"ws вообще без security",
			"vless://11111111-2222-3333-4444-555555555555@203.0.113.22:80?type=ws&path=%2F&host=example.org#uzel", true},
		{"ws с security=tls это TLS",
			"vless://11111111-2222-3333-4444-555555555555@203.0.113.23:443?type=ws&security=tls&sni=example.org&path=%2F#uzel", false},
		{"raw с reality это TLS",
			"vless://11111111-2222-3333-4444-555555555555@203.0.113.24:443?type=tcp&security=reality&pbk=AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8&sid=01ab&sni=example.org&flow=xtls-rprx-vision#uzel", false},
	}
	for _, s := range sluchai {
		t.Run(s.imya, func(t *testing.T) {
			srv, err := ssylki.Razobrat(s.ssylka)
			if err != nil {
				t.Fatalf("ссылка не разобралась: %v", err)
			}
			if got := srv.BezTLS; got != s.bezTLS {
				t.Fatalf("BezTLS=%v, ожидалось %v: генератор построит не тот исходящий", got, s.bezTLS)
			}
		})
	}
}
