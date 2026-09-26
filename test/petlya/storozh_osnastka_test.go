package petlya

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Сторож рабочей машины.
//
// Тесты этого пакета идут на рабочем ПК, где живёт личный sing-box и живой
// интернет. Требование от 07.09.2026 дословно: «можно на моем пк,
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

// processMashiny это строка снимка процессов: кто он и чей ребёнок.
type processMashiny struct {
	Pid, Roditel int
	Imya         string // имя образа в нижнем регистре
}

// pidySingBox отдаёт PID ядер, за которыми сторож обязан следить.
//
// До 26.09.2026 здесь стоял список ВСЕХ sing-box.exe через tasklist, и ворота
// падали без вины продукта. `go test ./...` гоняет пакеты одновременно, а
// genkonfig, ssylki и yadra зовут `sing-box check`: короткая проверка соседа
// рождалась и умирала посреди теста петли, и сторож объявлял, что тест тронул
// чужое ядро. Снимок системы, а не tasklist, потому что только в нём виден
// родитель процесса.
func pidySingBox() ([]int, error) {
	vse, err := processyMashiny()
	if err != nil {
		return nil, err
	}
	return otobratYadra(vse, os.Getpid()), nil
}

func processyMashiny() ([]processMashiny, error) {
	snimok, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, fmt.Errorf("снимок процессов: %w", err)
	}
	defer windows.CloseHandle(snimok)

	var vse []processMashiny
	var z windows.ProcessEntry32
	z.Size = uint32(unsafe.Sizeof(z))
	for err = windows.Process32First(snimok, &z); err == nil; err = windows.Process32Next(snimok, &z) {
		vse = append(vse, processMashiny{
			Pid:     int(z.ProcessID),
			Roditel: int(z.ParentProcessID),
			Imya:    strings.ToLower(windows.UTF16ToString(z.ExeFile[:])),
		})
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return nil, fmt.Errorf("обход процессов: %w", err)
	}
	return vse, nil
}

// otobratYadra оставляет ядра, которые тест обязан не тронуть или убрать за
// собой, и выкидывает проверки соседних тестовых пакетов.
//
// Сосед узнаётся по родителю: ядро запустил ДРУГОЙ тестовый бинарник
// (`*.test.exe`, но не этот процесс). Свои дети остаются в списке, иначе сторож
// перестанет видеть ядро, забытое самим тестом. Ядро, запущенное службой или
// оставшееся без живого родителя, тоже остаётся: это ровно то, что сторож ищет.
func otobratYadra(vse []processMashiny, svoy int) []int {
	imena := make(map[int]string, len(vse))
	for _, p := range vse {
		imena[p.Pid] = p.Imya
	}
	var pidy []int
	for _, p := range vse {
		if p.Imya != "sing-box.exe" {
			continue
		}
		roditel, zhiv := imena[p.Roditel]
		if zhiv && p.Roditel != svoy && strings.HasSuffix(roditel, ".test.exe") {
			continue
		}
		pidy = append(pidy, p.Pid)
	}
	return pidy
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
