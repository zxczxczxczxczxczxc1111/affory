package genkonfig

import "testing"

// Исключения по доменам (§5 п.1). Держатся на сниффинге: в режиме TUN приходят
// голые адреса, имя достаётся из рукопожатия TLS. Правило удобное, значит
// ниже hijack-dns и отсутствует в режиме «весь трафик».

func vhodSDomenami() Vhod {
	v := obraztsovyyVhod()
	v.Domeny = []string{"example.org", "cdn.example.net"}
	return v
}

func estDomeny(m map[string]any) bool { _, ok := m["domain_suffix"]; return ok }

func TestIsklyucheniyaDomenovNizheHijackISnifa(t *testing.T) {
	k := sobrat(t, vhodSDomenami())
	i := indeksPravila(t, k, estDomeny)
	if i < 0 {
		t.Fatal("правила по исключённым доменам нет")
	}
	p := pravilaIz(t, k)[i].(map[string]any)
	if p["outbound"] != TegPryamo {
		t.Errorf("исключённый домен ведёт в %v, а не в direct", p["outbound"])
	}
	suff, _ := p["domain_suffix"].([]any)
	if len(suff) != 2 || suff[0] != "example.org" || suff[1] != "cdn.example.net" {
		t.Errorf("суффиксы доехали не те: %v", suff)
	}
	if h := indeksPravila(t, k, estHijack); i < h {
		t.Errorf("исключение домена (%d) стоит выше hijack-dns (%d)", i, h)
	}
	// The name comes from the handshake, so the sniff action has to run
	// before any domain rule can match. It is the first rule today; this
	// pins that, because a reorder would not fail check.
	if s := indeksPravila(t, k, func(m map[string]any) bool { return m["action"] == "sniff" }); s < 0 || s > i {
		t.Errorf("sniff (%d) не раньше правила по доменам (%d)", s, i)
	}
}

func TestVesTrafikOtmenyaetDomeny(t *testing.T) {
	v := vhodSDomenami()
	v.VesTrafik = true
	k := sobrat(t, v)
	if i := indeksPravila(t, k, estDomeny); i >= 0 {
		t.Fatalf("в режиме «весь трафик» исключение домена осталось (индекс %d)", i)
	}
}

func TestBezDomenovNetPravila(t *testing.T) {
	k := sobrat(t, obraztsovyyVhod())
	if i := indeksPravila(t, k, estDomeny); i >= 0 {
		t.Fatalf("правило по доменам без единого домена (индекс %d)", i)
	}
}

func TestDnsModeHijackZapisanYavno(t *testing.T) {
	// dns_mode on the TUN sits above every route rule and decides whether TUN
	// intercepts DNS at all. sing-box check does not judge its value ("zzz"
	// exits 0, measured), so the generator's test is the only judge.
	k := sobrat(t, obraztsovyyVhod())
	vh := spisok(k["inbounds"])
	if len(vh) == 0 {
		t.Fatal("нет входящих")
	}
	tun, _ := vh[0].(map[string]any)
	if tun["type"] != "tun" || tun["dns_mode"] != "hijack" {
		t.Fatalf("первый входящий %v/%v, ждали tun с dns_mode hijack", tun["type"], tun["dns_mode"])
	}
}

// DNS должен идти той же дорогой, что и трафик: домен, отправленный мимо
// туннеля, резолвится местным резолвером, иначе российский сайт получает
// голландский узел CDN и прямой путь становится медленнее, чем без VPN
// (решено 03.09.2026, план «шесть удобств» §2).
func dnsPravilaIz(t *testing.T, k map[string]any) []map[string]any {
	t.Helper()
	d, ok := k["dns"].(map[string]any)
	if !ok {
		t.Fatal("нет блока dns")
	}
	var itog []map[string]any
	for _, v := range spisok(d["rules"]) {
		m, ok := v.(map[string]any)
		if !ok {
			t.Fatalf("правило dns не объект: %v", v)
		}
		itog = append(itog, m)
	}
	return itog
}

func TestDnsIsklyuchennyhDomenovIdyotMestnym(t *testing.T) {
	var nashli bool
	for _, p := range dnsPravilaIz(t, sobrat(t, vhodSDomenami())) {
		suff, ok := p["domain_suffix"].([]any)
		if !ok || len(suff) != 2 || suff[0] != "example.org" {
			continue
		}
		nashli = true
		if p["server"] != TegMestnyy {
			t.Errorf("DNS исключённого домена идёт в %v, а не в местный резолвер", p["server"])
		}
	}
	if !nashli {
		t.Fatal("в dns.rules нет правила по исключённым доменам")
	}
}

func TestVesTrafikOtmenyaetDnsDomenov(t *testing.T) {
	v := vhodSDomenami()
	v.VesTrafik = true
	// Суффиксы .local и подобные это петлевое правило, оно остаётся всегда;
	// ищем именно домены человека.
	for _, p := range dnsPravilaIz(t, sobrat(t, v)) {
		for _, s := range spisok(p["domain_suffix"]) {
			if s == "example.org" {
				t.Fatal("в режиме «весь трафик» DNS-правило по доменам осталось")
			}
		}
	}
}
