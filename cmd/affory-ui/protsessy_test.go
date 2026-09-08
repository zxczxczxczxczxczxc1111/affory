package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestSpisokProtsessovAppData(t *testing.T) {
	if os.Getenv("AFFORY_PROCESS_LIST_CHILD") == "1" {
		time.Sleep(30 * time.Second)
		return
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	for _, variable := range []string{"LOCALAPPDATA", "APPDATA"} {
		t.Run(variable, func(t *testing.T) {
			root := os.Getenv(variable)
			if root == "" {
				t.Fatalf("%s is missing", variable)
			}
			dir, err := os.MkdirTemp(root, "Affory process test ")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(dir)
			path := filepath.Join(dir, "test application.exe")
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			child := exec.Command(path, "-test.run=^TestSpisokProtsessovAppData$")
			child.Env = append(os.Environ(), "AFFORY_PROCESS_LIST_CHILD=1")
			if err := child.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() { _ = child.Process.Kill(); _ = child.Wait() }()
			actual, readable := putProtsessa(uint32(child.Process.Pid))
			if !readable {
				t.Fatal("cannot read child image path")
			}
			// Packaged shells virtualize AppData; Windows enjoys moving the cheese.
			wanted, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			resolved, err := os.Stat(actual)
			if err != nil {
				t.Fatal(err)
			}
			if !os.SameFile(wanted, resolved) {
				t.Fatal("image path resolves to another file")
			}
			list, err := protsessySeansa()
			if err != nil {
				t.Fatal(err)
			}
			for _, process := range list {
				if strings.EqualFold(process.Put, actual) {
					return
				}
			}
			t.Fatalf("running application from %s was omitted", variable)
		})
	}
}

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
