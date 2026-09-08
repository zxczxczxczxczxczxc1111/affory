package yadra

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

func perehvat(t *testing.T) *bytes.Buffer {
	t.Helper()
	var b bytes.Buffer
	prezhniy := log.Writer()
	log.SetOutput(&b)
	t.Cleanup(func() { log.SetOutput(prezhniy) })
	return &b
}

// Вывод ядра до 02.09.2026 выбрасывался целиком: exec.Command запускался без
// Stdout и Stderr. Из-за этого отказ подъёма был недиагностируем в поле, и
// причину находки 43 пришлось доставать запуском ядра руками в госте.
func TestVyvodYadraPopadaetVZhurnal(t *testing.T) {
	b := perehvat(t)
	z := &zhurnalYadra{imya: "sing-box.exe"}

	// Поток приходит кусками, и граница куска не совпадает с границей строки.
	// Наивная запись «что пришло, то и в журнал» рвала бы строки посередине.
	if _, err := z.Write([]byte("FATAL[0000] initialize outb")); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(b.String(), "initialize outb") {
		t.Fatal("недописанная строка уехала в журнал: строки будут рваться посередине")
	}
	if _, err := z.Write([]byte("ound[24]: invalid public_key\n")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "initialize outbound[24]: invalid public_key") {
		t.Fatalf("строка ядра не доехала до журнала: %q", b.String())
	}
	if !strings.Contains(b.String(), "sing-box.exe") {
		t.Error("в журнале не видно, ЧЬЯ это строка: при двух ядрах разобрать будет нечем")
	}
}

func TestVyvodYadraOchishchaetsyaOtTsveta(t *testing.T) {
	b := perehvat(t)
	z := &zhurnalYadra{imya: "sing-box.exe"}
	if _, err := z.Write([]byte("\x1b[31mFATAL\x1b[0m[0000] beda\n")); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(b.String(), "\x1b[") {
		t.Errorf("цветовые последовательности в журнале это мусор при поиске: %q", b.String())
	}
	if !strings.Contains(b.String(), "FATAL[0000] beda") {
		t.Errorf("вместе с цветом ушёл текст: %q", b.String())
	}
}

// Ядро может начать говорить без остановки (например, при потере сети). Журнал
// службы это файл на диске, и заливать его чужим потоком нельзя.
func TestVyvodYadraOgranichenPoStrokam(t *testing.T) {
	b := perehvat(t)
	z := &zhurnalYadra{imya: "sing-box.exe"}
	for i := 0; i < predelStrokYadra+50; i++ {
		if _, err := z.Write([]byte("stroka\n")); err != nil {
			t.Fatal(err)
		}
	}
	if n := strings.Count(b.String(), "stroka"); n > predelStrokYadra {
		t.Errorf("строк ядра в журнале %d при пределе %d", n, predelStrokYadra)
	}
	if !strings.Contains(b.String(), "дальше молчим") {
		t.Error("о том, что вывод обрезан, не сказано: журнал будет выглядеть полным")
	}
}

// Отсутствие драйвера и занятое имя адаптера дают ОДИН И ТОТ ЖЕ таймаут
// ожидания. Различить их догадкой нельзя, поэтому судьёй берётся ядро: оно
// единственное знает, обо что споткнулось, и оно про это говорит.
//
// Строка ловится из потока ядра, а не из файла рядом с ним: wintun.dll лежит
// ВНУТРИ sing-box.exe (спека §14.1), отдельного файла у нас нет и не было.
func TestZhurnalZapominaetZhalobuYadraNaDrayver(t *testing.T) {
	perehvat(t)
	ZabytZhalobyYadra()
	z := &zhurnalYadra{imya: "sing-box.exe"}

	if _, err := z.Write([]byte("FATAL[0000] create service: wintun: Access is denied.\n")); err != nil {
		t.Fatal(err)
	}
	zh := ZhalobaNaDrayver()
	if zh == "" {
		t.Fatal("жалоба ядра на драйвер не запомнена: отказ подъёма снова неразличим")
	}
	if !strings.Contains(zh, "wintun") {
		t.Fatalf("запомнено %q, а это не та строка", zh)
	}
}

// Обратная сторона: жалоба на СОВСЕМ другое драйвером не считается. Иначе
// невставший драйвер стал бы ответом на любой отказ ядра, то есть догадкой,
// только дорогой.
func TestChuzhayaZhalobaYadraNeSchitaetsyaOtkazomDrayvera(t *testing.T) {
	perehvat(t)
	ZabytZhalobyYadra()
	z := &zhurnalYadra{imya: "sing-box.exe"}

	if _, err := z.Write([]byte("FATAL[0000] initialize outbound[24]: invalid public_key\n")); err != nil {
		t.Fatal(err)
	}
	if zh := ZhalobaNaDrayver(); zh != "" {
		t.Fatalf("чужая жалоба принята за отказ драйвера: %q", zh)
	}
}

// Жалоба ПРОШЛОГО подъёма не имеет права обвинять драйвер в этом. Забвение
// зовётся подъёмом перед стартом ядра, и без него второй отказ подряд по любой
// причине назывался бы отсутствием драйвера.
func TestZhalobaNaDrayverZabyvaetsyaPeredPodyomom(t *testing.T) {
	perehvat(t)
	ZabytZhalobyYadra()
	z := &zhurnalYadra{imya: "sing-box.exe"}
	if _, err := z.Write([]byte("FATAL[0000] create service: wintun failed\n")); err != nil {
		t.Fatal(err)
	}
	if ZhalobaNaDrayver() == "" {
		t.Fatal("жалоба не запомнена: забывать нечего, тест ничего не проверяет")
	}
	ZabytZhalobyYadra()
	if zh := ZhalobaNaDrayver(); zh != "" {
		t.Fatalf("жалоба прошлого подъёма пережила забвение: %q", zh)
	}
}
