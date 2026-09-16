package katalog_test

import (
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/katalog"
)

// Каталог сервисов уезжает в правила ядра как `domain_suffix`, и ядро берёт
// ПЕРВОЕ подошедшее правило. Значит два дефекта данных меняют маршрут молча:
//
//   - один домен в двух сервисах: включённый и выключенный набор дерутся за
//     него, и побеждает тот, который человек переключил раньше;
//   - домен внутри домена другого сервиса (sub.example.com против
//     example.com): набор-родитель накрывает чужой поддомен целиком.
//
// Ни то, ни другое не видно на экране: тумблеры стоят как поставили, а трафик
// идёт не туда. Поэтому данные проверяются воротами, а не глазами при правке.
func TestKatalogNeDayotDvusmyslennyhDomenov(t *testing.T) {
	k, err := katalog.Chitat()
	if err != nil {
		t.Fatalf("каталог не прочитан: %v", err)
	}
	if len(k.Servisy) < 2 {
		t.Fatalf("в каталоге %d сервис: проверять нечего, сломан разбор", len(k.Servisy))
	}
	gde := map[string]string{}
	for _, s := range k.Servisy {
		for _, d := range s.Domeny {
			if d != strings.ToLower(strings.Trim(strings.TrimSpace(d), ".")) {
				t.Errorf("домен %q сервиса %q не нормализован", d, s.Id)
			}
			if prev, est := gde[d]; est {
				t.Errorf("домен %q лежит и в %q, и в %q: маршрут решит порядок переключений", d, prev, s.Id)
				continue
			}
			gde[d] = s.Id
		}
	}
	for domen, servis := range gde {
		for drugoy, chey := range gde {
			if servis != chey && strings.HasSuffix(domen, "."+drugoy) {
				t.Errorf("домен %q сервиса %q лежит внутри %q сервиса %q: набор-родитель накроет его целиком",
					domen, servis, drugoy, chey)
			}
		}
	}
}
