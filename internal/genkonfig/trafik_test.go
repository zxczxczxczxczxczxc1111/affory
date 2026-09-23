package genkonfig

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/ssylki"
)

func TestDistinctEndpointProfilesAcceptedByShippingCore(t *testing.T) {
	core := os.Getenv("AFFORY_SINGBOX")
	if core == "" {
		t.Skip("AFFORY_SINGBOX not set")
	}
	r, err := ssylki.RazobratSpisok([]byte("trojan://first@192.0.2.225:443?sni=a.example#One\ntrojan://second@192.0.2.225:443?sni=b.example#Two"))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Servery) != 2 {
		t.Fatal("parser lost a profile")
	}
	v := obraztsovyyVhod()
	v.Server = r.Servery[1]
	v.Servery = r.Servery
	k := sobrat(t, v)
	b, err := json.Marshal(k)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "profiles.json")
	if err := os.WriteFile(p, b, 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(core, "check", "-c", p).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
}

func TestSelectiveTrafficKeepsExplicitRoutesAndDNS(t *testing.T) {
	v := obraztsovyyVhod()
	v.PortProksi = 10809
	v.Trafik = &protokol.PravilaTrafika{PoUmolchaniyu: protokol.TrafikPryamo,
		Prilozheniya: []protokol.PraviloPrilozheniya{{Put: `C:\Games\Steam\steam.exe`, Potomki: true, Marshrut: protokol.TrafikVPN}},
		Domeny:       []protokol.PraviloDomena{{Domen: "static.youtube.com", Marshrut: protokol.TrafikPryamo}},
		Servisy:      []protokol.PraviloServisa{{Id: "youtube", Marshrut: protokol.TrafikVPN}}}
	k := sobrat(t, v)
	route, dns := k["route"].(map[string]any), k["dns"].(map[string]any)
	if route["final"] != TegPryamo {
		t.Fatal("default traffic was silently sent through VPN")
	}
	rules := route["rules"].([]any)
	indices := map[string]int{}
	for i, raw := range rules {
		r := raw.(map[string]any)
		if r["process_path_tree"] != nil {
			indices["app"] = i
			if r["outbound"] != TegSelector {
				t.Fatal(r)
			}
		}
		if d, ok := r["domain_suffix"].([]any); ok {
			if d[0] == "static.youtube.com" {
				indices["domain"] = i
				if r["outbound"] != TegPryamo {
					t.Fatal(r)
				}
			}
			if d[0] == "youtube.com" {
				indices["service"] = i
				if r["outbound"] != TegSelector {
					t.Fatal(r)
				}
			}
		}
	}
	if len(indices) != 3 || indices["app"] >= indices["domain"] || indices["domain"] >= indices["service"] {
		t.Fatal(indices)
	}
	// Windows DNS may belong to svchost, because apparently one owner was too easy.
	if dns["final"] != TegTunnel {
		t.Fatal("selected VPN apps must resolve blocked names through the tunnel")
	}
	dnsRules := dns["rules"].([]any)
	if dnsRules[2].(map[string]any)["server"] != TegMestnyy || dnsRules[3].(map[string]any)["server"] != TegTunnel {
		t.Fatal(dnsRules)
	}
	v.Trafik.Prilozheniya = nil
	k = sobrat(t, v)
	if k["dns"].(map[string]any)["final"] != TegMestnyy {
		t.Fatal("service-only mode changed unrelated DNS")
	}
}

// Неизвестный сервис конфиг НЕ рушит, но и правила из него не делает.
//
// Прежде здесь стоял отказ, и он стоил жалобы 21.09.2026: каталог едет внутри
// программы, WhatsApp ушёл из него 16.09.2026, а правило осталось лежать в
// наборе человека. Отказ сборки означал, что подключиться нельзя ВООБЩЕ, и
// снять правило было негде: окно рисует список из каталога. Годность ввода
// проверяет setRules, набор на диске чинится приведением на чтении, а сборка
// конфига просто не выдумывает доменов.
func TestNeizvestnyySeriviNeRushitKonfigINeDayotPravil(t *testing.T) {
	v := obraztsovyyVhod()
	v.Trafik = &protokol.PravilaTrafika{PoUmolchaniyu: protokol.TrafikVPN, Servisy: []protokol.PraviloServisa{{Id: "imaginary", Marshrut: protokol.TrafikVPN}}}
	k := sobrat(t, v)
	telo, err := json.Marshal(k)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(telo), "imaginary") {
		t.Fatalf("правило неизвестного сервиса уехало в конфиг: %s", telo)
	}
	// Правила трафика собираются те же, что и без сервиса вовсе.
	bez := obraztsovyyVhod()
	bez.Trafik = &protokol.PravilaTrafika{PoUmolchaniyu: protokol.TrafikVPN}
	teloBez, err := json.Marshal(sobrat(t, bez))
	if err != nil {
		t.Fatal(err)
	}
	if string(telo) != string(teloBez) {
		t.Fatal("конфиг с осиротевшим правилом отличается от конфига без него")
	}
}

// Блокировка сети вне VPN больше не ОТВЕРГАЕТ прямые правила, а перестаёт их
// применять (23.09.2026). До этого сборка падала, и окно запрещало включать
// защиту, пока человек сам не перепишет каждое правило; теперь его набор
// остаётся на диске целиком и возвращается в дело, когда защиту выключат.
func TestPryamyeNeMeshayutBlokirovke(t *testing.T) {
	v := obraztsovyyVhod()
	v.Trafik = &protokol.PravilaTrafika{PoUmolchaniyu: protokol.TrafikPryamo}
	v.VesTrafik = true
	if _, err := SingBox(v); err != nil {
		t.Fatalf("блокировка с прямым умолчанием не собралась: %v", err)
	}
}

func TestBlokirovkaNePrimenyaetPryamyePravila(t *testing.T) {
	v := obraztsovyyVhod()
	v.Trafik = &protokol.PravilaTrafika{
		PoUmolchaniyu: protokol.TrafikPryamo,
		Prilozheniya: []protokol.PraviloPrilozheniya{
			{Put: `C:\Games\Steam\steam.exe`, Potomki: true, Marshrut: protokol.TrafikVPN},
			{Put: `C:\Windows
otepad.exe`, Potomki: false, Marshrut: protokol.TrafikPryamo},
		},
		Domeny:  []protokol.PraviloDomena{{Domen: "work.example", Marshrut: protokol.TrafikPryamo}},
		Servisy: []protokol.PraviloServisa{{Id: "youtube", Marshrut: protokol.TrafikPryamo}},
	}
	v.VesTrafik = true
	telo, err := SingBox(v)
	if err != nil {
		t.Fatal(err)
	}
	vidno := string(telo)
	for _, chuzhoe := range []string{"notepad.exe", "work.example", "youtube.com"} {
		if strings.Contains(vidno, chuzhoe) {
			t.Fatalf("прямое правило %q попало в конфиг под блокировкой", chuzhoe)
		}
	}
	if !strings.Contains(vidno, "steam.exe") {
		t.Fatal("правило через VPN пропало вместе с прямыми")
	}
	// Тег direct остаётся среди исходящих: на нём держатся петлевые исключения.
	// Значение имеет то, куда уходит ВЕСЬ остальной трафик.
	var k map[string]any
	if err := json.Unmarshal(telo, &k); err != nil {
		t.Fatal(err)
	}
	final := k["route"].(map[string]any)["final"]
	if final != TegSelector {
		t.Fatalf("итоговый маршрут под блокировкой %v, а не через VPN", final)
	}
}

// Набор человека НЕ меняется: отсечение живёт в копии на время сборки.
func TestBlokirovkaNeTrogaetNaborCheloveka(t *testing.T) {
	v := obraztsovyyVhod()
	trafik := &protokol.PravilaTrafika{
		PoUmolchaniyu: protokol.TrafikPryamo,
		Domeny:        []protokol.PraviloDomena{{Domen: "work.example", Marshrut: protokol.TrafikPryamo}},
	}
	v.Trafik, v.VesTrafik = trafik, true
	if _, err := SingBox(v); err != nil {
		t.Fatal(err)
	}
	if trafik.PoUmolchaniyu != protokol.TrafikPryamo || len(trafik.Domeny) != 1 {
		t.Fatalf("сборка переписала набор человека: %+v", trafik)
	}
}

func TestModernTrafficAcceptedByShippingCore(t *testing.T) {
	core := os.Getenv("AFFORY_SINGBOX")
	if core == "" {
		t.Skip("AFFORY_SINGBOX not set")
	}
	v := obraztsovyyVhod()
	v.Trafik = &protokol.PravilaTrafika{PoUmolchaniyu: protokol.TrafikPryamo,
		Prilozheniya: []protokol.PraviloPrilozheniya{{Put: `C:\Games\Steam\steam.exe`, Potomki: true, Marshrut: protokol.TrafikVPN}},
		Servisy:      []protokol.PraviloServisa{{Id: "youtube", Marshrut: protokol.TrafikVPN}}}
	k := sobrat(t, v)
	b, err := json.Marshal(k)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(p, b, 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(core, "check", "-c", p).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
}
