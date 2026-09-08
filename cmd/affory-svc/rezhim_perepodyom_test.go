package main

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/set"
)

// Находка 16, вторая половина. Решение владельца 02.09.2026: перезапуск ядра
// ДО запирания брандмауэра.
//
// Порядок и есть всё решение. Обратный порядок тоже работает в хорошем случае,
// но при неудачном перезапуске оставляет машину запертой БЕЗ СЕТИ, и лечится
// это только аварийным файлом. Поэтому тест проверяет последовательность, а не
// конечное состояние.
func TestVklyucheniyeRezhimaSnachalaPerepodnimaetYadro(t *testing.T) {
	s := podstavnaya(t, nil)
	var mu sync.Mutex
	var poryadok []string
	otmetit := func(chto string) { mu.Lock(); poryadok = append(poryadok, chto); mu.Unlock() }

	prezhniy := s.podnyatTunnel
	s.podnyatTunnel = func(ctx context.Context) (set.Adapter, error) {
		otmetit("подъём")
		return prezhniy(ctx)
	}
	s.vklyuchitVes = func(set.Razreshyonnoe, bool) error { otmetit("запирание"); return nil }

	if err := s.Connect(context.Background()); err != nil {
		t.Fatalf("первый подъём не удался: %v", err)
	}
	t.Cleanup(s.Otklyuchit)
	mu.Lock()
	poryadok = nil
	mu.Unlock()

	if err := s.SetKillSwitch(true); err != nil {
		t.Fatalf("режим не включился: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(poryadok) < 2 {
		t.Fatalf("шагов %v, ожидались подъём и запирание", poryadok)
	}
	if poryadok[0] != "подъём" {
		t.Fatalf("порядок %v: заперли раньше, чем перепподняли ядро", poryadok)
	}
	if !soderzhit(poryadok, "запирание") {
		t.Fatalf("порядок %v: машина не заперта вовсе", poryadok)
	}
}

// Неудачный перезапуск обязан оставить машину ОТКРЫТОЙ и режим выключенным.
// Ровно за это владелец и выбрал этот порядок.
func TestNeudachnyyPerepodyomNeZapiraetMashinu(t *testing.T) {
	s := podstavnaya(t, nil)
	if err := s.Connect(context.Background()); err != nil {
		t.Fatalf("первый подъём не удался: %v", err)
	}
	t.Cleanup(s.Otklyuchit)

	zaperli := false
	s.vklyuchitVes = func(set.Razreshyonnoe, bool) error { zaperli = true; return nil }
	s.podnyatTunnel = func(ctx context.Context) (set.Adapter, error) {
		return set.Adapter{}, errors.New("адаптер не поднялся")
	}

	err := s.SetKillSwitch(true)
	if err == nil {
		t.Fatal("режим включился поверх неподнятого туннеля")
	}
	if zaperli {
		t.Fatal("машина заперта при неудачном перезапуске: сети нет и вернуть её нечем")
	}
	if s.Status().KillSwitch {
		t.Fatal("режим считается включённым после неудачи")
	}
	// Причина обязана быть НАЗВАНА. Ниже по коду стоит второй страж («нет
	// адреса туннеля»), и он поймает ту же ситуацию, но скажет человеку не то:
	// туннель не поднялся, а не адаптер оказался без адреса.
	if !strings.Contains(err.Error(), "переподнялся") {
		t.Fatalf("причина отказа подменена вторым стражем: %v", err)
	}
}

// В перезапущенном ядре правило про частные сети обязано исчезнуть: ради этого
// перезапуск и делается.
func TestPosleVklyucheniyaRezhimaKonfigBezChastnyhSetey(t *testing.T) {
	s := podstavnaya(t, nil)
	if err := s.Connect(context.Background()); err != nil {
		t.Fatalf("первый подъём не удался: %v", err)
	}
	t.Cleanup(s.Otklyuchit)
	s.vklyuchitVes = func(set.Razreshyonnoe, bool) error { return nil }

	if err := s.SetKillSwitch(true); err != nil {
		t.Fatalf("режим не включился: %v", err)
	}

	telo, _, _, err := s.sobratTun(nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(telo), "ip_is_private") {
		t.Fatal("после включения режима частные сети всё ещё идут мимо туннеля")
	}
	if s.Status().Sostoyanie != protokol.SostPodnyat {
		t.Fatalf("состояние %s вместо podnyat: перезапуск не довёл туннель", s.Status().Sostoyanie)
	}
}

func soderzhit(s []string, chto string) bool {
	for _, x := range s {
		if x == chto {
			return true
		}
	}
	return false
}
