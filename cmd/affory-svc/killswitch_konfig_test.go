package main

import (
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Находка 16. Инвариант 6 спеки: в режиме «весь трафик» удобные исключения
// отменяются, петлевые остаются.
//
// Генератор это умел с самого начала, поле `Vhod.VesTrafik` было на месте и
// покрыто тестом, который зовёт генератор НАПРЯМУЮ. А сборщик конфига службы
// поле не заполнял никогда, поэтому обещанное поведение не наступало ни разу.
// Ровно тот случай, когда зелёный тест соседствует с мёртвым продом.
func TestVRezhimeVsegoTrafikaChastnyeSetiNeIsklyuchayutsya(t *testing.T) {
	s := podstavnaya(t, nil)
	s.mu.Lock()
	s.killSwitch = true
	s.mu.Unlock()

	telo, _, _, err := s.sobratTun(nil)
	if err != nil {
		t.Fatalf("конфиг туннеля не собран: %v", err)
	}
	if strings.Contains(string(telo), "ip_is_private") {
		t.Fatal("в режиме весь трафик частные сети всё ещё идут мимо туннеля")
	}
}

// КОНТРОЛЬ: без режима исключение обязано БЫТЬ. Иначе правка выше проходит и на
// сборщике, который выкинул исключение навсегда, а это уже сломанный принтер и
// недоступный роутер в обычной работе.
func TestBezRezhimaChastnyeSetiIsklyuchayutsya(t *testing.T) {
	s := podstavnaya(t, nil)

	telo, _, _, err := s.sobratTun(nil)
	if err != nil {
		t.Fatalf("конфиг туннеля не собран: %v", err)
	}
	if !strings.Contains(string(telo), "ip_is_private") {
		t.Fatal("частные сети загнаны в туннель без режима: роутер и принтер недоступны")
	}
}

// Петлевые исключения остаются ВСЕГДА, режим их не отменяет. Без них туннель
// съедает сам себя, и это отказ другого класса, чем неработающий принтер.
func TestRezhimNeOtmenyaetPetlevyeIsklyucheniya(t *testing.T) {
	s := podstavnaya(t, nil)
	s.mu.Lock()
	s.killSwitch = true
	s.sost = protokol.SostPodnyat
	s.mu.Unlock()

	telo, _, _, err := s.sobratTun(nil)
	if err != nil {
		t.Fatalf("конфиг туннеля не собран: %v", err)
	}
	// Адрес выбранного сервера обязан остаться в правиле петли.
	servery, err := s.serveryDlyaPodyoma()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(telo), servery[0].Host) {
		t.Fatal("режим снёс правило петли: туннель будет есть сам себя")
	}
}
