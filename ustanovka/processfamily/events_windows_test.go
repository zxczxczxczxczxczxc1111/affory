package afforyprocess

import (
	"crypto/rand"
	"errors"
	"fmt"
	"github.com/0xrawsec/golang-etw/etw"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestShortEventHelper(t *testing.T) {
	if os.Getenv("AFFORY_SHORT_EVENT_HELPER") != "1" {
		t.Skip("helper only")
	}
}

func TestWindowsEventsCaptureExitedProcessWithoutPolling(t *testing.T) {
	g := NewGraph()
	events, err := newProcessEvents(g, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := events.Close(); err != nil {
			t.Error(err)
		}
	}()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(self, "-test.run=^TestShortEventHelper$")
	cmd.Env = append(os.Environ(), "AFFORY_SHORT_EVENT_HELPER=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	started := time.Now()
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	t.Logf("helper lifetime %v; no process polling in this test", time.Since(started))
	pid := uint32(cmd.Process.Pid)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		events.flush()
		if err := events.Err(); err != nil {
			t.Fatal(err)
		}
		g.mu.Lock()
		r := g.records[g.latest[pid]]
		g.mu.Unlock()
		if r.exited != 0 && len(r.paths) >= 2 {
			if r.process.Parent != uint32(os.Getpid()) || !strings.EqualFold(r.process.Path, self) {
				t.Fatalf("wrong event identity: %+v", r)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	t.Fatalf("short-lived process missing: %+v; error=%v", g.records[g.latest[pid]], events.Err())
}

func TestEventLifetimesRepairDelayedStartsWithoutPIDGuessing(t *testing.T) {
	g := NewGraph()
	root := Process{PID: 10, Created: 10, Path: `C:\Root.exe`}
	child := Process{PID: 20, Parent: 10, Created: 20, Path: `C:\Child.exe`}
	g.Started(root)
	g.Started(child)
	if len(g.Paths(child)) != 1 {
		t.Fatal("start alone invented a later parent lifetime")
	}
	g.Exited(root.PID, root.Created, 30)
	if len(g.Paths(child)) != 2 {
		t.Fatal("confirmed lifetime did not repair chain")
	}
	g.Started(Process{PID: 10, Created: 40, Path: `C:\Different.exe`})
	later := Process{PID: 30, Parent: 10, Created: 35, Path: `C:\Later.exe`}
	g.Started(later)
	if len(g.Paths(later)) != 1 {
		t.Fatal("event gap guessed old or new PID owner")
	}
	// Stop can be delivered before its matching start; identity is still exact.
	g.Exited(50, 50, 70)
	g.Started(Process{PID: 60, Parent: 50, Created: 60, Path: `C:\LateChild.exe`})
	g.Started(Process{PID: 50, Created: 50, Path: `C:\LateStart.exe`})
	if len(g.Paths(Process{PID: 60, Created: 60})) != 2 {
		t.Fatal("out-of-order start lost a confirmed lifetime")
	}
}

func TestLateObservationDoesNotExtendAProcessLifetime(t *testing.T) {
	g := NewGraph()
	launcher := Process{PID: 10, Created: 10, Path: `C:\Old.exe`, observed: 25}
	child := Process{PID: 20, Parent: 10, Created: 50, Path: `C:\App.exe`}
	g.Observe([]Process{launcher, child}, 100)
	if len(g.Paths(child)) != 1 {
		t.Fatal("delayed processing invented a later launcher lifetime")
	}
	g.Exited(launcher.PID, launcher.Created, 40)
	if len(g.Paths(child)) != 1 {
		t.Fatal("child born after exit inherited old PID")
	}
}

func TestPartlyDeliveredChainWaitsForTheRemainingLifetime(t *testing.T) {
	g := NewGraph()
	a := Process{PID: 10, Created: 10, Path: `C:\A.exe`}
	b := Process{PID: 20, Parent: 10, Created: 20, Path: `C:\B.exe`}
	c := Process{PID: 30, Parent: 20, Created: 30, Path: `C:\C.exe`}
	g.Started(a)
	g.Started(b)
	g.Started(c)
	g.Exited(b.PID, b.Created, 40)
	if len(g.Paths(c)) != 2 {
		t.Fatal("expected a partially delivered chain")
	}
	if g.Settled(c, 5) {
		t.Fatal("partial chain accepted before queued launcher exit")
	}
	g.Exited(a.PID, a.Created, 50)
	if !g.Settled(c, 5) || len(g.Paths(c)) != 3 {
		t.Fatal("completed chain was not propagated")
	}
}

// A chain whose launcher never shows up waits for queued events, then answers
// with its confirmed part. Refusing it forever broke every connection of that
// program until the service restarted (H4 of the 1.8.0 audit).
func TestUnconfirmedChainWaitsThenAnswersWithConfirmedPart(t *testing.T) {
	w, err := NewEventWatcher(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	started := time.Now()
	paths, err := w.resolve(Process{PID: 0xfffffffe, Parent: 0xfffffffd, Created: clockTicks(), Path: `C:\Pending.exe`})
	if err != nil {
		t.Fatalf("unconfirmed chain refused: %v", err)
	}
	if waited := time.Since(started); waited < settleWait {
		t.Fatalf("answered after %v without waiting for queued launch events", waited)
	}
	if len(paths) != 1 || paths[0] != `C:\Pending.exe` {
		t.Fatalf("invented lineage: %v", paths)
	}
}

func TestStoppedEventSessionIsReportedAsUnavailable(t *testing.T) {
	w, err := NewEventWatcher(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := w.Close(); err != nil {
			t.Error(err)
		}
	}()
	s := w.events
	s.control.Lock()
	err = etw.ControlTrace(s.session, nil, s.props, etw.EVENT_TRACE_CONTROL_STOP)
	s.control.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-s.done:
	case <-time.After(2 * time.Second):
		t.Fatal("stopped event stream did not exit")
	}
	if !errors.Is(s.Err(), ErrSharedUnavailable) {
		t.Fatalf("stopped event stream was treated as healthy: %v", s.Err())
	}
}

func shortEventRestarts(t *testing.T) {
	t.Helper()
	before := eventRestartDelays
	eventRestartDelays = []time.Duration{10 * time.Millisecond}
	t.Cleanup(func() { eventRestartDelays = before })
}

// H4 of the 1.8.0 audit: one lost event used to fail every request until the
// service restarted. Now the watcher answers from snapshots and starts a new
// event session.
func TestLostEventsFallBackToSnapshotsAndRestart(t *testing.T) {
	shortEventRestarts(t)
	w, err := NewEventWatcher(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	first := w.healthyEvents()
	first.fail(errors.New("injected event loss"))
	self, err := readProcess(uint32(os.Getpid()))
	if err != nil {
		t.Fatal(err)
	}
	parent, err := readProcess(self.Parent)
	if err != nil {
		t.Fatal(err)
	}
	api, err := NewSharedServer(w.FindIdentity)
	if err != nil {
		t.Fatal(err)
	}
	defer api.Close()
	client, err := NewSharedClient(api.Address, api.Secret)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	paths, err := client.Find(self.PID)
	if err != nil {
		t.Fatalf("lost events failed the request: %v", err)
	}
	if !slices.Contains(paths, self.Path) || !slices.Contains(paths, parent.Path) {
		t.Fatalf("snapshot answer lost the lineage: %v", paths)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		if again := w.healthyEvents(); again != nil && again != first {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("event session was not restarted")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, err := client.Find(self.PID); err != nil {
		t.Fatalf("restarted session: %v", err)
	}
}

func TestEventStartFailureStartsOnSnapshots(t *testing.T) {
	shortEventRestarts(t)
	before := startProcessEvents
	var failing atomic.Bool
	failing.Store(true)
	startProcessEvents = func(g *Graph, report func(error)) (*processEvents, error) {
		if failing.Load() {
			return nil, errors.New("injected start failure")
		}
		return before(g, report)
	}
	t.Cleanup(func() { startProcessEvents = before })
	w, err := NewEventWatcher(nil)
	if err != nil {
		t.Fatalf("event start failure failed the watcher: %v", err)
	}
	defer w.Close()
	if _, err := w.Find(uint32(os.Getpid())); err != nil {
		t.Fatalf("no answer from snapshots: %v", err)
	}
	failing.Store(false)
	deadline := time.Now().Add(10 * time.Second)
	for w.healthyEvents() == nil {
		if time.Now().After(deadline) {
			t.Fatal("event session never started")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestNoEventsAndNoSnapshotIsUnavailable(t *testing.T) {
	w := &Watcher{graph: NewGraph(), snap: &snapshotter{}, withEvents: true, done: make(chan struct{}), snapErr: errors.New("injected snapshot failure")}
	if _, err := w.Find(uint32(os.Getpid())); !errors.Is(err, ErrSharedUnavailable) {
		t.Fatalf("answered without events and snapshots: %v", err)
	}
}

func TestDOSPathConversionDoesNotGuessAnExecutable(t *testing.T) {
	for input, want := range map[string]string{`\??\C:\Apps\app.exe`: `C:\Apps\app.exe`, `\??\UNC\server\share\app.exe`: `\\server\share\app.exe`, `\Device\Mup\server\share\app.exe`: `\\server\share\app.exe`, `\??\relative.exe`: "", "app.exe": ""} {
		if got := dosEventPath(input); got != want {
			t.Fatalf("%q -> %q want %q", input, got, want)
		}
	}
}

func TestOrphanCleanupPreservesLiveAndUnrelatedTraces(t *testing.T) {
	live, err := newProcessEvents(NewGraph(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	if !ownedTraceName.MatchString(live.name) {
		t.Fatal("current trace cannot be recognized for crash cleanup")
	}
	for _, owned := range []bool{false, true} {
		name := "Affory-Test-Unrelated-" + rand.Text()
		if owned {
			name = fmt.Sprintf("Affory-Processes-%d-%016x-%s", uint32(0xfffffffe), uint64(1234), rand.Text())
		}
		props := etw.NewRealTimeEventTraceSessionProperties(name)
		props.Wnode.Flags = 0x00020000
		var handle syscall.Handle
		ptr, err := windows.UTF16PtrFromString(name)
		if err != nil {
			t.Fatal(err)
		}
		if err := etw.StartTrace(&handle, ptr, props); err != nil {
			t.Fatal(err)
		}
		func() {
			defer etw.ControlTrace(handle, nil, props, etw.EVENT_TRACE_CONTROL_STOP)
			if err := cleanupOrphanProcessTraces(); err != nil {
				t.Fatal(err)
			}
			err := etw.ControlTrace(handle, nil, props, etw.EVENT_TRACE_CONTROL_QUERY)
			if owned && !errors.Is(err, windows.ERROR_WMI_INSTANCE_NOT_FOUND) {
				t.Fatalf("orphan not reclaimed: %v", err)
			}
			if !owned && err != nil {
				t.Fatalf("unrelated trace changed: %v", err)
			}
		}()
	}
	live.flush()
	if err := live.Err(); err != nil {
		t.Fatalf("live owner trace was stopped: %v", err)
	}
}
