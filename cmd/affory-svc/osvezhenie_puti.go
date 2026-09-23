package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// Путь программы, в котором стоит номер версии (23.09.2026).
//
// Правило хранит ПОЛНЫЙ путь к exe, и это правильно: имя ловит любой
// Discord.exe на машине, включая чужой. Но у части программ версия стоит прямо
// в пути, и обновление самой программы делает путь несуществующим:
//
//	%LOCALAPPDATA%\Discord\app-1.0.9258\Discord.exe      -> app-1.0.9259
//	C:\Program Files\WindowsApps\Claude_2.2553.1.0_x64__…\app\Claude.exe
//	C:\Program Files\WindowsApps\OpenAI.Codex_26.917.…\app\ChatGPT.exe
//
// Проверено 23.09.2026 на живых машинах: у Discord номер сборки в каталоге, у
// Claude и ChatGPT версия пакета MSIX. Discord обновляется сам раз в неделю-две.
//
// До этой правки путь обновлялся ровно в одном случае: человек снова менял
// маршрут карточки при ЗАПУЩЕННОЙ программе (trafik.ts:70). То есть после
// автообновления правило молча переставало совпадать, а карточка на экране
// выглядела настроенной - трафик шёл мимо, и причину было не увидеть.
//
// Здесь путь чинится при чтении набора, рядом со снятием сирот и чисткой. Не
// молча: каждая замена уезжает в журнал, как и они.

// osvezhitPut ищет тот же файл в соседнем каталоге версии.
//
// Ищется ТОЛЬКО замена сегмента-версии и только среди каталогов с тем же
// префиксом: `app-1.0.9258` -> `app-*`, `Claude_2.2553.1.0_x64__…` ->
// `Claude_*`. Хвост пути после этого сегмента сохраняется целиком, поэтому
// `…\app\Claude.exe` остаётся `…\app\Claude.exe`.
//
// Шире искать нельзя. Прародитель у MSIX это весь WindowsApps, и поиск по имени
// файла нашёл бы там чужую программу, которой правило человека не касалось.
func osvezhitPut(put string) (string, bool) {
	if put == "" {
		return "", false
	}
	if _, err := os.Stat(put); err == nil {
		return "", false // файл на месте, чинить нечего
	}
	ded, versiya, hvost, ok := razlozhitPut(put)
	if !ok {
		return "", false
	}
	prefiks := prefiksDoVersii(versiya)
	if prefiks == "" {
		return "", false
	}
	zapisi, err := os.ReadDir(ded)
	if err != nil {
		return "", false
	}
	var luchshiy string
	var luchshee int64
	for _, z := range zapisi {
		if !z.IsDir() || z.Name() == versiya || !strings.HasPrefix(z.Name(), prefiks) {
			continue
		}
		kandidat := filepath.Join(ded, z.Name(), hvost)
		st, err := os.Stat(kandidat)
		if err != nil || st.IsDir() {
			continue
		}
		// Самый свежий: рядом со старой версией лежит не только новая. Discord
		// оставляет прежний каталог до перезапуска, и взять первый попавшийся
		// значило бы починить путь на ту же мёртвую сборку.
		if m := st.ModTime().UnixNano(); m > luchshee {
			luchshiy, luchshee = kandidat, m
		}
	}
	if luchshiy == "" {
		return "", false
	}
	return luchshiy, true
}

// razlozhitPut делит путь на прародителя, сегмент-версию и хвост.
//
// Сегментом-версией считается ПОСЛЕДНИЙ каталог пути, в имени которого есть
// цифра: у Discord это `app-1.0.9258`, у MSIX - `Claude_2.2553.1.0_x64__…`, а
// `app` после него цифр не содержит и остаётся хвостом.
func razlozhitPut(put string) (ded, versiya, hvost string, ok bool) {
	dir, fayl := filepath.Split(put)
	chasti := strings.Split(strings.TrimRight(dir, `\/`), string(filepath.Separator))
	for i := len(chasti) - 1; i >= 1; i-- {
		if !estCifra(chasti[i]) {
			continue
		}
		ded = strings.Join(chasti[:i], string(filepath.Separator))
		versiya = chasti[i]
		hvost = filepath.Join(append(append([]string{}, chasti[i+1:]...), fayl)...)
		// Корень диска без разделителя (`C:`) каталогом не является.
		if len(ded) == 2 && strings.HasSuffix(ded, ":") {
			ded += string(filepath.Separator)
		}
		return ded, versiya, hvost, ded != ""
	}
	return "", "", "", false
}

func estCifra(s string) bool {
	return strings.ContainsFunc(s, unicode.IsDigit)
}

// prefiksDoVersii отрезает имя каталога по первой цифре: `app-1.0.9258` даёт
// `app-`, `Claude_2.2553.1.0_x64__hash` даёт `Claude_`.
//
// Пустой префикс значит, что имя начинается с цифры и опознать по нему семью
// каталогов нечем. Такой путь не чинится: подставить туда что угодно из соседей
// значит увести правило человека на чужую программу.
func prefiksDoVersii(imya string) string {
	for i, r := range imya {
		if unicode.IsDigit(r) {
			return imya[:i]
		}
	}
	return ""
}

// osvezhitPutiProgramm чинит пути правил приложений и программ сервисов.
//
// Возвращает отчёт, по строке на замену. Пустой отчёт значит «менять было
// нечего»: набор при этом не трогается вовсе.
func (n *Nabor) osvezhitPutiProgramm() []string {
	if n.Pravila.Trafik == nil {
		return nil
	}
	var otchyot []string
	for i, a := range n.Pravila.Trafik.Prilozheniya {
		novyy, est := osvezhitPut(a.Put)
		if !est {
			continue
		}
		n.Pravila.Trafik.Prilozheniya[i].Put = novyy
		otchyot = append(otchyot, fmt.Sprintf(
			"путь правила %s обновлён после обновления программы: %s -> %s",
			filepath.Base(a.Put), a.Put, novyy))
	}
	for si, s := range n.Pravila.Trafik.Servisy {
		for pi, put := range s.Programmy {
			novyy, est := osvezhitPut(put)
			if !est {
				continue
			}
			n.Pravila.Trafik.Servisy[si].Programmy[pi] = novyy
			otchyot = append(otchyot, fmt.Sprintf(
				"путь программы сервиса %q обновлён после обновления программы: %s -> %s",
				s.Id, put, novyy))
		}
	}
	return otchyot
}
