package ssylki_test

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/ssylki"
)

// tudaObratno проверяет главное правило сборки: Razobrat(Sobrat(s)) == s.
// NebezopasnyyIgnorirovan сравнивается отдельно: сборка его не пишет нарочно.
func tudaObratno(t *testing.T, ishodnaya string) {
	t.Helper()
	pervyy, err := ssylki.Razobrat(ishodnaya)
	if err != nil {
		t.Fatalf("исходная ссылка не разобралась: %v", err)
	}
	sobrannaya, err := ssylki.Sobrat(pervyy)
	if err != nil {
		t.Fatalf("сборка отказала: %v", err)
	}
	vtoroy, err := ssylki.Razobrat(sobrannaya)
	if err != nil {
		t.Fatalf("собранная ссылка не разобралась: %v", err)
	}
	if vtoroy.NebezopasnyyIgnorirovan {
		t.Errorf("собранная ссылка несёт insecure")
	}
	pervyy.NebezopasnyyIgnorirovan = false
	if pervyy != vtoroy {
		t.Errorf("запись изменилась при сборке\nбыло  %+v\nстало %+v", pervyy, vtoroy)
	}
}

// Весь корпус, который разбор принимает: и наши формы, и снятые с чужих панелей.
// Число принятых ссылок сверяется явно: молча опустевший корпус тоже зелёный.
func TestSborkaTudaObratnoPoKorpusu(t *testing.T) {
	fayly, err := filepath.Glob(filepath.Join("testdata", "*.txt"))
	if err != nil {
		t.Fatal(err)
	}
	var proveneno int
	for _, put := range fayly {
		b, err := os.ReadFile(put)
		if err != nil {
			t.Fatal(err)
		}
		for n, stroka := range strings.Split(string(b), "\n") {
			stroka = strings.TrimSpace(stroka)
			if stroka == "" || strings.HasPrefix(stroka, "#") {
				continue
			}
			if _, err := ssylki.Razobrat(stroka); err != nil {
				continue
			}
			proveneno++
			t.Run(filepath.Base(put)+"/"+strconv.Itoa(n+1), func(t *testing.T) {
				tudaObratno(t, stroka)
			})
		}
	}
	if proveneno < 30 {
		t.Fatalf("проверено ссылок %d, корпус подозрительно мал", proveneno)
	}
	t.Logf("туда-обратно прошли %d ссылок", proveneno)
}

// Случаи, которых в корпусе нет, а у живых серверов бывают.
func TestSborkaKraynieSluchai(t *testing.T) {
	pin := "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8="
	pbk := "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8"
	for _, ss := range []string{
		// хоппинг, полоса, obfs, пин и русское имя с пробелом
		"hy2://p@1.2.3.4:11444?sni=a.example&obfs=salamander&obfs-password=o&mport=20000-20050&upmbps=50&downmbps=200&pinPubKeySHA256=" + pin + "#%D0%B4%D0%BE%D0%BC%20hy2",
		// двоеточие внутри пароля anytls это один секрет, не пара
		"anytls://a%3Ab@1.2.3.4:995?sni=a.example#anytls",
		"tuic://11111111-2222-3333-4444-555555555555:p@1.2.3.4:443?sni=a.example&alpn=h3&congestion_control=bbr&udp_relay_mode=native#tuic",
		"vless://11111111-2222-3333-4444-555555555555@1.2.3.4:110?type=grpc&serviceName=abc&security=reality&pbk=" + pbk + "&sid=0102&sni=a.example&fp=chrome#reality-grpc",
		"vless://11111111-2222-3333-4444-555555555555@[2001:db8::1]:443?type=tcp&security=reality&pbk=" + pbk + "&flow=xtls-rprx-vision&sni=a.example#v6",
		"vless://11111111-2222-3333-4444-555555555555@1.2.3.4:8443?type=ws&security=tls&path=%2Fws&host=h.example&sni=a.example&pinPubKeySHA256=" + pin + "#ws",
		"trojan://p@1.2.3.4:993?security=tls&type=ws&path=%2Ft&sni=a.example&alpn=h2#trojan-ws",
		"ss://" + "Y2hhY2hhMjAtaWV0Zi1wb2x5MTMwNTphOmI" + "@1.2.3.4:8388#ss",
		// имя с плюсом: фрагмент снимается PathUnescape, плюс обязан остаться плюсом
		"hy2://p@1.2.3.4:443?sni=a.example#NL+2",
	} {
		t.Run(ss[:strings.Index(ss, ":")], func(t *testing.T) { tudaObratno(t, ss) })
	}
}

// insecure не уезжает дальше: здесь он выключен сознательно.
func TestSborkaNeNesyotInsecure(t *testing.T) {
	srv, err := ssylki.Razobrat("hy2://p@1.2.3.4:443?sni=a.example&insecure=1#x")
	if err != nil {
		t.Fatal(err)
	}
	s, err := ssylki.Sobrat(srv)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(s), "insecure") {
		t.Fatalf("в собранной ссылке insecure: %s", s)
	}
}

func TestSborkaOtkazy(t *testing.T) {
	if _, err := ssylki.Sobrat(protokol.Server{Transport: "hy2", Parol: "p"}); !errors.Is(err, ssylki.ErrSsylkaKrivaya) {
		t.Errorf("запись без адреса: ждали ErrSsylkaKrivaya, получили %v", err)
	}
	if _, err := ssylki.Sobrat(protokol.Server{Transport: "xhttp", Host: "1.2.3.4", Port: 443}); !errors.Is(err, ssylki.ErrTransportNePodderzhan) {
		t.Errorf("неизвестный транспорт: ждали ErrTransportNePodderzhan, получили %v", err)
	}
}
