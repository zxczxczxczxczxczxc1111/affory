package main

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Список запущенных процессов для формы правил (§5 п.1: «процесс добавляется
// как из списка запущенных, так и путём к файлу»).
//
// The shell, not the service, builds this list: the shell runs in the
// person's session and sees their processes, the service sits in session 0
// and sees svchost. Paths come from QueryFullProcessImageName, which is the
// Win32 form the core compares against; the service still normalizes on
// setRules, so a stale or odd spelling here costs nothing.

// Protsess это одна строка списка: имя для глаза, путь для правила.
type Protsess struct {
	Imya string `json:"imya"`
	Put  string `json:"put"`
}

// SpisokProtsessov отдаёт запущенные процессы сеанса, по одному на путь.
func (m *most) SpisokProtsessov() ([]Protsess, error) {
	return protsessySeansa()
}

func protsessySeansa() ([]Protsess, error) {
	snimok, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, fmt.Errorf("снимок процессов не получен: %w", err)
	}
	defer windows.CloseHandle(snimok)

	var zapis windows.ProcessEntry32
	// dwSize is a contract, not a hint: Process32First refuses a zero.
	zapis.Size = uint32(unsafe.Sizeof(zapis))
	vidno := map[string]bool{}
	var itog []Protsess
	for err = windows.Process32First(snimok, &zapis); err == nil; err = windows.Process32Next(snimok, &zapis) {
		put, ok := putProtsessa(zapis.ProcessID)
		if !ok {
			// System, protected and other-session processes refuse the
			// handle. They are not the person's programs and not rules.
			continue
		}
		klyuch := strings.ToLower(put)
		if vidno[klyuch] {
			// Ten Chrome windows are one exe and one rule.
			continue
		}
		vidno[klyuch] = true
		itog = append(itog, Protsess{Imya: put[strings.LastIndex(put, `\`)+1:], Put: put})
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return nil, fmt.Errorf("обход процессов оборвался: %w", err)
	}
	sort.Slice(itog, func(i, j int) bool {
		return strings.ToLower(itog[i].Imya) < strings.ToLower(itog[j].Imya)
	})
	return itog, nil
}

// putProtsessa спрашивает полный путь у процесса; отказ в доступе это «не
// наш», а не ошибка списка.
func putProtsessa(pid uint32) (string, bool) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "", false
	}
	defer windows.CloseHandle(h)
	bufer := make([]uint16, windows.MAX_LONG_PATH)
	dlina := uint32(len(bufer))
	if err := windows.QueryFullProcessImageName(h, 0, &bufer[0], &dlina); err != nil || dlina == 0 {
		return "", false
	}
	return windows.UTF16ToString(bufer[:dlina]), true
}
