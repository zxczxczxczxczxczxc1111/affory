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
			return fmt.Errorf("для прямого трафика сначала отключите блокировку сети вне VPN")
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
		if _, err := katalog.Domeny(s.Id); err != nil {
			return err
		}
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
	for _, a := range v.Trafik.Prilozheniya {
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
		for _, a := range v.Trafik.Prilozheniya {
			field := "process_path"
			if a.Potomki {
				field = "process_path_tree"
			}
			p = append(p, map[string]any{field: []string{a.Put}, key: tegMarshruta(a.Marshrut, false)})
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
		p = append(p, map[string]any{"domain_suffix": domains, key: tegMarshruta(s.Marshrut, dns)})
	}
	return p
}
