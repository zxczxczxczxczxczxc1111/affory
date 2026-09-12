package sostoyanie

import (
	"os"
	"os/exec"
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

// Чужая явная запись в списке доступа обязана ИСЧЕЗАТЬ, а не сохраняться.
//
// `/inheritance:r` убирает унаследованное, `/grant:r` заменяет права названным
// доверенным лицам, но запись для постороннего SID не трогает ни то, ни другое.
// На рабочей машине 12.09.2026 в каталоге ключей так и жила третья запись, на
// SID учётки человека: доступ к DPAPI-блобам без всякого повышения прав, то
// есть ровно та дыра, ради закрытия которой каталог и запирается. Заметил это
// не человек и не судья, а этот тест, и полгода он был единственным, кто её
// видел, потому что на чистой машине такой записи не заводится.
func TestChuzhayaZapisVSpiskeDostupaUbiraetsya(t *testing.T) {
	if !windows.GetCurrentProcessToken().IsElevated() {
		t.Skip("нужен повышенный процесс: список доступа иначе не переписать")
	}
	k := t.TempDir()
	// Кому дать лишний доступ: своей же учётке. Она заведомо существует, и
	// именно она оказалась лишней на рабочей машине.
	tok := windows.GetCurrentProcessToken()
	kto, err := tok.GetTokenUser()
	if err != nil {
		t.Fatalf("свой SID не читается: %v", err)
	}
	svoy := kto.User.Sid.String()
	// Звёздочка перед SID обязательна: без неё icacls ищет УЧЁТКУ с таким
	// именем и отвечает «no mapping between account names and security IDs».
	if out, err := exec.Command("icacls", k, "/grant", "*"+svoy+":(OI)(CI)F").CombinedOutput(); err != nil {
		t.Fatalf("подготовка не удалась: %v: %s", err, out)
	}

	if err := zavestiKatalog(k); err != nil {
		t.Fatalf("каталог не заведён: %v", err)
	}

	for _, sid := range sidyKataloga(t, k) {
		if sid == svoy {
			t.Fatalf("чужая запись осталась в списке доступа: %v", sidyKataloga(t, k))
		}
	}
}

// sidyKataloga читает DACL и отдаёт SID его записей строками.
func sidyKataloga(t *testing.T, put string) []string {
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(put, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatalf("дескриптор не читается: %v", err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		t.Fatalf("DACL не читается: %v", err)
	}
	if dacl == nil {
		t.Fatal("DACL пуст, а это не запрет, а разрешение всем")
	}
	var sidy []string
	for i := uint16(0); i < dacl.AceCount; i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, uint32(i), &ace); err != nil {
			t.Fatalf("запись %d не читается: %v", i, err)
		}
		sidy = append(sidy, (*windows.SID)(unsafe.Pointer(&ace.SidStart)).String())
	}
	sort.Strings(sidy)
	return sidy
}
