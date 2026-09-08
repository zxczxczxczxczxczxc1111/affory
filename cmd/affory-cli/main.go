// affory-cli is NOT a product. It exists so waves 1 through 3 can poke the
// service without an interface, and it does not go into the release. When wave 4
// lands, this is the thing that gets deleted, not maintained.
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/kanal"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// versiyaProgrammy подставляет сборка выпуска через -ldflags -X; в дереве dev.
var versiyaProgrammy = "dev"

const pomoshch = `команды:
  version
  status
  connect [--server <id>]
  disconnect
  switch <id>
  mode [avto|ruchnoy]
  killswitch on|off
  events [--seconds N]
  servers list
  servers add <ссылка>
  servers remove <id>
  autostart on|off       программа при входе в Windows (в трее)
  autoconnect on|off     туннель при старте службы
  subscription set <url>
  subscription refresh
  profile export --out <файл>
  profile import --in <файл>

Флаги ставятся ПОСЛЕ команды и её подкоманды.
Пароль профиля спрашивается в терминале и НЕ берётся из аргументов.`

func main() {
	// Слова команды отделяются от флагов ВРУЧНУЮ, а не flag.Parse.
	//
	// Стандартный разбор останавливается на первом позиционном аргументе,
	// поэтому «profile export --out файл» отдал бы --out в остаток, и экспорт
	// ушёл бы писать в никуда. Проверка на таком CLI печатала бы зелёный,
	// ничего не сделав, а это уже второй декоративный прогон в проекте после
	// пробы волны 1.
	slova, hvost := razdelit(os.Args[1:])
	if len(slova) == 0 {
		fmt.Fprintln(os.Stderr, pomoshch)
		os.Exit(2)
	}
	komanda := slova[0]
	if komanda == "version" {
		fmt.Println("affory " + versiyaProgrammy + ", протокол " + strconv.Itoa(protokol.Versiya))
		return
	}
	pod := ""
	if len(slova) > 1 {
		pod = slova[1]
	}

	fs := flag.NewFlagSet(komanda, flag.ExitOnError)
	server := fs.String("server", "", "идентификатор сервера")
	sekundy := fs.Int("seconds", 10, "сколько секунд слушать события в команде events")
	kuda := fs.String("out", "", "куда положить файл профиля")
	otkuda := fs.String("in", "", "откуда взять файл профиля")
	// Флаг существует ТОЛЬКО чтобы отказать вслух. В плане он назывался
	// --password, и человек, идущий по плану буквально, обязан узнать причину, а
	// не гадать, почему флаг не опознан.
	parolVArgv := fs.String("password", "", "не поддерживается: пароль спрашивается в терминале")
	if err := fs.Parse(hvost); err != nil {
		os.Exit(2)
	}
	if *parolVArgv != "" {
		// Аргументы командной строки на Windows видны любому процессу через
		// Win32_Process.CommandLine и остаются в истории PowerShell. Пароль от
		// ВСЕХ ключей в истории оболочки это не мелочь стиля.
		fmt.Fprintln(os.Stderr, "--password не поддерживается: аргументы видны другим процессам и остаются в истории оболочки")
		os.Exit(2)
	}

	k, err := kanal.Podklyuchitsya()
	if err != nil {
		// The most common failure by far, and the one whose cause is least
		// obvious from an errno. Say the likely reason out loud.
		fmt.Fprintf(os.Stderr, "служба недоступна: %v\n", err)
		os.Exit(1)
	}
	defer k.Zakryt()

	switch komanda {
	case "events":
		// Цифры экрана (stats) идут только подписчику этого соединения: без
		// подписки events показывал бы одни state и молчал про статистику.
		if _, err := k.Zvat("subscribeStats", map[string]bool{"vkl": true}); err != nil {
			fmt.Fprintf(os.Stderr, "подписка на статистику не принята: %v\n", err)
		}
		slushat(k, time.Duration(*sekundy)*time.Second)

	case "killswitch":
		if pod != "on" && pod != "off" {
			vyhod("killswitch принимает on или off")
		}
		pechat(k.Zvat("setKillSwitch", map[string]bool{"vkl": pod == "on"}))

	// Журнал соединений (6.2): on, off и clear. Без аргумента печатает статус,
	// как mode: команда, которая на пустой аргумент что-то делает, это
	// команда, которой боятся.
	case "journal":
		switch pod {
		case "on", "off":
			pechat(k.Zvat("setJournal", map[string]bool{"vkl": pod == "on"}))
		case "clear":
			pechat(k.Zvat("clearJournal", nil))
		case "":
			pechat(k.Zvat("status", nil))
		default:
			vyhod("journal принимает on, off или clear")
		}

	// Two decisions, two commands (§9.2, task 4.8): the program at logon and
	// the tunnel at boot. One word for both would be the merged switch the
	// spec forbids, only on the command line.
	case "autostart":
		if pod != "on" && pod != "off" {
			vyhod("autostart принимает on или off")
		}
		pechat(k.Zvat("setAutostart", map[string]bool{"vkl": pod == "on"}))

	case "autoconnect":
		if pod != "on" && pod != "off" {
			vyhod("autoconnect принимает on или off")
		}
		pechat(k.Zvat("setConnectOnStart", map[string]bool{"vkl": pod == "on"}))

	case "connect":
		telo := map[string]string{}
		if *server != "" {
			telo["server"] = *server
		}
		pechat(k.Zvat("connect", telo))

	case "switch":
		// Отличается от connect --server тем, что ходит в ЖИВОЕ ядро: connect
		// выбор только запоминает, а здесь трафик переезжает под работающим
		// человеком, без перезапуска туннеля.
		if pod == "" {
			vyhod("switch принимает идентификатор сервера: switch de")
		}
		pechat(k.Zvat("setServer", map[string]string{"id": pod}))

	case "mode":
		// Без аргумента ПЕЧАТАЕТ текущий, а не переключает. Команда, которая на
		// пустой аргумент что-то делает, это команда, которой боятся.
		if pod == "" {
			pechat(k.Zvat("status", nil))
			break
		}
		pechat(k.Zvat("setRouteMode", map[string]string{"rezhim": pod}))

	case "status", "disconnect":
		pechat(k.Zvat(komanda, nil))

	case "servers":
		servery(k, pod, slova)

	case "subscription":
		podpiska(k, pod, slova)

	case "profile":
		profil(k, pod, *kuda, *otkuda)

	case "rules":
		pravila(k, pod, *otkuda)

	default:
		// Anything else is passed through untouched; a second word is the raw
		// JSON body (installUpdate {"path": ...}). The stubs answer with the
		// wave they arrive in, which is more useful than this tool guessing.
		var telo any
		if pod != "" {
			if !json.Valid([]byte(pod)) {
				vyhod("тело команды должно быть JSON: " + pod)
			}
			telo = json.RawMessage(pod)
		}
		pechat(k.Zvat(komanda, telo))
	}
}

// razdelit отдаёт ведущие слова команды и всё остальное как флаги.
//
// Первое слово, начинающееся с дефиса, начинает хвост. Ссылки vless:// и
// адреса подписки с дефиса не начинаются, поэтому граница однозначна.
func razdelit(argv []string) (slova, hvost []string) {
	for i, a := range argv {
		if strings.HasPrefix(a, "-") {
			return argv[:i], argv[i:]
		}
	}
	return argv, nil
}

func servery(k *kanal.Klient, pod string, slova []string) {
	switch pod {
	case "list", "":
		pechat(k.Zvat("listServers", nil))
	case "add":
		if len(slova) < 3 {
			vyhod("servers add ждёт ссылку")
		}
		// Ссылка тоже уезжает в историю оболочки, и в ней лежит uuid. Молчать
		// об этом значит дать человеку решить проблему, о которой он не знает.
		fmt.Fprintln(os.Stderr, "внимание: ссылка осталась в истории оболочки, в ней ключ")
		pechat(k.Zvat("addServer", map[string]string{"ssylka": slova[2]}))
	case "remove":
		if len(slova) < 3 {
			vyhod("servers remove ждёт идентификатор")
		}
		pechat(k.Zvat("removeServer", map[string]string{"id": slova[2]}))
	default:
		vyhod("servers принимает list, add, remove")
	}
}

func podpiska(k *kanal.Klient, pod string, slova []string) {
	switch pod {
	case "set":
		if len(slova) < 3 {
			vyhod("subscription set ждёт адрес")
		}
		fmt.Fprintln(os.Stderr, "внимание: адрес подписки остался в истории оболочки, это пропуск")
		pechat(k.Zvat("setSubscription", map[string]string{"adres": slova[2]}))
	case "refresh":
		pechat(k.Zvat("refreshSubscription", nil))
	default:
		vyhod("subscription принимает set и refresh")
	}
}

// pravilaFayla это тело setRules, как оно лежит в файле: два списка и
// выключатель российского набора.
//
// Выключатель указателем, а не bool: служба различает «выключить» и «не
// трогали», и файл со списками, написанный до его появления, не должен
// возвращать набор на место молча.
type pravilaFayla struct {
	Protsessy   []string `json:"protsessy"`
	Domeny      []string `json:"domeny"`
	BezRuSpiska *bool    `json:"bez_ru_spiska,omitempty"`
}

// pravilaIzFayla читает правила из файла. Файл, а не argv: список путей на
// командной строке не перепечатает никто, а сам путь к файлу не секрет.
func pravilaIzFayla(put string) (pravilaFayla, error) {
	b, err := os.ReadFile(put)
	if err != nil {
		return pravilaFayla{}, fmt.Errorf("файл правил не прочитан: %w", err)
	}
	// Windows пишет UTF-8 с сигнатурой почти всегда: `Set-Content -Encoding
	// UTF8` в PowerShell 5.1, «Блокнот», половина редакторов. Для JSON это
	// мусор перед первой скобкой, и разбор отвечает «invalid character 'ï'»,
	// что человек читает как сломанную программу, а не как сломанный файл.
	b = bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF})
	var p pravilaFayla
	if err := json.Unmarshal(b, &p); err != nil {
		return pravilaFayla{}, fmt.Errorf("файл правил не разобран: %w", err)
	}
	if p.Protsessy == nil {
		p.Protsessy = []string{}
	}
	if p.Domeny == nil {
		p.Domeny = []string{}
	}
	return p, nil
}

func pravila(k *kanal.Klient, pod, otkuda string) {
	switch pod {
	case "list", "":
		pechat(k.Zvat("listRules", nil))
	case "set":
		if otkuda == "" {
			vyhod("rules set ждёт --in <файл с {\"protsessy\":[],\"domeny\":[],\"bez_ru_spiska\":false}>")
		}
		p, err := pravilaIzFayla(otkuda)
		if err != nil {
			vyhod(err.Error())
		}
		pechat(k.Zvat("setRules", p))
	default:
		vyhod("rules принимает list и set --in <файл>")
	}
}

func profil(k *kanal.Klient, pod, kuda, otkuda string) {
	switch pod {
	case "export":
		if kuda == "" {
			vyhod("profile export ждёт --out <файл>")
		}
		parol, err := sprositParol("Пароль для нового профиля: ")
		if err != nil {
			vyhod(err.Error())
		}
		o, err := k.Zvat("exportProfile", map[string]string{"parol": parol})
		proverit(o, err)
		// Тело НЕ печатается ни при какой погоде: в нём весь профиль. Оно
		// разбирается и уезжает в файл.
		var t struct {
			Profil string `json:"profil"`
		}
		if err := json.Unmarshal(o.Telo, &t); err != nil {
			vyhod("ответ службы не разобран")
		}
		blob, err := base64.StdEncoding.DecodeString(t.Profil)
		if err != nil {
			vyhod("профиль не декодируется")
		}
		// 0600 бессмысленны на NTFS без ACL, но файл в любом случае не должен
		// создаваться с правами каталога, а перезаписывать чужой профиль молча
		// эта команда не имеет права.
		if err := os.WriteFile(kuda, blob, 0o600); err != nil {
			vyhod(fmt.Sprintf("файл не записан: %v", err))
		}
		fmt.Printf("профиль записан: %s, %d байт\n", kuda, len(blob))

	case "import":
		if otkuda == "" {
			vyhod("profile import ждёт --in <файл>")
		}
		blob, err := os.ReadFile(otkuda)
		if err != nil {
			vyhod(fmt.Sprintf("файл не прочитан: %v", err))
		}
		parol, err := sprositParol("Пароль профиля: ")
		if err != nil {
			vyhod(err.Error())
		}
		pechat(k.Zvat("importProfile", map[string]string{
			"parol": parol, "profil": base64.StdEncoding.EncodeToString(blob)}))

	default:
		vyhod("profile принимает export и import")
	}
}

// pechat печатает ответ, но НЕ тело секретных кадров.
//
// Список секретных команд живёт в protokol рядом с проверкой, а не здесь:
// печать через полгода добавит кто-то другой и в другом месте.
func pechat(o protokol.Kadr, err error) {
	proverit(o, err)
	fmt.Println(dlyaPechati(o))
}

// dlyaPechati вынесена отдельно, потому что pechat зовёт os.Exit и проверить
// её тестом нечем. Решение «печатать или скрыть» проверять надо.
func dlyaPechati(o protokol.Kadr) string {
	if !protokol.TeloMozhnoLogirovat(o.Imya) {
		return "<скрыто>"
	}
	if len(o.Telo) == 0 {
		return "пусто"
	}
	var krasivo any
	if err := json.Unmarshal(o.Telo, &krasivo); err != nil {
		return string(o.Telo)
	}
	b, _ := json.MarshalIndent(krasivo, "", "  ")
	return string(b)
}

// tekstOtkaza строит строку, которой отказ уходит человеку, и отвечает, отказ
// ли это вообще.
//
// Вынесена отдельно по той же причине, что и dlyaPechati: proverit зовёт
// os.Exit и тестом не проверяется, а решение «каким видом отказ уходит наружу»
// проверять надо.
//
// Кадр смотрится ПЕРВЫМ, ошибка второй. kanal.Zvat отдаёт отказ И кадром, И
// ошибкой вида «код: текст», и при обратном порядке до скобок дело не доходило
// никогда: вид отказа зависел от того, какой барьер сработал раньше. Ветка со
// скобками была недостижима, хотя её объяснял целый абзац, а приёмка читает
// именно вид. Барьеров по-прежнему два: Zvat отдаёт обе половины, и достаточно
// одному вызывающему однажды проигнорировать вторую, чтобы отказ стал «пусто».
func tekstOtkaza(o protokol.Kadr, err error) (string, bool) {
	if o.Oshib != nil {
		return fmt.Sprintf("отказ [%s]: %s", o.Oshib.Kod, o.Oshib.Tekst), true
	}
	// Ошибка СВЯЗИ это не отказ службы: кода у неё нет, и выдумывать скобки не
	// из чего.
	if err != nil {
		return err.Error(), true
	}
	return "", false
}

// proverit валит выполнение на отказе службы.
func proverit(o protokol.Kadr, err error) {
	if tekst, ploho := tekstOtkaza(o, err); ploho {
		fmt.Fprintln(os.Stderr, tekst)
		os.Exit(1)
	}
}

func vyhod(tekst string) {
	fmt.Fprintln(os.Stderr, tekst)
	os.Exit(2)
}

var errParolPust = errors.New("пароль пуст")

// Needed to answer one question the guest checks ask: did the state event
// actually arrive, or did the status just happen to look right later.
func slushat(k *kanal.Klient, skolko time.Duration) {
	ctx, otmena := context.WithTimeout(context.Background(), skolko)
	defer otmena()
	for {
		select {
		case <-ctx.Done():
			return
		case kadr, ok := <-k.Sobytiya():
			if !ok {
				return
			}
			fmt.Printf("%s %s\n", kadr.Imya, protokol.TeloDlyaZhurnala(kadr.Imya, kadr.Telo))
		}
	}
}
