package afforyprocess

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type Watcher struct {
	graph      *Graph
	snap       *snapshotter
	report     func(error)
	withEvents bool
	cancel     context.CancelFunc
	done       chan struct{}

	mu sync.Mutex
	// events is nil while a failed session waits for its restart; the watcher
	// then polls snapshots like one created without events.
	events  *processEvents
	snapErr error
}

// A failed event session is not permanent: the watcher falls back to snapshot
// polling and starts a new session after these delays. A variable for tests.
var eventRestartDelays = []time.Duration{5 * time.Second, 30 * time.Second, 5 * time.Minute}

// A session that stayed healthy this long starts the delays over.
const eventHealthyReset = 10 * time.Minute

var startProcessEvents = newProcessEvents

func NewWatcher(report func(error)) (*Watcher, error) {
	return newWatcher(report, false)
}

func NewEventWatcher(report func(error)) (*Watcher, error) { return newWatcher(report, true) }

func newWatcher(report func(error), withEvents bool) (*Watcher, error) {
	g := NewGraph()
	w := &Watcher{graph: g, snap: &snapshotter{}, report: report, withEvents: withEvents, done: make(chan struct{})}
	var restart eventRestart
	if withEvents {
		events, err := startProcessEvents(g, report)
		if err != nil {
			wait := restart.schedule(time.Now())
			w.warn(fmt.Errorf("process events not started, snapshot polling until retry in %s: %w", wait, err))
		}
		w.events = events
	}
	at := clockTicks()
	initial, err := w.snap.take(g, nil)
	if err != nil {
		if w.events != nil {
			w.events.Close()
		}
		return nil, err
	}
	g.Observe(initial, at)
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	go w.run(ctx, restart)
	return w, nil
}

type eventRestart struct {
	at, started time.Time
	step        int
}

func (r *eventRestart) schedule(now time.Time) time.Duration {
	if !r.started.IsZero() && now.Sub(r.started) >= eventHealthyReset {
		r.step = 0
	}
	wait := eventRestartDelays[min(r.step, len(eventRestartDelays)-1)]
	r.step++
	r.at = now.Add(wait)
	return wait
}

func (w *Watcher) run(ctx context.Context, restart eventRestart) {
	defer close(w.done)
	timer := time.NewTimer(pollInterval(w.healthyEvents() != nil))
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		if w.withEvents {
			w.superviseEvents(&restart)
		}
		at := clockTicks()
		processes, err := w.snap.take(w.graph, nil)
		w.mu.Lock()
		first := err != nil && w.snapErr == nil
		w.snapErr = err
		w.mu.Unlock()
		if first {
			w.warn(err)
		}
		if err == nil {
			w.graph.Observe(processes, at)
		}
		timer.Reset(pollInterval(w.healthyEvents() != nil))
	}
}

// superviseEvents replaces a failed event session: close it, poll snapshots,
// start a new one when its delay is over.
func (w *Watcher) superviseEvents(restart *eventRestart) {
	w.mu.Lock()
	events := w.events
	w.mu.Unlock()
	now := time.Now()
	if events != nil {
		events.flush()
		err := events.Err()
		if err == nil {
			return
		}
		w.mu.Lock()
		w.events = nil
		w.mu.Unlock()
		if closeErr := events.Close(); closeErr != nil {
			w.warn(fmt.Errorf("close failed process events: %w", closeErr))
		}
		wait := restart.schedule(now)
		w.warn(fmt.Errorf("process events stopped, snapshot polling until restart in %s: %w", wait, err))
		return
	}
	if now.Before(restart.at) {
		return
	}
	events, err := startProcessEvents(w.graph, w.report)
	if err != nil {
		wait := restart.schedule(now)
		w.warn(fmt.Errorf("process events not restarted, next attempt in %s: %w", wait, err))
		return
	}
	w.mu.Lock()
	w.events = events
	w.mu.Unlock()
	restart.started = now
	w.warn(errors.New("process events restarted"))
}

func (w *Watcher) warn(err error) {
	if w.report != nil {
		w.report(err)
	}
}

// healthyEvents is the event session when it is running and has lost nothing.
func (w *Watcher) healthyEvents() *processEvents {
	w.mu.Lock()
	events := w.events
	w.mu.Unlock()
	if events == nil || events.Err() != nil {
		return nil
	}
	return events
}

// unavailable reports what makes an answer impossible: a stopped watcher, or
// an event watcher left with neither events nor a working snapshot.
func (w *Watcher) unavailable() error {
	select {
	case <-w.done:
		return fmt.Errorf("%w: watcher stopped", ErrSharedUnavailable)
	default:
	}
	if !w.withEvents || w.healthyEvents() != nil {
		return nil
	}
	w.mu.Lock()
	err := w.snapErr
	w.mu.Unlock()
	if err != nil {
		return fmt.Errorf("%w: no process events and no snapshot: %v", ErrSharedUnavailable, err)
	}
	return nil
}

// Err is non-nil once the watcher can no longer answer at all.
func (w *Watcher) Err() error {
	select {
	case <-w.done:
		return fmt.Errorf("%w: watcher stopped", ErrSharedUnavailable)
	default:
		return nil
	}
}

// Without events only the poll catches a short-lived launcher. With them start
// and exit events keep the graph current, and the poll merely refreshes live
// processes well inside retentionTicks.
func pollInterval(withEvents bool) time.Duration {
	if withEvents {
		return 2 * time.Second
	}
	return 250 * time.Millisecond
}

func (w *Watcher) Close() error {
	w.cancel()
	<-w.done
	w.mu.Lock()
	events := w.events
	w.events = nil
	w.mu.Unlock()
	if events != nil {
		return events.Close()
	}
	return nil
}

func (w *Watcher) Find(pid uint32) ([]string, error) {
	if err := w.unavailable(); err != nil {
		return nil, err
	}
	p, err := readProcess(pid)
	if err != nil {
		return nil, err
	}
	return w.resolve(p)
}

// FindIdentity is used across core restarts. A PID alone is not an identity.
func (w *Watcher) FindIdentity(pid uint32, created uint64) ([]string, error) {
	if err := w.unavailable(); err != nil {
		return nil, err
	}
	p, err := readProcess(pid)
	if err != nil {
		return nil, err
	}
	if p.Created != created {
		return nil, errors.New("process identity changed")
	}
	return w.resolve(p)
}

// Launch information queued in the event stream gets this long to arrive.
const settleWait = 750 * time.Millisecond

func (w *Watcher) resolve(p Process) ([]string, error) {
	paths := w.find(p)
	events := w.healthyEvents()
	if events == nil {
		// Without events the snapshot graph is the answer, as for a watcher
		// created without them.
		if err := w.unavailable(); err != nil {
			return nil, err
		}
		return paths, nil
	}
	paths, settled := w.graph.PathsSince(p, events.started)
	if settled {
		return paths, nil
	}
	// Give an already queued start/exit pair time to reach the consumer. A
	// vanished launcher cannot be recovered by another live-process snapshot.
	events.flush()
	deadline := time.NewTimer(settleWait)
	defer deadline.Stop()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for {
		if events.Err() != nil {
			// The session failed while we waited: the watcher is on snapshots now.
			if err := w.unavailable(); err != nil {
				return nil, err
			}
			return w.graph.Paths(p), nil
		}
		if paths, settled = w.graph.PathsSince(p, events.started); settled {
			return paths, nil
		}
		select {
		case <-w.done:
			return nil, fmt.Errorf("%w: watcher stopped", ErrSharedUnavailable)
		case <-deadline.C:
			// After a flush and the wait the stream has delivered what it had.
			// A launcher that ended before the tracker saw it never arrives, and
			// refusing every connection of that program until the service
			// restarts is worse than routing by the confirmed part of the chain.
			paths, _ = w.graph.PathsSince(p, events.started)
			return paths, nil
		case <-tick.C:
		}
	}
}

func (w *Watcher) find(p Process) []string {
	if paths := w.graph.Paths(p); len(paths) > 1 {
		w.graph.Observe([]Process{p}, p.observed)
		return paths
	}
	chain := []Process{p}
	at := clockTicks()
	child := p
	for len(chain) < maxDepth && child.Parent != 0 && child.Parent != child.PID {
		parent, err := readProcess(child.Parent)
		if err != nil || parent.Created >= child.Created {
			break
		}
		chain = append(chain, parent)
		child = parent
	}
	w.graph.Observe(chain, at)
	return w.graph.Paths(p)
}

func clockTicks() uint64 {
	return uint64(time.Now().UnixNano()/100) + 116444736000000000
}

var errProcessExited = errors.New("process has exited")

func readProcess(pid uint32) (Process, error) {
	observed := clockTicks()
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return Process{}, err
	}
	defer windows.CloseHandle(h)
	var basic windows.PROCESS_BASIC_INFORMATION
	size := uint32(unsafe.Sizeof(basic))
	if err := windows.NtQueryInformationProcess(h, windows.ProcessBasicInformation, unsafe.Pointer(&basic), size, &size); err != nil {
		return Process{}, err
	}
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &created, &exited, &kernel, &user); err != nil {
		return Process{}, err
	}
	if exited.HighDateTime != 0 || exited.LowDateTime != 0 {
		return Process{}, errProcessExited
	}
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &n); err != nil {
		return Process{}, err
	}
	return Process{PID: pid, Parent: uint32(basic.InheritedFromUniqueProcessId),
		Created: uint64(created.HighDateTime)<<32 | uint64(created.LowDateTime), Path: windows.UTF16ToString(buf[:n]), observed: observed}, nil
}

// snapshotter keeps one buffer between polls: the returned processes never
// point into it, so the next snapshot may overwrite it.
type snapshotter struct {
	mu  sync.Mutex
	buf []byte
}

func (s *snapshotter) take(g *Graph, session *uint32) ([]Process, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var used uint32
	for size := max(uint32(len(s.buf)), 1<<20); ; {
		if size > 16<<20 {
			return nil, errors.New("process snapshot exceeds 16 MiB")
		}
		if uint32(len(s.buf)) < size {
			s.buf = make([]byte, size)
		}
		err := windows.NtQuerySystemInformation(windows.SystemProcessInformation, unsafe.Pointer(&s.buf[0]), size, &used)
		if err == nil {
			break
		}
		if !errors.Is(err, windows.STATUS_INFO_LENGTH_MISMATCH) {
			return nil, err
		}
		size = max(size*2, used+65536)
	}
	buf := s.buf
	defer runtime.KeepAlive(buf)
	var result []Process
	for offset := uint32(0); ; {
		if uint64(offset)+uint64(unsafe.Sizeof(windows.SYSTEM_PROCESS_INFORMATION{})) > uint64(len(buf)) {
			return nil, fmt.Errorf("invalid process snapshot offset %d", offset)
		}
		info := (*windows.SYSTEM_PROCESS_INFORMATION)(unsafe.Pointer(&buf[offset]))
		if info.UniqueProcessID > 4 && info.CreateTime > 0 && info.NumberOfThreads > 0 && (session == nil || info.SessionID == *session) {
			pid, created := uint32(info.UniqueProcessID), uint64(info.CreateTime)
			path := g.KnownPath(pid, created)
			if path == "" {
				if p, err := readProcess(pid); err == nil && p.Created == created {
					path = p.Path
				}
			}
			if path != "" {
				result = append(result, Process{PID: pid, Parent: uint32(info.InheritedFromUniqueProcessID), Created: created, Path: path, Session: info.SessionID})
			}
		}
		if info.NextEntryOffset == 0 {
			break
		}
		if info.NextEntryOffset < uint32(unsafe.Sizeof(*info)) || uint64(offset)+uint64(info.NextEntryOffset) >= uint64(len(buf)) {
			return nil, errors.New("invalid process snapshot entry length")
		}
		offset += info.NextEntryOffset
	}
	return result, nil
}
