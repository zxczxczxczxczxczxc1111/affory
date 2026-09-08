package set

import (
	"strings"
	"testing"
)

// Находка 10. Правило Affory-IPv6-Block-Out переживало смерть службы.
//
// Снятие жило только в Disconnect. При нештатной смерти уборка идёт через
// SnyatOsirotevshee, а та при существующем файле отката возвращалась сразу,
// правило v6 не трогая. Ветка VyklyuchitVesTrafik его тоже не снимает: она
// перебирает имена ИЗ ФАЙЛА, а IPv6 туда не вносился намеренно.
//
// Итог: IPv6 заблокирован без туннеля до следующего подъёма. Самолечение есть,
// но оно наступает позже, чем человек открывает браузер.
func TestSnyatOsirotevsheeVsegdaSnimaetPraviloIPv6(t *testing.T) {
	// Намеренно запертая машина в список НЕ входит: там сеть закрыта целиком
	// политикой, вреда от правила v6 нет, а трогать её нельзя вовсе. Это не
	// послабление, а граница: вред от находки существует ровно там, где у
	// человека сеть есть.
	for _, sluchay := range []struct {
		imya      string
		namerenno bool
		zaperta   bool
	}{
		{"файл устарел, машина открыта", true, false},
		{"наш собственный мусор", false, false},
	} {
		t.Run(sluchay.imya, func(t *testing.T) {
			zhurnal := perehvat(t, otvetProfiley(vyvodProfilyaRu))
			if err := ZapisatOtkat(Otkat{
				Namerenno: sluchay.namerenno,
				Pravila:   []string{PravAllowTun, PravAllowSrv},
				Profili:   []ProfilDo{{Imya: "domain", Vklyuchen: true, Politika: "BlockInbound,AllowOutbound"}},
			}); err != nil {
				t.Fatal(err)
			}
			if sluchay.zaperta {
				if _, err := vypolnit([]string{"advfirewall", "set", "allprofiles", "firewallpolicy", "blockinbound,blockoutbound"}); err != nil {
					t.Fatal(err)
				}
			}

			if _, _, err := SnyatOsirotevshee(); err != nil {
				t.Fatalf("уборка отказала: %v", err)
			}

			for _, z := range *zhurnal {
				soed := strings.Join(z.argumenty, " ")
				if strings.Contains(soed, "delete") && strings.Contains(soed, ImyaPravilaIPv6) {
					return
				}
			}
			t.Fatalf("правило %s не снято: IPv6 заблокирован без туннеля", ImyaPravilaIPv6)
		})
	}
}
