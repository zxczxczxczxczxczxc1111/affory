package main

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/set"
)

//go:embed ESLI-NET-INTERNETA.txt
var shablonAvariynogo string

const imyaAvariynogo = "ESLI-NET-INTERNETA.txt"

// polozhitAvariynyy кладёт лист рядом с программой и СОБИРАЕТ список правил из
// констант.
//
// Прежде файл клал только скрипт стенда, а настоящая установка нет: на реальной
// машине его не было ровно тогда, когда он нужен. И список правил в нём был
// написан руками, поэтому отставал: правило Affory-Allow-Dns-Tcp, заведённое
// 02.09.2026, в него не попало в тот же день. Человек без интернета набирает с
// листа то, что там написано, и одно недоснятое правило означает, что интернет
// не вернулся, а почему, не видно.
func polozhitAvariynyy(kuda string) error {
	imena := set.VseImenaPravil()

	var b strings.Builder
	b.WriteString("    for %r in (")
	b.WriteString(strings.Join(imena, " "))
	b.WriteString(") do netsh advfirewall firewall delete rule name=%r\n")
	b.WriteString("\nОдной строкой. Если ругается на \"не найдено правило\" это нормально:\n")
	b.WriteString("значит его и не было.\n")
	b.WriteString("\nЕсли строка слишком длинная, можно по одной:\n\n")
	for _, imya := range imena {
		fmt.Fprintf(&b, "    netsh advfirewall firewall delete rule name=%s\n", imya)
	}

	telo := strings.Replace(shablonAvariynogo, "{{ПРАВИЛА}}", b.String(), 1)
	if strings.Contains(telo, "{{") {
		return fmt.Errorf("в аварийном файле осталась неподставленная метка")
	}
	// BOM намеренно: файл открывают блокнотом, а блокнот без BOM читает
	// кириллицу в UTF-8 как мусор ровно у того человека, у которого нет
	// интернета и нечем это загуглить.
	const bom = "\ufeff"
	if !strings.HasPrefix(telo, bom) {
		telo = bom + telo
	}
	return os.WriteFile(filepath.Join(kuda, imyaAvariynogo), []byte(telo), 0o644)
}
