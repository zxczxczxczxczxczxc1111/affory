package main

import (
	"os"
	"sort"
	"strings"
	"testing"
)

// §5 п.1: процесс добавляется и из списка запущенных. Список берёт оболочка,
// а не служба: она в сеансе человека и видит его процессы, служба видит
// сеанс 0. Пути уже в том виде, который сравнивает ядро (QueryFullProcessImageName
// отдаёт Win32-путь), нормализацию на setRules служба всё равно повторит.
func TestSpisokProtsessovVidiySvoyProtsess(t *testing.T) {
	svoy, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	spisok, err := protsessySeansa()
	if err != nil {
		t.Fatal(err)
	}
	if len(spisok) == 0 {
		t.Fatal("список запущенных пуст: хотя бы этот тест запущен")
	}
	nashli := false
	puti := map[string]bool{}
	for _, p := range spisok {
		if p.Imya == "" || p.Put == "" {
			t.Fatalf("запись без имени или пути: %+v", p)
		}
		if !strings.EqualFold(p.Imya, strings.TrimPrefix(p.Put[strings.LastIndex(p.Put, `\`)+1:], "")) {
			t.Fatalf("имя %q не хвост пути %q", p.Imya, p.Put)
		}
		if puti[strings.ToLower(p.Put)] {
			t.Fatalf("путь %q в списке дважды: один exe с десятью окнами это одно правило", p.Put)
		}
		puti[strings.ToLower(p.Put)] = true
		if strings.EqualFold(p.Put, svoy) {
			nashli = true
		}
	}
	if !nashli {
		t.Fatalf("своего процесса %q в списке нет", svoy)
	}
	if !sort.SliceIsSorted(spisok, func(i, j int) bool {
		return strings.ToLower(spisok[i].Imya) < strings.ToLower(spisok[j].Imya)
	}) {
		t.Fatal("список не отсортирован по имени: человеку искать глазами")
	}
}
