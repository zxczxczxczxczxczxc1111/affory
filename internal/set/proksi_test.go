package set

import (
	"testing"

	"golang.org/x/sys/windows/registry"
)

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
	p, err := proksiIzKusta(registry.CURRENT_USER, putProksi)
	if err != nil {
		t.Fatalf("чтение реестра отказало: %v", err)
	}
	t.Logf("в своём кусте: включён=%v адрес=%q", p.Vklyuchen, p.Adres)
}

func TestSidChelovekaOtseivaetSluzhebnye(t *testing.T) {
	// Служба живёт под S-1-5-18, и куст этого SID ни при чём: сторож смотрел
	// именно туда и не срабатывал ни разу. Ветки *_Classes это не люди вовсе.
	lyudi := []string{"S-1-5-21-2287932854-2364132564-4052434809-1001", "S-1-12-1-1-2-3-4"}
	ne := []string{"S-1-5-18", "S-1-5-19", "S-1-5-20", ".DEFAULT",
		"S-1-5-21-2287932854-2364132564-4052434809-1001_Classes"}
	for _, s := range lyudi {
		if !sidCheloveka(s) {
			t.Errorf("%s не признан человеком", s)
		}
	}
	for _, s := range ne {
		if sidCheloveka(s) {
			t.Errorf("%s принят за человека", s)
		}
	}
}

func TestProksiLyudeyChitaetKustyANeSvoy(t *testing.T) {
	// Живое чтение реестра: своего куста у службы нет, а у запустившего тесты
	// человека он есть. Проверяем не значение (оно чужое и меняется), а то, что
	// перечисление вообще доходит до людей и не падает.
	lyudi, err := ProksiLyudey()
	if err != nil {
		t.Fatalf("кусты людей не прочитаны: %v", err)
	}
	if len(lyudi) == 0 {
		t.Skip("ни один профиль не загружен: читать нечего")
	}
	for _, p := range lyudi {
		if !sidCheloveka(p.Sid) {
			t.Fatalf("в список попал служебный куст %s", p.Sid)
		}
	}
}
