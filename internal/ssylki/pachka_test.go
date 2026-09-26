package ssylki_test

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/ssylki"
)

// Шесть форм наших ключей, как их отдаёт свой сервер.
var nashiShest = []string{
	"nash-anytls.txt", "nash-trojan.txt", "hy2.txt", "nash-tuic.txt", "vless-reality-grpc.txt", "ss-sip002.txt",
}

func shestStrok(t *testing.T) []string {
	t.Helper()
	var itog []string
	for _, f := range nashiShest {
		itog = append(itog, vzyat(t, f))
	}
	return itog
}

func TestPachkaShestKlyuchey(t *testing.T) {
	stroki := shestStrok(t)
	// CRLF, отступы по краям и пустые строки: так приходит текст из блокнота.
	tekst := "\r\n  " + strings.Join(stroki, "  \r\n\r\n\t") + "\r\n"
	r, err := ssylki.RazobratPachku(tekst)
	if err != nil {
		t.Fatalf("пачка отвергнута: %v", err)
	}
	if len(r.Servery) != len(stroki) || len(r.Otkazy) != 0 {
		t.Fatalf("серверов %d, отказов %d (%v), ждали %d и 0", len(r.Servery), len(r.Otkazy), r.Otkazy, len(stroki))
	}
	for _, s := range r.Servery {
		if s.IzPodpiski {
			t.Errorf("%s помечен как из подписки: слияние подписки его потом сотрёт", s.Imya)
		}
	}
}

// Один base64 на весь текст, с переносами каждые 76 знаков, как у почтовых
// библиотек. Так выгружают v2rayN и наша же выгрузка.
func TestPachkaBase64(t *testing.T) {
	syroe := base64.StdEncoding.EncodeToString([]byte(strings.Join(shestStrok(t), "\n")))
	var b strings.Builder
	for len(syroe) > 76 {
		b.WriteString(syroe[:76] + "\n")
		syroe = syroe[76:]
	}
	b.WriteString(syroe)
	r, err := ssylki.RazobratPachku(b.String())
	if err != nil || len(r.Servery) != len(nashiShest) {
		t.Fatalf("base64: серверов %d, ошибка %v", len(r.Servery), err)
	}
}

// Смешанный текст: годные, битая, адрес подписки. Номер строки у отказа
// настоящий, от начала текста, с пустыми строками: человек ищет её глазами.
func TestPachkaSmeshannaya(t *testing.T) {
	stroki := shestStrok(t)
	tekst := strings.Join([]string{
		stroki[0],
		"",
		"https://panel.example/sub/abc",
		"vless://11111111-2222-3333-4444-555555555555@1.2.3.4:443?type=tcp&security=reality#bez-pbk",
		stroki[1],
		stroki[1], // повтор внутри вставки не удваивает список
	}, "\n")
	r, err := ssylki.RazobratPachku(tekst)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Servery) != 2 {
		t.Errorf("серверов %d, ждали 2", len(r.Servery))
	}
	if len(r.Otkazy) != 1 || r.Otkazy[0].Stroka != 4 {
		t.Fatalf("отказы %+v, ждали один на строке 4", r.Otkazy)
	}
	if strings.Contains(r.Otkazy[0].Prichina, "11111111") {
		t.Errorf("в причине отказа uuid: %q", r.Otkazy[0].Prichina)
	}
}

func TestPachkaOtkazyCelikom(t *testing.T) {
	for imya, sluchay := range map[string]struct {
		tekst string
		zhdem error
	}{
		"json":        {`{"outbounds":[{"type":"vless"}]}`, ssylki.ErrNeSsylki},
		"clash":       {"port: 7890\nproxies:\n  - name: a\n", ssylki.ErrNeSsylki},
		"wireguard":   {"[Interface]\nPrivateKey = x\n", ssylki.ErrNeSsylki},
		"pusto":       {"  \n\n", ssylki.ErrPachkaPusta},
		"tolko-adres": {"https://panel.example/sub/abc\n", ssylki.ErrPachkaPusta},
		"velika":      {strings.Repeat("a", 1<<20+1), ssylki.ErrPachkaVelika},
	} {
		t.Run(imya, func(t *testing.T) {
			if _, err := ssylki.RazobratPachku(sluchay.tekst); !errors.Is(err, sluchay.zhdem) {
				t.Fatalf("ждали %v, получили %v", sluchay.zhdem, err)
			}
		})
	}
}

// Сообщение панели во вставке это отказ строки с текстом, а не истекшая подписка.
func TestPachkaUvedomlenieVOtkazah(t *testing.T) {
	r, err := ssylki.RazobratPachku(vzyat(t, "uvedomlenie-0000-443.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Otkazy) != 1 || !strings.HasPrefix(r.Otkazy[0].Prichina, "вместо сервера сообщение") || len(r.Uvedomleniya) != 0 {
		t.Fatalf("отказы %+v, уведомления %+v", r.Otkazy, r.Uvedomleniya)
	}
}

func TestPachkaSto(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 100; i++ {
		fmt.Fprintf(&b, "hy2://p%d@1.2.3.4:%d?sni=a.example#k%d\n", i, 10000+i, i)
	}
	r, err := ssylki.RazobratPachku(b.String())
	if err != nil || len(r.Servery) != 100 {
		t.Fatalf("серверов %d, ошибка %v", len(r.Servery), err)
	}
}

func TestSobratSpisokNeTeryaetMolcha(t *testing.T) {
	servery := []protokol.Server{
		{Transport: "hy2", Host: "1.2.3.4", Port: 443, Parol: "p", Imya: "hy2"},
		{Transport: "xhttp", Host: "1.2.3.4", Port: 443, Imya: "chuzhoy"},
	}
	ssylkiSpisok, propushcheny := ssylki.SobratSpisok(servery)
	if len(ssylkiSpisok) != 1 || len(propushcheny) != 1 || propushcheny[0].Imya != "chuzhoy" {
		t.Fatalf("ссылок %d, пропущено %+v", len(ssylkiSpisok), propushcheny)
	}
}
