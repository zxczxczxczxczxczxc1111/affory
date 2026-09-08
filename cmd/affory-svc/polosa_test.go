package main

import (
	"context"
	"encoding/json"
	"net/netip"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/sostoyanie"
)

// Полоса канала доезжает от настройки до конфига ядра.
//
// Тот же класс дефекта, что находка 16 (см. killswitch_konfig_test.go):
// генератор поле умеет, тест генератора зелёный, а сборщик службы его не
// заполняет, и обещанное поведение не наступает ни разу. Здесь цена конкретная:
// без объявленной полосы sing-box уходит на BBR вместо Brutal, а вход
// `hy2-brutal` на сервере стоит с ignore_client_bandwidth false и объявления
// ЖДЁТ. То есть вход работает не тем, чем назван, и молча.
func TestPolosaKanalaDoezzhaetDoKonfiga(t *testing.T) {
	s := podstavnayaSHy2(t)
	if err := s.SetPolosa(250, 440); err != nil {
		t.Fatalf("полоса не записана: %v", err)
	}

	telo, _, _, err := s.sobratTun(nil)
	if err != nil {
		t.Fatalf("конфиг туннеля не собран: %v", err)
	}
	o := hy2Ishodyashchiy(t, telo)
	if o["up_mbps"] != float64(250) || o["down_mbps"] != float64(440) {
		t.Fatalf("полоса не дошла до ядра: up=%v down=%v", o["up_mbps"], o["down_mbps"])
	}
}

// КОНТРОЛЬ: без настройки полей быть НЕ должно. Иначе правка выше проходит и на
// сборщике, который объявляет полосу всегда, а объявленная наугад полоса это
// восемь провалов и p95 3214 мс на узком канале (замер 05.09.2026).
func TestBezNastroykiPolosaNeObyavlyaetsya(t *testing.T) {
	s := podstavnayaSHy2(t)

	telo, _, _, err := s.sobratTun(nil)
	if err != nil {
		t.Fatalf("конфиг туннеля не собран: %v", err)
	}
	o := hy2Ishodyashchiy(t, telo)
	if _, est := o["up_mbps"]; est {
		t.Fatalf("полоса объявлена без настройки: %v", o["up_mbps"])
	}
}

// Отрицательная и односторонняя полоса ОТВЕРГАЮТСЯ службой, а не уезжают в
// генератор. Генератор их тоже отвергнет, но там отказ означает несобранный
// конфиг всего туннеля, то есть человек получает «туннель не поднялся» вместо
// «в поле полосы ерунда».
func TestNegodnayaPolosaOtvergaetsyaSluzhboy(t *testing.T) {
	s := podstavnayaSHy2(t)
	for _, sl := range []struct{ vverh, vniz int }{
		{-1, 100}, {100, -1}, {0, 100}, {100, 0},
	} {
		if err := s.SetPolosa(sl.vverh, sl.vniz); err == nil {
			t.Fatalf("полоса %d/%d принята", sl.vverh, sl.vniz)
		}
	}
	// Пара нулей это законное «не измерена»: так полоса снимается.
	if err := s.SetPolosa(0, 0); err != nil {
		t.Fatalf("снятие полосы отвергнуто: %v", err)
	}
}

// Полоса это НАСТРОЙКА, а не состояние туннеля: она обязана пережить и
// отключение, и перезапуск службы.
//
// Оба пути теряют поля молча и по-разному. sbrositSost стирает файл целиком и
// оставляет поимённый список переживающих; zagruzitNastroyki читает файл при
// старте и копирует в снимок тоже поимённо. Забыть можно в любом из двух, и
// человек увидит одно и то же: настройка «сбросилась сама».
func TestPolosaPerezhivaetSbrosIPerezapusk(t *testing.T) {
	t.Run("отключение туннеля", func(t *testing.T) {
		s := podstavnayaSHy2(t)
		if err := s.SetPolosa(250, 440); err != nil {
			t.Fatal(err)
		}
		if err := s.sbrositSost(); err != nil {
			t.Fatalf("сброс состояния: %v", err)
		}
		s.mu.Lock()
		vverh, vniz := s.snimok.PolosaVverh, s.snimok.PolosaVniz
		s.mu.Unlock()
		if vverh != 250 || vniz != 440 {
			t.Fatalf("полоса стёрта отключением: %d/%d", vverh, vniz)
		}
	})

	t.Run("перезапуск службы", func(t *testing.T) {
		s := podstavnaya(t, nil)
		// Файл на диске уже содержит настройку, службы ещё нет: ровно то, что
		// видит служба при старте Windows.
		s.prochitat = func() (sostoyanie.SostoyanieFayla, error) {
			return sostoyanie.SostoyanieFayla{PolosaVverh: 250, PolosaVniz: 440}, nil
		}
		s.zagruzitNastroyki()
		s.mu.Lock()
		vverh, vniz := s.snimok.PolosaVverh, s.snimok.PolosaVniz
		s.mu.Unlock()
		if vverh != 250 || vniz != 440 {
			t.Fatalf("полоса не прочитана при старте: %d/%d", vverh, vniz)
		}
	})
}

// Команда setBandwidth: единственный путь, которым число попадает в службу от
// человека, и отказ обязан доезжать до него текстом, а не молчанием.
func TestKomandaSetBandwidth(t *testing.T) {
	s := podstavnayaSHy2(t)

	t.Run("годная пара записывается и видна в статусе", func(t *testing.T) {
		k := s.Obrabotat(context.Background(), protokol.Kadr{
			Tip: "komanda", Id: 1, Imya: "setBandwidth",
			Telo: json.RawMessage(`{"vverh":250,"vniz":440}`),
		})
		if k.Oshib != nil {
			t.Fatalf("отказ на годной паре: %v", k.Oshib)
		}
		st := s.Status()
		if st.PolosaVverh != 250 || st.PolosaVniz != 440 {
			t.Fatalf("статус не отдаёт полосу: %d/%d", st.PolosaVverh, st.PolosaVniz)
		}
	})

	t.Run("негодная пара это отказ с текстом, а не тихий успех", func(t *testing.T) {
		k := s.Obrabotat(context.Background(), protokol.Kadr{
			Tip: "komanda", Id: 2, Imya: "setBandwidth",
			Telo: json.RawMessage(`{"vverh":100,"vniz":0}`),
		})
		if k.Oshib == nil {
			t.Fatal("односторонняя полоса принята командой")
		}
		if k.Oshib.Tekst == "" {
			t.Fatal("отказ без текста: человеку нечего прочитать")
		}
		// Прежнее значение остаётся: отвергнутая правка не имеет права стереть
		// то, что человек ввёл раньше.
		if st := s.Status(); st.PolosaVverh != 250 {
			t.Fatalf("отвергнутая правка стёрла прежнюю полосу: %d", st.PolosaVverh)
		}
	})
}

func podstavnayaSHy2(t *testing.T) *Sluzhba {
	t.Helper()
	s := podstavnaya(t, nil)
	srv := protokol.Server{
		Id: "h2", Imya: "hy2", Transport: "hy2",
		Host: "203.0.113.13", Port: 443, Parol: "parol",
	}
	if err := s.pravitNabor(func(n *Nabor) error {
		n.Servery = []protokol.Server{srv}
		n.Vybran = srv.Id
		return nil
	}); err != nil {
		t.Fatalf("набор не записан: %v", err)
	}
	// Адрес кандидата обязан быть в правиле петли, иначе генератор отвергает
	// конфиг целиком (sveritKandidatov). Своей сборки адресов здесь нет
	// намеренно: подставляем ровно тот адрес, что у сервера фикстуры.
	s.sobratAdresa = func() ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr(srv.Host)}, nil
	}
	return s
}

// hy2Ishodyashchiy достаёт единственный исходящий hysteria2 из конфига.
func hy2Ishodyashchiy(t *testing.T, telo []byte) map[string]any {
	t.Helper()
	var k struct {
		Outbounds []map[string]any `json:"outbounds"`
	}
	if err := json.Unmarshal(telo, &k); err != nil {
		t.Fatalf("конфиг не разбирается: %v", err)
	}
	for _, o := range k.Outbounds {
		if o["type"] == "hysteria2" {
			return o
		}
	}
	t.Fatal("в конфиге нет исходящего hysteria2: сломана фикстура, а не продукт")
	return nil
}
