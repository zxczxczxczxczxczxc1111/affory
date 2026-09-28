package set

import (
	"context"
	"errors"
	"net/netip"
	"slices"
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
	if len(a.Vse) != 2 {
		t.Fatalf("адресов %v, ожидалось два", stroki(a.Vse))
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
	if !soderzhitStroku(stroki(a.Vse), "198.51.100.9") {
		t.Fatalf("адреса подписки нет в списке: %v", stroki(a.Vse))
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
	if !soderzhitStroku(stroki(a.Vse), "198.51.100.9") {
		t.Fatalf("порт в адресе подписки сломал резолв: %v", stroki(a.Vse))
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
	if !soderzhitStroku(stroki(a.Vse), "203.0.113.1") {
		t.Fatalf("разрешившийся адрес потерян вместе с ошибкой: %v", stroki(a.Vse))
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
	if len(a.Vse) != 1 {
		t.Fatalf("адресов %v, ожидался один", stroki(a.Vse))
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
	if stroki(a.Vse)[0] != stroki(b.Vse)[0] || stroki(a.Vse)[1] != stroki(b.Vse)[1] {
		t.Fatalf("порядок неустойчив: %v против %v", stroki(a.Vse), stroki(b.Vse))
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

// С2 аудита 1.6.1. Правило петли в ядре берёт только адреса серверов: адрес
// подписки и хост наборов, попав туда, пускали мимо туннеля весь их хостинг.
func TestAdresaServerovOtdelnyOtPodpiskiIZagruzok(t *testing.T) {
	a, err := sobratAdresaS(context.Background(),
		rezolverIz(map[string][]string{
			"srv.example":      {"203.0.113.5", "203.0.113.4"},
			"podpiska.example": {"198.51.100.9"},
			"nabory.example":   {"198.51.100.20"},
		}),
		[]protokol.Server{{Host: "srv.example", Port: 443}, {Host: "192.0.2.225", Port: 8443}},
		"https://podpiska.example/sub", "https://nabory.example/ru.srs")
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Servery) != 2 || len(a.Servery["srv.example"]) != 2 || a.Servery["192.0.2.225"][0] != netip.MustParseAddr("192.0.2.225") {
		t.Fatalf("адреса серверов: %v", a.Servery)
	}
	if got := stroki(a.Servery["srv.example"]); got[0] != "203.0.113.4" {
		t.Fatalf("порядок адресов имени неустойчив: %v", got)
	}
	if len(a.Vse) != 5 {
		t.Fatalf("брандмауэру нужны все адреса: %v", stroki(a.Vse))
	}
}

// Порты уходят в правило брандмауэра. Мусор в портах hy2 отбрасывается: netsh
// отверг бы правило целиком, и с ним весь режим «весь трафик».
func TestPortyServerovIZagruzok(t *testing.T) {
	a, err := sobratAdresaS(context.Background(),
		rezolverIz(map[string][]string{"podpiska.example": {"198.51.100.9"}}),
		[]protokol.Server{
			{Host: "192.0.2.225", Port: 443},
			{Host: "192.0.2.226", Porty: "20000-30000, abc,443,70000,5-3"},
		},
		"https://podpiska.example:8443/sub", "http://192.0.2.1/ru.srs")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"20000-30000", "443", "80", "8443"}; !slices.Equal(a.Porty, want) {
		t.Fatalf("порты %v, ждали %v", a.Porty, want)
	}
}

func TestAdresaZagruzokPopadayutVSpisok(t *testing.T) {
	// Наборы rule_set качаются мимо туннеля (решено 31.08.2026). Хост набора
	// входит в список разрешающих правил брандмауэра, то есть здесь.
	a, err := sobratAdresaS(context.Background(),
		rezolverIz(map[string][]string{"nabory.example": {"198.51.100.20"}}),
		[]protokol.Server{{Host: "192.0.2.225"}},
		"", "https://nabory.example/geosite-category-ru.srs")
	if err != nil {
		t.Fatal(err)
	}
	if !soderzhitStroku(stroki(a.Vse), "198.51.100.20") {
		t.Fatalf("адреса хоста наборов нет в списке: %v", stroki(a.Vse))
	}
}
