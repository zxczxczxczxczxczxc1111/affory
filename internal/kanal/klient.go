package kanal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// The client checks the server, not just the other way round. A squatter running
// as a normal user cannot make SYSTEM the owner of its pipe, so this one call
// rules out the whole class of "somebody got here first".
//
// This only works because the listener's SDDL starts with O:SYG:SY (task 1.3). A
// DACL-only descriptor leaves the owner to the creating token, which for a
// LocalSystem service is routinely BUILTIN\Administrators, and this check would
// then reject our own service every single time.
func proveritVladeltsa() error { return proveritVladeltsaPoImeni(ImyaKanala) }

// Split from the wrapper above so the test can aim it at a name we do not own,
// without standing up a real listener to be suspicious of.
func proveritVladeltsaPoImeni(imya string) error {
	sd, err := windows.GetNamedSecurityInfo(imya, windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		// Канала НЕТ это не захват канала. Служба остановлена или не
		// установлена, и по §9.1 это no-admin: «служба не установлена ИЛИ не
		// отвечает», экран первого запуска. Обвинять чужую программу в захвате
		// там, где захватывать нечего, значит посылать человека искать
		// несуществующего виновника (находка 25).
		if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND) {
			return fmt.Errorf("%s: служба не запущена", protokol.KodNoAdmin)
		}
		// А вот дальше именно захват. Скваттер волен отдать нам DACL без
		// READ_CONTROL, и тогда вызов падает невнятной ошибкой, которая человеку
		// не говорит ничего. Положение то же, код тот же: наше имя держит не наша
		// служба.
		return fmt.Errorf("%s: владелец канала не читается: %w", protokol.KodPipeSquatted, err)
	}
	vlad, _, err := sd.Owner()
	if err != nil {
		return fmt.Errorf("%s: владелец канала не разбирается: %w", protokol.KodPipeSquatted, err)
	}
	if !vlad.IsWellKnown(windows.WinLocalSystemSid) {
		return fmt.Errorf("%s: владелец канала %s", protokol.KodPipeSquatted, vlad)
	}
	return nil
}

type helloTelo struct {
	Protocol int `json:"protocol"`
}

// The whole point of the version field: turn a subtle mismatch between a new UI
// and an old service into one boring error the human can act on.
func proveritHello(k protokol.Kadr) error {
	var h helloTelo
	if err := json.Unmarshal(k.Telo, &h); err != nil {
		return fmt.Errorf("%s: приветствие не разбирается: %w", protokol.KodProtocolMismatch, err)
	}
	if h.Protocol != protokol.Versiya {
		return fmt.Errorf("%s: служба говорит на версии %d, интерфейс на %d",
			protokol.KodProtocolMismatch, h.Protocol, protokol.Versiya)
	}
	return nil
}

// Events are dropped, not queued forever, when nobody reads them. A UI that
// stopped reading is a UI that is gone, and an unbounded queue behind a dead
// reader is just a slow memory leak with extra steps.
const glubinaSobytiy = 64

type Klient struct {
	c net.Conn

	pishet sync.Mutex // one writer at a time: frames must not interleave

	mu    sync.Mutex
	sled  uint64
	zhdut map[uint64]chan protokol.Kadr

	sobytiya chan protokol.Kadr
	gotovo   chan struct{}
	odin     sync.Once
	pochemu  error
}

var ErrKanalZakryt = errors.New("канал закрыт")

func Podklyuchitsya() (*Klient, error) {
	if err := proveritVladeltsa(); err != nil {
		return nil, err
	}
	// DialPipe and every other DialPipe* helper connect at PipeImpLevelAnonymous
	// (winio pipe.go:273, stated outright in the comment above it). The server
	// then impersonates an anonymous token, and OpenThreadToken refuses it with
	// 1347, ERROR_CANT_OPEN_ANONYMOUS. Measured against the running service, not
	// deduced: every unit test passed because none of them ever connected.
	//
	// So the impersonation level is ours to choose, and choosing it is not a
	// detail: at anonymous level the whole client check in task 1.3 is decorative.
	ctx, otmena := context.WithTimeout(context.Background(), TaymautOtveta)
	defer otmena()
	c, err := winio.DialPipeAccessImpLevel(ctx, ImyaKanala,
		uint32(windows.GENERIC_READ|windows.GENERIC_WRITE), winio.PipeImpLevelImpersonation)
	if err != nil {
		return nil, fmt.Errorf("канал не открылся: %w", err)
	}
	k := &Klient{
		c:        c,
		zhdut:    make(map[uint64]chan protokol.Kadr),
		sobytiya: make(chan protokol.Kadr, glubinaSobytiy),
		gotovo:   make(chan struct{}),
	}
	go k.chitat()

	otvet, err := k.Zvat("hello", helloTelo{Protocol: protokol.Versiya})
	if err != nil {
		k.Zakryt()
		return nil, err
	}
	if err := proveritHello(otvet); err != nil {
		k.Zakryt()
		return nil, err
	}
	return k, nil
}

func (k *Klient) Sobytiya() <-chan protokol.Kadr { return k.sobytiya }

func (k *Klient) Zakryt() error {
	k.odin.Do(func() {
		if k.pochemu == nil {
			k.pochemu = ErrKanalZakryt
		}
		close(k.gotovo)
		_ = k.c.Close()
	})
	return nil
}

// Zvat sends one command and waits for the answer with that id. The deadline is
// per call, not per connection: an event subscription lives for hours and must
// not die because the first status took four seconds.
func (k *Klient) Zvat(imya string, telo any) (protokol.Kadr, error) {
	var pusto protokol.Kadr

	var syroe json.RawMessage
	if telo != nil {
		b, err := json.Marshal(telo)
		if err != nil {
			return pusto, fmt.Errorf("тело команды %s не сериализуется: %w", imya, err)
		}
		syroe = b
	}

	k.mu.Lock()
	select {
	case <-k.gotovo:
		k.mu.Unlock()
		return pusto, k.pochemu
	default:
	}
	k.sled++
	id := k.sled
	otvet := make(chan protokol.Kadr, 1)
	k.zhdut[id] = otvet
	k.mu.Unlock()

	defer func() {
		k.mu.Lock()
		delete(k.zhdut, id)
		k.mu.Unlock()
	}()

	k.pishet.Lock()
	_ = k.c.SetWriteDeadline(time.Now().Add(TaymautOtveta))
	err := PisatKadr(k.c, protokol.Kadr{Tip: "cmd", Id: id, Imya: imya, Telo: syroe})
	k.pishet.Unlock()
	if err != nil {
		return pusto, fmt.Errorf("команда %s не ушла: %w", imya, err)
	}

	select {
	case o := <-otvet:
		if o.Oshib != nil {
			return o, fmt.Errorf("%s: %s", o.Oshib.Kod, o.Oshib.Tekst)
		}
		return o, nil
	case <-k.gotovo:
		return pusto, k.pochemu
	case <-time.After(protokol.SrokOtveta(imya)):
		return pusto, fmt.Errorf("служба не ответила на %s за %s", imya, protokol.SrokOtveta(imya))
	}
}

func (k *Klient) chitat() {
	for {
		kadr, err := ChitatKadr(k.c)
		if err != nil {
			k.odin.Do(func() {
				k.pochemu = fmt.Errorf("канал оборвался: %w", err)
				close(k.gotovo)
				_ = k.c.Close()
			})
			return
		}
		if kadr.Tip == "sobytie" {
			select {
			case k.sobytiya <- kadr:
			default: // nobody is listening; see glubinaSobytiy
			}
			continue
		}
		k.mu.Lock()
		zhdet, est := k.zhdut[kadr.Id]
		k.mu.Unlock()
		if est {
			zhdet <- kadr
		}
		// An answer to an id nobody waits for is a timed-out call. Dropping it is
		// correct: the caller has already been told the service went quiet.
	}
}
