package petlya

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/genkonfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

func TestPriorityApplicationHelper(t *testing.T) {
	if os.Getenv("AFFORY_PRIORITY_HELPER") != "1" {
		t.Skip("helper only")
	}
	if next := os.Getenv("AFFORY_PRIORITY_NEXT"); next != "" {
		cmd := exec.Command(next, "-test.run=^TestPriorityApplicationHelper$")
		for _, v := range os.Environ() {
			if !strings.HasPrefix(v, "AFFORY_PRIORITY_NEXT=") {
				cmd.Env = append(cmd.Env, v)
			}
		}
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("launched application: %v\n%s", err, out)
		}
		return
	}
	proxy, err := url.Parse(os.Getenv("AFFORY_PRIORITY_PROXY"))
	if err != nil {
		t.Fatal(err)
	}
	transport := &http.Transport{Proxy: http.ProxyURL(proxy), DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	r, err := client.Get(os.Getenv("AFFORY_PRIORITY_TARGET"))
	allowed := false
	if err == nil {
		defer r.Body.Close()
		body, readErr := io.ReadAll(r.Body)
		allowed = readErr == nil && r.StatusCode == 200 && string(body) == "priority-target"
	}
	want := os.Getenv("AFFORY_PRIORITY_ALLOW") == "true"
	if allowed != want {
		t.Fatalf("route outcome allowed=%v want=%v err=%v", allowed, want, err)
	}
}

// Local mixed input substitutes TUN. Application rules come from the product
// generator unchanged except that the VPN route is replaced with reject: this
// gives a deterministic route oracle without contacting any external server.
func TestRealCoreApplicationRulePriority(t *testing.T) {
	Storozhit(t)
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	inner, leaf := filepath.Join(t.TempDir(), "inner.exe"), filepath.Join(t.TempDir(), "leaf.exe")
	for _, path := range []string{inner, leaf} {
		if err := os.WriteFile(path, body, 0700); err != nil {
			t.Fatal(err)
		}
	}
	target := NovayaMishen(t, "priority-target")
	for _, tc := range []struct {
		name                                         string
		childTree, grandchild, explicitLeaf, reverse bool
	}{
		{"exact-child", false, false, false, false},
		{"exact-rule-does-not-capture-launched-apps", false, true, false, false},
		{"tree-child", true, false, false, false},
		{"nearest-launcher", true, true, false, false},
		{"nearest-reversed", true, true, false, true},
		{"explicit-leaf", true, true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			apps := []protokol.PraviloPrilozheniya{
				{Put: strings.ToUpper(self), Potomki: true, Marshrut: protokol.TrafikPryamo},
				{Put: strings.ToUpper(inner), Potomki: tc.childTree, Marshrut: protokol.TrafikVPN},
			}
			if tc.explicitLeaf {
				apps = append(apps, protokol.PraviloPrilozheniya{Put: leaf, Potomki: true, Marshrut: protokol.TrafikPryamo})
			}
			if tc.reverse {
				apps[0], apps[1] = apps[1], apps[0]
			}
			generated, err := genkonfig.SingBox(genkonfig.Vhod{
				Server:         protokol.Server{Id: "test", Host: "127.0.0.1", Port: 443, Transport: "trojan", Parol: "test"},
				Resolver:       netip.MustParseAddr("127.0.0.1"),
				Kandidaty:      []netip.Addr{netip.MustParseAddr("127.0.0.1")},
				PutiProtsessov: []string{`C:\affory-test\svc.exe`, `C:\affory-test\core.exe`},
				ClashApi:       genkonfig.ClashApi{Adres: "127.0.0.1", Port: 9090, Sekret: "unused"},
				Trafik:         &protokol.PravilaTrafika{PoUmolchaniyu: protokol.TrafikVPN, Prilozheniya: apps},
			})
			if err != nil {
				t.Fatal(err)
			}
			var config struct {
				Route struct {
					Rules []map[string]any `json:"rules"`
				} `json:"route"`
			}
			if err := json.Unmarshal(generated, &config); err != nil {
				t.Fatal(err)
			}
			var rules []any
			for _, r := range config.Route.Rules {
				if r["process_path"] == nil && r["process_path_tree"] == nil {
					continue
				}
				// Exclude the service/core loop protection, unrelated to user rules.
				encoded, _ := json.Marshal(r)
				if strings.Contains(string(encoded), "affory-test") {
					continue
				}
				if r["outbound"] == genkonfig.TegSelector {
					delete(r, "outbound")
					r["action"] = "reject"
				}
				rules = append(rules, r)
			}
			rules = append(rules, map[string]any{"action": "reject"})
			y := PodnyatYadro(t, map[string]any{
				"inbounds":  []any{map[string]any{"type": "mixed", "listen": "127.0.0.1", "listen_port": SvobodnyyPort(t)}},
				"outbounds": []any{map[string]any{"type": "direct", "tag": genkonfig.TegPryamo}},
				"route":     map[string]any{"rules": rules},
			})
			if got := y.SprositCherezProksi(t, target.Adres); got != "priority-target" {
				t.Fatal("control failed")
			}
			cmd := exec.Command(inner, "-test.run=^TestPriorityApplicationHelper$")
			allowed := tc.explicitLeaf || (tc.grandchild && !tc.childTree)
			cmd.Env = append(os.Environ(), "AFFORY_PRIORITY_HELPER=1", fmt.Sprintf("AFFORY_PRIORITY_PROXY=http://127.0.0.1:%d", y.PortProksi), "AFFORY_PRIORITY_TARGET="+target.Adres, fmt.Sprintf("AFFORY_PRIORITY_ALLOW=%v", allowed))
			if tc.grandchild {
				cmd.Env = append(cmd.Env, "AFFORY_PRIORITY_NEXT="+leaf)
			}
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("application priority: %v\n%s\n%s", err, out, y.Zhurnal())
			}
		})
	}
}
