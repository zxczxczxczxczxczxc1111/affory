//go:build windows

package diagnostika

import "testing"

// netsh отвечает на языке системы, поэтому разбор держится за числа, а не за
// слова. Оба образца сняты с настоящих машин.

func TestDiapazonRazbiraetsyaSAngliyskogoVyvoda(t *testing.T) {
	vyvod := "\r\nProtocol tcp Dynamic Port Range\r\n" +
		"---------------------------------\r\n" +
		"Start Port      : 49152\r\n" +
		"Number of Ports : 16384\r\n\r\n"
	n, v, ok := razobratDiapazon(vyvod)
	if !ok || n != 49152 || v != 16384 {
		t.Fatalf("разобрано %d/%d, ok=%v", n, v, ok)
	}
}

func TestDiapazonRazbiraetsyaSRusskogoVyvoda(t *testing.T) {
	vyvod := "\r\nДиапазон динамических портов протокола tcp\r\n" +
		"---------------------------------\r\n" +
		"Начальный порт      : 61000\r\n" +
		"Число портов        : 255\r\n\r\n"
	n, v, ok := razobratDiapazon(vyvod)
	if !ok || n != 61000 || v != 255 {
		t.Fatalf("разобрано %d/%d, ok=%v", n, v, ok)
	}
}

// Чужой формат обязан отдавать «не разобрал», а не выдуманные числа: молча
// подставленный диапазон это молча неверный счётчик занятости.
func TestChuzhoyVyvodNeRazbiraetsya(t *testing.T) {
	for _, vyvod := range []string{
		"",
		"The following command was not found: int ipv4 show dynamicport tcp.",
		"Start Port : 49152\r\nNumber of Ports : 16384\r\nЛишнее число : 7\r\n",
	} {
		if _, _, ok := razobratDiapazon(vyvod); ok {
			t.Fatalf("чужой вывод разобран как годный: %q", vyvod)
		}
	}
}

// Живые источники: числа обязаны быть осмысленными на любой машине, где идёт
// набор. Это чтение, ничего не меняется.
func TestZhivyeIstochnikiOtvechayutChislami(t *testing.T) {
	n, err := DeskriptorovProtsessa(0)
	if err != nil {
		t.Fatalf("свои дескрипторы: %v", err)
	}
	if n <= 0 {
		t.Fatalf("дескрипторов %d, у живого процесса их не может быть столько", n)
	}
	zanyato, vsego, err := PortyEfemernye()
	if err != nil {
		t.Fatalf("порты: %v", err)
	}
	if vsego <= 0 || zanyato < 0 || zanyato > vsego {
		t.Fatalf("занято %d из %d, это не диапазон", zanyato, vsego)
	}
	gorutin, pamyat := Runtime()
	if gorutin <= 0 || pamyat == 0 {
		t.Fatalf("runtime отдал %d горутин и %d байт", gorutin, pamyat)
	}
}

// Несуществующий процесс обязан отдать ошибку, а не ноль: ноль дескрипторов
// читается как «течи нет», то есть ровно как здоровье.
func TestMyortvyyProtsessOtdayotOshibku(t *testing.T) {
	if _, err := DeskriptorovProtsessa(0x7FFFFFF0); err == nil {
		t.Fatalf("несуществующий процесс отдал число вместо ошибки")
	}
}
