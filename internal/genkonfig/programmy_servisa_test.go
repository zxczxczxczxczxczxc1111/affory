package genkonfig

import (
	"reflect"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Программы в карточке сервиса (D2, 22.09.2026).
//
// Карточка сервиса несёт и домены, и клиента. Правило по клиенту обязано быть
// таким же, как ручное правило приложения на тот же exe: иначе «Discord» в
// сервисах молча слабее «Discord» в приложениях, а человеку это один Discord.

const putSteam = `C:\Program Files (x86)\Steam\steam.exe`

// pravilaMarshruta берёт правила до сериализации: через готовый конфиг типы
// уезжают в []any, и сравнение путей читалось бы хуже, чем проверяет.
func pravilaMarshruta(t *testing.T, v Vhod) []map[string]any {
	t.Helper()
	syrye := trafikPravila(v, false)
	itog := make([]map[string]any, 0, len(syrye))
	for _, r := range syrye {
		itog = append(itog, r.(map[string]any))
	}
	return itog
}

func TestProgrammaServisaDayotPraviloPoPutiISDerevom(t *testing.T) {
	v := obraztsovyyVhod()
	v.Trafik = &protokol.PravilaTrafika{PoUmolchaniyu: protokol.TrafikPryamo,
		Servisy: []protokol.PraviloServisa{{Id: "steam", Marshrut: protokol.TrafikVPN, Programmy: []string{putSteam}}}}

	var put, derevo map[string]any
	for _, r := range pravilaMarshruta(t, v) {
		if pp, ok := r["process_path"].([]string); ok && pp[0] == putSteam {
			put = r
		}
		if pp, ok := r["process_path_tree"].([]string); ok && pp[0] == putSteam {
			derevo = r
		}
	}
	if put == nil {
		t.Fatal("правила по пути программы сервиса нет: карточка Steam не накрывает сам Steam")
	}
	if put["outbound"] != TegSelector {
		t.Errorf("маршрут программы %v, ожидался туннель", put["outbound"])
	}
	// Дерево не галочка на карточке, а её смысл: Steam это лаунчер и игры.
	if derevo == nil {
		t.Fatal("игры, запущенные лаунчером, мимо правила: дерева процессов нет")
	}
	if derevo["outbound"] != TegSelector {
		t.Errorf("маршрут дерева %v, ожидался туннель", derevo["outbound"])
	}
}

func TestKorniDerevaVidyatIRuchnyeIServisnyeProgrammy(t *testing.T) {
	// Корни обрезают чужое дерево. Программа сервиса, не попавшая в корни,
	// утащила бы к себе процессы соседнего правила.
	ruchnoy := `C:\Games\Launcher\launcher.exe`
	v := obraztsovyyVhod()
	v.Trafik = &protokol.PravilaTrafika{PoUmolchaniyu: protokol.TrafikPryamo,
		Prilozheniya: []protokol.PraviloPrilozheniya{{Put: ruchnoy, Potomki: true, Marshrut: protokol.TrafikVPN}},
		Servisy:      []protokol.PraviloServisa{{Id: "steam", Marshrut: protokol.TrafikVPN, Programmy: []string{putSteam}}}}

	nashli := false
	for _, r := range pravilaMarshruta(t, v) {
		korni, ok := r["process_path_tree_roots"].([]string)
		if !ok {
			continue
		}
		nashli = true
		if !reflect.DeepEqual(korni, []string{ruchnoy, putSteam}) {
			t.Fatalf("корни дерева %v", korni)
		}
	}
	if !nashli {
		t.Fatal("правил с деревом нет вовсе")
	}
}

func TestServisBezDomenovNeDayotPustogoPravila(t *testing.T) {
	// У Steam доменного набора нет. Пустой domain_suffix ядро проглотит, но
	// такое правило не совпадёт ни с чем и читается как забытая строка.
	v := obraztsovyyVhod()
	v.Trafik = &protokol.PravilaTrafika{PoUmolchaniyu: protokol.TrafikPryamo,
		Servisy: []protokol.PraviloServisa{{Id: "steam", Marshrut: protokol.TrafikVPN, Programmy: []string{putSteam}}}}

	for _, r := range pravilaMarshruta(t, v) {
		if d, ok := r["domain_suffix"].([]string); ok && len(d) == 0 {
			t.Fatal("пустое доменное правило: сервис без доменов оставил строку ни о чём")
		}
	}
}

func TestProgrammaServisaCherezVPNTyanetDNSVTunnel(t *testing.T) {
	// Общий системный DNS не знает, кто спросил. Имя, разрешённое на месте,
	// побеждает правило по программе, поэтому при программе через VPN имена
	// разрешаются в туннеле - то же самое уже делает ручное правило.
	v := obraztsovyyVhod()
	v.Trafik = &protokol.PravilaTrafika{PoUmolchaniyu: protokol.TrafikPryamo,
		Servisy: []protokol.PraviloServisa{{Id: "steam", Marshrut: protokol.TrafikVPN, Programmy: []string{putSteam}}}}
	if got := trafikDNSFinal(v); got != TegTunnel {
		t.Fatalf("DNS по умолчанию %q: имена программы сервиса остались на местном разрешении", got)
	}
}
