package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// The data directory holds the DPAPI blob with the keys. It survives
// `sc delete` on purpose (a reinstall must not lose servers), so wiping it
// is a separate, explicit decision carried on the command line as a flag
// the interface sets only after the human answered the question.
func TestDannyeStirayutsyaTolkoPoFlagu(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Affory")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sekrety.bin"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := snyatDannye(dir, false); err != nil {
		t.Fatalf("без флага упало: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "sekrety.bin")); err != nil {
		t.Fatal("без флага ключи исчезли: это ровно та потеря, ради которой задан вопрос")
	}

	if err := snyatDannye(dir, true); err != nil {
		t.Fatalf("с флагом упало: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("с флагом каталог данных остался")
	}
	// Second wipe of a missing directory is fine: the human may run it twice.
	if err := snyatDannye(dir, true); err != nil {
		t.Fatalf("повторное стирание упало: %v", err)
	}
}

func TestArgumentyUninstall(t *testing.T) {
	// Only the exact flag means "erase". A typo is "keep": losing keys to a
	// misspelling would be the worst possible reading of an ambiguous line.
	for _, c := range []struct {
		args   []string
		steret bool
	}{
		{[]string{"uninstall"}, false},
		{[]string{"uninstall", flagSteretKlyuchi}, true},
		{[]string{"uninstall", "--steret"}, false},
		{[]string{"uninstall", "--STERET-KLYUCHI"}, false},
	} {
		if got := steretKlyuchiIz(c.args); got != c.steret {
			t.Errorf("%v: стереть=%v, ждали %v", c.args, got, c.steret)
		}
	}
}

// The program directory is removed by a detached cmd.exe AFTER this process
// exits. Guest run 02.09.2026: the directory stayed intact, and the host
// reproduction showed why: exec.Command quotes the whole `/c` argument, cmd
// strips the outer quotes and chokes on the inner ones. The test aims the
// same launch at a scratch directory and waits for it to vanish.
func TestKatalogProgrammyUdalyaetsyaSnaruzhi(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Affory")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "affory-svc.exe"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	star := katalogDlyaUdaleniya
	katalogDlyaUdaleniya = func() string { return dir }
	t.Cleanup(func() { katalogDlyaUdaleniya = star })

	if err := udalitKatalogProgrammy(); err != nil {
		t.Fatalf("запуск не запланирован: %v", err)
	}
	for i := 0; i < 20; i++ {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatal("каталог программы не удалён за 10 с: отложенное удаление не работает")
}

// Одной попытки rmdir мало, и это не теория. Каталог программы держит не только
// служба: там же лежит affory-ui.exe, а окно закрывается САМО и не мгновенно
// (most.go зовёт app.Quit после того, как команда уже ушла). Успевает оно за три
// секунды или нет, зависит от машины, и в неудачный день uninstall отчитывался
// кодом 0, оставив каталог на диске.
//
// Здесь занятый файл отпускается на седьмой секунде, то есть заведомо позже
// первой попытки. Отчёт об успехе при живом каталоге хуже честного отказа: он
// закрывает вопрос, которого никто больше не откроет.
func TestKatalogProgrammyUdalyaetsyaZanyatyy(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Affory")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	zanyatyy := filepath.Join(dir, "affory-ui.exe")
	f, err := os.OpenFile(zanyatyy, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	// Держим файл открытым: Windows не даёт удалить файл, открытый без
	// FILE_SHARE_DELETE, а Go открывает файлы именно так.
	otpushchen := make(chan struct{})
	go func() {
		time.Sleep(7 * time.Second)
		f.Close()
		close(otpushchen)
	}()
	t.Cleanup(func() { f.Close() })

	star := katalogDlyaUdaleniya
	katalogDlyaUdaleniya = func() string { return dir }
	t.Cleanup(func() { katalogDlyaUdaleniya = star })

	if err := udalitKatalogProgrammy(); err != nil {
		t.Fatalf("запуск не запланирован: %v", err)
	}
	<-otpushchen
	for i := 0; i < 40; i++ {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatal("каталог программы остался: одной попытки удаления не хватило, а другой не было")
}

// Снятие из КОМАНДНОЙ СТРОКИ обязано освобождать каталог само.
//
// Окно закрывается само только тогда, когда снятие пошло ЧЕРЕЗ НЕГО: most.go
// зовёт app.Quit после того, как команда ушла. Про `affory-svc.exe uninstall`,
// набранный руками или запущенный оснасткой, окно не узнаёт никогда, остаётся
// жить и держит каталог своим образом: Windows не даёт удалить exe работающего
// процесса. Уборщик отрабатывает все восемь кругов впустую, а uninstall к тому
// времени уже отчитался кодом 0.
//
// Поймано 07.09.2026 живым прогоном: судья аудита умер исключением
// «affory-svc.exe не найден» сразу после снятия, а в каталоге лежал
// affory-ui.exe с прошлой проверки кнопок.
//
// Здесь роль окна играет копия ping.exe: тесту нужен процесс, чей ОБРАЗ лежит
// внутри каталога, а не настоящий интерфейс. Проверено отдельно, что такой
// процесс и правда держит каталог от `rmdir /s /q`.
func TestKatalogProgrammyUdalyaetsyaPriZhivomOkne(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Affory")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	telo, err := os.ReadFile(filepath.Join(os.Getenv("SystemRoot"), "System32", "PING.EXE"))
	if err != nil {
		t.Skipf("нет чем изобразить окно: %v", err)
	}
	okno := filepath.Join(dir, "affory-ui.exe")
	if err := os.WriteFile(okno, telo, 0o700); err != nil {
		t.Fatal(err)
	}
	zhivyot := exec.Command(okno, "-n", "120", "127.0.0.1")
	zhivyot.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := zhivyot.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = zhivyot.Process.Kill()
		_, _ = zhivyot.Process.Wait()
	})
	// Даём образу открыться: до этого каталог ещё не занят, и тест доказывал бы
	// не то, ради чего написан.
	time.Sleep(500 * time.Millisecond)

	star := katalogDlyaUdaleniya
	katalogDlyaUdaleniya = func() string { return dir }
	t.Cleanup(func() { katalogDlyaUdaleniya = star })

	if err := udalitKatalogProgrammy(); err != nil {
		t.Fatalf("запуск не запланирован: %v", err)
	}
	for i := 0; i < 80; i++ {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatal("каталог программы остался: живое окно держит его, а снятие отчиталось успехом")
}

// Удаление снимает ТОЛЬКО наше, даже если программа стоит в общем каталоге.
//
// До 1.6.2 здесь стоял `rmdir /s /q` по каталогу программы, а каталогом
// программы считается каталог бинаря. Установщик слушается того пути, который
// человек вписал руками, то есть Affory, поставленная в `D:\Games`, при удалении
// стирала `D:\Games` целиком. Пункт К1 аудита 1.6.1.
func TestUdalenieNeTrogaetChuzhoeVKataloge(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Games")
	nashi := []string{"affory-svc.exe", "affory-ui.exe", "Uninstall.exe", "GPL-3.0.txt", "affory-ui.exe.ubrat"}
	for _, p := range []string{"predydushchaya", "sohraneniya"} {
		if err := os.MkdirAll(filepath.Join(dir, p), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range append(nashi, `predydushchaya\affory-svc.exe`, "igra.exe", `sohraneniya\slot1.dat`) {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	star := katalogDlyaUdaleniya
	katalogDlyaUdaleniya = func() string { return dir }
	t.Cleanup(func() { katalogDlyaUdaleniya = star })

	if err := udalitKatalogProgrammy(); err != nil {
		t.Fatalf("запуск не запланирован: %v", err)
	}
	ushli := func() bool {
		for _, f := range append(nashi, "predydushchaya") {
			if _, err := os.Stat(filepath.Join(dir, f)); !os.IsNotExist(err) {
				return false
			}
		}
		return true
	}
	for i := 0; i < 30 && !ushli(); i++ {
		time.Sleep(500 * time.Millisecond)
	}
	for _, f := range []string{"igra.exe", `sohraneniya\slot1.dat`} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Fatalf("удаление Affory стёрло чужой файл %s: %v", f, err)
		}
	}
	if !ushli() {
		t.Fatal("наши файлы остались: удаление по списку не отработало")
	}
}

// Два удаления, один список. Каталог программы снимают и служба (uninstall,
// в том числе из окна), и деинсталлятор NSIS. Разойдись их списки, одно из
// двух оставляло бы файл, который другое считает своим, и каталог переживал бы
// удаление по одному пути из двух.
func TestSpisokUdaleniyaSovpadaetSUstanovshchikom(t *testing.T) {
	telo, err := os.ReadFile(filepath.Join("..", "..", "ustanovka", "affory.nsi"))
	if err != nil {
		t.Fatalf("установщик не читается: %v", err)
	}
	tekst := string(telo)
	i := strings.Index(tekst, `Section "Uninstall"`)
	if i < 0 {
		t.Fatal("в установщике нет раздела Uninstall")
	}
	razdel := tekst[i:]
	razdel = razdel[:strings.Index(razdel, "SectionEnd")]
	// Имя файла причины приходит в NSIS определением, а не строкой.
	razdel = strings.ReplaceAll(razdel, "${FAYL_PRICHINY}", imyaFaylaPrichiny)

	nsis := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^\s*(Delete|RMDir /r) "\$INSTDIR\\([^"]+)"`).FindAllStringSubmatch(razdel, -1) {
		nsis[m[2]] = true
	}
	nashi := map[string]bool{}
	for _, f := range append(append(append([]string{}, faylyProgrammy...), maskiProgrammy...), katalogiProgrammy...) {
		nashi[f] = true
	}
	for f := range nashi {
		if !nsis[f] {
			t.Errorf("служба удаляет %s, а деинсталлятор нет", f)
		}
	}
	for f := range nsis {
		if !nashi[f] {
			t.Errorf("деинсталлятор удаляет %s, а служба нет", f)
		}
	}
}

// Освобождение каталога гасит только НАШИ программы.
//
// Установка и удаление завершают процессы, чей образ лежит в каталоге
// программы. Для `D:\Games` это значило убить запущенную оттуда игру ещё до
// копирования файлов. Решает не место, а место вместе с именем.
func TestOsvobozhdenieNeTrogaetChuzhieProgrammyIzKataloga(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Games")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	telo, err := os.ReadFile(filepath.Join(os.Getenv("SystemRoot"), "System32", "PING.EXE"))
	if err != nil {
		t.Skipf("нет чем изобразить игру: %v", err)
	}
	igra := filepath.Join(dir, "igra.exe")
	if err := os.WriteFile(igra, telo, 0o700); err != nil {
		t.Fatal(err)
	}
	zhivyot := exec.Command(igra, "-n", "30", "127.0.0.1")
	zhivyot.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := zhivyot.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = zhivyot.Process.Kill()
		_, _ = zhivyot.Process.Wait()
	})
	time.Sleep(500 * time.Millisecond)

	osvoboditKatalog(dir)

	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(zhivyot.Process.Pid))
	if err != nil {
		t.Fatalf("чужая программа из каталога убита: %v", err)
	}
	defer windows.CloseHandle(h)
	var kod uint32
	if err := windows.GetExitCodeProcess(h, &kod); err != nil {
		t.Fatalf("состояние чужой программы не читается: %v", err)
	}
	if kod != 259 { // STILL_ACTIVE
		t.Fatalf("чужая программа из каталога завершена с кодом %d", kod)
	}
}

// КОНТРОЛЬ: чужое не трогаем.
//
// Освобождение каталога ищет процессы по ПУТИ ОБРАЗА именно ради этого. Поиск
// по имени `affory-ui.exe` однажды убил бы чужой процесс с тем же именем, а
// «программа при снятии убила что-то постороннее» это худший исход из всех, что
// можно получить от уборки за собой.
func TestOsvobozhdenieNeTrogaetChuzhih(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Affory")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	// Процесс ВНЕ каталога программы и с тем же именем, что у нашего окна:
	// совпадает всё, кроме места, а решает именно место.
	chuzhoy := filepath.Join(t.TempDir(), "affory-ui.exe")
	telo, err := os.ReadFile(filepath.Join(os.Getenv("SystemRoot"), "System32", "PING.EXE"))
	if err != nil {
		t.Skipf("нет чем изобразить чужой процесс: %v", err)
	}
	if err := os.WriteFile(chuzhoy, telo, 0o700); err != nil {
		t.Fatal(err)
	}
	zhivyot := exec.Command(chuzhoy, "-n", "30", "127.0.0.1")
	zhivyot.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := zhivyot.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = zhivyot.Process.Kill()
		_, _ = zhivyot.Process.Wait()
	})
	time.Sleep(500 * time.Millisecond)

	osvoboditKatalog(dir)

	// Жив, а не «не сообщил об ошибке»: судим по факту, а не по коду возврата.
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(zhivyot.Process.Pid))
	if err != nil {
		t.Fatalf("чужой процесс убит уборкой за собой: %v", err)
	}
	defer windows.CloseHandle(h)
	var kod uint32
	if err := windows.GetExitCodeProcess(h, &kod); err != nil {
		t.Fatalf("состояние чужого процесса не читается: %v", err)
	}
	if kod != 259 { // STILL_ACTIVE
		t.Fatalf("чужой процесс завершён с кодом %d, а его никто не трогал", kod)
	}
}
