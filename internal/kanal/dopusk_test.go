package kanal

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

func TestDopuskIzPustogoKontekstaNeAdmin(t *testing.T) {
	// Отсутствие значения обязано читаться как «не админ». Толковать пустой
	// контекст в пользу вызывающего значит раздавать права по недосмотру, а
	// выглядит это как работающая программа.
	if DopuskIz(context.Background()).Admin {
		t.Fatal("контекст без допуска сошёл за админский")
	}
}

func TestDopuskEdetVKontekste(t *testing.T) {
	ctx := SDopuskom(context.Background(), Dopusk{Admin: true, Sid: "S-1-5-18"})
	d := DopuskIz(ctx)
	if !d.Admin || d.Sid != "S-1-5-18" {
		t.Fatalf("допуск не доехал: %+v", d)
	}
}

// TestSveritKlientaVidyotAdminstvo проверяет то, чего не проверял НИ ОДИН тест
// до 01.09.2026: что админство клиента вообще вычисляется.
//
// Мутационный прогон показал, что удаление проверки членства в
// Administrators не замечает никто. Цена ошибки прямая: экспорт профиля отдаёт
// все ключи, и если админство всегда ложно, команда не работает ни у кого, а
// если всегда истинно, работает у всех.
func TestSveritKlientaVidyotAdminstvo(t *testing.T) {
	admin, err := etoAdmin()
	if err != nil {
		t.Skipf("не удалось узнать собственное админство: %v", err)
	}

	imya := fmt.Sprintf(`\\.\pipe\affory-dopusk-%d`, os.Getpid())
	// Дескриптор без владельца SYSTEM: поставить его может только SYSTEM, а тест
	// идёт из-под обычного процесса. Проверяемое поведение к владельцу отношения
	// не имеет.
	l, err := slushatImenem(imya, "D:P(A;;GA;;;WD)")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	gotovo := make(chan Dopusk, 1)
	oshibki := make(chan error, 1)
	go func() {
		c, err := l.Accept()
		if err != nil {
			oshibki <- err
			return
		}
		defer c.Close()
		// СТРОГО после первого кадра: до него ImpersonateNamedPipeClient
		// отвечает ERROR_CANNOT_IMPERSONATE, и проверка стала бы декоративной.
		if _, err := ChitatKadr(c); err != nil {
			oshibki <- err
			return
		}
		d, err := SveritKlienta(c)
		if err != nil {
			oshibki <- err
			return
		}
		gotovo <- d
	}()

	ctx, otmena := context.WithTimeout(context.Background(), 5*time.Second)
	defer otmena()
	// НЕ DialPipeContext: он подключается на PipeImpLevelAnonymous, и тогда
	// OpenThreadToken отвечает 1347 (ERROR_BAD_IMPERSONATION_LEVEL). Это уже
	// записано в klient.go:98, и тест, звавший не тот Dial, повторил ошибку,
	// которую проект однажды разобрал.
	c, err := winio.DialPipeAccessImpLevel(ctx, imya,
		uint32(windows.GENERIC_READ|windows.GENERIC_WRITE), winio.PipeImpLevelImpersonation)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := PisatKadr(c, protokol.Kadr{Tip: "cmd", Id: 1, Imya: "hello"}); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-oshibki:
		t.Fatalf("сервер не свёл клиента: %v", err)
	case d := <-gotovo:
		if d.Sid == "" {
			t.Fatal("допуск без SID: в журнал будет нечего писать")
		}
		if d.Admin != admin {
			t.Fatalf("админство определено как %v, а процесс на самом деле admin=%v",
				d.Admin, admin)
		}
	case <-ctx.Done():
		t.Fatal("сервер не ответил")
	}
	_ = net.Conn(c)
}

// TestSveritKlientaNazyvaetProtsess закрывает долг 0 волны 6.
//
// 02.09.2026 набор на хосте получил Vybran и ручной режим, то есть кто-то
// послал setServer, и разбираться было нечем: журнал писал только старт, ядро и
// подписку. Строка «команда пришла от такого-то процесса» стоит один вызов, а
// её отсутствие стоило вечера догадок.
//
// Проверяется на СЕБЕ: клиент это сам тестовый процесс, его pid известен
// независимо от проверяемого кода.
func TestSveritKlientaNazyvaetProtsess(t *testing.T) {
	imya := fmt.Sprintf(`\\.\pipe\affory-protsess-%d`, os.Getpid())
	l, err := slushatImenem(imya, "D:P(A;;GA;;;WD)")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	gotovo := make(chan Dopusk, 1)
	oshibki := make(chan error, 1)
	go func() {
		c, err := l.Accept()
		if err != nil {
			oshibki <- err
			return
		}
		defer c.Close()
		if _, err := ChitatKadr(c); err != nil {
			oshibki <- err
			return
		}
		d, err := SveritKlienta(c)
		if err != nil {
			oshibki <- err
			return
		}
		gotovo <- d
	}()

	ctx, otmena := context.WithTimeout(context.Background(), 5*time.Second)
	defer otmena()
	c, err := winio.DialPipeAccessImpLevel(ctx, imya,
		uint32(windows.GENERIC_READ|windows.GENERIC_WRITE), winio.PipeImpLevelImpersonation)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := PisatKadr(c, protokol.Kadr{Tip: "cmd", Id: 1, Imya: "hello"}); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-oshibki:
		t.Fatalf("сервер не свёл клиента: %v", err)
	case d := <-gotovo:
		if d.Pid != uint32(os.Getpid()) {
			t.Fatalf("pid клиента %d, а на самом деле %d", d.Pid, os.Getpid())
		}
		svoy, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.EqualFold(d.Protsess, svoy) {
			t.Fatalf("процесс клиента %q, а на самом деле %q", d.Protsess, svoy)
		}
	case <-ctx.Done():
		t.Fatal("сервер не ответил")
	}
}

// etoAdmin спрашивает систему НАПРЯМУЮ, минуя проверяемый код.
//
// Спросить у него же значило бы сравнить код с самим собой.
func etoAdmin() (bool, error) {
	sid, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return false, err
	}
	return windows.Token(0).IsMember(sid)
}
