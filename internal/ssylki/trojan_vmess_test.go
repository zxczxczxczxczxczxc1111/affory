package ssylki

import (
	"encoding/base64"
	"testing"
)

// trojan, vmess и httpupgrade до 0.7.0 разбирались только чтобы быть
// помеченными «не поддержан». Ядро при этом умело их всё это время: у пакетов
// protocol/trojan, protocol/vmess и transport/v2rayhttpupgrade в sing-box нет
// ни одного условия сборки, и регистрируются они безусловно.

func TestTrojanRazbiraetsya(t *testing.T) {
	s, err := Razobrat("trojan://parol-servera@example.org:443?sni=example.org&fp=chrome#Германия")
	if err != nil {
		t.Fatal(err)
	}
	if s.Transport != "trojan" {
		t.Fatalf("транспорт %q", s.Transport)
	}
	if s.Parol != "parol-servera" {
		t.Fatalf("пароль %q", s.Parol)
	}
	if s.Host != "example.org" || s.Port != 443 || s.Imya != "Германия" || s.Sni != "example.org" {
		t.Fatalf("разобрано не то: %+v", s)
	}
	if s.BezTLS {
		t.Fatal("trojan без TLS не бывает, флаг выставлен неверно")
	}
}

func TestTrojanPoverhWs(t *testing.T) {
	s, err := Razobrat("trojan://p@example.org:443?type=ws&path=%2Fput&host=example.org")
	if err != nil {
		t.Fatal(err)
	}
	if s.Transport != "trojan-ws" || s.Put != "/put" || s.HostZagolovka != "example.org" {
		t.Fatalf("разобрано не то: %+v", s)
	}
}

func TestTrojanBezParolyaOtvergaetsya(t *testing.T) {
	if _, err := Razobrat("trojan://@example.org:443"); err == nil {
		t.Fatal("trojan без пароля принят")
	}
}

// vmess это base64 от JSON, и панели пишут числа то числами, то строками.
func vmessSsylka(t *testing.T, telo string) string {
	t.Helper()
	return "vmess://" + base64.StdEncoding.EncodeToString([]byte(telo))
}

func TestVmessRazbiraetsya(t *testing.T) {
	s, err := Razobrat(vmessSsylka(t, `{"v":"2","ps":"Нидерланды","add":"example.org","port":"443","id":"11111111-2222-3333-4444-555555555555","aid":"0","scy":"auto","net":"tcp","tls":"tls","sni":"example.org"}`))
	if err != nil {
		t.Fatal(err)
	}
	if s.Transport != "vmess" {
		t.Fatalf("транспорт %q", s.Transport)
	}
	if s.Uuid != "11111111-2222-3333-4444-555555555555" || s.Port != 443 || s.Imya != "Нидерланды" {
		t.Fatalf("разобрано не то: %+v", s)
	}
	if s.Shifr != "auto" {
		t.Fatalf("шифр %q", s.Shifr)
	}
	if s.BezTLS {
		t.Fatal("tls в ссылке был, флаг выставлен неверно")
	}
}

func TestVmessChislaChislamiIStrokami(t *testing.T) {
	// Одна и та же панель пишет port и aid то так, то этак. Разбор под одну
	// форму объявляет исправную ссылку битой.
	for _, telo := range []string{
		`{"add":"h","port":443,"id":"u","aid":2,"net":"tcp"}`,
		`{"add":"h","port":"443","id":"u","aid":"2","net":"tcp"}`,
	} {
		s, err := Razobrat(vmessSsylka(t, telo))
		if err != nil {
			t.Fatalf("%s: %v", telo, err)
		}
		if s.Port != 443 || s.AlterId != 2 {
			t.Fatalf("%s: порт %d, alterId %d", telo, s.Port, s.AlterId)
		}
	}
}

func TestVmessPoverhWs(t *testing.T) {
	s, err := Razobrat(vmessSsylka(t, `{"add":"example.org","port":"443","id":"u","net":"ws","path":"/put","host":"example.org","tls":"tls"}`))
	if err != nil {
		t.Fatal(err)
	}
	if s.Transport != "vmess-ws" || s.Put != "/put" || s.HostZagolovka != "example.org" {
		t.Fatalf("разобрано не то: %+v", s)
	}
}

func TestVmessBezTlsEtoNeOshibka(t *testing.T) {
	// vmess поверх открытого tcp живёт на портах 80 и 8080 в живых сборниках.
	s, err := Razobrat(vmessSsylka(t, `{"add":"h","port":"80","id":"u","net":"tcp"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !s.BezTLS {
		t.Fatal("tls в ссылке не было, а флаг не выставлен")
	}
}

func TestVmessBityyBase64EtoKrivayaSsylkaANeTransport(t *testing.T) {
	// Разница важна на экране: «не поддержан» посылает человека искать другой
	// сервер, «ссылка не разобрана» посылает искать опечатку.
	if _, err := Razobrat("vmess://???"); err == nil {
		t.Fatal("мусор принят")
	}
}

func TestVlessPoverhHttpupgrade(t *testing.T) {
	s, err := Razobrat("vless://11111111-2222-3333-4444-555555555555@example.org:443?type=httpupgrade&security=tls&path=%2Fput&host=example.org")
	if err != nil {
		t.Fatal(err)
	}
	if s.Transport != "httpupgrade" || s.Put != "/put" || s.HostZagolovka != "example.org" {
		t.Fatalf("разобрано не то: %+v", s)
	}
}
