package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/ssylki"
)

const (
	pachkaHy2    = "hy2://parol-a@203.0.113.20:11444?sni=a.example&obfs=salamander&obfs-password=o#hy2"
	pachkaAnytls = "anytls://parol-b@203.0.113.20:995?sni=a.example#anytls"
)

type itogPachki struct {
	Dobavleno int                  `json:"dobavleno"`
	Obnovleno int                  `json:"obnovleno"`
	UzheBylo  int                  `json:"uzhe_bylo"`
	Otkazy    []ssylki.OtkazStroki `json:"otkazy"`
}

func vstavit(t *testing.T, s *Sluzhba, tekst string) itogPachki {
	t.Helper()
	o := vypolnit(t, s, "addServers", map[string]string{"tekst": tekst})
	if o.Oshib != nil {
		t.Fatalf("пачка отвергнута: %+v", o.Oshib)
	}
	var itog itogPachki
	if err := json.Unmarshal(o.Telo, &itog); err != nil {
		t.Fatal(err)
	}
	return itog
}

// Три ссылки и битая строка: три сервера одной записью набора, отказ с
// номером строки. Повтор той же вставки ничего не удваивает, а повтор с
// новыми именами переименовывает записи на месте.
func TestPachkaDobavlyaetOdnoyZapisyu(t *testing.T) {
	s := podstavnaya(t, nil)
	pisat := s.sekretyPisat
	zapisey := 0
	s.sekretyPisat = func(b []byte) error { zapisey++; return pisat(b) }
	do := len(spisokServerov(t, s))

	tekst := strings.Join([]string{ssylkaProby, pachkaHy2, "vless://bitaya", pachkaAnytls}, "\n")
	itog := vstavit(t, s, tekst)
	if itog.Dobavleno != 3 || itog.Obnovleno != 0 || itog.UzheBylo != 0 {
		t.Fatalf("итог %+v, ждали три добавленных", itog)
	}
	if len(itog.Otkazy) != 1 || itog.Otkazy[0].Stroka != 3 {
		t.Fatalf("отказы %+v, ждали один на строке 3", itog.Otkazy)
	}
	if zapisey != 1 {
		t.Errorf("набор записан %d раз, ждали один: каждая запись пересобирает правила", zapisey)
	}
	if posle := len(spisokServerov(t, s)); posle != do+3 {
		t.Fatalf("в списке %d, ждали %d", posle, do+3)
	}

	if itog := vstavit(t, s, tekst); itog.Dobavleno != 0 || itog.UzheBylo != 3 {
		t.Fatalf("повтор вставки: %+v, ждали три «уже были»", itog)
	}
	pereimenovano := strings.NewReplacer("#hy2", "#hy2-hop", "#anytls", "#anytls-novyy").Replace(tekst)
	if itog := vstavit(t, s, pereimenovano); itog.Obnovleno != 2 || itog.UzheBylo != 1 {
		t.Fatalf("вставка с новыми именами: %+v, ждали два обновлённых", itog)
	}
	if posle := len(spisokServerov(t, s)); posle != do+3 {
		t.Fatalf("после переименования в списке %d, ждали %d", posle, do+3)
	}
}

func TestPachkaOtkazCelikom(t *testing.T) {
	s := podstavnaya(t, nil)
	o := vypolnit(t, s, "addServers", map[string]string{"tekst": `{"outbounds":[]}`})
	if o.Oshib == nil || o.Oshib.Kod != protokol.KodSubscriptionMalformed || o.Oshib.Tekst != ssylki.ErrNeSsylki.Error() {
		t.Fatalf("отказ %+v", o.Oshib)
	}
}

func TestVygruzkaTolkoAdminu(t *testing.T) {
	s := podstavnaya(t, nil)
	o := s.Obrabotat(neAdminom(context.Background()), protokol.Kadr{Tip: "cmd", Id: 1, Imya: "exportServers", Telo: []byte(`{}`)})
	if o.Oshib == nil || o.Oshib.Kod != protokol.KodTrebuetsyaAdmin {
		t.Fatalf("выгрузка без прав: %+v", o.Oshib)
	}
}

// Выгрузка обязана дать ровно те серверы, что лежат в наборе: каждая строка
// разбирается обратно в тот же профиль. Удержанный не выгружается.
func TestVygruzkaVozvrashchaetTeZheServery(t *testing.T) {
	s := podstavnaya(t, nil)
	vstavit(t, s, strings.Join([]string{ssylkaProby, pachkaHy2, pachkaAnytls}, "\n"))
	if err := s.pravitNabor(func(n *Nabor) error {
		n.Servery = append(n.Servery, protokol.Server{Id: "uderzhan", Transport: "hy2", Host: "203.0.113.99", Port: 1, Parol: "p", Uderzhan: true})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	n, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}

	o := vypolnit(t, s, "exportServers", map[string]any{})
	if o.Oshib != nil {
		t.Fatalf("выгрузка отвергнута: %+v", o.Oshib)
	}
	var v struct {
		Tekst  string `json:"tekst"`
		Base64 string `json:"base64"`
		Vsego  int    `json:"vsego"`
	}
	if err := json.Unmarshal(o.Telo, &v); err != nil {
		t.Fatal(err)
	}
	r, err := ssylki.RazobratPachku(v.Tekst)
	if err != nil {
		t.Fatal(err)
	}
	zhivyh := len(n.Servery) - 1
	if v.Vsego != zhivyh || len(r.Servery) != zhivyh {
		t.Fatalf("выгружено %d, разобрано %d, в наборе живых %d", v.Vsego, len(r.Servery), zhivyh)
	}
	for _, vygr := range r.Servery {
		nashli := false
		for _, srv := range n.Servery {
			if !srv.Uderzhan && ssylki.TotZheProfil(srv, vygr) {
				nashli = true
			}
		}
		if !nashli {
			t.Errorf("выгруженный %s не совпал ни с одним сервером набора", vygr.Imya)
		}
	}
	if b, err := base64.StdEncoding.DecodeString(v.Base64); err != nil || string(b) != v.Tekst {
		t.Errorf("base64 не совпал с текстом: %v", err)
	}

	// Выгрузка отобранных: только то, что попросили.
	var hy2Id string
	for _, srv := range n.Servery {
		if srv.Imya == "hy2" {
			hy2Id = srv.Id
		}
	}
	o = vypolnit(t, s, "exportServers", map[string]any{"ids": []string{hy2Id}})
	if o.Oshib != nil || json.Unmarshal(o.Telo, &v) != nil || v.Vsego != 1 || !strings.HasPrefix(v.Tekst, "hy2://") {
		t.Fatalf("выгрузка одного: %+v, %+v", o.Oshib, v)
	}
}

func TestPachkaIVygruzkaNeLogiruyutsya(t *testing.T) {
	for _, imya := range []string{"addServers", "exportServers"} {
		if protokol.TeloMozhnoLogirovat(imya) {
			t.Errorf("тело %s попадёт в журнал, а в нём ключи", imya)
		}
	}
}
