package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"unicode/utf8"

	"golang.org/x/sys/windows"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/puti"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/ssylki"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/zhurnaly"
)

// Журнал окна (1.10.0). До него окно писало log.Printf в stderr, которого у
// программы без консоли нет: отказ моста, отказ Wails при старте и ошибка
// страницы не оставляли следа нигде, и разбор жалобы на окно начинался с
// просьбы «запусти из консоли».
//
// Журнал лежит у человека, а не в ProgramData: окно работает от его имени и
// в каталог службы не пишет.

// versiyaProgrammy подставляет сборка выпуска через -ldflags -X; в дереве dev.
// До 1.10.0 сборка передавала её и окну, а переменной в окне не было, и
// компоновщик молча пропускал флаг.
var versiyaProgrammy = "dev"

// katalogZhurnalaOkna это шов: тест не пишет в журнал живого окна.
var katalogZhurnalaOkna = func() string {
	if put, err := windows.KnownFolderPath(windows.FOLDERID_LocalAppData, 0); err == nil {
		return filepath.Join(put, "Affory", puti.PodkatalogZhurnalov)
	}
	return filepath.Join(os.TempDir(), "Affory", puti.PodkatalogZhurnalov)
}

// zavestiZhurnalOkna направляет log в okno.log, а стек падения в
// padenie-okna.log. Отказ окно не останавливает: без журнала хуже, но
// работать можно. Отдаёт закрытие журнала.
func zavestiZhurnalOkna() func() {
	kat := katalogZhurnalaOkna()
	zakryt := func() {}
	if zh, err := zhurnaly.Otkryt(kat, puti.ZhurnalOkna); err == nil {
		log.SetOutput(zh)
		zakryt = func() { _ = zh.Close() }
	}
	if f, proshloe, err := zhurnaly.OtkrytFaylPadeniya(kat, puti.PadenieOkna); err != nil {
		log.Printf("файл падений окна не открыт: %v", err)
	} else {
		if !proshloe.IsZero() {
			log.Printf("прошлый запуск окна упал %s, стек в %s.1", proshloe.Format("2006-01-02 15:04:05"), puti.PadenieOkna)
		}
		// SetCrashOutput берёт копию дескриптора, свой можно закрыть сразу.
		if err := debug.SetCrashOutput(f, debug.CrashOptions{}); err != nil {
			log.Printf("файл падений окна не подключён: %v", err)
		}
		_ = f.Close()
	}
	log.Printf("окно запускается: %s", opisanieOkna())
	return zakryt
}

// opisanieOkna это одна строка про окно для журнала и выгрузки.
func opisanieOkna() string {
	v := windows.RtlGetVersion()
	exe, err := os.Executable()
	if err != nil {
		exe = "путь не прочитан: " + err.Error()
	}
	return fmt.Sprintf("версия %s, Windows %d.%d.%d, WebView2 %s, %s, аргументы %q",
		versiyaProgrammy, v.MajorVersion, v.MinorVersion, v.BuildNumber, versiyaWebView2(), exe, os.Args[1:])
}

// OpisanieVersii это строка «Affory 1.10.0, Windows 11 (26200)» под отказом:
// её переписывают в жалобу первой. Берётся у окна, а не у службы: самый
// нужный отказ это служба, которая не отвечает.
func (m *most) OpisanieVersii() string {
	return podpisVersii(versiyaProgrammy, windows.RtlGetVersion())
}

func podpisVersii(versiya string, v *windows.OsVersionInfoEx) string {
	// Windows 11 называет себя 10.0, отличается номером сборки с 22000.
	imya := fmt.Sprintf("Windows %d", v.MajorVersion)
	if v.MajorVersion == 10 && v.BuildNumber >= 22000 {
		imya = "Windows 11"
	}
	return fmt.Sprintf("Affory %s, %s (%d)", versiya, imya, v.BuildNumber)
}

// predelStrokiStranicy: ошибка страницы приходит со стеком, а стек React
// бывает в десятки килобайт. Начало несёт причину, хвост только повторы.
const predelStrokiStranicy = 4000

// ZapisatVZhurnal пишет в журнал окна строку от страницы: показанный отказ,
// ошибку JavaScript, отвергнутое обещание. Страница своего журнала не имеет,
// а то, что видел человек на экране, при разборе нужно дословно.
func (m *most) ZapisatVZhurnal(tekst string) {
	tekst = ssylki.Vychistit(tekst, nil, nil)
	if len(tekst) > predelStrokiStranicy {
		obrez := predelStrokiStranicy
		for obrez > 0 && !utf8.RuneStart(tekst[obrez]) {
			obrez--
		}
		tekst = tekst[:obrez] + " (обрезано)"
	}
	log.Printf("страница: %s", strings.ReplaceAll(tekst, "\n", "\n    "))
}

// soobshchitNeZapustilos показывает причину, когда окно не поднялось: прежде
// log.Fatal писал её в никуда, и человек видел, что программа просто не
// открылась. Предлагает открыть папку журнала.
func soobshchitNeZapustilos(prichina error) {
	kat := katalogZhurnalaOkna()
	log.Printf("окно не запустилось: %v", prichina)
	const idYes = 6
	tekst, _ := windows.UTF16PtrFromString(fmt.Sprintf("Окно Affory не запустилось: %v\r\n\r\n"+
		"Подробности в журнале окна: %s\r\n\r\nОткрыть папку с журналом? Его можно переслать для разбора.",
		prichina, filepath.Join(kat, puti.ZhurnalOkna)))
	zagolovok, _ := windows.UTF16PtrFromString("Affory")
	otvet, err := windows.MessageBox(0, tekst, zagolovok, windows.MB_YESNO|windows.MB_ICONERROR|windows.MB_SETFOREGROUND)
	if otvet == 0 {
		log.Printf("сообщение об отказе запуска не показано: %v", err)
		return
	}
	if otvet != idYes {
		return
	}
	otkryt, _ := windows.UTF16PtrFromString("open")
	papka, _ := windows.UTF16PtrFromString(kat)
	if err := windows.ShellExecute(0, otkryt, papka, nil, nil, windows.SW_SHOWNORMAL); err != nil {
		log.Printf("папка журнала окна не открылась: %v", err)
	}
}
