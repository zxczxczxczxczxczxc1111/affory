package petlya

import (
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Сторож рабочей машины.
//
// Тесты этого пакета идут на ПК владельца, где живёт ЕГО личный sing-box и его
// интернет. Требование владельца от 07.09.2026 дословно: «можно на моем пк,
// только если это не будет влиять на мой действующий процесс sing-box и убивать
// мне интернет».
//
// Обещания недостаточно, потому что нарушить его можно случайно и не заметить:
// достаточно погасить процесс по ИМЕНИ вместо PID, и чужое ядро умрёт вместе с
// нашим. Такое уже случалось, и правило записано в памяти отдельной строкой.
// Поэтому здесь механическая проверка: снимок до, снимок после, расхождение это
// падение теста.
type Sostoyanie struct {
	// PID всех процессов sing-box на машине. Наши сюда тоже попадут, поэтому
	// снимок «после» снимается ПОСЛЕ гашения своих.
	PidyYadra []int
	// Куда уходит трафик по умолчанию. Меняется, если кто-то поднял TUN.
	MarshrutPoUmolchaniyu string
	// Системный прокси браузера. Пустой значит выключен.
	SistemnyyProksi string
	// Непустая, если снимок снять не удалось. Пустой снимок молча это не сторож.
	Oshibka string
}

// SnyatSostoyanie снимает то, что тесты обязаны оставить нетронутым.
func SnyatSostoyanie() Sostoyanie {
	var s Sostoyanie
	var bedy []string

	pidy, err := pidySingBox()
	if err != nil {
		bedy = append(bedy, "процессы: "+err.Error())
	}
	s.PidyYadra = pidy

	marshrut, err := marshrutPoUmolchaniyu()
	if err != nil {
		bedy = append(bedy, "маршрут: "+err.Error())
	}
	s.MarshrutPoUmolchaniyu = marshrut

	proksi, err := sistemnyyProksi()
	if err != nil {
		bedy = append(bedy, "прокси: "+err.Error())
	}
	s.SistemnyyProksi = proksi

	s.Oshibka = strings.Join(bedy, "; ")
	return s
}

// Sverit возвращает пустую строку, если машина не изменилась, и человеческое
// описание расхождения, если изменилась.
func Sverit(do, posle Sostoyanie) string {
	var bedy []string

	bylo, stalo := append([]int(nil), do.PidyYadra...), append([]int(nil), posle.PidyYadra...)
	sort.Ints(bylo)
	sort.Ints(stalo)
	if !ravnyChisla(bylo, stalo) {
		bedy = append(bedy, fmt.Sprintf("ядра sing-box на машине изменились: было %v, стало %v", bylo, stalo))
	}
	if do.MarshrutPoUmolchaniyu != posle.MarshrutPoUmolchaniyu {
		bedy = append(bedy, fmt.Sprintf("маршрут по умолчанию уехал: было %q, стало %q",
			do.MarshrutPoUmolchaniyu, posle.MarshrutPoUmolchaniyu))
	}
	if do.SistemnyyProksi != posle.SistemnyyProksi {
		bedy = append(bedy, fmt.Sprintf("системный прокси изменился: было %q, стало %q",
			do.SistemnyyProksi, posle.SistemnyyProksi))
	}
	return strings.Join(bedy, "; ")
}

// Storozhit ставится первой строкой теста, который поднимает ядра.
func Storozhit(t *testing.T) {
	t.Helper()
	do := SnyatSostoyanie()
	if do.Oshibka != "" {
		t.Fatalf("состояние машины не снято, охранять нечего: %s", do.Oshibka)
	}
	t.Cleanup(func() {
		posle := SnyatSostoyanie()
		if raznica := Sverit(do, posle); raznica != "" {
			t.Errorf("тест изменил рабочую машину, это запрещено: %s", raznica)
		}
	})
}

func ravnyChisla(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// tasklist, а не Get-Process: запуск PowerShell стоит полсекунды, а снимок
// берётся дважды на каждый тест.
func pidySingBox() ([]int, error) {
	vyvod, err := exec.Command("tasklist", "/FI", "IMAGENAME eq sing-box.exe", "/FO", "CSV", "/NH").Output()
	if err != nil {
		return nil, err
	}
	var pidy []int
	for _, s := range strings.Split(string(vyvod), "\n") {
		polya := strings.Split(s, "\",\"")
		if len(polya) < 2 {
			continue
		}
		pid, err := strconv.Atoi(strings.Trim(polya[1], "\" \r"))
		if err != nil {
			continue
		}
		pidy = append(pidy, pid)
	}
	return pidy, nil
}

// Шлюз маршрута по умолчанию. Поднятый TUN меняет именно его, и меняет молча.
func marshrutPoUmolchaniyu() (string, error) {
	vyvod, err := exec.Command("route", "print", "-4", "0.0.0.0").Output()
	if err != nil {
		return "", err
	}
	for _, s := range strings.Split(string(vyvod), "\n") {
		polya := strings.Fields(s)
		if len(polya) >= 3 && polya[0] == "0.0.0.0" && polya[1] == "0.0.0.0" {
			return polya[2], nil
		}
	}
	return "", fmt.Errorf("в выводе route нет строки 0.0.0.0")
}

func sistemnyyProksi() (string, error) {
	klyuch := `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`
	vkl, err := exec.Command("reg", "query", klyuch, "/v", "ProxyEnable").Output()
	if err != nil {
		return "", err
	}
	if !strings.Contains(string(vkl), "0x1") {
		return "", nil
	}
	adres, err := exec.Command("reg", "query", klyuch, "/v", "ProxyServer").Output()
	if err != nil {
		// Включён без адреса это странно, но не наша беда: отдаём признак.
		return "вкл", nil
	}
	polya := strings.Fields(string(adres))
	return polya[len(polya)-1], nil
}
