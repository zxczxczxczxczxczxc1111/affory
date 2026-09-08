package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"runtime/debug"
	"sync"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/kanal"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// otdatDispetcheru is the seam the connection loop dispatches through.
//
// A package variable and not a field of Sluzhba: the struct lives in another
// file owned by another task, and a test-only flag inside it would sit dead in
// the shipped binary forever. Production reads the real dispatcher through this
// variable, so the seam is not dead code; the test replaces it to make a
// handler panic on demand, which no real command is allowed to do.
var otdatDispetcheru = (*Sluzhba).Obrabotat

// The accept loop belongs to no task in the plan: 1.3 built the listener, 1.9
// built the commands, and nobody wrote the part that puts a frame from one into
// the other. Written here, recorded in the plan, because a gap that compiles is
// a gap nobody notices.
func (s *Sluzhba) Obsluzhivat(ctx context.Context) error {
	l, err := kanal.Slushat()
	if err != nil {
		// A taken name means somebody is pretending to be us. Dying is correct:
		// handing our clients to a squatter is worse than not starting.
		return err
	}
	go func() {
		<-ctx.Done()
		_ = l.Close()
	}()
	for {
		c, err := l.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
			}
			return fmt.Errorf("канал не принимает: %w", err)
		}
		go s.obsluzhitOdnogo(ctx, c)
	}
}

func (s *Sluzhba) obsluzhitOdnogo(ctx context.Context, c net.Conn) {
	// Registered FIRST so it runs LAST: the connection still gets closed on the
	// way out of a panic, and only then is the panic stopped. Losing one client
	// is bad, losing the process is worse, because the process holds the tunnel.
	defer perehvatitPaniku("соединении")
	defer c.Close()

	var pishet sync.Mutex
	otpravit := func(k protokol.Kadr) error {
		pishet.Lock()
		defer pishet.Unlock()
		return kanal.PisatKadr(c, k)
	}

	pervyy, err := kanal.ChitatKadr(c)
	if err != nil {
		return
	}
	// AFTER the first frame, never before: ImpersonateNamedPipeClient returns
	// ERROR_CANNOT_IMPERSONATE until the server has consumed client data. This is
	// documented, easy to miss, and the difference between a real check and a
	// decorative one.
	dopusk, err := kanal.SveritKlienta(c)
	if err != nil {
		// The reason goes to the log, never to the wire: a rejected client is not
		// entitled to learn how the check works.
		log.Printf("клиент отвергнут на кадре %q: %v", pervyy.Imya, err)
		_ = otpravit(otkaz(pervyy.Id, pervyy.Imya, protokol.KodPipeSquatted, "клиент не допущен"))
		return
	}
	// Дальше по этому соединению команды видят, КТО пришёл. Контекст соединения,
	// а не поле службы: см. kanal/dopusk.go.
	ctx = kanal.SDopuskom(ctx, dopusk)

	if pervyy.Imya != "hello" {
		_ = otpravit(otkaz(pervyy.Id, pervyy.Imya, protokol.KodProtocolMismatch,
			"первым кадром должен быть hello"))
		return
	}
	if err := otpravit(s.Obrabotat(ctx, pervyy)); err != nil {
		return
	}

	id, sob := s.Podpisatsya()
	defer s.Otpisatsya(id)
	// Команды этого соединения знают свой id: подписка на статистику живёт с ним.
	ctx = sPodpischikom(ctx, id)
	go rassylat(sob, otpravit)

	for {
		k, err := kanal.ChitatKadr(c)
		if err != nil {
			return
		}
		// One goroutine per command on purpose: connect blocks for seconds while
		// it waits for the first probe, and a client that cannot ask for status
		// meanwhile is a client that draws a frozen window.
		go s.obsluzhitKomandu(ctx, k, otpravit)
	}
}

// obsluzhitKomandu runs one command and hands the answer back to the client.
//
// Named, not an inline closure, because the panic guard around it has to be
// reachable from a test: the loop above needs a real named pipe and a real
// client token, neither of which a host test can build.
func (s *Sluzhba) obsluzhitKomandu(ctx context.Context, k protokol.Kadr, otpravit func(protokol.Kadr) error) {
	_ = otpravit(s.obrabotatBezPaniki(ctx, k))
}

// obrabotatBezPaniki runs the dispatcher for one frame and turns a panic into a
// refusal frame.
//
// Answering matters as much as surviving: without a frame the client sits out
// its whole deadline and then reports "the service did not answer", which sends
// the human to fix the pipe instead of the command that broke.
//
// Код СВОЙ, а не protocol-mismatch. Полоса Г ставила второй, потому что строки
// «служба сломалась внутри» в §9.1 не было; экран на него говорит «служба и
// программа разных версий, обнови программу», и человек с упавшей командой шёл
// переустанавливать исправную программу. Версии тут сошлись, сломались мы, и
// делать человеку надо ровно одно: повторить.
func (s *Sluzhba) obrabotatBezPaniki(ctx context.Context, k protokol.Kadr) (otv protokol.Kadr) {
	defer func() {
		if r := recover(); r != nil {
			zapisatPaniku("команде "+k.Imya, r)
			otv = otkaz(k.Id, k.Imya, protokol.KodVnutrennyayaOshibka,
				"команда не выполнена: внутренняя ошибка службы")
		}
	}()
	return otdatDispetcheru(s, ctx, k)
}

// rassylat pushes events to one client until the subscription closes or the
// connection stops taking frames.
//
// Guard внутри, а не у вызывающего: горутина здесь одна, и оставить перехват
// снаружи значило бы завести место, где его можно забыть.
func rassylat(sob <-chan protokol.Kadr, otpravit func(protokol.Kadr) error) {
	// Отвечать некому: событие никто не ждёт по Id. Остаётся журнал и то, ради
	// чего всё затевалось, живой процесс с живым туннелем.
	defer perehvatitPaniku("рассылке событий")
	for k := range sob {
		if err := otpravit(k); err != nil {
			return
		}
	}
}

// perehvatitPaniku is deferred, never called directly: recover only works from
// the deferred function itself.
func perehvatitPaniku(gde string) {
	if r := recover(); r != nil {
		zapisatPaniku(gde, r)
	}
}

// zapisatPaniku turns a panic into a journal entry with the stack.
//
// A silently swallowed panic is worse than a crashed service: the crash at
// least says that something happened, while a swallowed one leaves a command
// that answers "no" for a reason nobody can ever find.
func zapisatPaniku(gde string, r any) {
	log.Printf("паника в %s: %v\n%s", gde, r, debug.Stack())
}
