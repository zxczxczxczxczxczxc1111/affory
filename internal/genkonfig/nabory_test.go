package genkonfig

import (
	"bytes"
	"errors"
	"testing"
)

// vhodSNaborami is the sample input plus one remote rule set and a cache file.
// The set's URL is a documentation host: the generator never invents URLs, the
// service hands them in, and a test that phoned home would be a leak by design.
func vhodSNaborami() Vhod {
	v := obraztsovyyVhod()
	v.Nabory = []NaborPravil{{
		Teg:  "ru",
		URL:  "https://example.com/geosite-category-ru.srs",
		Fayl: `C:\ProgramData\Affory\nabory\ru.srs`,
	}}
	v.FaylKesha = `C:\ProgramData\Affory\kesh.db`
	return v
}

func TestNaboryNeIspolzuyutUstarevshee(t *testing.T) {
	// download_detour still works in 1.14 and disappears in 1.16. Writing it
	// today means a migration task in six months for zero benefit now.
	k, err := SingBox(vhodSNaborami())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(k, []byte("download_detour")) {
		t.Fatal("в конфиге устаревшее download_detour")
	}
	if !bytes.Contains(k, []byte("http_client")) {
		t.Fatal("нет http_client")
	}
	if !bytes.Contains(k, []byte("initial_path")) {
		t.Fatal("нет initial_path, холодный старт не закрыт")
	}
}

func naboryIz(t *testing.T, k map[string]any) []map[string]any {
	t.Helper()
	r, ok := k["route"].(map[string]any)
	if !ok {
		t.Fatal("нет блока route")
	}
	var itog []map[string]any
	for _, v := range spisok(r["rule_set"]) {
		m, ok := v.(map[string]any)
		if !ok {
			t.Fatalf("набор не объект: %v", v)
		}
		itog = append(itog, m)
	}
	return itog
}

func TestNaborKachaetsyaMimoTunnelya(t *testing.T) {
	// Owner's decision of 31.08.2026: downloads bypass the tunnel. Inside
	// sing-box that is the http_client's detour, and it has to say "direct"
	// in so many words: the implicit default client is deprecated and, worse,
	// it would follow route.final, which is the tunnel.
	n := naboryIz(t, sobrat(t, vhodSNaborami()))
	if len(n) != 1 {
		t.Fatalf("наборов %d, ждали 1", len(n))
	}
	if n[0]["type"] != "remote" || n[0]["format"] != "binary" {
		t.Errorf("набор должен быть remote/binary, а не %v/%v", n[0]["type"], n[0]["format"])
	}
	if n[0]["tag"] != "ru" || n[0]["url"] != "https://example.com/geosite-category-ru.srs" {
		t.Errorf("тег или адрес набора не доехали: %v", n[0])
	}
	if n[0]["initial_path"] != `C:\ProgramData\Affory\nabory\ru.srs` {
		t.Errorf("initial_path не доехал: %v", n[0]["initial_path"])
	}
	hc, ok := n[0]["http_client"].(map[string]any)
	if !ok {
		t.Fatalf("http_client не объект: %v", n[0]["http_client"])
	}
	if hc["detour"] != TegPryamo {
		t.Errorf("загрузка набора идёт через %v, а не мимо туннеля", hc["detour"])
	}
}

func TestKeshFaylVklyuchen(t *testing.T) {
	// A remote set without cache_file is re-downloaded on every start and
	// forgotten on every stop; sing-box documents cache_file as required.
	k := sobrat(t, vhodSNaborami())
	e, _ := k["experimental"].(map[string]any)
	kesh, ok := e["cache_file"].(map[string]any)
	if !ok {
		t.Fatal("нет experimental.cache_file")
	}
	if kesh["enabled"] != true || kesh["path"] != `C:\ProgramData\Affory\kesh.db` {
		t.Errorf("кэш-файл не включён или не там: %v", kesh)
	}
	if _, est := kesh["store_rdrc"]; est {
		t.Error("store_rdrc устарело в 1.14 и уходит в 1.16")
	}
}

func estNabor(m map[string]any) bool { _, ok := m["rule_set"]; return ok }

func TestNaborDayotPravilo(t *testing.T) {
	k := sobrat(t, vhodSNaborami())
	i := indeksPravila(t, k, estNabor)
	if i < 0 {
		t.Fatal("правила по набору нет")
	}
	p := pravilaIz(t, k)[i].(map[string]any)
	if p["outbound"] != TegPryamo {
		t.Errorf("правило по набору ведёт в %v, а не в direct", p["outbound"])
	}
	// A domain match is a convenience exception, so it lives BELOW hijack-dns
	// with ip_is_private, never among the loop rules above it. Above hijack it
	// would let a sniffed domain steal DNS packets from the hijack.
	if h := indeksPravila(t, k, estHijack); i < h {
		t.Errorf("правило по набору (%d) стоит выше hijack-dns (%d)", i, h)
	}
}

func TestVesTrafikOtmenyaetNabory(t *testing.T) {
	// Invariant 6: in kill-switch mode there is no domain rule at all. The
	// rule_set section itself may stay, an unreferenced set costs nothing.
	v := vhodSNaborami()
	v.VesTrafik = true
	k := sobrat(t, v)
	if i := indeksPravila(t, k, estNabor); i >= 0 {
		t.Fatalf("в режиме «весь трафик» правило по набору осталось (индекс %d)", i)
	}
}

func TestBezNaborovNetRazdela(t *testing.T) {
	// The sample input has no sets and no cache: the config must not grow a
	// dangling empty rule_set or a cache_file at an empty path.
	k := sobrat(t, obraztsovyyVhod())
	r := k["route"].(map[string]any)
	if _, est := r["rule_set"]; est {
		t.Error("пустой rule_set в конфиге без наборов")
	}
	if e, _ := k["experimental"].(map[string]any); e["cache_file"] != nil {
		t.Error("cache_file в конфиге без пути к кэшу")
	}
	if i := indeksPravila(t, k, estNabor); i >= 0 {
		t.Error("правило по набору без наборов")
	}
}

func TestVisyachiyNaborLovitsya(t *testing.T) {
	// Measured: sing-box check accepts a rule that names a rule_set nobody
	// declared. Our own tag check is the only judge, so it must know about
	// rule_set references too.
	k := map[string]any{
		"outbounds": []any{map[string]any{"type": "direct", "tag": TegPryamo}},
		"route": map[string]any{
			"rules": []any{map[string]any{"rule_set": []any{"net-takogo"}, "outbound": TegPryamo}},
		},
	}
	err := sveritTegi(k)
	if !errors.Is(err, ErrVisyachiyTeg) {
		t.Fatalf("висячий набор не пойман: %v", err)
	}
}

// Домены из набора идут мимо туннеля, значит и резолвиться обязаны местным
// резолвером (план «шесть удобств» §2).
func TestDnsNaborovIdyotMestnym(t *testing.T) {
	var nashli bool
	for _, p := range dnsPravilaIz(t, sobrat(t, vhodSNaborami())) {
		rs, ok := p["rule_set"].([]any)
		if !ok || len(rs) != 1 || rs[0] != "ru" {
			continue
		}
		nashli = true
		if p["server"] != TegMestnyy {
			t.Errorf("DNS набора идёт в %v, а не в местный резолвер", p["server"])
		}
	}
	if !nashli {
		t.Fatal("в dns.rules нет правила по наборам")
	}
}

// sing-box 1.14 отвергает detour на прямой исходящий БЕЗ единой опции:
// «detour to an empty direct outbound makes no sense» (protocol/direct/
// outbound.go, isEmpty через DeepEqual). Замерено в госте 03.09.2026: набор
// ru ни разу не обновился с первого подъёма. Опция domain_resolver делает
// исходящий непустым и по смыслу верна: прямой трафик резолвится местным.
func TestPryamoyIshodyashchiyNePustoy(t *testing.T) {
	for _, v := range spisok(sobrat(t, vhodSNaborami())["outbounds"]) {
		m, _ := v.(map[string]any)
		if m["tag"] != TegPryamo {
			continue
		}
		if m["domain_resolver"] != TegMestnyy {
			t.Fatalf("прямой исходящий пустой, ядро откажет загрузке набора: %v", m)
		}
		return
	}
	t.Fatal("прямого исходящего нет")
}
