package main

import (
	"context"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

// Н11 аудита 1.6.1. После сна туннель проверялся только на следующем тике
// наблюдателя, до минуты спустя: всё это время мёртвый туннель числился живым.
func TestProbuzhdenieZapuskaetVneocherednuyuProbu(t *testing.T) {
	s := podstavnaya(t, nil)
	s.period = time.Hour
	s.periodNesushchego = time.Hour
	var zamerov atomic.Int32
	s.zamerit = func(context.Context, string, string, string) (time.Duration, error) {
		zamerov.Add(1)
		return 42 * time.Millisecond, nil
	}
	if err := s.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	do := zamerov.Load()
	s.ProbaPosleSna()
	dozhdatsya(t, "внеочередная проба после сна", func() bool { return zamerov.Load() > do })
}

func TestSobytiyaSCM(t *testing.T) {
	const pbtApmResumeSuspend = 0x7
	if !sonZakonchen(svc.ChangeRequest{Cmd: svc.PowerEvent, EventType: pbtApmResumeAutomatic}) {
		t.Fatal("пробуждение не узнано")
	}
	if sonZakonchen(svc.ChangeRequest{Cmd: svc.PowerEvent, EventType: pbtApmResumeSuspend}) {
		t.Fatal("пробуждение по действию человека дублирует автоматическое: проба ушла бы дважды")
	}
	for _, c := range []svc.Cmd{svc.Stop, svc.Shutdown, svc.PreShutdown} {
		if !ostanovka(svc.ChangeRequest{Cmd: c}) {
			t.Fatalf("команда %d не останавливает службу", c)
		}
	}
	if priemlet&svc.AcceptPreShutdown == 0 || priemlet&svc.AcceptPowerEvent == 0 {
		t.Fatal("служба не принимает предвыключение или события питания")
	}
}

// Служба зависит только от BFE: без него брандмауэр не принимает правил, а
// первое, что служба делает на старте, это уборка осиротевших правил.
func TestSluzhbaZavisitTolkoOtBFE(t *testing.T) {
	if got := nastroykiSluzhby().Dependencies; !slices.Equal(got, []string{"BFE"}) {
		t.Fatalf("зависимости новой службы %v", got)
	}
	if got := obnovitNastroyki(mgr.Config{Dependencies: []string{"Tcpip", "Dhcp"}}, `C:\A\affory-svc.exe`).Dependencies; !slices.Equal(got, []string{"BFE"}) {
		t.Fatalf("установка поверх оставила зависимости %v", got)
	}
}
