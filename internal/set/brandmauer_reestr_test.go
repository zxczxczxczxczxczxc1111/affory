package set

import (
	"errors"
	"strings"
	"testing"

	"golang.org/x/sys/windows/registry"
)

// Числа реестра превращаются ровно в ту строку, которую netsh принимает назад.
//
// Формат не наш: он уезжает в `netsh advfirewall set <профиль>profile
// firewallpolicy` при выключении режима. Ошибка здесь вернула бы человеку
// политику, которой у него не было.
func TestPolitikaSobiraetsyaVFormateNetsh(t *testing.T) {
	sluchai := []struct {
		vhod, vyhod uint64
		zhdyom      string
	}{
		{1, 0, "BlockInbound,AllowOutbound"}, // умолчание Windows
		{1, 1, "BlockInbound,BlockOutbound"}, // наш режим «весь трафик»
		{0, 0, "AllowInbound,AllowOutbound"},
		{0, 1, "AllowInbound,BlockOutbound"},
	}
	for _, s := range sluchai {
		if got := politikaIzChisel(s.vhod, s.vyhod); got != s.zhdyom {
			t.Errorf("вход %d, выход %d дали %q вместо %q", s.vhod, s.vyhod, got, s.zhdyom)
		}
	}
	// Та же форма, что понимает разбор вывода netsh: если эти двое разойдутся,
	// половина кода будет читать политику, которую другая половина не узнаёт.
	if !rePolitika.MatchString("Firewall Policy   " + politikaIzChisel(1, 0)) {
		t.Error("собранная политика не совпала с той, что разбирается из netsh")
	}
}

// П2 аудита 1.8.0: на нетронутой русской Windows значений профиля в
// реестре может не быть, и прежде это уводило в разбор «ВКЛ». Недостающее
// значение это умолчание Windows, а не отказ.
func TestNedostayushcheeZnachenieEtoUmolchanieWindows(t *testing.T) {
	pusto := func(string) (uint64, error) { return 0, registry.ErrNotExist }
	pr, err := sostoyanieIzZnacheniy("private", pusto)
	if err != nil {
		t.Fatal(err)
	}
	if !pr.Vklyuchen || pr.Politika != "BlockInbound,AllowOutbound" {
		t.Fatalf("пустая ветка дала %+v, ждали умолчание Windows", pr)
	}

	// Есть только одно значение: оно берётся, остальные умолчанием.
	pr, err = sostoyanieIzZnacheniy("public", func(imya string) (uint64, error) {
		if imya == "DefaultOutboundAction" {
			return 1, nil
		}
		return 0, registry.ErrNotExist
	})
	if err != nil || !pr.Vklyuchen || pr.Politika != "BlockInbound,BlockOutbound" {
		t.Fatalf("частичная ветка дала %+v, %v", pr, err)
	}

	// Отказ чтения, кроме отсутствия, не подменяется умолчанием.
	if _, err := sostoyanieIzZnacheniy("domain", func(string) (uint64, error) {
		return 0, errors.New("тест: доступ запрещён")
	}); err == nil {
		t.Fatal("отказ чтения реестра выдан за умолчание")
	}
}

// Профиль private лежит в ветке Standard, и перепутать их нельзя: чтение
// пошло бы не в тот профиль, а человек получил бы обратно чужое состояние.
func TestPrivateZhivyotVVetkeStandard(t *testing.T) {
	if vetkiProfiley["private"] != "StandardProfile" {
		t.Errorf("private читается из %q", vetkiProfiley["private"])
	}
	for _, imya := range []string{"domain", "private", "public"} {
		if _, est := vetkiProfiley[imya]; !est {
			t.Errorf("для профиля %s нет ветки реестра", imya)
		}
	}
}

// Два пути к одному факту обязаны сходиться на живой машине.
//
// Реестр читается потому, что вывод netsh переводится на язык системы: на
// русской Windows состояние печатается как «ВКЛ», регулярка искала ON и не
// находила, а отказ поднимался наверх — режим «весь трафик» нельзя было ни
// включить, ни снять. Проверить это на английской машине нельзя, зато можно
// проверить обратное: что новый путь отвечает то же самое, что старый, там,
// где старый заведомо работает.
func TestReestrISheshNetshSoglasny(t *testing.T) {
	for _, p := range imenaProfiley {
		imya := strings.TrimSuffix(p, "profile")
		izReestra, err := sostoyanieIzReestra(imya)
		if err != nil {
			t.Fatalf("профиль %s не прочитан из реестра: %v", imya, err)
		}

		vyhod, err := vypolnitNetsh([]string{"advfirewall", "show", p})
		if err != nil {
			t.Skipf("netsh не ответил про профиль %s: %v", imya, err)
		}
		m := reSostoyanie.FindStringSubmatch(vyhod)
		if m == nil {
			// Ровно то, что происходит на неанглийской машине. Не провал
			// продукта: именно ради этого случая и читается реестр.
			t.Skipf("netsh не сказал ON или OFF про профиль %s: язык системы не английский", imya)
		}
		if vklyuchen := strings.EqualFold(m[1], "ON"); vklyuchen != izReestra.Vklyuchen {
			t.Errorf("профиль %s: netsh говорит включён=%v, реестр говорит %v", imya, vklyuchen, izReestra.Vklyuchen)
		}
		if mp := rePolitika.FindStringSubmatch(vyhod); mp != nil && mp[1] != izReestra.Politika {
			t.Errorf("профиль %s: netsh говорит политика %q, реестр говорит %q", imya, mp[1], izReestra.Politika)
		}
	}
}
