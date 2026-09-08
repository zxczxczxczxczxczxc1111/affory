package set

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

// Решение 28 плана волны 2 требует сказать ВСЛУХ: после первого включения режима
// постоянное значение профиля навсегда перестаёт быть NotConfigured. Вернуть его
// нечем: netsh отвергает notconfigured дословно, а командлеты запрещены правилом
// «одна утилита на всё». Разницу не видно ничем, кроме командлета чтения, то есть
// человек узнает об этом только отсюда.
func perehvatZhurnala(t *testing.T) *bytes.Buffer {
	t.Helper()
	var b bytes.Buffer
	prezhniy := log.Writer()
	prezhniyFlag := log.Flags()
	log.SetOutput(&b)
	log.SetFlags(0)
	t.Cleanup(func() { log.SetOutput(prezhniy); log.SetFlags(prezhniyFlag) })
	return &b
}

func TestPervoeVklyuchenieGovoritProBezvozvratnost(t *testing.T) {
	perehvat(t, otvetProfiley(vyvodProfilyaRu))
	b := perehvatZhurnala(t)

	if err := VklyuchitVesTrafik(obraztsovoeRazreshyonnoe(), true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.ToLower(b.String()), "безвозвратно") {
		t.Fatalf("о безвозвратной правке чужой настройки не сказано ни слова, журнал: %q", b.String())
	}
}

func TestPeresborkaPravilNePovtoryaetPredupreZhdenie(t *testing.T) {
	// Пересборка зовёт ту же функцию на каждое добавление сервера при включённом
	// режиме. Повтор строки превратил бы предупреждение в шум, а шум не читают.
	perehvat(t, otvetProfiley(vyvodProfilyaRu))
	if err := VklyuchitVesTrafik(obraztsovoeRazreshyonnoe(), true); err != nil {
		t.Fatal(err)
	}

	b := perehvatZhurnala(t)
	if err := VklyuchitVesTrafik(obraztsovoeRazreshyonnoe(), true); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(b.String()), "безвозвратно") {
		t.Fatalf("предупреждение повторено на пересборке правил, журнал: %q", b.String())
	}
}
