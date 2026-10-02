package yadra

import (
	"strings"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/sboi"
)

// Строки в том виде, в каком ядро пишет их в yadro.log: с меткой времени,
// уровнем и номером соединения. Первые две из журнала живой машины 02.10.2026
// (адреса и теги заменены), остальные того же устройства по route/conn.go.
const (
	strokaZhivogoYadra   = "+0300 2026-10-02 20:12:53 ERROR [3243729584 145ms] connection: open connection to 198.51.100.20:443 using outbound/selector[vybor]: failed to create session: tls: failed to verify certificate: x509: certificate has expired or is not yet valid: "
	strokaQUIC           = "+0300 2026-10-02 20:22:13 ERROR [3069853351 34ms] connection: open connection to www.gstatic.com:443 using outbound/hysteria2[srv-0a1b2c3d4e5f]: open connection: CRYPTO_ERROR 0x12a (local): tls: failed to verify certificate: x509: certificate has expired or is not yet valid: "
	strokaUDP            = "+0300 2026-10-02 20:22:14 ERROR [1594721246 15.8s] connection: listen packet connection using  using outbound/trojan[srv-ffeeddccbbaa]: tls: failed to verify certificate: x509: certificate signed by unknown authority"
	strokaShumUDP        = "+0300 2026-10-02 20:22:15 ERROR [2087197712 2ms] connection: open connection to 198.51.100.21:443 using outbound/selector[vybor]: dial udp 198.51.100.21:443: An invalid argument was supplied."
	strokaChuzhogoVyhoda = "+0300 2026-10-02 20:22:16 ERROR [1155290006 365ms] connection: connection download closed: remote error: open connection to 198.51.100.9:443 using outbound/direct[direct]: tls: failed to verify certificate: x509: certificate signed by unknown authority"
)

func zhurnalSChasami(t *testing.T, konfig string, chasy *time.Time) *zhurnalYadra {
	t.Helper()
	perehvat(t)
	zabytZhalobyVyhodov(konfig)
	t.Cleanup(func() { zabytZhalobyVyhodov(konfig) })
	return &zhurnalYadra{imya: "sing-box.exe", konfig: konfig, seychas: func() time.Time { return *chasy }}
}

func napisat(t *testing.T, z *zhurnalYadra, stroki ...string) {
	t.Helper()
	for _, s := range stroki {
		if _, err := z.Write([]byte(s + "\n")); err != nil {
			t.Fatal(err)
		}
	}
}

func TestZhalobaVyhodaPoTeguIzStrokiYadra(t *testing.T) {
	chasy := time.Date(2026, 10, 2, 20, 12, 53, 0, time.UTC)
	z := zhurnalSChasami(t, t.Name(), &chasy)
	napisat(t, z, strokaZhivogoYadra, strokaQUIC, strokaUDP)

	sluchai := []struct {
		teg    string
		zhdyom sboi.PrichinaYadra
	}{
		{"vybor", sboi.SrokSertifikata},
		{"srv-0a1b2c3d4e5f", sboi.SrokSertifikata},
		// UDP-сессия тоже звонок серверу: hy2 и tuic отказывают ею.
		{"srv-ffeeddccbbaa", sboi.NeizvestnyyPodpisant},
	}
	for _, sl := range sluchai {
		zh, ok := ZhalobaVyhoda(t.Name(), sl.teg, chasy)
		if !ok || zh.Prichina != sl.zhdyom {
			t.Errorf("тег %s: %q (есть %v), ждали %q", sl.teg, zh.Prichina, ok, sl.zhdyom)
		}
		if ok && !zh.Vremya.Equal(chasy) {
			t.Errorf("тег %s: время жалобы %v, ждали %v", sl.teg, zh.Vremya, chasy)
		}
	}
}

// У живого ядра отказы UDP идут сотнями вперемешку с отказами сертификата.
// Затирай их нераспознанная строка, причина не доживала бы до вопроса.
func TestShumNeZatiraetUznannuyuZhalobu(t *testing.T) {
	chasy := time.Date(2026, 10, 2, 20, 12, 53, 0, time.UTC)
	z := zhurnalSChasami(t, t.Name(), &chasy)
	napisat(t, z, strokaZhivogoYadra)
	chasy = chasy.Add(time.Second)
	napisat(t, z, strokaShumUDP)
	if zh, ok := ZhalobaVyhoda(t.Name(), "vybor", time.Time{}); !ok || zh.Prichina != sboi.SrokSertifikata {
		t.Fatalf("после шума: %q (есть %v)", zh.Prichina, ok)
	}
}

// «remote error: open connection to ... using outbound/direct[direct]» это
// отказ выхода СЕРВЕРА, пересказанный нам. Тег в ней серверный, и записать
// его на наш direct значило бы обвинить в нём нашу сторону.
func TestPereskazannyyOtkazServeraNeNashVyhod(t *testing.T) {
	chasy := time.Date(2026, 10, 2, 20, 12, 53, 0, time.UTC)
	z := zhurnalSChasami(t, t.Name(), &chasy)
	napisat(t, z, strokaChuzhogoVyhoda)
	if zh, ok := ZhalobaVyhoda(t.Name(), "direct", time.Time{}); ok {
		t.Fatalf("пересказ сервера записан на наш выход: %q", zh.Prichina)
	}
}

func TestStarayaZhalobaNeOtdayotsya(t *testing.T) {
	chasy := time.Date(2026, 10, 2, 20, 12, 53, 0, time.UTC)
	z := zhurnalSChasami(t, t.Name(), &chasy)
	napisat(t, z, strokaZhivogoYadra)
	if _, ok := ZhalobaVyhoda(t.Name(), "vybor", chasy.Add(time.Second)); ok {
		t.Fatal("жалоба раньше срока posle отдана как свежая")
	}
	if _, ok := ZhalobaVyhoda(t.Name(), "vybor", chasy); !ok {
		t.Fatal("жалоба ровно в срок posle не отдана")
	}
}

// Основное ядро и ядро пинга одноимённы. Жалоба одного не должна всплывать
// в вопросе про другое, а новый запуск начинает с нуля.
func TestZhalobyRazdelenyPoKonfiguIZabyvayutsya(t *testing.T) {
	chasy := time.Date(2026, 10, 2, 20, 12, 53, 0, time.UTC)
	odin, drugoy := t.Name()+"/sing-box.json", t.Name()+"/sing-box.zamer.json"
	z := zhurnalSChasami(t, odin, &chasy)
	zabytZhalobyVyhodov(drugoy)
	napisat(t, z, strokaZhivogoYadra)
	if _, ok := ZhalobaVyhoda(drugoy, "vybor", time.Time{}); ok {
		t.Fatal("жалоба основного ядра видна в конфиге ядра пинга")
	}
	if _, ok := ZhalobaVyhoda(odin, "vybor", time.Time{}); !ok {
		t.Fatal("жалоба не запомнена: разделять нечего, тест ничего не проверяет")
	}
	zabytZhalobyVyhodov(odin)
	if zh, ok := ZhalobaVyhoda(odin, "vybor", time.Time{}); ok {
		t.Fatalf("жалоба прошлого запуска пережила забвение: %q", zh.Prichina)
	}
}

// Как и жалоба на драйвер: за пределом строк журнал молчит, и называть
// причину строкой, которой в журнале нет, нечестно.
func TestZaPredelomStrokZhalobaNeZapominaetsya(t *testing.T) {
	chasy := time.Date(2026, 10, 2, 20, 12, 53, 0, time.UTC)
	z := zhurnalSChasami(t, t.Name(), &chasy)
	for range predelStrokYadra {
		napisat(t, z, "stroka")
	}
	napisat(t, z, strokaZhivogoYadra)
	if zh, ok := ZhalobaVyhoda(t.Name(), "vybor", time.Time{}); ok {
		t.Fatalf("строка за пределом запомнена: %q", zh.Prichina)
	}
}

func TestRazobratOtkazVyhoda(t *testing.T) {
	sluchai := []struct {
		stroka, teg, oshibka string
		ok                   bool
	}{
		{strokaZhivogoYadra, "vybor", "failed to create session: tls: failed to verify certificate: x509: certificate has expired or is not yet valid: ", true},
		{strokaUDP, "srv-ffeeddccbbaa", "tls: failed to verify certificate: x509: certificate signed by unknown authority", true},
		{strokaChuzhogoVyhoda, "", "", false},
		{"+0300 2026-10-02 20:12:53 ERROR [1 1ms] router: process DNS packet: failed to create session: tls: failed to verify certificate: x509: certificate has expired or is not yet valid: ", "", "", false},
		{"+0300 2026-10-02 20:12:53 ERROR [1 1ms] connection: open connection to 198.51.100.20:443 using outbound/selector[]: x", "", "", false},
		{"", "", "", false},
	}
	for _, sl := range sluchai {
		teg, oshibka, ok := razobratOtkazVyhoda(sl.stroka)
		if teg != sl.teg || oshibka != sl.oshibka || ok != sl.ok {
			t.Errorf("razobratOtkazVyhoda(%q)\n  = %q, %q, %v\n  ждали %q, %q, %v",
				strings.TrimSpace(sl.stroka), teg, oshibka, ok, sl.teg, sl.oshibka, sl.ok)
		}
	}
}
