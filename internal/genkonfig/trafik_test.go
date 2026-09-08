package genkonfig

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

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

func TestTrafficRejectsUnknownServiceAndStrictDirectConflict(t *testing.T) {
	v := obraztsovyyVhod()
	v.Trafik = &protokol.PravilaTrafika{PoUmolchaniyu: protokol.TrafikVPN, Servisy: []protokol.PraviloServisa{{Id: "imaginary", Marshrut: protokol.TrafikVPN}}}
	if _, err := SingBox(v); err == nil {
		t.Fatal("unknown bundle silently disappeared")
	}
	v.Trafik.Servisy = nil
	v.Trafik.PoUmolchaniyu, v.VesTrafik = protokol.TrafikPryamo, true
	if _, err := SingBox(v); err == nil {
		t.Fatal("strict protection accepted direct traffic")
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
