package genkonfig

import (
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"reflect"
	"testing"
)

func TestExplicitApplicationsPrecedeAllInheritedRoutes(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		apps := []protokol.PraviloPrilozheniya{
			{Put: `C:\Outer.exe`, Potomki: true, Marshrut: protokol.TrafikPryamo},
			{Put: `C:\Inner.exe`, Potomki: true, Marshrut: protokol.TrafikVPN},
			{Put: `C:\Only.exe`, Potomki: false, Marshrut: protokol.TrafikVPN},
		}
		if reverse {
			apps[0], apps[1] = apps[1], apps[0]
		}
		v := obraztsovyyVhod()
		v.Trafik = &protokol.PravilaTrafika{PoUmolchaniyu: protokol.TrafikVPN, Prilozheniya: apps}
		rules := trafikPravila(v, false)
		if len(rules) != 5 {
			t.Fatalf("expected three exact and two inherited rules: %v", rules)
		}
		for i, a := range apps {
			r := rules[i].(map[string]any)
			if !reflect.DeepEqual(r["process_path"], []string{a.Put}) || r["outbound"] != tegMarshruta(a.Marshrut, false) {
				t.Fatalf("exact rule not first: %v", r)
			}
		}
		for _, raw := range rules[3:] {
			r := raw.(map[string]any)
			if !reflect.DeepEqual(r["process_path_tree_roots"], []string{apps[0].Put, apps[1].Put}) {
				t.Fatalf("missing nearest-launcher boundary: %v", r)
			}
		}
		for _, raw := range trafikPravila(v, true) {
			if raw.(map[string]any)["process_path_tree"] != nil {
				t.Fatal("application identity invented for DNS")
			}
		}
	}
}
