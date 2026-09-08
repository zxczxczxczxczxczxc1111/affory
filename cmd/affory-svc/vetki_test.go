package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strconv"
	"testing"
)

// vetkiDispetchera reads the dispatcher source instead of a hand-kept list.
// A hand-kept list is exactly what let setAutostart and setConnectOnStart slip
// past every gate: the gate and the code were two copies of the same mistake.
func vetkiDispetchera(t *testing.T) []string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), "dispetcher.go", nil, 0)
	if err != nil {
		t.Fatalf("dispetcher.go не разобрался: %v", err)
	}
	// Obrabotat is only the wrapper that logs the command; the switch itself
	// sits in the unexported obrabotat. Both names are collected so a rename of
	// either half does not silently empty this gate: the anchors below decide
	// whether the switch was really found.
	var vnutri []*ast.FuncDecl
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if ok && (fn.Name.Name == "Obrabotat" || fn.Name.Name == "obrabotat") {
			vnutri = append(vnutri, fn)
		}
	}
	if len(vnutri) == 0 {
		t.Fatal("в dispetcher.go нет функции Obrabotat: разбор смотрит не туда")
	}
	var imena []string
	sobrat := func(n ast.Node) bool {
		c, ok := n.(*ast.CaseClause)
		if !ok {
			return true
		}
		for _, e := range c.List {
			lit, ok := e.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				continue
			}
			s, err := strconv.Unquote(lit.Value)
			if err != nil {
				t.Fatalf("литерал ветки не разобрался: %v", err)
			}
			imena = append(imena, s)
		}
		return true
	}
	for _, fn := range vnutri {
		ast.Inspect(fn, sobrat)
	}
	// Anchors: if the parse silently starts looking at the wrong node these
	// three disappear and the whole gate turns into a green nothing.
	for _, yakor := range []string{"hello", "status", "connect"} {
		var est bool
		for _, i := range imena {
			if i == yakor {
				est = true
			}
		}
		if !est {
			t.Fatalf("среди веток нет %q: разбор сломан, а не диспетчер", yakor)
		}
	}
	sort.Strings(imena)
	return imena
}

func TestImenaKomandSovpadayutSVetkami(t *testing.T) {
	vetki := vetkiDispetchera(t)
	objavleno := map[string]bool{}
	for _, i := range imenaKomand() {
		objavleno[i] = true
	}
	vDispetchere := map[string]bool{}
	for _, v := range vetki {
		vDispetchere[v] = true
		if !objavleno[v] {
			t.Errorf("ветка %q есть в диспетчере, но её нет в imenaKomand(): ворота её не видят", v)
		}
	}
	for i := range objavleno {
		if !vDispetchere[i] {
			t.Errorf("имя %q объявлено в imenaKomand(), а ветки в диспетчере нет", i)
		}
	}
}
