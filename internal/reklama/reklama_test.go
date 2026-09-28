package reklama

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// spisokDlya собирает годный по форме список уровня u: заголовок как у HaGeZi,
// все обязательные домены и добивка до минимума уровня. Лишние строки идут
// после добивки и входят в «Number of entries», если schitat.
func spisokDlya(u Uroven, lishnie []string, schitat bool) []byte {
	o := urovni[u]
	pravila := make([]string, 0, o.minimum+len(obyazatelnye)+len(lishnie))
	for _, d := range obyazatelnye {
		pravila = append(pravila, "||"+d+"^")
	}
	for i := len(pravila); i < o.minimum; i++ {
		pravila = append(pravila, fmt.Sprintf("||dobivka%d.example^", i))
	}
	n := len(pravila)
	if schitat {
		n += len(lishnie)
	}
	pravila = append(pravila, lishnie...)
	var b strings.Builder
	b.WriteString("[Adblock Plus]\n")
	fmt.Fprintf(&b, "! Title: HaGeZi's %s - test\n", o.metka)
	b.WriteString("! Last modified: 28 Sep 2026 08:46 UTC\n")
	b.WriteString("! Version: 2026.0928.0846.00\n")
	fmt.Fprintf(&b, "! Number of entries: %d\n!\n", n)
	for _, p := range pravila {
		b.WriteString(p + "\n")
	}
	return []byte(b.String())
}

func TestRazobratGodnyy(t *testing.T) {
	sp, err := Razobrat(spisokDlya(Bazovyy, nil, true), Bazovyy)
	if err != nil {
		t.Fatalf("годный список отвергнут: %v", err)
	}
	if sp.Pravil != urovni[Bazovyy].minimum {
		t.Fatalf("правил %d, ждали %d", sp.Pravil, urovni[Bazovyy].minimum)
	}
	if sp.Versiya != "2026.0928.0846.00" {
		t.Fatalf("версия %q", sp.Versiya)
	}
	if sp.Sobran == nil || !sp.Sobran.Equal(time.Date(2026, 9, 28, 8, 46, 0, 0, time.UTC)) {
		t.Fatalf("дата сборки %v", sp.Sobran)
	}
	// На вход convert идут только правила, без заголовка и комментариев.
	for i, s := range strings.Split(strings.TrimSuffix(string(sp.Tekst), "\n"), "\n") {
		if !strings.HasPrefix(s, "||") || !strings.HasSuffix(s, "^") {
			t.Fatalf("строка %d текста для ядра не правило: %q", i+1, s)
		}
	}
}

// CRLF, пустые строки и комментарии посреди списка это не повод отказать.
func TestRazobratTerpitFormu(t *testing.T) {
	telo := strings.ReplaceAll(string(spisokDlya(Bazovyy, nil, true)), "\n", "\r\n")
	telo = strings.Replace(telo, "||dobivka100.example^\r\n", "||dobivka100.example^\r\n\r\n! раздел\r\n", 1)
	if _, err := Razobrat([]byte(telo), Bazovyy); err != nil {
		t.Fatalf("список с CRLF, пустой строкой и комментарием отвергнут: %v", err)
	}
}

func TestRazobratOtkazy(t *testing.T) {
	godnyy := string(spisokDlya(Bazovyy, nil, true))
	minimum := urovni[Bazovyy].minimum
	chislo := func(n int) string { return fmt.Sprintf("entries: %d\n", n) }
	// Список с запасом над минимумом, у которого срезан хвост: поймать его
	// может только сверка с «Number of entries».
	zapas := make([]string, 500)
	for i := range zapas {
		zapas[i] = fmt.Sprintf("||zapas%d.example^", i)
	}
	sZapasom := string(spisokDlya(Bazovyy, zapas, true))
	sluchai := map[string]string{
		"нет заголовка":       strings.Replace(godnyy, "[Adblock Plus]\n", "", 1),
		"чужой Title":         strings.Replace(godnyy, "Multi LIGHT", "Multi NORMAL", 1),
		"не HaGeZi":           strings.Replace(godnyy, "HaGeZi's", "Someone's", 1),
		"нет числа правил":    strings.Replace(godnyy, "! Number of entries:", "! Entries:", 1),
		"число правил больше": strings.Replace(godnyy, chislo(minimum), chislo(minimum+1), 1),
		"нет обязательного":   strings.Replace(godnyy, "||an.yandex.ru^\n", "||dobivka-an.example^\n", 1),
		"мало правил": strings.Replace(strings.Replace(godnyy, "||dobivka100.example^\n", "", 1),
			chislo(minimum), chislo(minimum-1), 1),
		"html вместо списка":   "<!DOCTYPE html><html><body>rate limited</body></html>",
		"пустой ответ":         "",
		"обрезан посреди":      sZapasom[:strings.Index(sZapasom, "||zapas400.example^")],
		"правило на зону":      "",
		"жизненно важный":      "",
		"родитель важного":     "",
		"исключение @@":        "",
		"регулярное выражение": "",
		"двойная точка":        "",
		"метка длиннее 63":     "",
		"имя длиннее 253":      "",
		"модификатор":          "",
		"звёздочка":            "",
		"адрес IP":             "",
		"заглавные":            "",
	}
	lishnie := map[string]string{
		"правило на зону":      "||com^",
		"жизненно важный":      "||google.com^",
		"родитель важного":     "||ytimg.com^",
		"исключение @@":        "@@||x.org^",
		"регулярное выражение": "/re/",
		"двойная точка":        "||a..b^",
		"метка длиннее 63":     "||" + strings.Repeat("a", 64) + ".org^",
		"имя длиннее 253":      "||" + strings.Repeat(strings.Repeat("a", 60)+".", 5) + "org^",
		"модификатор":          "||x.org^$third-party",
		"звёздочка":            "||ads*.x.org^",
		"адрес IP":             "||1.2.3.4^",
		"заглавные":            "||Ads.X.org^",
	}
	for imya, stroka := range lishnie {
		sluchai[imya] = string(spisokDlya(Bazovyy, []string{stroka}, true))
	}
	for imya, telo := range sluchai {
		if _, err := Razobrat([]byte(telo), Bazovyy); err == nil {
			t.Errorf("%s: список принят", imya)
		}
	}
	if _, err := Razobrat([]byte(godnyy), rasshirennyy); err == nil {
		t.Error("базовый список принят под адресом расширенного")
	}
}

// Отказ называет строку: без номера и текста разбирать нечего.
func TestRazobratNazyvaetStroku(t *testing.T) {
	_, err := Razobrat(spisokDlya(Bazovyy, []string{"@@||x.org^"}, true), Bazovyy)
	if err == nil || !strings.Contains(err.Error(), "@@||x.org^") || !strings.Contains(err.Error(), "строка") {
		t.Fatalf("отказ не называет строку: %v", err)
	}
}

func TestPrivesti(t *testing.T) {
	sluchai := []struct {
		vhod     string
		uroven   Uroven
		izvesten bool
	}{
		{"", Bazovyy, true},
		{"light", Bazovyy, true},
		{"multi", rasshirennyy, true},
		{"pro", Bazovyy, false},
		{"LIGHT", Bazovyy, false},
	}
	for _, s := range sluchai {
		u, ok := Privesti(s.vhod)
		if u != s.uroven || ok != s.izvesten {
			t.Errorf("Privesti(%q) = %q, %v; ждали %q, %v", s.vhod, u, ok, s.uroven, s.izvesten)
		}
	}
}

func TestVseAdresa(t *testing.T) {
	a := vseAdresa()
	if len(a) != len(urovni) {
		t.Fatalf("адресов %d, уровней %d", len(a), len(urovni))
	}
	for _, s := range a {
		if !strings.HasPrefix(s, "https://raw.githubusercontent.com/hagezi/dns-blocklists/") {
			t.Errorf("чужой адрес %q", s)
		}
	}
}

func TestSleduyushchiyZahod(t *testing.T) {
	seychas := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	chasNazad := seychas.Add(-time.Hour)
	budushchee := seychas.Add(time.Hour)
	sluchai := []struct {
		imya string
		m    *meta
		zhdu time.Time
	}{
		{"нет меты", nil, seychas},
		{"другой уровень", &meta{Uroven: rasshirennyy, Proveren: &chasNazad}, seychas},
		{"встроенный", &meta{Uroven: Bazovyy, Vstroennyy: true, Proveren: &chasNazad}, seychas},
		{"ни одного удачного", &meta{Uroven: Bazovyy}, seychas},
		{"отметка из будущего", &meta{Uroven: Bazovyy, Proveren: &budushchee}, seychas},
		{"удача час назад", &meta{Uroven: Bazovyy, Proveren: &chasNazad}, chasNazad.Add(period)},
	}
	for _, s := range sluchai {
		if got := sleduyushchiyZahod(s.m, Bazovyy, seychas); !got.Equal(s.zhdu) {
			t.Errorf("%s: %v, ждали %v", s.imya, got, s.zhdu)
		}
	}
}

func TestSverkaSPrezhnim(t *testing.T) {
	t0 := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	m := &meta{Uroven: Bazovyy, Pravil: 1000}

	if ok, _, _ := sverkaSPrezhnim(750, m, Bazovyy, t0); !ok {
		t.Fatal("падение на 25% отвергнуто")
	}
	ok, padenie, povtor := sverkaSPrezhnim(600, m, Bazovyy, t0)
	if ok || padenie == nil || padenie.Pravil != 600 || !padenie.Vpervye.Equal(t0) || !povtor.Equal(t0.Add(20*time.Hour)) {
		t.Fatalf("падение на 40%%: %v %+v %v", ok, padenie, povtor)
	}
	m.Padenie = padenie

	ok, p2, povtor2 := sverkaSPrezhnim(600, m, Bazovyy, t0.Add(10*time.Hour))
	if ok || p2 != m.Padenie || !povtor2.Equal(povtor) {
		t.Fatalf("повтор через 10 ч: %v %+v %v", ok, p2, povtor2)
	}
	if ok, _, _ := sverkaSPrezhnim(610, m, Bazovyy, t0.Add(21*time.Hour)); !ok {
		t.Fatal("то же падение через 21 ч не принято")
	}
	ok, p3, povtor3 := sverkaSPrezhnim(660, m, Bazovyy, t0.Add(21*time.Hour))
	if ok || p3 == nil || p3.Pravil != 660 || !povtor3.Equal(t0.Add(41*time.Hour)) {
		t.Fatalf("другое падение через 21 ч: %v %+v %v", ok, p3, povtor3)
	}
	if ok, _, _ := sverkaSPrezhnim(100, &meta{Uroven: rasshirennyy, Pravil: 1000}, Bazovyy, t0); !ok {
		t.Fatal("другой уровень не принят")
	}
	if ok, _, _ := sverkaSPrezhnim(100, &meta{Uroven: Bazovyy, Pravil: 1000, Vstroennyy: true}, Bazovyy, t0); !ok {
		t.Fatal("после встроенного не принят")
	}
	if ok, _, _ := sverkaSPrezhnim(100, nil, Bazovyy, t0); !ok {
		t.Fatal("без меты не принят")
	}
}

func TestVstroennyyEtoSRS(t *testing.T) {
	b, _ := vstroennyy()
	if !bytes.HasPrefix(b, []byte("SRS")) || len(b) < 4 || b[3] != 2 {
		t.Fatalf("встроенный файл не набор .srs версии 2: % x", b[:min(len(b), 4)])
	}
	if len(b) < 200<<10 || len(b) > 2<<20 {
		t.Fatalf("размер встроенного %d вне 200 КБ..2 МБ", len(b))
	}
}

func TestVstroennyySovpadaetSPasportom(t *testing.T) {
	b, m := vstroennyy()
	h := sha256.Sum256(b)
	if hex.EncodeToString(h[:]) != m.Sha256 {
		t.Fatalf("sha256 встроенного %x, в паспорте %s", h, m.Sha256)
	}
	if !m.Vstroennyy || m.Uroven != Bazovyy {
		t.Fatalf("мета встроенного %+v", m)
	}
	ishodnik, err := os.ReadFile("hagezi-light.txt")
	if err != nil {
		t.Fatal(err)
	}
	sp, err := Razobrat(ishodnik, Bazovyy)
	if err != nil {
		t.Fatalf("исходник встроенного не разобрался: %v", err)
	}
	if sp.Pravil != m.Pravil || sp.Versiya != m.Versiya {
		t.Fatalf("исходник: %d правил, версия %s; паспорт: %d, %s", sp.Pravil, sp.Versiya, m.Pravil, m.Versiya)
	}
}

// Встроенный набор обязан собираться из исходника, лежащего рядом: это и есть
// Corresponding Source для GPL. Нужен живой sing-box, как инварианту 8.
func TestVstroennyySobiraetsyaIzIshodnika(t *testing.T) {
	yadro := os.Getenv("AFFORY_SINGBOX")
	if yadro == "" {
		t.Skip("AFFORY_SINGBOX не задан: сборку набора проверяют ворота с живым ядром")
	}
	ishodnik, err := os.ReadFile("hagezi-light.txt")
	if err != nil {
		t.Fatal(err)
	}
	sp, err := Razobrat(ishodnik, Bazovyy)
	if err != nil {
		t.Fatal(err)
	}
	kat := t.TempDir()
	tekst, srs := filepath.Join(kat, "light.txt"), filepath.Join(kat, "light.srs")
	if err := os.WriteFile(tekst, sp.Tekst, 0o600); err != nil {
		t.Fatal(err)
	}
	if v, err := exec.Command(yadro, "--disable-color", "rule-set", "convert", "--type", "adguard", "--output", srs, tekst).CombinedOutput(); err != nil {
		t.Fatalf("convert: %v: %s", err, v)
	}
	sobrannyy, err := os.ReadFile(srs)
	if err != nil {
		t.Fatal(err)
	}
	vstr, _ := vstroennyy()
	if !bytes.Equal(sobrannyy, vstr) {
		t.Fatalf("собранный из исходника набор (%d Б) не совпал со встроенным (%d Б)", len(sobrannyy), len(vstr))
	}
	for d, nado := range map[string]bool{"an.yandex.ru": true, "raw.githubusercontent.com": false} {
		v, err := exec.Command(yadro, "--disable-color", "rule-set", "match", "--format", "binary", srs, d).CombinedOutput()
		if err != nil {
			t.Fatalf("match %s: %v: %s", d, err, v)
		}
		if sovpal := strings.Contains(string(v), "match rules"); sovpal != nado {
			t.Errorf("%s: совпадение %v, ждали %v (%s)", d, sovpal, nado, v)
		}
	}
}

func TestNeizvestnyyUrovenOtvergaetsya(t *testing.T) {
	if _, err := Razobrat(spisokDlya(Bazovyy, nil, true), Uroven("pro")); !errors.Is(err, errNeizvestnyyUroven) {
		t.Fatalf("неизвестный уровень: %v", err)
	}
}
