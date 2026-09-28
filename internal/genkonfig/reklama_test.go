package genkonfig

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

func vhodSReklamoy(razresheno []string) Vhod {
	v := vhodSNaborami()
	v.Reklama = &Reklama{
		Fayl:       `C:\ProgramData\Affory\nabory\reklama.srs`,
		Razresheno: razresheno,
		Svoi:       []string{"api.github.com", "Panel.Example.NET"},
	}
	return v
}

func estBlokReklamy(m map[string]any) bool { return m["action"] == "reject" }

func blokIz(t *testing.T, k map[string]any) map[string]any {
	t.Helper()
	i := indeksPravila(t, k, estBlokReklamy)
	if i < 0 {
		t.Fatal("правила блокировки в маршруте нет")
	}
	return pravilaIz(t, k)[i].(map[string]any)
}

// estPetlevoe: правила, которым законно стоять выше блока. Без них туннель
// съедает сам себя, и блок их не отменяет.
func estPetlevoe(m map[string]any) bool {
	if m["action"] == "sniff" || estHijack(m) {
		return true
	}
	if vh, _ := m["inbound"].([]any); len(vh) == 1 && vh[0] == "tun-in" && m["process_path"] != nil {
		return true
	}
	if ip, _ := m["ip_cidr"].([]any); slices.Contains(ip, any("224.0.0.0/4")) {
		return true
	}
	// Правило петли: адреса серверов на их портах (praviloPetli).
	if m["type"] == "logical" && m["mode"] == "or" && m["outbound"] == TegPryamo {
		for _, p := range spisok(m["rules"]) {
			if pm, _ := p.(map[string]any); pm["ip_cidr"] == nil {
				return false
			}
		}
		return true
	}
	return false
}

func rezhimyReklamy() map[string]Vhod {
	s := map[string]Vhod{}

	s["весь трафик без правил"] = vhodSReklamoy([]string{"mc.yandex.ru"})

	sProchim := vhodSReklamoy([]string{"mc.yandex.ru"})
	sProchim.PortProksi = 10809
	sProchim.Domeny = []string{"example.org"}
	sProchim.Protsessy = []string{`C:\Program Files (x86)\Steam\steam.exe`}
	s["весь трафик с прокси и исключениями"] = sProchim

	vyborochnyy := vhodSReklamoy([]string{"mc.yandex.ru"})
	vyborochnyy.PortProksi = 10809
	vyborochnyy.Trafik = &protokol.PravilaTrafika{PoUmolchaniyu: protokol.TrafikPryamo,
		Prilozheniya: []protokol.PraviloPrilozheniya{{Put: `C:\Games\Steam\steam.exe`, Potomki: true, Marshrut: protokol.TrafikVPN}},
		Servisy:      []protokol.PraviloServisa{{Id: "spotify", Marshrut: protokol.TrafikVPN}}}
	s["только выбранное"] = vyborochnyy

	zaprVybor := vhodSReklamoy([]string{"mc.yandex.ru"})
	zaprVybor.Trafik = &protokol.PravilaTrafika{PoUmolchaniyu: protokol.TrafikVPN,
		Domeny: []protokol.PraviloDomena{{Domen: "work.example", Marshrut: protokol.TrafikPryamo}}}
	zaprVybor.VesTrafik = true
	s["блокировка сети с правилами"] = zaprVybor

	zapret := vhodSReklamoy(nil)
	zapret.VesTrafik = true
	s["блокировка сети без правил"] = zapret
	return s
}

// И2. Блок стоит сразу под hijack-dns и выше любого обхода во всех режимах:
// выше него только петлевые правила, без которых туннель съест сам себя.
func TestReklamaNaSvoyomMesteVoVsehRezhimah(t *testing.T) {
	for imya, v := range rezhimyReklamy() {
		k := sobrat(t, v)
		pr := pravilaIz(t, k)
		h, b := indeksPravila(t, k, estHijack), indeksPravila(t, k, estBlokReklamy)
		if h < 0 || b != h+1 {
			t.Errorf("%s: hijack-dns на %d, блок на %d, а обязан стоять сразу под ним", imya, h, b)
			continue
		}
		for i := 0; i < h; i++ {
			if m := pr[i].(map[string]any); !estPetlevoe(m) {
				t.Errorf("%s: выше блока стоит не петлевое правило %d: %v", imya, i, m)
			}
		}
		dns := dnsPravilaIz(t, k)
		if len(dns) < 4 {
			t.Fatalf("%s: DNS-правил %d", imya, len(dns))
		}
		if d, _ := dns[2]["domain"].([]any); len(d) != 1 || d[0] != kanareykaDoH || dns[2]["action"] != "predefined" {
			t.Errorf("%s: dns[2] не канарейка Firefox: %v", imya, dns[2])
		}
		if dns[3]["action"] != "predefined" || !strings.Contains(mustJSON(t, dns[3]), `"reklama"`) {
			t.Errorf("%s: dns[3] не блок: %v", imya, dns[3])
		}
		for i := 0; i < 2; i++ {
			if dns[i]["server"] != TegMestnyy {
				t.Errorf("%s: выше DNS-блока стоит не правило локальных имён: %v", imya, dns[i])
			}
		}
		if imya == "блокировка сети без правил" && len(pr) != b+1 {
			t.Errorf("%s: после блока ещё %d правил", imya, len(pr)-b-1)
		}
	}
}

// И3. Поле no_drop это bool true, иначе после 50 срабатываний ядро начинает
// ронять пакеты молча, и TCP через TUN висит до таймаута.
func TestReklamaPoleNoDropNaMeste(t *testing.T) {
	m := blokIz(t, sobrat(t, vhodSReklamoy(nil)))
	if nd, ok := m["no_drop"].(bool); !ok || !nd {
		t.Fatalf("no_drop %v (%T)", m["no_drop"], m["no_drop"])
	}
	if _, est := m["method"]; est {
		t.Fatal("у reject есть method: ядро ответит не так, как задумано")
	}
}

func TestReklamaDnsOtvetNxdomainSSoa(t *testing.T) {
	if !strings.HasPrefix(soaBloka, ". ") {
		t.Fatalf("владелец SOA не корень: %q", soaBloka)
	}
	dns := dnsPravilaIz(t, sobrat(t, vhodSReklamoy(nil)))
	if len(dns) < 4 {
		t.Fatalf("DNS-правил %d, блока нет", len(dns))
	}
	for _, d := range dns[2:4] {
		ns, _ := d["ns"].([]any)
		if d["action"] != "predefined" || d["rcode"] != "NXDOMAIN" || len(ns) != 1 || ns[0] != soaBloka {
			t.Errorf("ответ блока не NXDOMAIN с SOA: %v", d)
		}
	}
}

// И7. Свои имена точно (domain), исключения человека с поддоменами.
func TestReklamaSvoiNeRezhutsya(t *testing.T) {
	isk := isklyucheniyaReklamy(vhodSReklamoy(nil))
	svoi := isk["domain"].([]string)
	for _, d := range append(slices.Clone(domenyNeBlokiruyutsya), "api.github.com", "panel.example.net") {
		if !slices.Contains(svoi, d) {
			t.Errorf("свой хост %s режется", d)
		}
	}
	if !slices.IsSorted(svoi) || len(slices.Compact(slices.Clone(svoi))) != len(svoi) {
		t.Errorf("свои имена не по порядку или с повторами: %v", svoi)
	}
	if _, est := isk["domain_suffix"]; est {
		t.Error("без исключений человека есть domain_suffix: пустой совпал бы со всем")
	}
	s := isklyucheniyaReklamy(vhodSReklamoy([]string{"mc.yandex.ru"}))
	if suf, _ := s["domain_suffix"].([]string); len(suf) != 1 || suf[0] != "mc.yandex.ru" {
		t.Errorf("исключение человека не суффиксом: %v", s["domain_suffix"])
	}
}

func TestReklamaNashiProtsessyVProksiNeRezhutsya(t *testing.T) {
	v := vhodSReklamoy(nil)
	pod := spisok(blokIz(t, sobrat(t, v))["rules"])
	if len(pod) != 3 {
		t.Fatalf("подправил %d", len(pod))
	}
	tretye := pod[2].(map[string]any)
	pp, _ := tretye["process_path"].([]any)
	if tretye["invert"] != true || len(pp) != len(v.PutiProtsessov) || pp[0] != v.PutiProtsessov[0] || pp[1] != v.PutiProtsessov[1] {
		t.Fatalf("наши процессы через прокси не исключены: %v", tretye)
	}
}

func TestReklamaNaborLocalPosleRemote(t *testing.T) {
	k := sobrat(t, vhodSReklamoy(nil))
	rs := spisok(k["route"].(map[string]any)["rule_set"])
	if len(rs) != 2 {
		t.Fatalf("наборов %d", len(rs))
	}
	if n := rs[0].(map[string]any); n["tag"] != "ru" || n["type"] != "remote" {
		t.Errorf("rule_set[0] %v", n)
	}
	if n := rs[1].(map[string]any); n["tag"] != tegReklamy || n["type"] != "local" || n["format"] != "binary" || n["path"] != `C:\ProgramData\Affory\nabory\reklama.srs` {
		t.Errorf("rule_set[1] %v", n)
	}
	// Без remote-наборов раздел всё равно есть: блоку нужен свой набор.
	bez := vhodSReklamoy(nil)
	bez.Nabory, bez.FaylKesha = nil, ""
	if rs := spisok(sobrat(t, bez)["route"].(map[string]any)["rule_set"]); len(rs) != 1 {
		t.Errorf("без remote-наборов наборов %d", len(rs))
	}
}

// И8. Набор блокировки не становится обходом: ни одно правило, ведущее мимо
// туннеля или к местному резолверу, его не упоминает.
func TestReklamaNeStanovitsyaObhodom(t *testing.T) {
	for imya, v := range rezhimyReklamy() {
		k := sobrat(t, v)
		vse := append(slices.Clone(pravilaIz(t, k)), spisok(k["dns"].(map[string]any)["rules"])...)
		for _, p := range vse {
			m := p.(map[string]any)
			if (m["outbound"] == TegPryamo || m["server"] == TegMestnyy) && strings.Contains(mustJSON(t, m), `"reklama"`) {
				t.Errorf("%s: обход упоминает набор блокировки: %v", imya, m)
			}
		}
	}
}

// И1. Выключено: в конфиге нет ничего от блокировки.
func TestReklamaVyklyuchenaNetNichego(t *testing.T) {
	for _, v := range []Vhod{obraztsovyyVhod(), vhodSNaborami()} {
		telo := mustJSON(t, sobrat(t, v))
		for _, chuzhoe := range []string{`"reklama"`, "predefined", kanareykaDoH, `"reject"`} {
			if strings.Contains(telo, chuzhoe) {
				t.Errorf("блокировка выключена, а в конфиге %s", chuzhoe)
			}
		}
	}
}

func TestReklamaBezFaylaOtkaz(t *testing.T) {
	bezFayla := vhodSReklamoy(nil)
	bezFayla.Reklama.Fayl = ""
	pustoe := vhodSReklamoy([]string{""})
	for _, v := range []Vhod{bezFayla, pustoe} {
		if _, err := SingBox(v); !errors.Is(err, errReklamaNepolnaya) {
			t.Errorf("неполная блокировка: %v", err)
		}
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
