package kanal

import (
	"fmt"
	"net"
	"runtime"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

const ImyaKanala = `\\.\pipe\affory-v1`

// O:SY G:SY is not decoration and not a habit: a DACL-only descriptor sets no
// owner at all, and the owner then comes from the creating process token. For a
// service running as LocalSystem that is routinely BUILTIN\Administrators, not
// S-1-5-18 - so the client's owner check in task 1.4 would reject our OWN pipe
// and report a squatter that does not exist. Fixed here, at the source, rather
// than by weakening the check: an attacker with elevation can sit under
// Administrators too.
//
// SYSTEM and Administrators get everything; INTERACTIVE gets read and write.
// INTERACTIVE is wider than "the console user" and we know it: the descriptor is
// applied once, at listener creation, which happens before anybody logs in, so
// the console user's SID does not exist yet. Narrowing it would mean recreating
// the listener per session, which fights the first-instance protection below.
const sddl = "O:SYG:SYD:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GRGW;;;IU)"

// Squatting protection is a creation mode, not a flag: go-winio never calls
// CreateNamedPipe and never uses FILE_FLAG_FIRST_PIPE_INSTANCE. It calls
// ntCreateNamedPipeFile with disposition FILE_CREATE, which fails when the name
// is taken. Same effect, different spelling, and grepping for the flag is a
// waste of an afternoon.
func Slushat() (net.Listener, error) {
	return slushatImenem(ImyaKanala, sddl)
}

// Split out for the test, and for one measured reason: setting the owner to SYSTEM
// requires being SYSTEM. An ordinary process gets ERROR_INVALID_OWNER, so a test
// running on the host cannot use the production descriptor and would have to skip,
// which is a test that reports green without checking anything. The behaviour under
// test - the second listener on a taken name loses - belongs to the creation mode,
// not to the descriptor, so the test supplies its own name and a DACL-only SDDL.
func slushatImenem(imya, sd string) (net.Listener, error) {
	l, err := winio.ListenPipe(imya, &winio.PipeConfig{
		SecurityDescriptor: sd,
		MessageMode:        false,
		InputBufferSize:    64 << 10,
		OutputBufferSize:   64 << 10,
	})
	if err != nil {
		// Name taken means somebody else is pretending to be us. Dying is the
		// correct response: handing the pipe to a squatter would be worse.
		return nil, fmt.Errorf("%s: %w", protokol.KodPipeSquatted, err)
	}
	return l, nil
}

// Not in x/sys/windows and not in go-winio, checked by grep, not by memory. This
// is the third call in this project that looked obviously available and was not,
// so the declaration lives next to its only user instead of in some utility file
// where the next person assumes it came from the standard library.
var (
	advapi32                    = windows.NewLazySystemDLL("advapi32.dll")
	procImpersonateNamedPipeCli = advapi32.NewProc("ImpersonateNamedPipeClient")
)

func olicetvorit(h windows.Handle) error {
	r1, _, e := procImpersonateNamedPipeCli.Call(uintptr(h))
	if r1 == 0 {
		return e
	}
	return nil
}

// Dopusk это КТО пришёл, а не только «пускать ли».
//
// Канал пускает INTERACTIVE осознанно: интерфейс не должен требовать админа на
// каждый запуск. Но экспорт профиля отдаёт все ключи и адрес подписки, а импорт
// уводит весь трафик машины на чужой выход. Значит, две эти команды обязаны
// спрашивать отдельно, и для этого допуск нужен подробнее булева «да».
type Dopusk struct {
	Admin  bool
	System bool
	Sid    string

	// Pid и Protsess нужны журналу команд, а не проверке прав. 02.09.2026 набор
	// на хосте получил Vybran и ручной режим, то есть кто-то послал setServer, и
	// разбираться было нечем: SID сказал бы «это ты», а вопрос был «какая
	// программа». Путь полный, а не имя файла: `affory-cli.exe` из каталога
	// программы и он же, положенный куда попало, это разные новости.
	Pid      uint32
	Protsess string
}

// Called AFTER the hello frame has been read. Calling it earlier returns
// ERROR_CANNOT_IMPERSONATE (1368), because the server must have consumed client
// data first. This is documented, easy to miss, and the difference between a
// real check and a decorative one.
func SveritKlienta(c net.Conn) (Dopusk, error) {
	f, ok := c.(interface{ Fd() uintptr })
	if !ok {
		return Dopusk{}, fmt.Errorf("соединение не отдаёт дескриптор")
	}
	h := windows.Handle(f.Fd())

	// Клиент опознаётся ДО олицетворения. Под чужим лицом OpenProcess идёт с
	// правами клиента, и путь процесса службе может стать недоступен ровно там,
	// где он нужен: у процесса, пришедшего не из своего каталога.
	pid, put := ktoNaTomKontse(h)

	// Lock first, unlock last. The impersonation token belongs to the OS thread,
	// and a goroutine that migrates mid-check leaves a LocalSystem thread wearing
	// somebody else's face for the rest of the process lifetime.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := olicetvorit(h); err != nil {
		return Dopusk{}, fmt.Errorf("олицетворение не удалось: %w", err)
	}
	defer windows.RevertToSelf()

	var tok windows.Token
	if err := windows.OpenThreadToken(windows.CurrentThread(), windows.TOKEN_QUERY, true, &tok); err != nil {
		return Dopusk{}, fmt.Errorf("токен потока недоступен: %w", err)
	}
	defer tok.Close()

	u, err := tok.GetTokenUser()
	if err != nil {
		return Dopusk{}, fmt.Errorf("пользователь токена недоступен: %w", err)
	}
	// The ACL already keeps strangers out. This is the belt to those suspenders,
	// and it also gives us the SID to put in the journal when something odd
	// connects.
	d := Dopusk{Sid: u.User.Sid.String(), Pid: pid, Protsess: put}
	if u.User.Sid.IsWellKnown(windows.WinLocalSystemSid) {
		d.System, d.Admin = true, true
		return d, nil
	}
	// Админство считается ОТДЕЛЬНО от допуска, и обход цикла здесь не годится:
	// прежний код возвращался на первой же подошедшей группе, то есть для
	// интерактивного админа членство в Administrators даже не проверялось бы.
	if v, err := tokenVGruppe(tok, windows.WinBuiltinAdministratorsSid); err == nil && v {
		d.Admin = true
	}
	if d.Admin {
		return d, nil
	}
	if v, err := tokenVGruppe(tok, windows.WinInteractiveSid); err == nil && v {
		return d, nil
	}
	return Dopusk{}, fmt.Errorf("клиент не из допустимых: %s", d.Sid)
}

// ktoNaTomKontse отвечает pid и путь клиента, и НИКОГДА не отказом.
//
// Журнал без имени процесса хуже, чем журнал без строки вовсе: он выглядит
// полным. Но отказ здесь не повод не пустить клиента: команда должна работать и
// тогда, когда процесс успел завершиться между запросом и ответом.
//
// PROCESS_QUERY_LIMITED_INFORMATION, а не полный доступ: этого хватает для
// QueryFullProcessImageName и это единственное право, которое даётся к процессу
// в другом сеансе без лишних привилегий.
func ktoNaTomKontse(h windows.Handle) (uint32, string) {
	var pid uint32
	if err := windows.GetNamedPipeClientProcessId(h, &pid); err != nil {
		return 0, ""
	}
	p, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return pid, ""
	}
	defer windows.CloseHandle(p)
	buf := make([]uint16, windows.MAX_PATH)
	n := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(p, 0, &buf[0], &n); err != nil {
		return pid, ""
	}
	return pid, windows.UTF16ToString(buf[:n])
}

func tokenVGruppe(t windows.Token, tip windows.WELL_KNOWN_SID_TYPE) (bool, error) {
	sid, err := windows.CreateWellKnownSid(tip)
	if err != nil {
		return false, err
	}
	return t.IsMember(sid)
}
