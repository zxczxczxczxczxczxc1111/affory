package sostoyanie

import (
	"os"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

func TestZapisAtomarna(t *testing.T) {
	// Half a file after a crash is worse than no file: the next start would
	// read a truncated JSON and decide the previous run never happened.
	dir := t.TempDir()
	for i := 0; i < 50; i++ {
		if err := zapisatV(dir, SostoyanieFayla{Sostoyanie: protokol.SostPodnyat}); err != nil {
			t.Fatalf("запись %d: %v", i, err)
		}
		if _, err := prochitatIz(dir); err != nil {
			t.Fatalf("чтение после записи %d: %v", i, err)
		}
	}
	// No leftovers: a temp file that survives is a temp file that will be read
	// by somebody eventually.
	fayly, _ := os.ReadDir(dir)
	if len(fayly) != 1 {
		t.Fatalf("в каталоге %d файлов, ожидался один", len(fayly))
	}
}

func TestOtmetkiPerezhivayutKrug(t *testing.T) {
	// The timestamps ARE the instrument: four of the twelve thresholds are read
	// off them. A field that quietly fails to round-trip turns every later
	// measurement into a number nobody can defend.
	dir := t.TempDir()
	nach := timeSeychas()
	if err := zapisatV(dir, SostoyanieFayla{
		Sostoyanie:  protokol.SostPodnyat,
		ConnectNach: &nach, ProbaPervaya: &nach,
		SelectorNach: &nach, SelectorGotov: &nach,
		SetSmena: &nach, SetGotov: &nach,
		SonVyhod: &nach, SonGotov: &nach,
	}); err != nil {
		t.Fatalf("запись: %v", err)
	}
	s, err := prochitatIz(dir)
	if err != nil {
		t.Fatalf("чтение: %v", err)
	}
	for imya, v := range map[string]*string{
		"connect_nachalo":    ptrStr(s.ConnectNach),
		"proba_pervyy_otvet": ptrStr(s.ProbaPervaya),
		"selector_nachalo":   ptrStr(s.SelectorNach),
		"selector_gotov":     ptrStr(s.SelectorGotov),
		"set_smenilas":       ptrStr(s.SetSmena),
		"set_vosstanovlena":  ptrStr(s.SetGotov),
		"son_vyhod":          ptrStr(s.SonVyhod),
		"son_vosstanovleno":  ptrStr(s.SonGotov),
	} {
		if v == nil {
			t.Errorf("отметка %s потерялась", imya)
		}
	}
	// Milliseconds, not seconds: three of the four thresholds are single digit
	// seconds, and a truncating format would flatten the difference we measure.
	if s.ConnectNach != nil && s.ConnectNach.UnixNano() == s.ConnectNach.Unix()*1e9 && nach.Nanosecond() != 0 {
		t.Error("доли секунды срезаны при записи")
	}
}
