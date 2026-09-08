package yadra

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/hranenie"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/sostoyanie"
)

// Проверка собранного конфига ЯДРОМ, до запуска.
//
// Заведена 02.09.2026 по находке 43, и это второй заход на один и тот же класс.
// Находка 33 (01.09) звучала так же: один негодный сервер в списке не даёт
// подняться туннелю вообще. Тогда починили ЧАСТНЫЙ случай, проверку ключа при
// разборе ссылки, и в записи о ней честно сказано, чего не сделали: «ядро не
// приняло то, что собрал генератор» не проверялось никогда. Через сутки чужая
// подписка принесла второй случай того же класса.
//
// Почему судья именно ядро, а не наша проверка. Конфиг собираем мы, а принимает
// его ядро, и полный список того, что оно отвергнет, знает только оно. Своя
// проверка повторила бы ровно ту ошибку: закрыла бы известные случаи и оставила
// класс.
//
// Цена на удачном пути это один запуск процесса на подъём туннеля. Подъём и так
// идёт секунды, а альтернатива это мёртвый туннель с диагнозом «адаптер не
// появился», по которому человек идёт искать причину в драйвере.
type OshibkaKonfiga struct {
	// Вывод ядра целиком, без цветовых последовательностей.
	Vyhod string
	// Номер исходящего, который ядро не приняло. Есть НЕ всегда: конфиг может
	// не разобраться целиком, и тогда исключать нечего.
	Nomer    int
	EstNomer bool
}

func (o *OshibkaKonfiga) Error() string {
	if o.EstNomer {
		return fmt.Sprintf("ядро не приняло конфиг, исходящий %d: %s", o.Nomer, o.Vyhod)
	}
	return "ядро не приняло конфиг: " + o.Vyhod
}

// Proverit спрашивает ядро, годен ли конфиг. Имя ядра, а не путь: сверка
// отпечатка обязана идти по тому же пути, по которому ядро потом запустится,
// иначе проверили бы один файл, а запустили другой.
func Proverit(imya string, putKonfiga string) error {
	put := filepath.Join(sostoyanie.KatalogProgrammy(), imya)
	if err := hranenie.Sverit(put); err != nil {
		return fmt.Errorf("конфиг не проверен, ядро %s не сошлось с отпечатком: %w", imya, err)
	}
	return proveritKonfig(put, putKonfiga)
}

func proveritKonfig(putYadra string, putKonfiga string) error {
	cmd := exec.Command(putYadra, "check", "-c", putKonfiga)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	vyhod, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	chistyy := bezTsveta(strings.TrimSpace(string(vyhod)))
	if chistyy == "" {
		chistyy = err.Error()
	}
	o := &OshibkaKonfiga{Vyhod: chistyy}
	o.Nomer, o.EstNomer = nomerIshodyashchego(chistyy)
	return o
}

// Ядро печатает номер исходящего в квадратных скобках. Это единственная
// зацепка, по которой отказ можно свести к КОНКРЕТНОМУ серверу.
var reIshodyashchiy = regexp.MustCompile(`initialize outbound\[(\d+)\]`)

func nomerIshodyashchego(vyhod string) (int, bool) {
	m := reIshodyashchiy.FindStringSubmatch(vyhod)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return n, true
}

// Ядро красит вывод всегда, даже когда его никто не читает глазами. В журнале
// службы эти последовательности превращаются в мусор, который потом мешает
// искать по тексту.
var reTsvet = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func bezTsveta(s string) string { return reTsvet.ReplaceAllString(s, "") }
