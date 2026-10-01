package sostoyanie

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
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

// Каталог данных, заведённый заранее обычным пользователем, после нас уже не
// его. Владелец может переписать список доступа всегда, и без смены владельца
// запертый каталог ключей он отпирает себе обратно одной командой (К2 аудита
// 1.6.1).
func TestKatalogDannyhMenyaetVladelca(t *testing.T) {
	if !windows.GetCurrentProcessToken().IsElevated() {
		t.Skip("нужен повышенный процесс: владельца иначе не сменить")
	}
	k := t.TempDir()
	svoy := svoySid(t)
	if err := windows.SetNamedSecurityInfo(k, windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION, svoy, nil, nil, nil); err != nil {
		t.Fatalf("подготовка владельца не удалась: %v", err)
	}
	if vladelec(t, k) != svoy.String() {
		t.Fatal("подготовка не сработала: владелец не свой, тест доказывал бы не то")
	}

	if err := zavestiKatalog(k); err != nil {
		t.Fatalf("каталог не заведён: %v", err)
	}
	if v := vladelec(t, k); v != "S-1-5-32-544" {
		t.Fatalf("владелец каталога данных %s, а должны быть администраторы", v)
	}
}

// Каталог программы запирается вместе с содержимым: подложенный заранее файл
// со своей явной записью в списке доступа её теряет.
func TestKatalogProgrammyZapiraetsyaSSoderzhimym(t *testing.T) {
	if !windows.GetCurrentProcessToken().IsElevated() {
		t.Skip("нужен повышенный процесс: список доступа иначе не переписать")
	}
	dir := filepath.Join(t.TempDir(), "Affory")
	if err := os.MkdirAll(filepath.Join(dir, "novaya"), 0o755); err != nil {
		t.Fatal(err)
	}
	fayly := []string{filepath.Join(dir, "affory-svc.exe"), filepath.Join(dir, "novaya", "sing-box.exe")}
	for _, f := range fayly {
		if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	svoy := svoySid(t).String()
	if out, err := exec.Command("icacls", fayly[0], "/grant", "*"+svoy+":F").CombinedOutput(); err != nil {
		t.Fatalf("подготовка не удалась: %v: %s", err, out)
	}

	if err := ZakrytKatalogProgrammy(dir); err != nil {
		t.Fatalf("каталог не заперт: %v", err)
	}
	hotim := "S-1-15-2-1 S-1-5-18 S-1-5-32-544 S-1-5-32-545"
	for _, put := range append([]string{dir, filepath.Join(dir, "novaya")}, fayly...) {
		if s := strings.Join(sidyKataloga(t, put), " "); s != hotim {
			t.Errorf("%s: записи %s, ждали %s", put, s, hotim)
		}
		if v := vladelec(t, put); v != "S-1-5-32-544" {
			t.Errorf("%s: владелец %s, а должны быть администраторы", put, v)
		}
	}
}

// Папка журналов читается пользователями машины, а запись у них не появляется
// (01.10.2026: кнопка «Открыть папку с журналами» не открывалась ни у кого).
// Файл внутри получает те же права: журналы службы пишутся в уже созданные
// файлы, и права одной папки их бы не открыли.
func TestZhurnalyChitayutsyaPolzovatelyami(t *testing.T) {
	if !windows.GetCurrentProcessToken().IsElevated() {
		t.Skip("нужен повышенный процесс: список доступа иначе не переписать")
	}
	dir := filepath.Join(t.TempDir(), "log")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	fayl := filepath.Join(dir, "sluzhba.log")
	if err := os.WriteFile(fayl, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := otkrytZhurnaly(dir); err != nil {
		t.Fatalf("папка журналов не открыта: %v", err)
	}
	hotim := "S-1-5-18 S-1-5-32-544 S-1-5-32-545"
	for _, put := range []string{dir, fayl} {
		if s := strings.Join(sidyKataloga(t, put), " "); s != hotim {
			t.Errorf("%s: записи %s, ждали %s", put, s, hotim)
		}
	}
	for _, d := range sidyZhurnalov {
		if d.sid == "S-1-5-32-545" && d.prava&(windows.FILE_WRITE_DATA|windows.FILE_APPEND_DATA|windows.DELETE) != 0 {
			t.Errorf("пользователи получили запись в журналы: %#x", d.prava)
		}
	}
}

// Общий каталог не трогаем: запереть `D:\Games` значит отнять его у человека.
func TestObshchiyKatalogNeZapiraetsya(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Games")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	do := strings.Join(sidyKataloga(t, dir), " ")
	if err := ZakrytKatalogProgrammy(dir); !errors.Is(err, ErrKatalogObshchiy) {
		t.Fatalf("общий каталог не опознан: %v", err)
	}
	if posle := strings.Join(sidyKataloga(t, dir), " "); posle != do {
		t.Fatalf("права общего каталога изменены: было %s, стало %s", do, posle)
	}
}

// Точка соединения на месте каталога не уводит права в чужой каталог. Права
// ставятся от SYSTEM, и ссылка на System32, заведённая заранее, превратила бы
// запирание нашего каталога в переписывание системного.
func TestSsylkaNaMesteKatalogaNeProhoditsya(t *testing.T) {
	baza := t.TempDir()
	tsel := filepath.Join(baza, "tsel")
	if err := os.MkdirAll(tsel, 0o755); err != nil {
		t.Fatal(err)
	}
	do := strings.Join(sidyKataloga(t, tsel), " ")
	ssylka := filepath.Join(baza, "Affory")
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", ssylka, tsel).CombinedOutput(); err != nil {
		t.Skipf("точка соединения не заводится: %v: %s", err, out)
	}

	if err := ZakrytKatalogProgrammy(ssylka); !errors.Is(err, ErrKatalogSsylka) {
		t.Fatalf("ссылка на месте каталога не опознана: %v", err)
	}
	if posle := strings.Join(sidyKataloga(t, tsel), " "); posle != do {
		t.Fatalf("права ушли по ссылке в чужой каталог: было %s, стало %s", do, posle)
	}
}

// Ссылка ВНУТРИ каталога пропускается, а сам каталог запирается.
func TestSsylkaVnutriKatalogaPropuskaetsya(t *testing.T) {
	if !windows.GetCurrentProcessToken().IsElevated() {
		t.Skip("нужен повышенный процесс: список доступа иначе не переписать")
	}
	baza := t.TempDir()
	dir := filepath.Join(baza, "Affory")
	tsel := filepath.Join(baza, "tsel")
	for _, k := range []string{dir, tsel} {
		if err := os.MkdirAll(k, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	do := strings.Join(sidyKataloga(t, tsel), " ")
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", filepath.Join(dir, "vnutri"), tsel).CombinedOutput(); err != nil {
		t.Skipf("точка соединения не заводится: %v: %s", err, out)
	}

	if err := ZakrytKatalogProgrammy(dir); err != nil {
		t.Fatalf("каталог не заперт: %v", err)
	}
	if posle := strings.Join(sidyKataloga(t, tsel), " "); posle != do {
		t.Fatalf("права ушли по ссылке в чужой каталог: было %s, стало %s", do, posle)
	}
}

func svoySid(t *testing.T) *windows.SID {
	t.Helper()
	kto, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatalf("свой SID не читается: %v", err)
	}
	return kto.User.Sid
}

func vladelec(t *testing.T, put string) string {
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(put, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		t.Fatalf("владелец %s не читается: %v", put, err)
	}
	v, _, err := sd.Owner()
	if err != nil {
		t.Fatalf("владелец %s не разобран: %v", put, err)
	}
	return v.String()
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
