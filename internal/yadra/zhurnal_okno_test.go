package yadra

import (
	"strings"
	"testing"
	"time"
)

// Разбор журнала живой машины 10.09.2026. В `yadro.log` пять раз стоит
// «строк больше 200, дальше молчим», и после каждой такой строки ядро молчит
// до самого перезапуска. Ядро при этом работает сутками.
//
// Цена выяснилась на этом же разборе: шторм отказов сокетов у ядра (DNS не
// резолвится, соединения не открываются) виден только до двухсотой строки.
// Сколько их было на самом деле, узнать неоткуда. Диагностика пропадает ровно
// тогда, когда становится нужна, потому что порог считался за ВЕСЬ запуск.
//
// Предел остаётся, но становится пределом на окно: залить файл потоком ядра
// по-прежнему нельзя, а через минуту журнал снова слышит.
func TestPredelStrokOtpuskaetPosleOkna(t *testing.T) {
	b := perehvat(t)
	z := &zhurnalYadra{imya: "sing-box.exe"}
	teper := time.Now()
	z.seychas = func() time.Time { return teper }

	for i := 0; i < predelStrokYadra+50; i++ {
		if _, err := z.Write([]byte("pervaya volna\n")); err != nil {
			t.Fatal(err)
		}
	}
	if !strings.Contains(b.String(), "дальше молчим") {
		t.Fatal("обрезка не объявлена: журнал будет выглядеть полным")
	}

	// Окно прошло. Ядро говорит снова, и его обязаны услышать.
	teper = teper.Add(oknoStrokYadra + time.Second)
	if _, err := z.Write([]byte("vtoraya volna\n")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "vtoraya volna") {
		t.Fatal("после окна журнал так и молчит: жалобы ядра за все следующие часы" +
			" потеряны, а именно они нужны при разборе")
	}
}

// Обратная половина: внутри окна предел обязан держать. Без неё «починка»
// свелась бы к снятию ограничения, а файл на диске снова заливался бы потоком.
func TestPredelStrokDerzhitVnutriOkna(t *testing.T) {
	b := perehvat(t)
	z := &zhurnalYadra{imya: "sing-box.exe"}
	teper := time.Now()
	z.seychas = func() time.Time { return teper }

	for i := 0; i < predelStrokYadra+500; i++ {
		if _, err := z.Write([]byte("shtorm\n")); err != nil {
			t.Fatal(err)
		}
		// Время идёт, но окна не хватает.
		teper = teper.Add(time.Millisecond)
	}
	if n := strings.Count(b.String(), "shtorm"); n > predelStrokYadra {
		t.Fatalf("внутри окна записано %d строк при пределе %d", n, predelStrokYadra)
	}
}

// Сколько строк проглочено, обязано быть сказано числом. Иначе по журналу
// нельзя отличить «ядро замолчало» от «мы перестали слушать».
func TestPosleOknaSkazanoSkolkoProglotili(t *testing.T) {
	b := perehvat(t)
	z := &zhurnalYadra{imya: "sing-box.exe"}
	teper := time.Now()
	z.seychas = func() time.Time { return teper }

	for i := 0; i < predelStrokYadra+37; i++ {
		if _, err := z.Write([]byte("shum\n")); err != nil {
			t.Fatal(err)
		}
	}
	teper = teper.Add(oknoStrokYadra + time.Second)
	if _, err := z.Write([]byte("posle\n")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "37") {
		t.Fatalf("число проглоченных строк не названо: %q", hvost(b.String()))
	}
}

func hvost(s string) string {
	if len(s) <= 300 {
		return s
	}
	return "..." + s[len(s)-300:]
}
