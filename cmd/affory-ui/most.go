package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"golang.org/x/sys/windows"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/kanal"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Most is the whole surface the frontend gets. One method, on purpose.
//
// The shell does not know what "status" or "connect" mean; it moves frames.
// Every screen in wave 4 speaks to the service through this and nothing else,
// so swapping the shell later is this file plus main.go, not eleven screens.
//
// It is an interface, not just a struct, because the dead-export gate
// (internal/kachestvo/mertvyy_test.go) spares exported methods only when they
// satisfy an interface of this project. Nobody in Go calls Zvat; JS does.
type Most interface {
	Zvat(imya string, telo string) (string, error)
	// First run (§9.2): the shell must tell "no service" from "service not
	// answering" without admin, and start the install with one UAC prompt.
	SluzhbaUstanovlena() (bool, error)
	UstanovitSluzhbu() error
	// Uninstall (§"Удаление"): one elevated call, the keys question already
	// answered by the human. The window quits right after: its own directory
	// is about to go.
	UdalitProgrammu(steretKlyuchi bool) error
	// Update (wave 6.5): native archive dialog; "" when nothing was chosen.
	VybratArhiv() (string, error)
	// Профиль (03.09.2026): exportProfile и importProfile были только в CLI,
	// а окно про них не знало вовсе. Диалог и файл лежат тут, пароль сюда не
	// приходит никогда: он уходит телом команды по каналу.
	VybratKudaSohranit() (string, error)
	VybratOtkuda() (string, error)
	SohranitProfil(put string, profil string) error
	ProchitatProfil(put string) (string, error)
	// Rules form (§5 п.1): processes of THIS session, which the service in
	// session 0 cannot see.
	SpisokProtsessov() ([]Protsess, error)
	VybratPrilozhenie() (string, error)
	// Screen QR (six comforts §3): shoot every display, decode, addServer.
	// Returns the added server's name; the link never reaches the screen.
	DobavitSEkrana() (string, error)
	// Повышение прав окна (03.09.2026): служба часть команд отдаёт только
	// администратору, а окно стартует ярлыком без повышения, и без этого
	// метода действие «Повторить от администратора» ничего не делало.
	PerezapustitSPravami() error
}

func (m *most) UdalitProgrammu(steretKlyuchi bool) error {
	args := "uninstall"
	if steretKlyuchi {
		args += " --steret-klyuchi"
	}
	if err := zapustitSluzhbuSPravami(args); err != nil {
		return err
	}
	// Quit AFTER the launch returned: the service's cleanup waits three
	// seconds for this process to release its directory.
	go m.app.Quit()
	return nil
}

// VybratArhiv opens the native file dialog for an update archive (wave 6.5).
// Empty string means the human closed the dialog; that is not an error and
// no command is sent. The service does the sha256 check, not the window.
func (m *most) VybratArhiv() (string, error) {
	put, err := m.app.Dialog.OpenFile().
		SetTitle("Архив сборки Affory").
		AddFilter("Архив сборки (*.zip)", "*.zip").
		PromptForSingleSelection()
	return otvetDialoga(put, err)
}

// tekstOtmenyDialoga это сообщение отмены из go-common-file-dialog, который
// Wails держит во ВНУТРЕННЕМ пакете: импортировать его сентинел нельзя, а
// сверять текст можно. Сверка по тексту некрасива и честна, а прежнее
// «любая ошибка это отмена» не было ни тем, ни другим.
const tekstOtmenyDialoga = "cancelled by user"

// otvetDialoga отделяет закрытый человеком диалог от сломанного.
//
// Пустая строка означает ровно одно: человек передумал. Всё остальное едет
// наверх ошибкой, потому что до 03.09.2026 сорванный диалог возвращал ту же
// пустую строку, окно на неё не делало ничего, и кнопка «Выбрать архив»
// молча не работала.
func otvetDialoga(put string, err error) (string, error) {
	if err == nil {
		return put, nil
	}
	if strings.Contains(strings.ToLower(err.Error()), tekstOtmenyDialoga) {
		return "", nil
	}
	return "", err
}

// VybratKudaSohranit это диалог сохранения для файла профиля. Пустая строка
// означает, что человек передумал.
func (m *most) VybratKudaSohranit() (string, error) {
	// Заголовка у диалога сохранения в Wails v3 beta.16 нет: SetTitle есть
	// только у OpenFile, а SaveFileDialogStruct его не отдаёт. Роль подписи
	// берёт на себя SetMessage.
	put, err := m.app.Dialog.SaveFile().
		SetMessage("Куда вынести профиль Affory").
		SetFilename("profil.affory").
		AddFilter("Профиль Affory (*.affory)", "*.affory").
		PromptForSingleSelection()
	return otvetDialoga(put, err)
}

// VybratOtkuda это диалог открытия для файла профиля.
func (m *most) VybratOtkuda() (string, error) {
	put, err := m.app.Dialog.OpenFile().
		SetTitle("Откуда внести профиль Affory").
		AddFilter("Профиль Affory (*.affory)", "*.affory").
		PromptForSingleSelection()
	return otvetDialoga(put, err)
}

// SohranitProfil кладёт на диск то, что отдала команда exportProfile.
func (m *most) SohranitProfil(put string, profil string) error {
	return sohranitProfil(put, profil)
}

// ProchitatProfil поднимает файл профиля для команды importProfile.
func (m *most) ProchitatProfil(put string) (string, error) {
	return prochitatProfil(put)
}

// sohranitProfil пишет ИСХОДНЫЕ байты, а не base64: по каналу профиль едет
// в base64, потому что тело кадра это JSON, но файл должен совпадать байт в
// байт с тем, что пишет affory-cli profile export.
//
// Разбор до записи, а не после: молча записанный мусор человек обнаружит
// только при попытке восстановиться на другой машине.
func sohranitProfil(put string, profil string) error {
	blob, err := base64.StdEncoding.DecodeString(profil)
	if err != nil {
		return fmt.Errorf("профиль не декодируется: %w", err)
	}
	if len(blob) == 0 {
		return errors.New("профиль пуст: записывать нечего")
	}
	// 0600 бессмысленны на NTFS без ACL, но файл в любом случае не должен
	// создаваться с правами каталога.
	if err := os.WriteFile(put, blob, 0o600); err != nil {
		return fmt.Errorf("файл не записан: %w", err)
	}
	return nil
}

// prochitatProfil читает файл профиля и отдаёт его в base64 для тела команды.
func prochitatProfil(put string) (string, error) {
	blob, err := os.ReadFile(put)
	if err != nil {
		return "", fmt.Errorf("файл не прочитан: %w", err)
	}
	if len(blob) == 0 {
		return "", errors.New("файл профиля пуст")
	}
	return base64.StdEncoding.EncodeToString(blob), nil
}

// SluzhbaUstanovlena asks the service manager whether AfforySvc exists.
func (m *most) SluzhbaUstanovlena() (bool, error) {
	return sluzhbaUstanovlena(imyaSluzhby)
}

// UstanovitSluzhbu launches `affory-svc install` elevated. ShellExecute with
// the runas verb is the one UAC prompt §4.4 allows; the call returns as soon
// as the prompt is answered, so the screen polls status to learn the rest.
// The service binary is looked for next to this one: that is the install
// layout, and a UI running from somewhere else has no service to install.
func (m *most) UstanovitSluzhbu() error {
	return zapustitSluzhbuSPravami("install")
}

// zapustitSluzhbuSPravami runs affory-svc.exe from next to this binary with
// the given arguments, elevated. Returns once UAC is answered.
func zapustitSluzhbuSPravami(argumenty string) error {
	svoy, err := os.Executable()
	if err != nil {
		return fmt.Errorf("свой путь не читается: %w", err)
	}
	put := filepath.Join(filepath.Dir(svoy), "affory-svc.exe")
	if _, err := os.Stat(put); err != nil {
		return fmt.Errorf("рядом с программой нет affory-svc.exe: %w", err)
	}
	verb, _ := windows.UTF16PtrFromString("runas")
	fayl, _ := windows.UTF16PtrFromString(put)
	args, _ := windows.UTF16PtrFromString(argumenty)
	// SW_HIDE: the subcommand prints a line and exits; a console flashing on
	// top of the window would read as an error to anyone watching.
	if err := windows.ShellExecute(0, verb, fayl, args, nil, windows.SW_HIDE); err != nil {
		// ERROR_CANCELLED is the human pressing "No" on UAC. Not a crash.
		return fmt.Errorf("affory-svc %s не запущена: %w", argumenty, err)
	}
	return nil
}

const imyaSluzhby = "AfforySvc"

// sluzhbaUstanovlena needs only SC_MANAGER_CONNECT and SERVICE_QUERY_STATUS,
// which every user has. A missing service is false, not an error.
func sluzhbaUstanovlena(imya string) (bool, error) {
	m, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return false, fmt.Errorf("диспетчер служб недоступен: %w", err)
	}
	defer windows.CloseServiceHandle(m)
	imya16, err := windows.UTF16PtrFromString(imya)
	if err != nil {
		return false, err
	}
	h, err := windows.OpenService(m, imya16, windows.SERVICE_QUERY_STATUS)
	if err != nil {
		if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
			return false, nil
		}
		return false, fmt.Errorf("служба %s не открылась: %w", imya, err)
	}
	windows.CloseServiceHandle(h)
	return true, nil
}

// most talks to the service over the named pipe, reconnecting lazily.
type most struct {
	app *application.App
	// naStatus is the tray's ear: every status that passes through here,
	// answer or event, is repeated to it. nil until the tray exists.
	naStatus func(protokol.StatusOtvet)
	// okno is hidden for the screen shot and shown again; nil in tests.
	okno *application.WebviewWindow

	mu sync.Mutex
	k  *kanal.Klient
}

// podklyuchen reports whether the pipe is currently dialled.
func (m *most) podklyuchen() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.k != nil
}

// zvatFonovo runs a command for the tray: no answer wanted, the state event
// (or the answer's status, repeated to naStatus) is the whole result.
func (m *most) zvatFonovo(komanda string) {
	_, _ = m.Zvat(komanda, "")
}

// soobshchitStatus repeats a frame's status body to the tray, if it is one.
func (m *most) soobshchitStatus(kadr protokol.Kadr) {
	if m.naStatus == nil || len(kadr.Telo) == 0 {
		return
	}
	var st protokol.StatusOtvet
	if err := json.Unmarshal(kadr.Telo, &st); err != nil || st.Sostoyanie == "" {
		return
	}
	m.naStatus(st)
}

var _ Most = (*most)(nil)

// Zvat sends one command by name with a JSON body and returns the answer frame
// as JSON text. Strings on both sides, deliberately: the binding layer would
// otherwise type json.RawMessage as a byte array, and every screen would be
// decoding base64 by hand.
func (m *most) Zvat(imya string, telo string) (string, error) {
	k, err := m.klient()
	if err != nil {
		return "", err
	}
	var syroe json.RawMessage
	if telo != "" {
		syroe = json.RawMessage(telo)
	}
	otvet, err := k.Zvat(imya, syroe)
	if obryvKanala(otvet, err) {
		// A dead pipe stays dead. Drop the client so the next call redials
		// instead of reporting the same corpse forever.
		m.sbrosit(k)
		if m.naStatus != nil {
			m.naStatus(protokol.StatusOtvet{Sostoyanie: protokol.SostSluzhbaMolchit})
		}
		return "", err
	}
	// A refusal frame is an answer: the screen reads oshibka off the frame
	// and shows the §9.1 screen for it. The pipe stays open.
	m.soobshchitStatus(otvet)
	b, err := json.Marshal(otvet)
	if err != nil {
		return "", fmt.Errorf("ответ %s не сериализуется: %w", imya, err)
	}
	return string(b), nil
}

// otkazVOtvete достаёт отказ службы из ответа, отданного Zvat строкой.
//
// Zvat возвращает отказ КАДРОМ и без ошибки Go, и это правильно: экран читает
// код отказа из кадра и рисует по нему свой §9.1. Но вызывающему, которому
// нужен факт «сделано или нет», одного err мало, а тишина здесь означала бы
// успех по умолчанию.
func otkazVOtvete(otvet string) error {
	if otvet == "" {
		return fmt.Errorf("служба не ответила")
	}
	var k protokol.Kadr
	if err := json.Unmarshal([]byte(otvet), &k); err != nil {
		return fmt.Errorf("ответ службы неразборчив: %w", err)
	}
	if k.Oshib != nil {
		return fmt.Errorf("%s (%s)", k.Oshib.Tekst, k.Oshib.Kod)
	}
	return nil
}

// SnyatRezhim выключает режим «весь трафик» и отвечает, ПОЛУЧИЛОСЬ ЛИ.
//
// Той же командой, что и переключатель на экране настроек: два пути к одному
// поступку разошлись бы молча. Зовётся выходом из трея, которому нельзя
// закрывать программу над запертой машиной.
func (m *most) SnyatRezhim() error {
	otvet, err := m.Zvat("setKillSwitch", `{"vkl":false}`)
	if err != nil {
		return err
	}
	return otkazVOtvete(otvet)
}

// Otklyuchit опускает туннель и отвечает, ПОЛУЧИЛОСЬ ЛИ.
//
// Той же командой, что и кнопка «Отключить» на главном экране, и по той же
// причине, что и SnyatRezhim: два пути к одному поступку разошлись бы молча.
// Зовётся выходом из трея, которому с 10.09.2026 нельзя закрывать программу над
// поднятым туннелем: значка не останется, а трафик пойдёт.
//
// Отказ приезжает КАДРОМ без ошибки Go, поэтому одного err мало. Ровно на этом
// однажды закрылась программа над запертой машиной.
func (m *most) Otklyuchit() error {
	otvet, err := m.Zvat("disconnect", "")
	if err != nil {
		return err
	}
	return otkazVOtvete(otvet)
}

// obryvKanala tells a dead pipe from a refusal. kanal.Klient.Zvat returns
// the frame AND an error for a refusal; only an error without a frame is
// the transport failing. Closing the pipe on a refusal cost the screen every
// refusal frame and killed concurrent calls (seen live 02.09.2026).
func obryvKanala(otvet protokol.Kadr, err error) bool {
	return err != nil && otvet.Oshib == nil
}

// klient returns the live connection, dialing once if there is none.
func (m *most) klient() (*kanal.Klient, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.k != nil {
		return m.k, nil
	}
	k, err := kanal.Podklyuchitsya()
	if err != nil {
		return nil, err
	}
	m.k = k
	// Events flow one way, service to screen. Forward them as JSON text under
	// a single event name; the frontend fans them out by frame name.
	go m.peredavatSobytiya(k)
	return k, nil
}

// sbrosit forgets the client, but only if it is still the one that failed.
// A concurrent redial may already have replaced it.
func (m *most) sbrosit(k *kanal.Klient) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.k == k {
		_ = k.Zakryt()
		m.k = nil
	}
}

func (m *most) peredavatSobytiya(k *kanal.Klient) {
	for kadr := range k.Sobytiya() {
		b, err := json.Marshal(kadr)
		if err != nil {
			continue
		}
		if kadr.Imya == "state" {
			m.soobshchitStatus(kadr)
		}
		m.app.Event.Emit(sobytieKanala, string(b))
	}
	// Channel closed: the reader died, so the pipe is gone. Tell the screen
	// once, so it can show "служба молчит" instead of a stale status.
	m.sbrosit(k)
	m.app.Event.Emit(sobytieKanala, kanalZakrytKadr())
	if m.naStatus != nil {
		m.naStatus(protokol.StatusOtvet{Sostoyanie: protokol.SostSluzhbaMolchit})
	}
}

// sobytieKanala is the single Wails event name that carries service events.
const sobytieKanala = "kanal"

// sobytieOkna несёт ОДИН вопрос: видно окно или нет.
//
// Своё событие, а не родные WindowShow и WindowHide Wails, по двум причинам,
// и обе замерены. Первая записана рядом в Pokazat: при первом показе окна,
// созданного скрытым, WindowShow не приходит вовсе. Вторая наша: крестик у нас
// прячет окно в трей вместо закрытия, и с точки зрения родных событий это не
// отличается ни от чего.
const sobytieOkna = "okno"

// kanalZakrytKadr is the synthetic frame the shell emits when the pipe dies.
// It is shaped like a real event so the frontend needs no second code path.
func kanalZakrytKadr() string {
	b, _ := json.Marshal(protokol.Kadr{Tip: "sobytie", Imya: "kanal-zakryt"})
	return string(b)
}

// PerezapustitSPravami перезапускает ОКНО с повышением прав.
//
// Зачем. Служба намеренно отдаёт часть команд только администратору
// (см. komandyDlyaAdmina в affory-svc): угроза тут не человек за машиной, а
// другая программа, запущенная от него же. Но окно у человека стартует
// ярлыком и ключом Run, то есть БЕЗ повышения, и до 03.09.2026 кнопка
// «Повторить от администратора» не делала ничего вовсе: живой прогон в госте
// показал, что из обычного окна нельзя ни добавить сервер, ни удалить его, ни
// обновить подписку, ни включить автозапуск. Программа при этом честно писала
// «эта команда только для администратора машины» и не давала способа им стать.
//
// Повышается ОКНО, а не отдельная команда: служба смотрит на токен того, кто
// пришёл в канал, и разовое повышение одной команды ей не показать никак.
func (m *most) PerezapustitSPravami() error {
	svoy, err := os.Executable()
	if err != nil {
		return fmt.Errorf("свой путь не читается: %w", err)
	}
	verb, _ := windows.UTF16PtrFromString("runas")
	fayl, _ := windows.UTF16PtrFromString(svoy)
	if err := windows.ShellExecute(0, verb, fayl, nil, nil, windows.SW_SHOWNORMAL); err != nil {
		// Отказ от запроса прав это не поломка: человек нажал «Нет».
		return fmt.Errorf("повышение не состоялось: %w", err)
	}
	// Старое окно уходит: два экземпляра означают два трея, два наблюдателя и
	// две подписки на события службы. Пауза даёт новому окну стартовать, чтобы
	// человек не увидел пустой экран между ними.
	go func() {
		time.Sleep(700 * time.Millisecond)
		if p := application.Get(); p != nil {
			p.Quit()
		}
	}()
	return nil
}
