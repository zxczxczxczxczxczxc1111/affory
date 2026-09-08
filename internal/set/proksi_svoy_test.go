package set

import "testing"

// Сторож прокси обязан отличать НАШ прокси от чужого.
//
// До появления входа mixed своего прокси у нас не было по построению, и
// «включён» означало «перехват». Вход mixed на 127.0.0.1:10809 заведён ровно
// затем, чтобы человек прописал его системным прокси, и с этого момента
// прежнее правило обвиняет в перехвате нас самих.
func TestSvoyProksiNeSchitaetsyaChuzhim(t *testing.T) {
	sluchai := []struct {
		imya    string
		adres   string
		nash    int
		chuzhoy bool
	}{
		{"наш, голый адрес", "127.0.0.1:10809", 10809, false},
		{"наш, через localhost", "localhost:10809", 10809, false},
		{"наш, по перечислению схем", "http=127.0.0.1:10809;https=127.0.0.1:10809", 10809, false},
		{"другой порт на петле", "127.0.0.1:8888", 10809, true},
		{"наш порт, но не петля", "10.0.0.5:10809", 10809, true},
		{"прокси мы не поднимали", "127.0.0.1:10809", 0, true},
		{"https уходит на сторону", "http=127.0.0.1:10809;https=10.0.0.5:3128", 10809, true},
		{"чужой хост целиком", "proxy.example:3128", 10809, true},
	}
	for _, s := range sluchai {
		t.Run(s.imya, func(t *testing.T) {
			p := Proksi{Vklyuchen: true, Adres: s.adres}
			if got := p.Chuzhoy(s.nash); got != s.chuzhoy {
				t.Fatalf("Chuzhoy(%d) для %q вернул %v, ожидалось %v", s.nash, s.adres, got, s.chuzhoy)
			}
		})
	}
}

func TestVyklyuchennyyProksiNikogdaNeChuzhoy(t *testing.T) {
	p := Proksi{Vklyuchen: false, Adres: "10.0.0.5:3128"}
	if p.Chuzhoy(0) {
		t.Fatal("выключенный прокси объявлен перехватом")
	}
}
