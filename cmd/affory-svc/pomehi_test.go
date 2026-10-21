package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/pomehi"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

func pomehiIz(n ...pomehi.Nahodka) func(pomehi.Svoi) ([]pomehi.Nahodka, error) {
	return func(pomehi.Svoi) ([]pomehi.Nahodka, error) { return n, nil }
}

var (
	zapret    = pomehi.Nahodka{Imya: "zapret", Vid: pomehi.VidObhod, Gruppa: pomehi.Lomaet, Priznak: "winws.exe"}
	kaspersky = pomehi.Nahodka{Imya: "Kaspersky", Vid: pomehi.VidAntivirus, Gruppa: pomehi.Meshaet, Priznak: "avp.exe"}
)

// На главный экран уходит только группа «ломает»: антивирус у большинства
// работает рядом с VPN без бед, и строка о нём была бы ложной тревогой.
func TestStatusNesyotTolkoLomayushchie(t *testing.T) {
	s := podstavnaya(t, nil)
	s.proveritPomehi = pomehiIz(zapret, kaspersky)
	s.obnovitPomehi()

	p := s.Status().Pomehi
	if len(p) != 1 || p[0].Imya != "zapret" || p[0].Vid != "obhod" {
		t.Fatalf("помехи в статусе: %+v", p)
	}

	// Программу закрыли: строка уходит сама, без команды человека.
	s.proveritPomehi = pomehiIz(kaspersky)
	s.obnovitPomehi()
	if p := s.Status().Pomehi; len(p) != 0 {
		t.Fatalf("строка осталась после закрытия zapret: %+v", p)
	}
}

// Окно узнаёт о новой помехе событием, а не через пять секунд опроса.
func TestNovayaPomehaIzveshchaetOkno(t *testing.T) {
	s := podstavnaya(t, nil)
	id, kanal := s.Podpisatsya()
	defer s.Otpisatsya(id)
	s.proveritPomehi = pomehiIz(zapret)
	s.obnovitPomehi()
	select {
	case k := <-kanal:
		if k.Imya != "state" || !strings.Contains(string(k.Telo), "zapret") {
			t.Fatalf("событие %s: %s", k.Imya, k.Telo)
		}
	case <-time.After(time.Second):
		t.Fatal("новая помеха не дошла до окна событием")
	}
	// Тот же ответ второй раз: событий нет, окно не дёргается каждые 30 с.
	s.obnovitPomehi()
	select {
	case k := <-kanal:
		t.Fatalf("повтор без перемен дал событие %s", k.Imya)
	default:
	}
}

// Свой туннель служба называет пакету и индексом, и именем, пока держит его.
func TestSvoiAdapteryUhodyatVPoisk(t *testing.T) {
	s := podstavnaya(t, nil)
	var svoi pomehi.Svoi
	s.proveritPomehi = func(sv pomehi.Svoi) ([]pomehi.Nahodka, error) { svoi = sv; return nil, nil }

	s.obnovitPomehi()
	if svoi.ImyaAdaptera != "" || len(svoi.Adaptery) != 0 || svoi.KatalogProgrammy != s.dirProgrammy {
		t.Fatalf("выключенная служба назвала своё: %+v", svoi)
	}
	if err := s.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.obnovitPomehi()
	if svoi.ImyaAdaptera != imyaAdapteraTun || len(svoi.Adaptery) != 1 || svoi.Adaptery[0] != s.tun.Indeks {
		t.Fatalf("поднятая служба не назвала свой туннель: %+v", svoi)
	}
}

// Отказ «данные не пошли» называет антивирус и путь к папке программы, а
// zapret оставляет главному экрану.
func TestOtkazDannyhNazyvaetMeshayushchih(t *testing.T) {
	s := podstavnaya(t, errors.New("исходящий не отвечает (код 504)"))
	s.proveritPomehi = pomehiIz(zapret, kaspersky)
	staryy := zhdatPodyoma
	zhdatPodyoma = 900 * time.Millisecond
	defer func() { zhdatPodyoma = staryy }()

	if err := s.Connect(context.Background()); err == nil {
		t.Fatal("туннель, не понёсший трафик, признан поднятым")
	}
	st := s.Status()
	if st.Sostoyanie != protokol.SostNeNeset || st.Oshib == nil {
		t.Fatalf("состояние %s, ошибка %+v", st.Sostoyanie, st.Oshib)
	}
	for _, nado := range []string{"данные через него не пошли", "Kaspersky", s.dirProgrammy} {
		if !strings.Contains(st.Oshib.Tekst, nado) {
			t.Errorf("в отказе нет %q: %s", nado, st.Oshib.Tekst)
		}
	}
	if strings.Contains(st.Oshib.Tekst, "zapret") {
		t.Errorf("zapret попал в отказ, а не на главный экран: %s", st.Oshib.Tekst)
	}
}
