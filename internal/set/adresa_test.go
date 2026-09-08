package set

import (
	"context"
	"errors"
	"net/netip"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

func rezolverIz(karta map[string][]string) rezolver {
	return func(_ context.Context, _, host string) ([]netip.Addr, error) {
		a, est := karta[host]
		if !est {
			return nil, errors.New("нет такого имени")
		}
		out := make([]netip.Addr, 0, len(a))
		for _, s := range a {
			out = append(out, netip.MustParseAddr(s))
		}
		return out, nil
	}
}

func stroki(a []netip.Addr) []string {
	s := make([]string, 0, len(a))
	for _, v := range a {
		s = append(s, v.String())
	}
	return s
}

func TestLiteralyNeRezolvyatsya(t *testing.T) {
	// Резолвер на литерал отвечает по-разному в зависимости от настроек системы,
	// а гадать тут не о чем. Заодно это единственный случай, работающий без сети.
	a, err := sobratAdresaS(context.Background(), rezolverIz(nil), []protokol.Server{
		{Host: "192.0.2.225"}, {Host: "203.0.113.7"},
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 2 {
		t.Fatalf("адресов %v, ожидалось два", stroki(a))
	}
}

func TestAdresPodpiskiPopadaetVSpisok(t *testing.T) {
	// Без него запертый режим отрезает обновление подписки ровно тогда, когда
	// список серверов протух и обновить его нужнее всего.
	a, err := sobratAdresaS(context.Background(),
		rezolverIz(map[string][]string{"podpiska.example": {"198.51.100.9"}}),
		[]protokol.Server{{Host: "192.0.2.225"}},
		"https://podpiska.example/sub/token")
	if err != nil {
		t.Fatal(err)
	}
	if !soderzhitStroku(stroki(a), "198.51.100.9") {
		t.Fatalf("адреса подписки нет в списке: %v", stroki(a))
	}
}

func TestPortVAdresePodpiskiNeMeshaet(t *testing.T) {
	// u.Host отдал бы «podpiska.example:8443», и резолвер ответил бы отказом,
	// который на экране выглядит как недоступная подписка.
	a, err := sobratAdresaS(context.Background(),
		rezolverIz(map[string][]string{"podpiska.example": {"198.51.100.9"}}),
		[]protokol.Server{{Host: "192.0.2.225"}},
		"https://podpiska.example:8443/sub")
	if err != nil {
		t.Fatal(err)
	}
	if !soderzhitStroku(stroki(a), "198.51.100.9") {
		t.Fatalf("порт в адресе подписки сломал резолв: %v", stroki(a))
	}
}

func TestNerazreshimoeImyaNeProglatyvaetsya(t *testing.T) {
	// Пропустить молча значит оставить дыру в правиле петли: сервер зарезолвится
	// по TTL уже внутри туннеля, и трафик к нему пойдёт в туннель, который на
	// нём же и держится.
	a, err := sobratAdresaS(context.Background(),
		rezolverIz(map[string][]string{"est.example": {"203.0.113.1"}}),
		[]protokol.Server{{Host: "est.example"}, {Host: "net-takogo-imeni.invalid"}}, "")
	if !errors.Is(err, ErrImyaNeRazreshilos) {
		t.Fatalf("неразрешимое имя проглочено молча: %v", err)
	}
	// Разрешившееся обязано доехать: решать, поднимать ли туннель с дырой, не
	// сборщику адресов.
	if !soderzhitStroku(stroki(a), "203.0.113.1") {
		t.Fatalf("разрешившийся адрес потерян вместе с ошибкой: %v", stroki(a))
	}
}

func TestNastoyashchiyRezolverNeRazreshaetInvalid(t *testing.T) {
	// Контроль к тесту выше на ЖИВОМ резолвере: .invalid зарезервирован RFC 2606
	// и не разрешается нигде. Без него подставной резолвер мог бы проверять
	// собственную выдумку.
	if _, err := SobratAdresa([]protokol.Server{{Host: "net-takogo-imeni.invalid"}}, ""); !errors.Is(err, ErrImyaNeRazreshilos) {
		t.Fatalf("живой резолвер разрешил .invalid: %v", err)
	}
}

func TestDublikatyShlopyvayutsya(t *testing.T) {
	// Два сервера на одном VPS это наш случай: входы 443 и 2053 живут на одном
	// адресе. Дубликат в remoteip у netsh не ошибка, но правило растёт линейно и
	// однажды упрётся в предел длины строки.
	a, err := sobratAdresaS(context.Background(),
		rezolverIz(map[string][]string{"odin.example": {"203.0.113.1"}}),
		[]protokol.Server{
			{Host: "odin.example"}, {Host: "odin.example"}, {Host: "203.0.113.1"},
		}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 1 {
		t.Fatalf("адресов %v, ожидался один", stroki(a))
	}
}

func TestPoryadokUstoychiv(t *testing.T) {
	// Резолвер возвращает адреса в переменном порядке. Без сортировки конфиг
	// переписывался бы на каждом подъёме, а ядро перезапускалось бы там, где
	// ничего не изменилось.
	pervyy := rezolverIz(map[string][]string{"a.example": {"203.0.113.5", "203.0.113.2"}})
	vtoroy := rezolverIz(map[string][]string{"a.example": {"203.0.113.2", "203.0.113.5"}})
	s := []protokol.Server{{Host: "a.example"}}
	a, err := sobratAdresaS(context.Background(), pervyy, s, "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := sobratAdresaS(context.Background(), vtoroy, s, "")
	if err != nil {
		t.Fatal(err)
	}
	if stroki(a)[0] != stroki(b)[0] || stroki(a)[1] != stroki(b)[1] {
		t.Fatalf("порядок неустойчив: %v против %v", stroki(a), stroki(b))
	}
}

func TestPustoyVhodEtoOtkaz(t *testing.T) {
	if _, err := sobratAdresaS(context.Background(), rezolverIz(nil), nil, ""); err == nil {
		t.Fatal("пустой вход дал пустой список без ошибки: правило петли будет пустым")
	}
}

func soderzhitStroku(s []string, chto string) bool {
	for _, v := range s {
		if v == chto {
			return true
		}
	}
	return false
}

func TestAdresaZagruzokPopadayutVSpisok(t *testing.T) {
	// Наборы rule_set качаются мимо туннеля (решение владельца 31.08.2026), а
	// мимо туннеля ходит только то, что стоит в правиле петли и в разрешающих
	// правилах. Хост набора обязан быть в обоих списках, то есть здесь.
	a, err := sobratAdresaS(context.Background(),
		rezolverIz(map[string][]string{"nabory.example": {"198.51.100.20"}}),
		[]protokol.Server{{Host: "192.0.2.225"}},
		"", "https://nabory.example/geosite-category-ru.srs")
	if err != nil {
		t.Fatal(err)
	}
	if !soderzhitStroku(stroki(a), "198.51.100.20") {
		t.Fatalf("адреса хоста наборов нет в списке: %v", stroki(a))
	}
}
