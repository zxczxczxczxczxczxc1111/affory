// Утилита живой сверки: печатает то, что пакет set думает о системе, чтобы это
// можно было сравнить с выводом Get-NetIPInterface и Get-DnsClientServerAddress.
//
// Prints the whole table, not just the winner. A tool that shows only its own
// conclusion cannot be used to check that conclusion, and this one exists for
// exactly that.
package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/set"
)

func main() {
	spisok, err := set.Adaptery()
	if err != nil {
		fmt.Fprintln(os.Stderr, "не удалось перечислить адаптеры:", err)
		os.Exit(1)
	}

	t := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(t, "индекс\tимя\tсост\tметрика\tшлюз\tрезолвер\tописание")
	for _, a := range spisok {
		fmt.Fprintf(t, "%d\t%s\t%d\t%d\t%s\t%s\t%s\n",
			a.Indeks, a.Imya, a.Sostoyanie, a.Metrika,
			pervyy(a.Shlyuzy), pervyy(a.Resolvery), a.Opisanie)
	}
	t.Flush()

	fmt.Println()
	imya, err := set.AktivnyyAdapter()
	pechat("активный адаптер", imya, err)

	g, err := set.Shlyuz()
	pechat("шлюз", g.String(), err)

	r, err := set.LokalnyyResolver()
	pechat("локальный резолвер", r.String(), err)
}

func pervyy[T fmt.Stringer](s []T) string {
	if len(s) == 0 {
		return "-"
	}
	return s[0].String()
}

func pechat(imya, znachenie string, err error) {
	if err != nil {
		fmt.Printf("%-20s ОШИБКА: %v\n", imya+":", err)
		return
	}
	fmt.Printf("%-20s %s\n", imya+":", znachenie)
}
