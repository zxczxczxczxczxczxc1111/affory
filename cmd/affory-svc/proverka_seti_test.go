package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/proby"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/set"
)

// Раздельная проверка слоёв (A3, 22.09.2026).
//
// Прежде продукт спрашивал одно: отвечает ли HTTP через выбранный исходящий.
// В госте измерено, чем это оборачивается: местный резолвер недоступен, имена
// не разрешаются по 12 секунд, а состояние всё это время «поднят».

func sloyPoVidu(sloi []sloyProverki, vid string) (sloyProverki, bool) {
	for _, s := range sloi {
		if s.Vid == vid {
			return s, true
		}
	}
	return sloyProverki{}, false
}

// bezSetevyhProb глушит пробы, которые иначе ушли бы в НАСТОЯЩУЮ сеть.
func bezSetevyhProb(s *Sluzhba) {
	s.probaImeni = func(_ context.Context, imya string) proby.Itog {
		return proby.Itog{Proshlo: true, Podrobno: imya + " -> 5.255.255.242 (заглушка)"}
	}
	s.adaptery = func() ([]set.Adapter, error) { return nil, nil }
}

func TestTunnelBezMarshrutaNeSchitaetsyaRabochim(t *testing.T) {
	// Адаптер жив, адрес есть, а трафик идёт мимо: так выглядит чужой клиент,
	// перетянувший маршрут на себя. HTTP-проба через clash_api этого не видит
	// вовсе - она ходит своим путём.
	s := sluzhbaPodnyataya(t)
	bezSetevyhProb(s)
	s.mu.Lock()
	indeks := s.tun.Indeks
	s.mu.Unlock()
	s.adaptery = func() ([]set.Adapter, error) {
		return []set.Adapter{{
			Indeks: indeks, Imya: "tun0",
			Adresa:     []netip.Addr{netip.MustParseAddr("172.19.0.1")},
			Umolchanie: false,
		}}, nil
	}

	sl := s.sloyTunnelya(context.Background())
	if sl.Proshlo {
		t.Fatal("туннель без маршрута по умолчанию признан рабочим")
	}
	if sl.Podrobno == "" {
		t.Error("отказ слоя ничего не объясняет")
	}
}

func TestZhivoyTunnelProhodit(t *testing.T) {
	s := sluzhbaPodnyataya(t)
	bezSetevyhProb(s)
	s.mu.Lock()
	indeks := s.tun.Indeks
	s.mu.Unlock()
	s.adaptery = func() ([]set.Adapter, error) {
		return []set.Adapter{{
			Indeks: indeks, Imya: "tun0",
			Adresa:     []netip.Addr{netip.MustParseAddr("172.19.0.1")},
			Umolchanie: true,
		}}, nil
	}
	if sl := s.sloyTunnelya(context.Background()); !sl.Proshlo {
		t.Fatalf("живой туннель не прошёл: %s", sl.Podrobno)
	}
}

func TestPropavshiyAdapterTunnelyaNazvanOtdelno(t *testing.T) {
	// Состояние «поднят» при отсутствующем адаптере это не то же самое, что
	// «трафик идёт мимо»: там чужой маршрут, здесь умерший драйвер.
	s := sluzhbaPodnyataya(t)
	bezSetevyhProb(s)
	sl := s.sloyTunnelya(context.Background())
	if sl.Proshlo {
		t.Fatal("слой туннеля прошёл без адаптера в системе")
	}
	if sl.Podrobno != "сетевого подключения Affory нет в системе" {
		t.Errorf("подробности %q не отличают пропажу адаптера от чужого маршрута", sl.Podrobno)
	}
}

func TestVyklyuchennyyVPNNeVydayotsyaZaPolomku(t *testing.T) {
	// Человек просто не подключён. Красная строка «туннель сломан» тут врёт.
	s := podstavnaya(t, nil)
	bezSetevyhProb(s)
	sl := s.sloyTunnelya(context.Background())
	if sl.Podrobno != "VPN выключен" {
		t.Fatalf("подробности слоя при выключенном VPN: %q", sl.Podrobno)
	}
}

func TestSloyYadraOtdelyaetMolchanieOtOtkaza(t *testing.T) {
	// Ядра нет и ядро не отвечает это разные беды: первое чинится
	// подключением, второе сменой сервера.
	s := podstavnaya(t, nil)
	bezSetevyhProb(s)
	if sl := s.sloyYadra(context.Background()); sl.Proshlo || sl.Podrobno != "ядро не запущено" {
		t.Fatalf("слой ядра при опущенном туннеле: %+v", sl)
	}

	s2 := sluzhbaPodnyataya(t)
	bezSetevyhProb(s2)
	s2.zamerit = func(context.Context, string, string, string) (time.Duration, error) {
		return 0, errors.New("сервер не принял рукопожатие")
	}
	sl := s2.sloyYadra(context.Background())
	if sl.Proshlo {
		t.Fatal("слой ядра прошёл при отказе замера")
	}
	if sl.Podrobno == "ядро не запущено" {
		t.Error("отказ замера выдан за отсутствие ядра")
	}
}

func TestSlomannyyMestnyyRezolverVidenOtdelnoOtTunnelya(t *testing.T) {
	// Тот самый случай из госта: туннель жив, сервер отвечает, а резолвер
	// прежней сети недоступен, и российский набор не разрешается.
	s := sluzhbaPodnyataya(t)
	bezSetevyhProb(s)
	s.mu.Lock()
	s.rezolverKonfiga = netip.MustParseAddr("192.168.0.1")
	indeks := s.tun.Indeks
	s.mu.Unlock()
	s.adaptery = func() ([]set.Adapter, error) {
		return []set.Adapter{{Indeks: indeks, Imya: "tun0",
			Adresa: []netip.Addr{netip.MustParseAddr("172.19.0.1")}, Umolchanie: true}}, nil
	}
	// Спрашивается ИМЯ через системный резолвер, а не адрес напрямую: прямой
	// вопрос на поднятом туннеле не возвращается вовсе, его забирает ядро
	// (измерено в госте 22.09.2026, 12.2 с к живому 192.168.0.1).
	var sprosili string
	s.probaImeni = func(_ context.Context, imya string) proby.Itog {
		sprosili = imya
		return proby.Itog{Podrobno: imya + " не разрешается: нет ответа за отведённый срок"}
	}

	sl := s.sloyMestnogoRezolvera(context.Background())
	if sprosili != imyaProbyMimoVPN {
		t.Fatalf("спросили %q, а путь мимо VPN проверяется российским именем", sprosili)
	}
	if sl.Proshlo {
		t.Fatal("недоступный местный резолвер признан рабочим")
	}
	// Адрес из конфига обязан быть в подробностях: чинить человек будет его, а
	// имя это только признак.
	if !strings.Contains(sl.Podrobno, "192.168.0.1") {
		t.Errorf("в подробностях %q нет адреса резолвера: чинить нечего", sl.Podrobno)
	}
	// И туннель при этом остаётся зелёным: в том и смысл раздельных слоёв.
	if tun := s.sloyTunnelya(context.Background()); !tun.Proshlo {
		t.Fatalf("слой туннеля покраснел из-за DNS: %s", tun.Podrobno)
	}
}

func TestProverkaSetiOtvechaetVsemiSloyami(t *testing.T) {
	s := sluzhbaPodnyataya(t)
	bezSetevyhProb(s)
	// Проба UDP уходит в настоящую сеть, поэтому команду целиком здесь не
	// зовём: проверяется состав ответа по слоям, которые считаются без неё.
	sloi := []sloyProverki{s.sloyTunnelya(context.Background()), s.sloyYadra(context.Background()),
		s.sloyMestnogoRezolvera(context.Background())}
	for _, vid := range []string{"tunnel", "yadro", "mestnyy-dns"} {
		sl, est := sloyPoVidu(sloi, vid)
		if !est {
			t.Fatalf("в ответе нет слоя %q", vid)
		}
		if sl.Podpis == "" {
			t.Errorf("у слоя %q нет подписи: окно нарисует пустую строку", vid)
		}
	}
}

func TestOtvetProverkiSetiRazbiraetsya(t *testing.T) {
	// Тело команды читает окно, и форма ответа это договор с ним.
	s := podstavnaya(t, nil)
	bezSetevyhProb(s)
	sl := s.sloyTunnelya(context.Background())
	b, err := json.Marshal(map[string]any{"vremya": s.seychas().UTC().Format(time.RFC3339), "sloi": []sloyProverki{sl}})
	if err != nil {
		t.Fatalf("ответ не сериализуется: %v", err)
	}
	var telo struct {
		Vremya string `json:"vremya"`
		Sloi   []struct {
			Vid      string `json:"vid"`
			Podpis   string `json:"podpis"`
			Proshlo  bool   `json:"proshlo"`
			Podrobno string `json:"podrobno"`
			Ms       int64  `json:"ms"`
		} `json:"sloi"`
	}
	if err := json.Unmarshal(b, &telo); err != nil {
		t.Fatalf("ответ не разбирается обратно: %v", err)
	}
	if len(telo.Sloi) != 1 || telo.Sloi[0].Vid != "tunnel" || telo.Vremya == "" {
		t.Fatalf("форма ответа разошлась с договором: %s", b)
	}
}

func TestKomandaProverkiSetiEstVDispetchere(t *testing.T) {
	// Без этого механизм мёртв: команда есть в коде, а позвать её нельзя.
	s := podstavnaya(t, nil)
	bezSetevyhProb(s)
	// Настоящая команда зовёт пробу UDP, а та ходит в сеть: здесь проверяется
	// только то, что ветка диспетчера существует и не отвечает «не знаю».
	k := s.Obrabotat(context.Background(), protokol.Kadr{Id: 1, Imya: "checkNetwork"})
	if k.Oshib != nil && k.Oshib.Kod == protokol.KodNeRealizovano {
		t.Fatal("диспетчер не знает команды checkNetwork")
	}
}

func TestSrokOtvetaPerekryvaetRabotuProverki(t *testing.T) {
	// Дефект живой машины, 22.09.2026: команда не стояла в списке долгих и
	// получала пять секунд, работая при этом до тридцати. На выключенном VPN
	// она отвечала за 1.8 с и выглядела здоровой; на поднятом канал рвал её
	// молча, и окно показало бы «служба не ответила» на работающей проверке.
	if srok := protokol.SrokOtveta("checkNetwork"); srok <= srokProverkiSeti {
		t.Fatalf("срок ответа %s не перекрывает работу проверки (%s): канал оборвёт её на середине",
			srok, srokProverkiSeti)
	}
}

func TestAvariyaNazyvaetPrichinu(t *testing.T) {
	// Прежде человек получал «VPN перестал нести трафик» на любой отказ. Чужой
	// маршрут поверх туннеля, умерший адаптер и недоступный резолвер чинятся
	// по-разному, а выглядели одинаково.
	s := sluzhbaPodnyataya(t)
	bezSetevyhProb(s)
	s.mu.Lock()
	indeks := s.tun.Indeks
	s.mu.Unlock()
	s.adaptery = func() ([]set.Adapter, error) {
		return []set.Adapter{{Indeks: indeks, Imya: "tun0",
			Adresa: []netip.Addr{netip.MustParseAddr("172.19.0.1")}, Umolchanie: false}}, nil
	}
	prichina := s.prichinaRazryva(context.Background())
	if prichina == "" {
		t.Fatal("причина разрыва не названа: сообщение останется общим")
	}
	if !strings.Contains(prichina, "мимо") {
		t.Errorf("причина %q не про чужой маршрут", prichina)
	}
}

func TestPrichinaRazryvaMolchitKogdaSlomanoNeTut(t *testing.T) {
	// Туннель цел, резолвер отвечает: сломано что-то на стороне сервера, и
	// выдумывать местную причину нельзя.
	s := sluzhbaPodnyataya(t)
	bezSetevyhProb(s)
	s.mu.Lock()
	indeks := s.tun.Indeks
	s.rezolverKonfiga = netip.MustParseAddr("192.168.0.1")
	s.mu.Unlock()
	s.adaptery = func() ([]set.Adapter, error) {
		return []set.Adapter{{Indeks: indeks, Imya: "tun0",
			Adresa: []netip.Addr{netip.MustParseAddr("172.19.0.1")}, Umolchanie: true}}, nil
	}
	if prichina := s.prichinaRazryva(context.Background()); prichina != "" {
		t.Fatalf("названа причина %q, хотя местные слои целы", prichina)
	}
}
