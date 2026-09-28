package yadra

import (
	"strings"
	"testing"
)

// convert печатает строку на каждое непонятое правило: вывод без предела съел
// бы память службы, а писатель, который отказывает, оборвал бы процесс.
func TestOgranichennyyVyvodNeRastyotINeOtkazyvaet(t *testing.T) {
	o := &ogranichennyy{predel: 10}
	for i := 0; i < 5; i++ {
		n, err := o.Write([]byte("0123456"))
		if n != 7 || err != nil {
			t.Fatalf("запись %d: %d, %v", i, n, err)
		}
	}
	if o.String() != "0123456012" {
		t.Fatalf("вывод %q", o.String())
	}
	if strings.Count(o.String(), "0") != 2 {
		t.Fatalf("предел не держится: %q", o.String())
	}
}
