package genkonfig

import (
	"fmt"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

func vseIshodyashchie(t *testing.T, k map[string]any) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, o := range spisok(k["outbounds"]) {
		m, ok := o.(map[string]any)
		if !ok {
			t.Fatalf("исходящий не объект: %v", o)
		}
		out = append(out, m)
	}
	return out
}

func poTegu(t *testing.T, k map[string]any, teg string) map[string]any {
	t.Helper()
	for _, o := range vseIshodyashchie(t, k) {
		if o["tag"] == teg {
			return o
		}
	}
	t.Fatalf("исходящего с тегом %q нет", teg)
	return nil
}

func mnogoKandidatov(t *testing.T) map[string]any {
	t.Helper()
	return sobrat(t, profili()["несколько кандидатов"])
}

func TestTegiKandidatovUnikalny(t *testing.T) {
	// Two candidates with the same tag collapse into one in the selector, and
	// sveritTegi cannot see it: a map swallows the duplicate without a word.
	k := mnogoKandidatov(t)
	tegi := map[string]int{}
	for _, o := range vseIshodyashchie(t, k) {
		teg, _ := o["tag"].(string)
		tegi[teg]++
	}
	for teg, n := range tegi {
		if n > 1 {
			t.Fatalf("тег %q встречается %d раз: кандидаты схлопнутся", teg, n)
		}
	}
	if k["route"].(map[string]any)["final"] != TegSelector {
		t.Fatal("route.final смотрит мимо селектора")
	}
}

func TestVseKandidatyVidnySelektoru(t *testing.T) {
	// Зеркало теста выше. Уникальные теги можно получить и потеряв кандидата по
	// дороге: три исходящих, а в селекторе два, и человек просто не увидит
	// сервер, за который платит.
	v := profili()["несколько кандидатов"]
	k := sobrat(t, v)
	sel := poTegu(t, k, TegSelector)
	avto := poTegu(t, k, TegAvto)

	vSelektore := strok(sel["outbounds"])
	vAvto := strok(avto["outbounds"])
	if len(vAvto) != len(v.Servery) {
		t.Fatalf("в urltest %d кандидатов, а серверов %d", len(vAvto), len(v.Servery))
	}
	// В селекторе на один больше: там ещё и сама группа «авто».
	if len(vSelektore) != len(v.Servery)+1 {
		t.Fatalf("в селекторе %v, ожидались все кандидаты плюс авто", vSelektore)
	}
	for _, s := range v.Servery {
		teg := TegKandidata(s.Id)
		if !soderzhit(vAvto, teg) {
			t.Fatalf("кандидата %s нет в urltest: его задержку никто не мерит", s.Id)
		}
		if !soderzhit(vSelektore, teg) {
			t.Fatalf("кандидата %s нет в селекторе: руками его не выбрать", s.Id)
		}
	}
}

func soderzhit(s []string, chto string) bool {
	for _, v := range s {
		if v == chto {
			return true
		}
	}
	return false
}

func TestChislaUrltestSverenySoSpekoy(t *testing.T) {
	// Числа выписаны, а не «взяты из спеки». Эталонная фикстура закрепила бы
	// любое значение, каким бы оно ни было, и отличить сделанное по спеке от
	// поставленного по вкусу было бы нечем. Расхождение ловится грепом по этим
	// именам.
	k := mnogoKandidatov(t)
	avto := poTegu(t, k, TegAvto)
	sverit := map[string]any{
		"url":          "https://www.gstatic.com/generate_204",
		"interval":     "3m",
		"tolerance":    float64(50),
		"idle_timeout": "30m",
	}
	for pole, hotim := range sverit {
		if avto[pole] != hotim {
			t.Fatalf("urltest.%s = %v, а по спеке §6 должно быть %v", pole, avto[pole], hotim)
		}
	}
}

func TestSushchestvuyushchieSoedineniyaNeRvutsya(t *testing.T) {
	// Порог «TCP не рвутся дольше секунды» снят из спеки именно потому, что
	// поведение задаётся ЭТИМ полем, а не удачей. Умолчание у sing-box обратное,
	// так что пропущенное поле означало бы разрыв на каждом переключении.
	k := mnogoKandidatov(t)
	for _, teg := range []string{TegAvto, TegSelector} {
		o := poTegu(t, k, teg)
		v, est := o["interrupt_exist_connections"]
		if !est {
			t.Fatalf("%s: поля interrupt_exist_connections нет вовсе", teg)
		}
		if v != false {
			t.Fatalf("%s: interrupt_exist_connections = %v", teg, v)
		}
	}
}

func TestVybrannyyServerStanovitsyaUmolchaniem(t *testing.T) {
	// Иначе после перезапуска человек оказывается на «авто» вместо того сервера,
	// который выбрал руками, и решит, что настройка не сохранилась.
	v := profili()["несколько кандидатов"]
	k := sobrat(t, v)
	if d := poTegu(t, k, TegSelector)["default"]; d != TegKandidata(v.Server.Id) {
		t.Fatalf("умолчание селектора %v, а выбран %s", d, v.Server.Id)
	}
}

// ИНВАРИАНТ ПЕРЕПИСАН ЗАДАЧЕЙ П5, и проверять стало нечего в прежнем смысле.
//
// Прежде оба кандидата шли через локальный SOCKS, и один порт на двоих
// означал бы, что они ведут в одно место: urltest замерил бы один путь дважды,
// объявил кандидатов одинаково быстрыми, а переключение не меняло бы ничего.
// Отсюда была раздача портов по кандидату. Со своим ядром цепочки нет: каждый
// кандидат это прямой исходящий на СВОЙ адрес, и схлопнуться они не могут по
// построению. Тест это и закрепляет.
func TestDvaKandidataVedutNaRaznyeServery(t *testing.T) {
	v := profili()["два кандидата"]
	k := sobrat(t, v)
	adresa := map[string]bool{}
	for _, s := range v.Servery {
		o := poTegu(t, k, TegKandidata(s.Id))
		if o["type"] != "vless" {
			t.Fatalf("кандидат %s не прямой vless: %v", s.Id, o["type"])
		}
		if o["server"] != s.Host {
			t.Fatalf("кандидат %s ведёт на %v, а не на свой адрес %s", s.Id, o["server"], s.Host)
		}
		kl := fmt.Sprintf("%v:%v", o["server"], o["server_port"])
		if adresa[kl] {
			t.Fatalf("адрес %s выдан двум кандидатам: они ведут в одно место", kl)
		}
		adresa[kl] = true
	}
}

func TestDnsIdyotCherezSelektorANeCherezKandidata(t *testing.T) {
	// detour на конкретного кандидата означал бы, что при переключении сервера
	// DNS продолжает ходить через ПРЕЖНИЙ, причём молча: имена резолвятся,
	// трафик идёт, и расхождение видно только в чужих журналах.
	k := mnogoKandidatov(t)
	for _, srv := range spisok(k["dns"].(map[string]any)["servers"]) {
		m := srv.(map[string]any)
		if m["tag"] != TegTunnel {
			continue
		}
		if m["detour"] != TegSelector {
			t.Fatalf("detour туннельного DNS = %v, а не селектор", m["detour"])
		}
	}
}

func TestKandidatBezAdresaVPetleOtvergaetsya(t *testing.T) {
	// urltest пробит ВСЕХ кандидатов, включая тех, куда селектор сейчас не
	// смотрит, и проба уходит до того, как выбран выход. Адрес, не попавший в
	// ip_cidr, это петля на старте, и проявляется она не сразу.
	v := profili()["несколько кандидатов"]
	v.Kandidaty = v.Kandidaty[:1]
	if _, err := SingBox(v); err == nil {
		t.Fatal("кандидат без адреса в правиле петли принят: петля проявится позже")
	}
}

func TestDvaKandidataSOdnimIdOtvergayutsya(t *testing.T) {
	// Одинаковые идентификаторы дают одинаковые теги, а дубликат тега наша
	// sveritTegi проглатывает молча: она складывает объявленные в map.
	v := profili()["несколько кандидатов"]
	v.Servery = append(v.Servery[:1], v.Servery[0])
	if _, err := SingBox(v); err == nil {
		t.Fatal("два кандидата с одним идентификатором приняты")
	}
}

func TestVAvtoUmolchanieSelektoraEtoAvto(t *testing.T) {
	// В ручном режиме умолчание это выбранный сервер, и на это есть отдельный
	// тест. Здесь противоположный случай, и без него «авто» остаётся чучелом:
	// группа объявлена, а встать по умолчанию не может никогда.
	v := profili()["несколько кандидатов"]
	v.Rezhim = protokol.RezhimAvto
	k := sobrat(t, v)
	if d := poTegu(t, k, TegSelector)["default"]; d != TegAvto {
		t.Fatalf("умолчание селектора %v, а в режиме авто обязано быть %q", d, TegAvto)
	}
}
