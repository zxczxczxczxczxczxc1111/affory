package afforyprocess

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type Watcher struct {
	graph  *Graph
	cancel context.CancelFunc
	done   chan struct{}
}

func NewWatcher(report func(error)) (*Watcher, error) {
	g := NewGraph()
	initial, err := snapshot(g)
	if err != nil {
		return nil, err
	}
	g.Observe(initial, clockTicks())
	ctx, cancel := context.WithCancel(context.Background())
	w := &Watcher{graph: g, cancel: cancel, done: make(chan struct{})}
	go func() {
		defer close(w.done)
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		failed := false
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				processes, err := snapshot(g)
				if err != nil {
					if !failed && report != nil {
						report(err)
					}
					failed = true
					continue
				}
				failed = false
				g.Observe(processes, clockTicks())
			}
		}
	}()
	return w, nil
}

func (w *Watcher) Close() error {
	w.cancel()
	<-w.done
	return nil
}

func (w *Watcher) Find(pid uint32) ([]string, error) {
	p, err := readProcess(pid)
	if err != nil {
		return nil, err
	}
	if paths := w.graph.Paths(p); len(paths) > 1 {
		return paths, nil
	}
	chain := []Process{p}
	child := p
	for len(chain) < maxDepth && child.Parent != 0 && child.Parent != child.PID {
		parent, err := readProcess(child.Parent)
		if err != nil || parent.Created >= child.Created {
			break
		}
		chain = append(chain, parent)
		child = parent
	}
	w.graph.Observe(chain, clockTicks())
	return w.graph.Paths(p), nil
}

func clockTicks() uint64 {
	return uint64(time.Now().UnixNano()/100) + 116444736000000000
}

func readProcess(pid uint32) (Process, error) {
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
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &n); err != nil {
		return Process{}, err
	}
	return Process{PID: pid, Parent: uint32(basic.InheritedFromUniqueProcessId),
		Created: uint64(created.HighDateTime)<<32 | uint64(created.LowDateTime), Path: windows.UTF16ToString(buf[:n])}, nil
}

func snapshot(g *Graph) ([]Process, error) {
	var buf []byte
	var used uint32
	for size := uint32(1 << 20); ; {
		if size > 16<<20 {
			return nil, errors.New("process snapshot exceeds 16 MiB")
		}
		buf = make([]byte, size)
		err := windows.NtQuerySystemInformation(windows.SystemProcessInformation, unsafe.Pointer(&buf[0]), size, &used)
		if err == nil {
			break
		}
		if !errors.Is(err, windows.STATUS_INFO_LENGTH_MISMATCH) {
			return nil, err
		}
		size = max(size*2, used+65536)
	}
	defer runtime.KeepAlive(buf)
	var result []Process
	for offset := uint32(0); ; {
		if uint64(offset)+uint64(unsafe.Sizeof(windows.SYSTEM_PROCESS_INFORMATION{})) > uint64(len(buf)) {
			return nil, fmt.Errorf("invalid process snapshot offset %d", offset)
		}
		info := (*windows.SYSTEM_PROCESS_INFORMATION)(unsafe.Pointer(&buf[offset]))
		if info.UniqueProcessID > 4 && info.CreateTime > 0 {
			pid, created := uint32(info.UniqueProcessID), uint64(info.CreateTime)
			path := g.KnownPath(pid, created)
			if path == "" {
				if p, err := readProcess(pid); err == nil && p.Created == created {
					path = p.Path
				}
			}
			if path != "" {
				result = append(result, Process{PID: pid, Parent: uint32(info.InheritedFromUniqueProcessID), Created: created, Path: path})
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
