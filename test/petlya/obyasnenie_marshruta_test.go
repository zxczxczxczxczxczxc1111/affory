package petlya

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/genkonfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/yadra"
)

func configDlyaObyasneniya(t *testing.T, trafik protokol.PravilaTrafika, block, ru bool) map[string]any {
	t.Helper()
	v := genkonfig.Vhod{
		Server:   protokol.Server{Id: "test", Host: "198.51.100.7", Port: 443, Transport: "trojan", Parol: "test"},
		Resolver: netip.MustParseAddr("127.0.0.1"), Kandidaty: []netip.Addr{netip.MustParseAddr("198.51.100.7")},
		PutiProtsessov: []string{`C:\affory-test\svc.exe`, `C:\affory-test\core.exe`},
		ClashApi:       genkonfig.ClashApi{Adres: "127.0.0.1", Port: 9090, Sekret: "unused"},
		Trafik:         &trafik, VesTrafik: block, PortProksi: SvobodnyyPort(t),
	}
	if ru {
		v.Nabory = []genkonfig.NaborPravil{{Teg: "test-list", URL: "https://198.51.100.7/list.srs", Fayl: `C:\test.srs`}}
	}
	body, err := genkonfig.SingBox(v)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(body, &config); err != nil {
		t.Fatal(err)
	}
	return config
}

func TestRealCoreConnectionExplanation(t *testing.T) {
	Storozhit(t)
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer target.Close()
	defer close(release)
	generated := configDlyaObyasneniya(t, protokol.PravilaTrafika{PoUmolchaniyu: protokol.TrafikVPN, Prilozheniya: []protokol.PraviloPrilozheniya{{Put: self, Marshrut: protokol.TrafikPryamo}}}, false, false)
	route := generated["route"].(map[string]any)
	var rules []any
	for _, raw := range route["rules"].([]any) {
		r := raw.(map[string]any)
		if r["outbound"] == genkonfig.TegSelector {
			delete(r, "outbound")
			r["action"] = "reject"
		}
		rules = append(rules, r)
	}
	rules = append(rules, map[string]any{"action": "reject"})
	route["rules"] = rules
	route["final"] = "direct"
	route["default_domain_resolver"] = "test-dns"
	apiPort := SvobodnyyPort(t)
	y := PodnyatYadro(t, map[string]any{
		"inbounds": []any{map[string]any{"type": "mixed", "tag": "probe-in", "listen": "127.0.0.1", "listen_port": SvobodnyyPort(t)}},
		"route":    route, "outbounds": []any{map[string]any{"type": "direct", "tag": "direct"}},
		"dns":          map[string]any{"servers": []any{map[string]any{"type": "hosts", "tag": "test-dns", "predefined": map[string]any{"api.example.org": []string{"127.0.0.1"}}}}},
		"experimental": map[string]any{"clash_api": map[string]any{"external_controller": fmt.Sprintf("127.0.0.1:%d", apiPort), "secret": "test-secret"}},
	})
	u, err := url.Parse(target.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxy, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", y.PortProksi))
	transport := &http.Transport{Proxy: http.ProxyURL(proxy)}
	defer transport.CloseIdleConnections()
	client := http.Client{Transport: transport, Timeout: 5 * time.Second}
	r, err := client.Get("http://api.example.org:" + u.Port())
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	connections, err := yadra.Soedineniya(ctx, fmt.Sprintf("127.0.0.1:%d", apiPort), "test-secret")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range connections {
		if c.Host == "api.example.org" && strings.EqualFold(c.Protsess, self) {
			if c.Vyhod != "direct" || !strings.HasPrefix(c.Pravilo, "process_path=") {
				t.Fatalf("wrong actual explanation: %+v", c)
			}
			return
		}
	}
	t.Fatalf("actual application/site connection missing: %+v", connections)
}

// DNS rules and their order come unchanged from the product generator. Only
// the two DNS servers are replaced by distinct local answers: no host DNS,
// real VPN, or Internet request is involved in the route-selection oracle.
func TestRealCoreDNSExplanation(t *testing.T) {
	Storozhit(t)
	for _, tc := range []struct {
		name, host                 string
		apps, domains              []string
		appRoute, domainRoute, def protokol.MarshrutTrafika
		service, ru, block, local  bool
	}{
		{name: "app-vpn-site-direct", host: "example.org", apps: []string{`C:\browser.exe`}, appRoute: protokol.TrafikVPN, domains: []string{"example.org"}, domainRoute: protokol.TrafikPryamo, def: protokol.TrafikPryamo, local: true},
		{name: "app-direct-site-vpn", host: "example.org", apps: []string{`C:\browser.exe`}, appRoute: protokol.TrafikPryamo, domains: []string{"example.org"}, domainRoute: protokol.TrafikVPN, def: protokol.TrafikPryamo},
		{name: "vpn-app-changes-dns-default", host: "unknown.org", apps: []string{`C:\browser.exe`}, appRoute: protokol.TrafikVPN, def: protokol.TrafikPryamo},
		{name: "direct-default", host: "unknown.org", def: protokol.TrafikPryamo, local: true},
		{name: "local-name-before-explicit-vpn", host: "router.local", domains: []string{"router.local"}, domainRoute: protokol.TrafikVPN, def: protokol.TrafikVPN, local: true},
		{name: "single-label", host: "printer", def: protokol.TrafikVPN, local: true},
		{name: "service", host: "youtube.com", service: true, def: protokol.TrafikPryamo},
		{name: "site-before-service", host: "youtube.com", service: true, domains: []string{"youtube.com"}, domainRoute: protokol.TrafikPryamo, def: protokol.TrafikVPN, local: true},
		{name: "ruleset-before-default", host: "listed.test", ru: true, def: protokol.TrafikVPN, local: true},
		{name: "explicit-before-ruleset", host: "listed.test", ru: true, domains: []string{"listed.test"}, domainRoute: protokol.TrafikVPN, def: protokol.TrafikPryamo},
		{name: "block-disables-ruleset", host: "listed.test", ru: true, block: true, def: protokol.TrafikVPN},
		{name: "block-keeps-local-names", host: "router.local", block: true, def: protokol.TrafikVPN, local: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			trafik := protokol.PravilaTrafika{PoUmolchaniyu: tc.def}
			for _, path := range tc.apps {
				trafik.Prilozheniya = append(trafik.Prilozheniya, protokol.PraviloPrilozheniya{Put: path, Marshrut: tc.appRoute})
			}
			for _, host := range tc.domains {
				trafik.Domeny = append(trafik.Domeny, protokol.PraviloDomena{Domen: host, Marshrut: tc.domainRoute})
			}
			if tc.service {
				trafik.Servisy = []protokol.PraviloServisa{{Id: "youtube", Marshrut: protokol.TrafikVPN}}
			}
			generated := configDlyaObyasneniya(t, trafik, tc.block, tc.ru)
			dns := generated["dns"].(map[string]any)
			dns["servers"] = []any{
				map[string]any{"type": "hosts", "tag": genkonfig.TegMestnyy, "predefined": map[string]any{tc.host: []string{"192.0.2.1"}}},
				map[string]any{"type": "hosts", "tag": genkonfig.TegTunnel, "predefined": map[string]any{tc.host: []string{"192.0.2.2"}}},
			}
			route := map[string]any{"default_domain_resolver": genkonfig.TegMestnyy, "rules": []any{map[string]any{"inbound": []string{"dns-test"}, "action": "hijack-dns"}}}
			if tc.ru {
				route["rule_set"] = []any{map[string]any{"type": "inline", "tag": "test-list", "rules": []any{map[string]any{"domain_suffix": []string{"listed.test"}}}}}
			}
			dnsPort := SvobodnyyPortTCPUDP(t)
			PodnyatYadro(t, map[string]any{
				"inbounds": []any{
					map[string]any{"type": "mixed", "listen": "127.0.0.1", "listen_port": SvobodnyyPort(t)},
					map[string]any{"type": "direct", "tag": "dns-test", "listen": "127.0.0.1", "listen_port": dnsPort},
				}, "dns": dns, "route": route, "outbounds": []any{map[string]any{"type": "direct", "tag": "direct"}},
			})
			resolver := net.Resolver{PreferGo: true, Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "udp", fmt.Sprintf("127.0.0.1:%d", dnsPort))
			}}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			ips, err := resolver.LookupIP(ctx, "ip4", tc.host+".")
			want := "192.0.2.2"
			if tc.local {
				want = "192.0.2.1"
			}
			if err != nil || len(ips) != 1 || ips[0].String() != want {
				t.Fatalf("DNS selection got=%v err=%v want=%s", ips, err, want)
			}
		})
	}
}

// The generated route order stays intact. A loopback mixed listener stands in
// for the TUN input, and VPN actions reject instead of contacting a server.
func TestRealCoreApplicationAndSiteExplanation(t *testing.T) {
	Storozhit(t)
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name                  string
		app                   protokol.MarshrutTrafika
		domains               []protokol.PraviloDomena
		service, proxy, allow bool
	}{
		{name: "app-vpn-before-direct-site", app: protokol.TrafikVPN, domains: []protokol.PraviloDomena{{Domen: "youtube.com", Marshrut: protokol.TrafikPryamo}}},
		{name: "app-direct-before-vpn-site", app: protokol.TrafikPryamo, domains: []protokol.PraviloDomena{{Domen: "youtube.com", Marshrut: protokol.TrafikVPN}}, allow: true},
		{name: "site-before-service", domains: []protokol.PraviloDomena{{Domen: "youtube.com", Marshrut: protokol.TrafikPryamo}}, service: true, allow: true},
		{name: "child-site-before-site", domains: []protokol.PraviloDomena{{Domen: "youtube.com", Marshrut: protokol.TrafikVPN}, {Domen: "www.youtube.com", Marshrut: protokol.TrafikPryamo}}, allow: true},
		{name: "service-before-default", service: true},
		{name: "explicit-proxy-before-direct-app", app: protokol.TrafikPryamo, proxy: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			trafik := protokol.PravilaTrafika{PoUmolchaniyu: protokol.TrafikPryamo, Domeny: tc.domains}
			if tc.app != "" {
				trafik.Prilozheniya = []protokol.PraviloPrilozheniya{{Put: self, Marshrut: tc.app}}
			}
			if tc.service {
				trafik.Servisy = []protokol.PraviloServisa{{Id: "youtube", Marshrut: protokol.TrafikVPN}}
			}
			generated := configDlyaObyasneniya(t, trafik, false, false)
			route := generated["route"].(map[string]any)
			for _, raw := range route["rules"].([]any) {
				r := raw.(map[string]any)
				if r["outbound"] == genkonfig.TegSelector {
					delete(r, "outbound")
					r["action"] = "reject"
				}
			}
			route["default_domain_resolver"] = "test-dns"
			inbound := "probe-in"
			if tc.proxy {
				inbound = genkonfig.TegProksiVhod
			}
			y := PodnyatYadro(t, map[string]any{
				"inbounds": []any{map[string]any{"type": "mixed", "tag": inbound, "listen": "127.0.0.1", "listen_port": SvobodnyyPort(t)}},
				"route":    route, "outbounds": []any{map[string]any{"type": "direct", "tag": "direct"}},
				"dns": map[string]any{"servers": []any{map[string]any{"type": "hosts", "tag": "test-dns", "predefined": map[string]any{"www.youtube.com": []string{"127.0.0.1"}}}}},
			})
			target := NovayaMishen(t, "explanation-target")
			u, err := url.Parse(target.Adres)
			if err != nil {
				t.Fatal(err)
			}
			proxy, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", y.PortProksi))
			transport := &http.Transport{Proxy: http.ProxyURL(proxy), DisableKeepAlives: true}
			defer transport.CloseIdleConnections()
			client := http.Client{Transport: transport, Timeout: 3 * time.Second}
			r, err := client.Get("http://www.youtube.com:" + u.Port())
			allowed := err == nil && r.StatusCode == 200
			if r != nil {
				r.Body.Close()
			}
			if allowed != tc.allow {
				t.Fatalf("route allowed=%v want=%v err=%v\n%s", allowed, tc.allow, err, y.Zhurnal())
			}
		})
	}
}
