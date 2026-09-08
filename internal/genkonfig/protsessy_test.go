package genkonfig

import "testing"

// Исключения по процессам это удобные исключения (§5 п.1): путь процесса,
// который человек попросил вести мимо туннеля. Правило по ним живёт НИЖЕ
// hijack-dns и отдельно от правила по нашим процессам: наше держит петлю,
// а это только удобство.

func vhodSProtsessami() Vhod {
	v := obraztsovyyVhod()
	v.Protsessy = []string{`C:\Program Files (x86)\Steam\steam.exe`}
	return v
}

func estChuzhieProtsessy(m map[string]any) bool {
	p, ok := m["process_path"].([]any)
	return ok && len(p) == 1 && p[0] == `C:\Program Files (x86)\Steam\steam.exe`
}

func TestIsklyucheniyaProtsessovNizheHijack(t *testing.T) {
	k := sobrat(t, vhodSProtsessami())
	i := indeksPravila(t, k, estChuzhieProtsessy)
	if i < 0 {
		t.Fatal("правила по исключённому процессу нет")
	}
	p := pravilaIz(t, k)[i].(map[string]any)
	if p["outbound"] != TegPryamo {
		t.Errorf("исключённый процесс ведёт в %v, а не в direct", p["outbound"])
	}
	// Invariant 3: above hijack-dns stand EXACTLY our own process rule and the
	// candidate rule. A user exclusion up there would let a process steal its
	// DNS from the hijack and resolve through the home provider in the clear.
	if h := indeksPravila(t, k, estHijack); i < h {
		t.Errorf("исключение процесса (%d) стоит выше hijack-dns (%d)", i, h)
	}
	// Our own loop rule is untouched and still separate.
	if n := indeksPravila(t, k, estProtsessy); n < 0 || n == i {
		t.Errorf("правило по нашим процессам слилось с исключениями (индексы %d и %d)", n, i)
	}
}

func TestVesTrafikOtmenyaetProtsessy(t *testing.T) {
	// Invariant 6: no user process rule in kill-switch mode, or Steam goes to
	// direct where the firewall kills it, and downloads at zero in silence.
	v := vhodSProtsessami()
	v.VesTrafik = true
	k := sobrat(t, v)
	if i := indeksPravila(t, k, estChuzhieProtsessy); i >= 0 {
		t.Fatalf("в режиме «весь трафик» исключение процесса осталось (индекс %d)", i)
	}
	if indeksPravila(t, k, estProtsessy) < 0 {
		t.Fatal("режим снёс и правило по нашим процессам: туннель съест сам себя")
	}
}

func TestBezIsklyucheniyOdnoPraviloProtsessov(t *testing.T) {
	k := sobrat(t, obraztsovyyVhod())
	n := 0
	for _, p := range pravilaIz(t, k) {
		if m, ok := p.(map[string]any); ok && estProtsessy(m) {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("правил по процессам %d без единого исключения, ждали одно (своё)", n)
	}
}
