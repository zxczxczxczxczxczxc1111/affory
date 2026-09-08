package genkonfig

import "testing"

// Найдено живьём 03.09.2026 в госте (stend\proverit-nablyudaemost.ps1):
// checkExitIp через локальный прокси показал домашний адрес, а checkLeaks
// объявил утечку. Причина в правиле петли по process_path: sing-box узнаёт
// процесс и у соединений, пришедших через mixed-вход, и наш же affory-svc.exe,
// нарочно постучавшийся в прокси, чтобы выйти ЧЕРЕЗ туннель, уезжал напрямую.
// Правило петли нужно только TUN-входу: через прокси свои процессы идут туда,
// куда просят.
func TestPetlyaProtsessovTolkoDlyaTun(t *testing.T) {
	k := sobrat(t, obraztsovyyVhod())
	for _, p := range pravilaIz(t, k) {
		m := p.(map[string]any)
		if !estProtsessy(m) {
			continue
		}
		if v, ok := m["outbound"]; !ok || v != TegPryamo {
			continue
		}
		vh := strok(m["inbound"])
		if len(vh) != 1 || vh[0] != "tun-in" {
			t.Fatalf("правило петли по процессам не ограничено входом tun-in: %v", m)
		}
		return
	}
	t.Fatal("правила петли по процессам нет")
}
