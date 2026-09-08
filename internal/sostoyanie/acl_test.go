package sostoyanie

import (
	"os"
	"sort"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestKatalogDannyhBezNasledovaniya(t *testing.T) {
	// Reads the ACL back instead of trusting the exit code of icacls. A command
	// that printed "Successfully processed 1 files" and changed nothing is a
	// thing that happens, and this directory is where the keys live.
	//
	// Runs only elevated: without rights the directory is not created at all and
	// the test would be measuring the wrong failure.
	if !windows.GetCurrentProcessToken().IsElevated() {
		t.Skip("нужен повышенный процесс: без прав каталог не создаётся и проверять нечего")
	}
	if err := ZavestiKatalogDannyh(); err != nil {
		t.Fatalf("каталог не заведён: %v", err)
	}
	if st, err := os.Stat(KatalogDannyh()); err != nil || !st.IsDir() {
		t.Fatalf("каталога нет: %v", err)
	}

	sd, err := windows.GetNamedSecurityInfo(KatalogDannyh(),
		windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatalf("дескриптор не читается: %v", err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		t.Fatalf("DACL не читается: %v", err)
	}
	if dacl == nil {
		// A nil DACL is not "no rights", it is "everyone gets everything". The
		// one ACL shape that looks locked down and is the opposite.
		t.Fatal("DACL пуст, а это не запрет, а разрешение всем")
	}

	var sidy []string
	for i := uint16(0); i < dacl.AceCount; i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, uint32(i), &ace); err != nil {
			t.Fatalf("запись %d не читается: %v", i, err)
		}
		if ace.Header.AceFlags&windows.INHERITED_ACE != 0 {
			t.Errorf("запись %d унаследована, наследование не разорвано", i)
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		sidy = append(sidy, sid.String())
	}
	sort.Strings(sidy)

	hotim := []string{"S-1-5-18", "S-1-5-32-544"}
	if len(sidy) != len(hotim) {
		t.Fatalf("записей %d, а должно быть %d: %v", len(sidy), len(hotim), sidy)
	}
	for i := range hotim {
		if sidy[i] != hotim[i] {
			t.Fatalf("список SID не тот: %v", sidy)
		}
	}
}
