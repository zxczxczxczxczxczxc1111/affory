package set

import "testing"

func TestChuzhoyTolkoKogdaVklyuchenIEstAdres(t *testing.T) {
	// Affory never writes a proxy under TUN, so any enabled one is somebody
	// else's. But ProxyEnable=1 with an empty ProxyServer is a leftover, not an
	// interception, and reporting it would train the user to ignore the warning.
	sluchai := []struct {
		imya   string
		p      Proksi
		chuzoy bool
	}{
		{"выключен", Proksi{}, false},
		{"включён с адресом", Proksi{Vklyuchen: true, Adres: "127.0.0.1:8080"}, true},
		{"включён без адреса", Proksi{Vklyuchen: true}, false},
		{"адрес без включения", Proksi{Adres: "127.0.0.1:8080"}, false},
	}
	// Ноль означает «своего прокси мы не поднимали»: прежнее поведение целиком,
	// и эти случаи обязаны продолжать работать как раньше.
	for _, s := range sluchai {
		if s.p.Chuzhoy(0) != s.chuzoy {
			t.Errorf("%s: Chuzhoy(0)=%v, ожидалось %v", s.imya, s.p.Chuzhoy(0), s.chuzoy)
		}
	}
}

func TestChteniyeRealnogoReestraNePadaet(t *testing.T) {
	// Reading is harmless and the key may legitimately be absent. The one thing
	// this must never do is fail in a way that looks like an interception.
	p, err := SistemnyyProksi()
	if err != nil {
		t.Fatalf("чтение реестра отказало: %v", err)
	}
	t.Logf("на этой машине: включён=%v адрес=%q", p.Vklyuchen, p.Adres)
}
