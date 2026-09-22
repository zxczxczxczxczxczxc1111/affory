package genkonfig

import (
	"fmt"
	"sort"
	"strings"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/katalog"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

func proveritMarshruty(v Vhod) error {
	if v.Trafik == nil {
		return nil
	}
	valid := func(r protokol.MarshrutTrafika) error {
		if r != protokol.TrafikVPN && r != protokol.TrafikPryamo {
			return fmt.Errorf("неизвестный маршрут: %q", r)
		}
		if v.VesTrafik && r == protokol.TrafikPryamo {
			return fmt.Errorf("для прямого трафика сначала выключи блокировку сети вне VPN")
		}
		return nil
	}
	if err := valid(v.Trafik.PoUmolchaniyu); err != nil {
		return err
	}
	for _, a := range v.Trafik.Prilozheniya {
		if err := valid(a.Marshrut); err != nil {
			return err
		}
	}
	for _, d := range v.Trafik.Domeny {
		if err := valid(d.Marshrut); err != nil {
			return err
		}
	}
	for _, s := range v.Trafik.Servisy {
		if err := valid(s.Marshrut); err != nil {
			return err
		}
		// Сервис, которого нет в каталоге, конфиг НЕ рушит. Каталог едет внутри
		// программы и меняется с выпусками, а набор на диске старше программы:
		// правило на ушедший сервис делало продукт неспособным подключиться
		// вовсе, причём без единого пути выхода из окна (жалоба 21.09.2026,
		// whatsapp). Правило пропускает и сборка ниже, и приведение набора на
		// чтении, а годность ВВОДА проверяется там, где вводят, - в setRules.
	}
	return nil
}

func tegMarshruta(r protokol.MarshrutTrafika, dns bool) string {
	if dns {
		if r == protokol.TrafikPryamo {
			return TegMestnyy
		}
		return TegTunnel
	}
	if r == protokol.TrafikPryamo {
		return TegPryamo
	}
	return TegSelector
}

func trafikFinal(v Vhod) string {
	if v.Trafik == nil {
		return TegSelector
	}
	return tegMarshruta(v.Trafik.PoUmolchaniyu, false)
}

func trafikDNSFinal(v Vhod) string {
	if v.Trafik == nil {
		return TegTunnel
	}
	// Shared Windows DNS has no reliable originating app identity. Resolve through
	// VPN when an app needs it, so a poisoned local answer cannot defeat that rule.
	//
	// Программы сервисов считаются наравне с ручными правилами (D2): иначе
	// карточка Steam через VPN оставляла бы его имена на местном разрешении.
	for _, a := range programmyTrafika(v) {
		if a.Marshrut == protokol.TrafikVPN {
			return TegTunnel
		}
	}
	return tegMarshruta(v.Trafik.PoUmolchaniyu, true)
}

func trafikPravila(v Vhod, dns bool) []any {
	p := []any{}
	key := "outbound"
	if dns {
		key = "server"
	}
	if !dns {
		// Every explicit executable wins before any inherited route, including
		// entries which also cover the programs they launch.
		//
		// Правила приложений и программы сервисов это ОДИН слой (D2,
		// 22.09.2026): карточка сервиса с клиентом обязана давать тот же охват,
		// что ручное правило на тот же exe, иначе «Discord» в сервисах молча
		// слабее «Discord» в приложениях. Ручные идут первыми: у них тот же вес,
		// а при совпадении путей человек переопределяет карточку точечно.
		programmy := programmyTrafika(v)
		var roots []string
		for _, a := range programmy {
			p = append(p, map[string]any{"process_path": []string{a.Put}, key: tegMarshruta(a.Marshrut, false)})
			if a.Potomki {
				roots = append(roots, a.Put)
			}
		}
		for _, a := range programmy {
			if a.Potomki {
				p = append(p, map[string]any{"process_path_tree": []string{a.Put}, "process_path_tree_roots": roots, key: tegMarshruta(a.Marshrut, false)})
			}
		}
	}
	// A specific child domain beats its parent, regardless of the order in the UI.
	domains := append([]protokol.PraviloDomena(nil), v.Trafik.Domeny...)
	sort.SliceStable(domains, func(i, j int) bool {
		return strings.Count(domains[i].Domen, ".") > strings.Count(domains[j].Domen, ".")
	})
	for _, d := range domains {
		p = append(p, map[string]any{"domain_suffix": []string{d.Domen}, key: tegMarshruta(d.Marshrut, dns)})
	}
	for _, s := range v.Trafik.Servisy {
		domains, err := katalog.Domeny(s.Id)
		if err != nil {
			continue
		} // Validation happens before generation; no invented domain fallback.
		// Сервис без доменов это законная запись (D2): у Steam и Epic Games
		// набора доменов нет вовсе, маршрут у них держится на программе. Пустой
		// domain_suffix ядро принимает, но такое правило не совпадает ни с чем и
		// читается в конфиге как забытая строка.
		if len(domains) == 0 {
			continue
		}
		p = append(p, map[string]any{"domain_suffix": domains, key: tegMarshruta(s.Marshrut, dns)})
	}
	return p
}

// programmyTrafika сводит ручные правила приложений и программы включённых
// сервисов в один список в порядке применения.
func programmyTrafika(v Vhod) []protokol.PraviloPrilozheniya {
	itog := append([]protokol.PraviloPrilozheniya(nil), v.Trafik.Prilozheniya...)
	for _, s := range v.Trafik.Servisy {
		for _, put := range s.Programmy {
			// Потомки у сервиса включены ВСЕГДА: карточка Steam это лаунчер и
			// игры, которые он запускает, а галочки на карточке нет. Правило,
			// накрывающее лаунчер и не накрывающее игру, человек прочитал бы как
			// сломанное.
			itog = append(itog, protokol.PraviloPrilozheniya{Put: put, Marshrut: s.Marshrut, Potomki: true})
		}
	}
	return itog
}
