package main

import (
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Окно рисует WebView2. Без него Wails не создаёт содержимое вовсе, и человек
// видит чёрный прямоугольник без рамки, который закрывается только из трея
// (пункт В8 аудита 1.6.1). На обычных Windows 10 и 11 рантайм стоит, а на LTSC
// и урезанных сборках его может не быть.
//
// Рантайм ищется ТЕМ ЖЕ способом, каким его потом ищет сам Wails
// (v3.0.0-beta.16, internal/webview2/webviewloader/find_dll_installed.go):
// папка из значения EBWebView в ClientState канала, версия из имени папки не
// ниже 86.0.616.0 и библиотека под свою разрядность внутри. Документированный
// Microsoft способ через Clients и pv живёт в установщике. Здесь он не годится:
// если бы проверка окна разошлась с Wails, человек получил бы отказ при
// работающем рантайме или чёрное окно при «найденном».

const klyuchSostoyaniyaEdge = `Software\Microsoft\EdgeUpdate\ClientState\`

// Каналы в порядке Wails: стабильный, beta, dev, canary.
var kanalyWebView2 = []string{
	"{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}",
	"{2CD8A007-E189-409D-A2C8-9AF4EF3C72AA}",
	"{0D50BFEC-CD6A-4F9A-964C-C7416E3ACB10}",
	"{65C35B14-6C1D-4122-AC46-7148CC9D6497}",
}

var minimalnyyWebView2 = [4]int{86, 0, 616, 0}

// ssylkaWebView2 отдаёт загрузчик рантайма с сайта Microsoft.
const ssylkaWebView2 = "https://go.microsoft.com/fwlink/p/?LinkId=2124703"

func webView2Est() bool {
	for _, kanal := range kanalyWebView2 {
		for _, koren := range []registry.Key{registry.LOCAL_MACHINE, registry.CURRENT_USER} {
			if papka, ok := papkaWebView2(koren, klyuchSostoyaniyaEdge+kanal); ok && webView2Goden(papka) {
				return true
			}
		}
	}
	return false
}

func papkaWebView2(koren registry.Key, put string) (string, bool) {
	k, err := registry.OpenKey(koren, put, registry.QUERY_VALUE|registry.WOW64_32KEY)
	if err != nil {
		return "", false
	}
	defer k.Close()
	papka, _, err := k.GetStringValue("EBWebView")
	if err != nil || papka == "" {
		return "", false
	}
	return papka, true
}

// webView2Goden повторяет две проверки Wails над найденной папкой: версию из
// её имени и библиотеку под свою разрядность.
func webView2Goden(papka string) bool {
	v, ok := razobratVersiyuWebView2(filepath.Base(papka))
	if !ok || sravnitVersiiWebView2(v, minimalnyyWebView2) < 0 {
		return false
	}
	arh := map[string]string{"amd64": "x64", "arm64": "arm64", "386": "x86"}[runtime.GOARCH]
	if arh == "" {
		return false
	}
	if !filepath.IsAbs(papka) {
		exe, err := os.Executable()
		if err != nil {
			return false
		}
		papka = filepath.Join(filepath.Dir(exe), papka)
	}
	_, err := os.Stat(filepath.Join(papka, "EBWebView", arh, "EmbeddedBrowserWebView.dll"))
	return err == nil
}

// razobratVersiyuWebView2 читает «120.0.2210.91», как Wails: до четырёх чисел
// через точку, недостающие считаются нулями.
func razobratVersiyuWebView2(s string) ([4]int, bool) {
	var v [4]int
	chasti := strings.Split(s, ".")
	if len(chasti) == 0 || len(chasti) > 4 {
		return v, false
	}
	for i, ch := range chasti {
		n, err := strconv.Atoi(ch)
		if err != nil || n < 0 {
			return v, false
		}
		v[i] = n
	}
	return v, true
}

func sravnitVersiiWebView2(a, b [4]int) int {
	for i := range a {
		if a[i] != b[i] {
			if a[i] < b[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

// soobshchitNetWebView2 говорит человеку, чего не хватает, и по согласию
// открывает загрузку. Окно Wails к этому моменту не создано, поэтому сообщение
// системное и без владельца.
func soobshchitNetWebView2() {
	const idYes = 6
	// Строки постоянные и без нулевых символов: перевод в UTF-16 не отказывает.
	tekst, _ := windows.UTF16PtrFromString("Для окна Affory нужен компонент Microsoft Edge WebView2, " +
		"а на этом компьютере его нет.\r\n\r\nСкачать его с сайта Microsoft? После установки запусти Affory снова.")
	zagolovok, _ := windows.UTF16PtrFromString("Affory")
	otvet, err := windows.MessageBox(0, tekst, zagolovok, windows.MB_YESNO|windows.MB_ICONWARNING|windows.MB_SETFOREGROUND)
	if otvet == 0 {
		log.Printf("WebView2 не найден, и сообщение об этом не показано: %v", err)
		return
	}
	if otvet != idYes {
		return
	}
	otkryt, _ := windows.UTF16PtrFromString("open")
	ssylka, _ := windows.UTF16PtrFromString(ssylkaWebView2)
	if err := windows.ShellExecute(0, otkryt, ssylka, nil, nil, windows.SW_SHOWNORMAL); err != nil {
		log.Printf("страница загрузки WebView2 не открылась: %v", err)
	}
}
