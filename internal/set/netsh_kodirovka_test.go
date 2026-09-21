package set

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/kodirovki"
)

// Ответ netsh «правил нет» на русской Windows, байт в байт.
//
// Это «Ни одно правило не соответствует указанным критериям.» в кодовой
// странице 866, той самой, в которой отвечает консоль. Байты зашиты нарочно:
// сгенерировать их через свою же перекодировку значило бы проверять прибор
// прибором, а машина разработчика вдобавок стоит на 437 и кириллицы в консоли
// не знает вовсе.
var otvet866 = []byte{
	0x8d, 0xa8, 0x20, 0xae, 0xa4, 0xad, 0xae, 0x20, 0xaf, 0xe0, 0xa0, 0xa2, 0xa8, 0xab, 0xae, 0x20,
	0xad, 0xa5, 0x20, 0xe1, 0xae, 0xae, 0xe2, 0xa2, 0xa5, 0xe2, 0xe1, 0xe2, 0xa2, 0xe3, 0xa5, 0xe2,
	0x20, 0xe3, 0xaa, 0xa0, 0xa7, 0xa0, 0xad, 0xad, 0xeb, 0xac, 0x20, 0xaa, 0xe0, 0xa8, 0xe2, 0xa5,
	0xe0, 0xa8, 0xef, 0xac, 0x2e,
}

// Сырые байты netsh НЕ распознаются: на этом и сломалась установка у человека
// 21.09.2026. Шаг стоит первым, потому что доказывает судью: без него зелёный
// тест ниже не отличить от «проверка ничего не проверяет».
func TestSyroyOtvetNetshNeRaspoznayotsya(t *testing.T) {
	if netPravil(string(otvet866)) {
		t.Error("байты консоли распознались как UTF-8: судья ниже ничего не доказывает")
	}
}

// Переведённый ответ распознаётся, и отсутствие правила перестаёт быть отказом.
//
// Цена вопроса: пока перевода не было, `prepare-install` падал на первой же
// попытке снять правило, которого нет. У кого правила брандмауэра оставались,
// установка проходила; у кого нет — нет.
func TestOtvetNetshRaspoznayotsyaPosleperevoda(t *testing.T) {
	vyhod := kodirovki.Iz(otvet866, 866)
	if !strings.Contains(strings.ToLower(vyhod), "ни одно правило") {
		t.Fatalf("перевод из 866 дал %q", vyhod)
	}
	if !netPravil(vyhod) {
		t.Errorf("ответ «правил нет» не распознан: %q", vyhod)
	}
}

// Английский ответ читается без всякого перевода: он ASCII в любой кодовой
// странице. Ровно поэтому дефект жил незамеченным — машина разработки и гость
// стоят на английской Windows.
func TestAngliyskiyOtvetRaspoznayotsyaVsegda(t *testing.T) {
	const otvet = "No rules match the specified criteria.\r\n"
	if !netPravil(kodirovki.Iz([]byte(otvet), 866)) {
		t.Error("английский ответ не распознан после перевода")
	}
	if !netPravil(otvet) {
		t.Error("английский ответ не распознан без перевода")
	}
}

// Снятие несуществующего правила это успех на ЛЮБОМ языке системы.
//
// Фразу netsh мы знаем на двух языках из сорока. Немецкая машина отвечала бы
// «Keine Regeln entsprechen den angegebenen Kriterien», не совпала бы ни с
// одной нашей строкой, и установка упала бы ровно так же, как 21.09.2026 у
// человека с русской. Поэтому решает не фраза, а имя правила.
func TestSnyatiePravilaNaNeznakomoyLokali(t *testing.T) {
	prezhniy := vypolnit
	t.Cleanup(func() { vypolnit = prezhniy })
	const chuzhaya = "Keine Regeln entsprechen den angegebenen Kriterien."
	vypolnit = func(a []string) (string, error) {
		return chuzhaya, errors.New("exit status 1")
	}

	if netPravil(chuzhaya) {
		t.Fatal("чужая фраза распозналась: возьмите другую, судья ничего не проверяет")
	}
	if err := SnyatPravilo("Affory-Allow-Tun"); err != nil {
		t.Errorf("снятие отсутствующего правила названо отказом: %v", err)
	}
}

// А вот стоящее правило, которое снять не вышло, отказом и остаётся: молча
// оставить его значило бы доложить о запертой машине, которая не заперта.
func TestNesnyatoeStoyashcheePraviloEtoOtkaz(t *testing.T) {
	prezhniy := vypolnit
	t.Cleanup(func() { vypolnit = prezhniy })
	vypolnit = func(a []string) (string, error) {
		if a[2] == "show" {
			// Показ удался и правило на месте.
			return "Rule Name: Affory-Allow-Tun\nEnabled: Yes\n", nil
		}
		return "Zugriff verweigert.", errors.New("exit status 1")
	}

	if err := SnyatPravilo("Affory-Allow-Tun"); err == nil {
		t.Error("стоящее правило не снято, а отказа нет")
	}
}

// Перевод вывода netsh стоит ровно в одном месте, и потерять его нельзя.
//
// Проверить перевод поведением на этой машине невозможно: её консоль отвечает
// по-английски, то есть ASCII, который одинаков в любой кодовой странице.
// Поэтому судья смотрит на исходник: строка с переводом либо есть, либо
// русские машины снова получают «ЌЁ ®¤­® Їа ўЁ«®» вместо фразы.
func TestVyvodNetshPerevoditsya(t *testing.T) {
	telo, err := os.ReadFile("ipv6.go")
	if err != nil {
		t.Fatalf("ipv6.go не прочитан: %v", err)
	}
	if !strings.Contains(string(telo), "kodirovki.Iz(syrye, kodirovki.Konsoli)") {
		t.Error("вывод netsh больше не переводится из кодовой страницы консоли: " +
			"распознавание ответов вернётся к зависимости от языка системы")
	}
}
