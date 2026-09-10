package diagnostika

import (
	"errors"
	"testing"
	"time"
)

// Срез снимается каждую секунду, и падать ему нельзя. Диагностика нужна ровно
// тогда, когда что-то уже сломано, и источник, который в этот момент отказал,
// обязан испортить СВОЁ поле, а не весь ряд.

func TestSrezSobiraetVsyoChtoDali(t *testing.T) {
	seychas := time.Date(2026, 9, 10, 18, 30, 0, 0, time.UTC)
	s := Snyat(Istochniki{
		Seychas:      func() time.Time { return seychas },
		PidYadra:     func() int { return 4242 },
		Deskriptorov: func(pid int) (int, error) { return 100 + pid, nil },
		Porty:        func() (int, int, error) { return 900, 16384, nil },
		Yadro: func() (Yadro, error) {
			return Yadro{Soedineniy: 37, Vverh: 1000, Vniz: 2000, Zaderzhka: 3 * time.Millisecond}, nil
		},
		Runtime: func() (gorutin int, pamyatBayt uint64) { return 42, 5 << 20 },
	})
	if !s.Vremya.Equal(seychas) {
		t.Fatalf("время среза %v, ждали %v", s.Vremya, seychas)
	}
	if s.DeskriptorovYadra != 4342 {
		t.Fatalf("дескрипторов ядра %d, ждали 4342", s.DeskriptorovYadra)
	}
	if s.PortovZanyato != 900 || s.PortovVsego != 16384 {
		t.Fatalf("порты %d из %d", s.PortovZanyato, s.PortovVsego)
	}
	if s.SoedineniyVYadre != 37 || s.BaytVverh != 1000 || s.BaytVniz != 2000 {
		t.Fatalf("ядро отдало не то: %+v", s)
	}
	if s.ZaderzhkaYadraMs != 3 {
		t.Fatalf("задержка %v, ждали 3", s.ZaderzhkaYadraMs)
	}
	if s.Gorutin != 42 || s.PamyatiMB != 5 {
		t.Fatalf("runtime отдал не то: горутин %d, память %v", s.Gorutin, s.PamyatiMB)
	}
	if s.Sboi != "" {
		t.Fatalf("сбоев не было, а поле заполнено: %q", s.Sboi)
	}
}

// Отказ одного источника не уносит остальные поля. Именно так и выглядит
// интересный момент: порты кончились, значит и опрос ядра по локальному сокету
// уже не проходит, а число дескрипторов в эту секунду ценнее всего.
func TestOtkazIstochnikaNeRonyaetSrez(t *testing.T) {
	s := Snyat(Istochniki{
		Seychas:      time.Now,
		PidYadra:     func() int { return 7 },
		Deskriptorov: func(int) (int, error) { return 3155, nil },
		Porty:        func() (int, int, error) { return 16380, 16384, nil },
		Yadro:        func() (Yadro, error) { return Yadro{}, errors.New("clash_api не отвечает: bind") },
		Runtime:      func() (int, uint64) { return 900, 60 << 20 },
	})
	if s.DeskriptorovYadra != 3155 {
		t.Fatalf("дескрипторы потерялись из-за чужого отказа: %d", s.DeskriptorovYadra)
	}
	if s.PortovZanyato != 16380 {
		t.Fatalf("порты потерялись: %d", s.PortovZanyato)
	}
	if s.Gorutin != 900 {
		t.Fatalf("горутины потерялись: %d", s.Gorutin)
	}
	if s.Sboi == "" {
		t.Fatalf("источник отказал, а сбой не назван")
	}
}

// Отсутствующий источник это не отказ: ядра может не быть вовсе, когда туннель
// опущен, и писать про это в сбои значит забить журнал шумом в покое.
func TestOtsutstvuyushchiyIstochnikNeSchitaetsyaSboem(t *testing.T) {
	s := Snyat(Istochniki{
		Seychas:      time.Now,
		PidYadra:     func() int { return 0 },
		Deskriptorov: func(int) (int, error) { return 120, nil },
		Porty:        func() (int, int, error) { return 500, 16384, nil },
		Runtime:      func() (int, uint64) { return 30, 1 << 20 },
	})
	if s.Sboi != "" {
		t.Fatalf("ядра нет, а это записано как сбой: %q", s.Sboi)
	}
	if s.DeskriptorovYadra != 0 {
		t.Fatalf("ядра нет, а дескрипторы у него нашлись: %d", s.DeskriptorovYadra)
	}
	if s.DeskriptorovSluzhby != 120 {
		t.Fatalf("свои дескрипторы потерялись: %d", s.DeskriptorovSluzhby)
	}
}
