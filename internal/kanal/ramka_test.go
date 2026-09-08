package kanal

import (
	"bytes"
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

func TestRamkaKruglyyReys(t *testing.T) {
	var b bytes.Buffer
	ishod := protokol.Kadr{Tip: "cmd", Id: 1, Imya: "status"}
	if err := PisatKadr(&b, ishod); err != nil {
		t.Fatalf("не пишется: %v", err)
	}
	// Four bytes of length, then the JSON. If this ever becomes five, both halves
	// break at once, which is why the number lives in a test and not in a doc.
	if b.Len() != 4+len(`{"tip":"cmd","id":1,"imya":"status"}`) {
		t.Fatalf("длина кадра не та: %d", b.Len())
	}
	nazad, err := ChitatKadr(&b)
	if err != nil {
		t.Fatalf("не читается: %v", err)
	}
	if nazad.Imya != "status" {
		t.Fatalf("кадр не тот: %+v", nazad)
	}
}

func TestRamkaOtvergaetGigant(t *testing.T) {
	// A length prefix is an allocation instruction from a stranger. Cap it.
	b := bytes.NewBuffer([]byte{0xFF, 0xFF, 0xFF, 0xFF})
	if _, err := ChitatKadr(b); err == nil {
		t.Fatal("кадр на 4 ГБ принят, а не должен")
	}
}

func TestZanyatoeImyaKanalaOtvergaetsya(t *testing.T) {
	// Squatting our own pipe on purpose: the first listener wins, the second must
	// fail, and the failure must carry pipe-squatted rather than a raw errno that
	// nobody upstream knows how to display.
	// Own name, own descriptor. The production one sets the owner to SYSTEM, which
	// only SYSTEM may do; borrowing it here would turn this into a permanent skip.
	const imya = `\\.\pipe\affory-test-zanyato`
	const sd = "D:P(A;;GA;;;SY)(A;;GA;;;BA)"

	pervyy, err := slushatImenem(imya, sd)
	if err != nil {
		t.Skipf("канал недоступен в этом окружении: %v", err)
	}
	defer pervyy.Close()

	vtoroy, err := slushatImenem(imya, sd)
	if err == nil {
		vtoroy.Close()
		t.Fatal("второй листенер поднялся на занятом имени")
	}
	if !strings.Contains(err.Error(), protokol.KodPipeSquatted) {
		t.Fatalf("отказ не тем кодом: %v", err)
	}
}
