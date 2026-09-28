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
	graph  *Graph
	events *processEvents
	snap   *snapshotter
	cancel context.CancelFunc
	done   chan struct{}
}

func NewWatcher(report func(error)) (*Watcher, error) {
	return newWatcher(report, false)
}

func NewEventWatcher(report func(error)) (*Watcher, error) { return newWatcher(report, true) }

func newWatcher(report func(error), withEvents bool) (*Watcher, error) {
	g := NewGraph()
	var events *processEvents
	var err error
	if withEvents {
		events, err = newProcessEvents(g, report)
		if err != nil {
			return nil, err
		}
	}
	snap := &snapshotter{}
	at := clockTicks()
	initial, err := snap.take(g, nil)
	if err != nil {
		if events != nil {
			events.Close()
		}
		return nil, err
	}
	g.Observe(initial, at)
	ctx, cancel := context.WithCancel(context.Background())
	w := &Watcher{graph: g, events: events, snap: snap, cancel: cancel, done: make(chan struct{})}
	go func() {
		defer close(w.done)
		ticker := time.NewTicker(pollInterval(withEvents))
		defer ticker.Stop()
		failed := false
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if events != nil {
					events.flush()
				}
				at := clockTicks()
				processes, err := snap.take(g, nil)
				if err != nil {
					if !failed && report != nil {
						report(err)
					}
					failed = true
					continue
				}
				failed = false
				g.Observe(processes, at)
			}
		}
	}()
	return w, nil
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
	if w.events != nil {
		return w.events.Close()
	}
	return nil
}

func (w *Watcher) Find(pid uint32) ([]string, error) {
	if w.events != nil {
		if err := w.events.Err(); err != nil {
			return nil, err
		}
	}
	p, err := readProcess(pid)
	if err != nil {
		return nil, err
	}
	return w.resolve(p)
}

// FindIdentity is used across core restarts. A PID alone is not an identity.
func (w *Watcher) FindIdentity(pid uint32, created uint64) ([]string, error) {
	if w.events != nil {
		if err := w.events.Err(); err != nil {
			return nil, err
		}
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

func (w *Watcher) resolve(p Process) ([]string, error) {
	if w.events != nil {
		if err := w.events.Err(); err != nil {
			return nil, err
		}
	}
	paths := w.find(p)
	if w.events == nil {
		return paths, nil
	}
	paths, settled := w.graph.PathsSince(p, w.events.started)
	if settled {
		return paths, nil
	}
	// Give an already queued start/exit pair time to reach the consumer. A
	// vanished launcher cannot be recovered by another live-process snapshot.
	w.events.flush()
	deadline := time.NewTimer(750 * time.Millisecond)
	defer deadline.Stop()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for {
		if err := w.events.Err(); err != nil {
			return nil, err
		}
		if paths, settled = w.graph.PathsSince(p, w.events.started); settled {
			return paths, nil
		}
		select {
		case <-w.done:
			return nil, fmt.Errorf("%w: watcher stopped", ErrSharedUnavailable)
		case <-deadline.C:
			if paths, settled = w.graph.PathsSince(p, w.events.started); settled {
				return paths, nil
			}
			return nil, fmt.Errorf("%w: process launch information is not confirmed yet", ErrSharedUnavailable)
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
