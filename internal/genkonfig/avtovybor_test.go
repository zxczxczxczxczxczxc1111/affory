package genkonfig

import "testing"

// Область автовыбора (A5). Убранный человеком сервер уходит из группы urltest
// и ОСТАЁТСЯ в селекторе: «не участвует в автомате» и «недоступен» это разные
// вещи, и выбрать такой сервер руками человек по-прежнему вправе.

func TestUbrannyyIzAvtoNeMeryaetsyaNoVybiraetsyaRukami(t *testing.T) {
	v := profili()["несколько кандидатов"]
	ubran := v.Servery[1].Id
	v.VneAvto = []string{ubran}
	k := sobrat(t, v)

	vAvto := strok(poTegu(t, k, TegAvto)["outbounds"])
	vSelektore := strok(poTegu(t, k, TegSelector)["outbounds"])
	teg := TegKandidata(ubran)

	if soderzhit(vAvto, teg) {
		t.Fatalf("убранный из авто %s остался в urltest: автомат всё равно уведёт на него трафик", ubran)
	}
	if len(vAvto) != len(v.Servery)-1 {
		t.Fatalf("в urltest %d кандидатов, а серверов %d при одном убранном", len(vAvto), len(v.Servery))
	}
	if !soderzhit(vSelektore, teg) {
		t.Fatalf("убранный из авто %s пропал из селектора: выбрать его руками стало нечем", ubran)
	}
	// Исходящий обязан остаться на месте: тег в селекторе, указывающий в
	// пустоту, это конфиг, который ядро отвергнет целиком.
	if poTegu(t, k, teg) == nil {
		t.Fatalf("исходящего %s больше нет", teg)
	}
}

func TestPustayaOblastAvtoVozvrashchaetVsehKandidatov(t *testing.T) {
	// Набор, в котором убраны ВСЕ. Службой такого не собрать (последнего убрать
	// она не даёт), но набор приезжает и чужим профилем, и обходом подписки,
	// после которого в области никого не осталось. Пустой urltest ядро
	// отвергает вместе со всем конфигом, то есть человек остался бы без
	// туннеля из-за настройки автомата.
	v := profili()["несколько кандидатов"]
	for _, s := range v.Servery {
		v.VneAvto = append(v.VneAvto, s.Id)
	}
	k := sobrat(t, v)
	vAvto := strok(poTegu(t, k, TegAvto)["outbounds"])
	if len(vAvto) != len(v.Servery) {
		t.Fatalf("в urltest %d кандидатов при пустой области, ожидались все %d", len(vAvto), len(v.Servery))
	}
}

func TestNeznakomyyIdentifikatorVIsklyucheniyahNichegoNeMenyaet(t *testing.T) {
	// След прежнего состава подписки. Служба чистит такие на чтении набора, но
	// генератор обязан пережить и неубранный: иначе один устаревший
	// идентификатор менял бы состав автомата молча.
	v := profili()["несколько кандидатов"]
	v.VneAvto = []string{"srv-kotorogo-net"}
	k := sobrat(t, v)
	if got := len(strok(poTegu(t, k, TegAvto)["outbounds"])); got != len(v.Servery) {
		t.Fatalf("в urltest %d кандидатов, ожидались все %d", got, len(v.Servery))
	}
}
