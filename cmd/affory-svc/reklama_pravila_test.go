package main

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

func pravilaReklamy(t *testing.T, s *Sluzhba) *ReklamaPravila {
	t.Helper()
	n, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}
	return n.Pravila.Reklama
}

func setRulesTelo(t *testing.T, s *Sluzhba, telo string) protokol.Kadr {
	t.Helper()
	return s.Obrabotat(ctxAdmina(), protokol.Kadr{Id: 1, Imya: "setRules", Telo: []byte(telo)})
}

// Окно прошлой версии и CLI поля reklama не знают: их правка не имеет права
// стереть настройку, заведённую новым окном.
func TestSetRulesBezReklamyNeTrogaetSohranyonnoe(t *testing.T) {
	s := podstavnaya(t, nil)
	if r := setRulesTelo(t, s, `{"reklama":{"vkl":true,"uroven":"multi","razresheno":["mc.yandex.ru"]}}`); r.Oshib != nil {
		t.Fatal(r.Oshib)
	}
	for _, telo := range []string{`{"trafik":{"po_umolchaniyu":"vpn"}}`, `{"protsessy":[],"domeny":["example.org"]}`} {
		if r := setRulesTelo(t, s, telo); r.Oshib != nil {
			t.Fatalf("%s: %+v", telo, r.Oshib)
		}
		r := pravilaReklamy(t, s)
		if r == nil || !r.Vkl || r.Uroven != "multi" || !slices.Equal(r.Razresheno, []string{"mc.yandex.ru"}) {
			t.Fatalf("%s стёр настройку рекламы: %+v", telo, r)
		}
	}
}

func TestSetRulesReklamaNormalizuetsya(t *testing.T) {
	s := podstavnaya(t, nil)
	r := setRulesTelo(t, s, `{"reklama":{"vkl":true,"uroven":"light","razresheno":[" Mc.Yandex.RU. ","mc.yandex.ru"]}}`)
	if r.Oshib != nil {
		t.Fatal(r.Oshib)
	}
	if got := pravilaReklamy(t, s); got == nil || !slices.Equal(got.Razresheno, []string{"mc.yandex.ru"}) {
		t.Fatalf("исключения не нормализованы: %+v", got)
	}
	var otvet struct {
		Izmenyon bool `json:"spisok_izmenyon"`
	}
	if err := json.Unmarshal(r.Telo, &otvet); err != nil || !otvet.Izmenyon {
		t.Fatalf("окну не сказано перерисовать исключения: %s", r.Telo)
	}
}

func TestSetRulesReklamaOtkazy(t *testing.T) {
	mnogo := make([]string, 257)
	for i := range mnogo {
		mnogo[i] = fmt.Sprintf("%q", fmt.Sprintf("s%d.example", i))
	}
	for imya, telo := range map[string]string{
		"незнакомый уровень": `{"reklama":{"vkl":true,"uroven":"pro"}}`,
		"257 исключений":     `{"reklama":{"vkl":true,"razresheno":[` + strings.Join(mnogo, ",") + `]}}`,
		"адрес вместо имени": `{"reklama":{"vkl":true,"razresheno":["1.2.3.4"]}}`,
		"адрес страницы":     `{"reklama":{"vkl":true,"razresheno":["https://example.org/x"]}}`,
	} {
		s := podstavnaya(t, nil)
		r := setRulesTelo(t, s, telo)
		if r.Oshib == nil || r.Oshib.Kod != protokol.KodPraviloNegodno {
			t.Errorf("%s: ждали rule-invalid, получили %+v", imya, r.Oshib)
		}
		if got := pravilaReklamy(t, s); got != nil {
			t.Errorf("%s: набор тронут: %+v", imya, got)
		}
	}
}

// Нулевая настройка сворачивается в nil: у тех, кто вкладку открыл и ничего не
// включил, JSON набора, отпечаток и ревизия прежние.
func TestNulevayaReklamaSvorachivaetsya(t *testing.T) {
	s := podstavnaya(t, nil)
	// Фикстура держит пустые списки как null, а любая правка кладёт []:
	// ревизию меряем от уже нормализованного набора.
	if r := setRulesTelo(t, s, `{"protsessy":[],"domeny":[]}`); r.Oshib != nil {
		t.Fatal(r.Oshib)
	}
	n, _ := s.nabor()
	bylo := reviziyaPravil(n.Pravila)
	r := setRulesTelo(t, s, `{"reklama":{"vkl":false,"uroven":"light","razresheno":[]}}`)
	if r.Oshib != nil {
		t.Fatal(r.Oshib)
	}
	n, _ = s.nabor()
	if n.Pravila.Reklama != nil {
		t.Fatalf("нулевая настройка сохранилась объектом: %+v", n.Pravila.Reklama)
	}
	if reviziyaPravil(n.Pravila) != bylo {
		t.Fatal("нулевая настройка сменила ревизию")
	}
	var otvet struct {
		Podyom bool `json:"trebuet_podyoma"`
	}
	if err := json.Unmarshal(r.Telo, &otvet); err != nil || otvet.Podyom {
		t.Fatalf("нулевая настройка требует подъёма: %s", r.Telo)
	}
}

func pokolenie(s *Sluzhba) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pokolenieP
}

func zapomnitPrimenyonnoe(t *testing.T, s *Sluzhba) {
	t.Helper()
	n, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}
	// Фикстура не запускает sobratTun, который запоминает применённые правила.
	s.mu.Lock()
	s.pravilaKonfiga = otpechatokPravil(n.Pravila)
	s.mu.Unlock()
}

// Включение меняет конфиг и переподнимает ядро, с trafik в теле и без него.
// Смена одного уровня конфиг не меняет: служба просто заменит файл.
func TestReklamaPerepodnimaetTolkoPriSmeneKonfiga(t *testing.T) {
	for imya, vkl := range map[string]string{
		"без trafik": `{"reklama":{"vkl":true}}`,
		"с trafik":   `{"trafik":{"po_umolchaniyu":"vpn"},"reklama":{"vkl":true}}`,
		"исключение": `{"reklama":{"vkl":true,"razresheno":["mc.yandex.ru"]}}`,
		"выключение": `{"reklama":{"vkl":false,"uroven":"multi"}}`,
	} {
		s := podstavnaya(t, nil)
		// Тот же trafik заранее: подъём обязан вызвать именно реклама.
		if r := setRulesTelo(t, s, `{"trafik":{"po_umolchaniyu":"vpn"}}`); r.Oshib != nil {
			t.Fatal(r.Oshib)
		}
		if imya == "выключение" {
			if r := setRulesTelo(t, s, `{"reklama":{"vkl":true,"uroven":"multi"}}`); r.Oshib != nil {
				t.Fatal(r.Oshib)
			}
		}
		if err := s.Connect(context.Background()); err != nil {
			t.Fatal(err)
		}
		zapomnitPrimenyonnoe(t, s)
		bylo := pokolenie(s)
		if r := setRulesTelo(t, s, vkl); r.Oshib != nil {
			t.Fatalf("%s: %+v", imya, r.Oshib)
		}
		if pokolenie(s) <= bylo {
			t.Errorf("%s: конфиг изменился, а ядро не переподнято", imya)
		}
	}

	s := podstavnaya(t, nil)
	if r := setRulesTelo(t, s, `{"reklama":{"vkl":true}}`); r.Oshib != nil {
		t.Fatal(r.Oshib)
	}
	if err := s.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	zapomnitPrimenyonnoe(t, s)
	bylo := pokolenie(s)
	r := setRulesTelo(t, s, `{"reklama":{"vkl":true,"uroven":"multi"}}`)
	if r.Oshib != nil {
		t.Fatal(r.Oshib)
	}
	if pokolenie(s) != bylo {
		t.Fatal("смена уровня переподняла ядро")
	}
	if got := pravilaReklamy(t, s); got == nil || got.Uroven != "multi" {
		t.Fatalf("уровень не сохранён: %+v", got)
	}
}

// Отпечаток конфига без рекламы байт-в-байт прежний, а ревизия видит уровень:
// иначе смена уровня из двух окон не дала бы конфликта черновиков.
func TestOtpechatokIReviziyaReklamy(t *testing.T) {
	p := PravilaNabora{Protsessy: []string{}, Domeny: []string{"example.org"}}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if otpechatokPravil(p) != string(b) {
		t.Fatal("отпечаток без рекламы изменился")
	}
	light := p
	light.Reklama = &ReklamaPravila{Vkl: true, Uroven: "light"}
	multi := p
	multi.Reklama = &ReklamaPravila{Vkl: true, Uroven: "multi"}
	if otpechatokPravil(light) != otpechatokPravil(multi) {
		t.Fatal("уровень попал в отпечаток конфига")
	}
	if reviziyaPravil(light) == reviziyaPravil(multi) {
		t.Fatal("ревизия не видит смены уровня")
	}
	vykl := p
	vykl.Reklama = &ReklamaPravila{Uroven: "multi", Razresheno: []string{"mc.yandex.ru"}}
	if otpechatokPravil(vykl) != otpechatokPravil(p) {
		t.Fatal("выключенная блокировка попала в отпечаток конфига")
	}
}

// Вкладка в окне показывается по полю reklama: оно всегда объект, исключения
// всегда список.
func TestListRulesOtdayotReklamu(t *testing.T) {
	s := podstavnaya(t, nil)
	r := s.Obrabotat(ctxAdmina(), protokol.Kadr{Id: 1, Imya: "listRules"})
	if r.Oshib != nil {
		t.Fatal(r.Oshib)
	}
	var otvet map[string]json.RawMessage
	if err := json.Unmarshal(r.Telo, &otvet); err != nil {
		t.Fatal(err)
	}
	if got := string(otvet["reklama"]); got != `{"razresheno":[],"uroven":"light","vkl":false}` {
		t.Fatalf("reklama в listRules: %s", got)
	}
}

// Блоб приходит и импортом чужого профиля: негодное приводится на чтении, а
// не роняет набор целиком.
func TestReklamaPrivoditsyaNaChtenii(t *testing.T) {
	s := podstavnaya(t, nil)
	s.sekretyChitat = func() ([]byte, error) {
		return []byte(`{"servery":[],"pravila":{"protsessy":[],"domeny":[],` +
			`"reklama":{"vkl":true,"uroven":"zzz","razresheno":["http://x y","1.2.3.4","Mc.Yandex.ru"]}}}`), nil
	}
	n, err := s.naborIzHranilishcha()
	if err != nil {
		t.Fatal(err)
	}
	r := n.Pravila.Reklama
	if r == nil || !r.Vkl || r.Uroven != "light" || !slices.Equal(r.Razresheno, []string{"mc.yandex.ru"}) {
		t.Fatalf("приведение на чтении: %+v", r)
	}
}
