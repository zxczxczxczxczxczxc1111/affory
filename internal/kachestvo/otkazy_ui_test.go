package kachestvo_test

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"testing"
)

// The refusal screens live in TypeScript, the truth about each code lives in
// spec §9.1, and the two cannot import each other. So the same Go gate that
// already parses §9.1 (deystviya_test.go) also reads otkazy.ts and compares
// code -> action both ways. Text is the frontend test's business; the ACTION
// is a contract with the spec, and a wrong action ships a button that does
// the wrong thing with a perfectly readable label.
const putOtkazovTS = "../../cmd/affory-ui/frontend/src/ekrany/otkazy.ts"

// One entry per line, `deystvie` first. The order is a convention the regexp
// depends on; if it drifts, the "no entries" guard below turns red instead
// of the gate going silently green.
var reZapisTS = regexp.MustCompile(`(?m)^\s*"([a-z0-9-]+)":\s*\{\s*deystvie:\s*"([a-z0-9-]+)"`)

func TestDeystviyaEkranovSovpadayutSoSpekoy(t *testing.T) {
	syroe, err := os.ReadFile(filepath.FromSlash(putSpeki))
	if err != nil {
		t.Skipf("спека недоступна (%v): сверять не с чем", err)
	}
	spec, est := razdel91(string(syroe))
	if !est {
		t.Fatal("в спеке не найден раздел 9.1")
	}
	vSpeke := map[string]string{}
	for _, m := range reDeystvie.FindAllStringSubmatch(spec, -1) {
		vSpeke[m[1]] = m[2]
	}
	if len(vSpeke) == 0 {
		t.Fatal("в §9.1 не нашлось ни одной строки с действием: разбор сломан")
	}

	ts, err := os.ReadFile(filepath.FromSlash(putOtkazovTS))
	if err != nil {
		t.Fatalf("otkazy.ts не прочитан: %v", err)
	}
	vTS := map[string]string{}
	for _, m := range reZapisTS.FindAllStringSubmatch(string(ts), -1) {
		vTS[m[1]] = m[2]
	}
	if len(vTS) == 0 {
		t.Fatal("в otkazy.ts не нашлось ни одной записи: формат уехал от регулярки, а не экраны исчезли")
	}

	var oshibki []string
	for kod, d := range vSpeke {
		if _, mozhno := kodyBezStroki[kod]; mozhno {
			continue
		}
		switch vTS[kod] {
		case "":
			oshibki = append(oshibki, kod+": есть в §9.1, нет экрана в otkazy.ts")
		case d:
		default:
			oshibki = append(oshibki, kod+": спека велит "+d+", экран делает "+vTS[kod])
		}
	}
	for kod := range vTS {
		if _, est := vSpeke[kod]; !est {
			oshibki = append(oshibki, kod+": есть экран, нет строки в §9.1")
		}
	}
	sort.Strings(oshibki)
	for _, o := range oshibki {
		t.Error(o)
	}
	t.Logf("кодов в §9.1 %d, записей в otkazy.ts %d", len(vSpeke), len(vTS))
}
