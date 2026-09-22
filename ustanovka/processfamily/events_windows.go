package afforyprocess

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"github.com/0xrawsec/golang-etw/etw"
	"golang.org/x/sys/windows"
)

var processProvider = etw.MustParseGUIDFromString("{22fb2cd6-0e7b-422b-a0c7-2fad1fd0e716}")
var eventRegistry sync.Map
var eventSequence atomic.Uint64
var eventCallbacks sync.Once
var recordCallback, bufferCallback uintptr
var openTraceNative = windows.NewLazySystemDLL("advapi32.dll").NewProc("OpenTraceW")

type processEvents struct {
	graph          *Graph
	started        uint64
	key            uintptr
	session, trace syscall.Handle
	name           string
	props          *etw.EventTraceProperties
	logfile        etw.EventTraceLogfile
	control        sync.Mutex
	mu             sync.Mutex
	err            error
	report         func(error)
	done           chan struct{}
	closing        atomic.Bool
	closeOnce      sync.Once
	closeErr       error
}

func newProcessEvents(g *Graph, report func(error)) (*processEvents, error) {
	owner, err := readProcess(uint32(os.Getpid()))
	if err != nil {
		return nil, err
	}
	if err := cleanupOrphanProcessTraces(); err != nil {
		return nil, fmt.Errorf("clean up stopped process tracking: %w", err)
	}
	s := &processEvents{graph: g, report: report, started: clockTicks(), name: fmt.Sprintf("Affory-Processes-%d-%016x-%s", os.Getpid(), owner.Created, rand.Text()), done: make(chan struct{})}
	s.key = uintptr(eventSequence.Add(1))
	s.props = etw.NewRealTimeEventTraceSessionProperties(s.name)
	s.props.Wnode.Flags = 0x00020000 // WNODE_FLAG_TRACED_GUID
	s.props.MinimumBuffers = 4
	s.props.MaximumBuffers = 32
	s.props.LogFileMode |= etw.EVENT_TRACE_NO_PER_PROCESSOR_BUFFERING
	s.props.FlushTimer = 1
	name, err := windows.UTF16PtrFromString(s.name)
	if err != nil {
		return nil, err
	}
	if err = etw.StartTrace(&s.session, name, s.props); err != nil {
		return nil, fmt.Errorf("start process events: %w", err)
	}
	// Stop only our newly created session handle, never a pre-existing logger.
	ok := false
	defer func() {
		if !ok {
			etw.ControlTrace(s.session, nil, s.props, etw.EVENT_TRACE_CONTROL_STOP)
		}
	}()
	params := &etw.EnableTraceParameters{Version: 2}
	if err = etw.EnableTraceEx2(s.session, processProvider, etw.EVENT_CONTROL_CODE_ENABLE_PROVIDER, 4, 0x10, 0, 0, params); err != nil {
		return nil, fmt.Errorf("enable process events: %w", err)
	}
	eventCallbacks.Do(func() {
		recordCallback = windows.NewCallback(func(r *etw.EventRecord) uintptr {
			if value, ok := eventRegistry.Load(r.UserContext); ok {
				value.(*processEvents).consume(r)
			}
			return 0
		})
		bufferCallback = windows.NewCallback(func(l *etw.EventTraceLogfile) uintptr {
			if value, ok := eventRegistry.Load(l.Context); ok {
				s := value.(*processEvents)
				if l.EventsLost != 0 {
					s.fail(errors.New("process events were lost"))
				}
				if !s.closing.Load() {
					return 1
				}
			}
			return 0
		})
	})
	s.logfile.LoggerName = name
	s.logfile.Context = s.key
	s.logfile.Callback = recordCallback
	s.logfile.BufferCallback = bufferCallback
	s.logfile.SetProcessTraceMode(etw.PROCESS_TRACE_MODE_EVENT_RECORD | etw.PROCESS_TRACE_MODE_REAL_TIME)
	eventRegistry.Store(s.key, s)
	// OpenTrace reports failure through INVALID_PROCESSTRACE_HANDLE, not a
	// stale last-error value after a successful call.
	h, _, callErr := openTraceNative.Call(uintptr(unsafe.Pointer(&s.logfile)))
	if h == ^uintptr(0) {
		eventRegistry.Delete(s.key)
		return nil, fmt.Errorf("open process events: %w", callErr)
	}
	s.trace = syscall.Handle(h)
	go func() {
		defer close(s.done)
		err := etw.ProcessTrace(&s.trace, 1, nil, nil)
		if !s.closing.Load() {
			if err == nil {
				err = errors.New("process event session ended unexpectedly")
			}
			s.fail(fmt.Errorf("consume process events: %w", err))
		}
	}()
	ok = true
	return s, nil
}
func (s *processEvents) fail(err error) {
	s.mu.Lock()
	first := s.err == nil
	if first {
		s.err = fmt.Errorf("%w: %v", ErrSharedUnavailable, err)
	}
	s.mu.Unlock()
	if first && s.report != nil {
		s.report(err)
	}
}
func (s *processEvents) Err() error { s.mu.Lock(); defer s.mu.Unlock(); return s.err }
func (s *processEvents) flush() {
	s.control.Lock()
	defer s.control.Unlock()
	if s.closing.Load() {
		return
	}
	if err := etw.ControlTrace(s.session, nil, s.props, etw.EVENT_TRACE_CONTROL_FLUSH); err != nil {
		s.fail(fmt.Errorf("flush process events: %w", err))
		return
	}
	if s.props.EventsLost != 0 || s.props.RealTimeBuffersLost != 0 || s.props.LogBuffersLost != 0 {
		s.fail(errors.New("process event buffers were lost"))
	}
}
func (s *processEvents) Close() error {
	s.closeOnce.Do(func() {
		s.closing.Store(true)
		s.control.Lock()
		s.closeErr = etw.ControlTrace(s.session, nil, s.props, etw.EVENT_TRACE_CONTROL_STOP)
		if errors.Is(s.closeErr, windows.ERROR_WMI_INSTANCE_NOT_FOUND) {
			s.closeErr = nil
		}
		s.control.Unlock()
		if err := etw.CloseTrace(s.trace); err != nil && err != etw.ERROR_CTX_CLOSE_PENDING && s.closeErr == nil {
			s.closeErr = err
		}
		<-s.done
		eventRegistry.Delete(s.key)
	})
	return s.closeErr
}

func eventProperty(r *etw.EventRecord, name string) ([]byte, error) {
	key, err := windows.UTF16FromString(name)
	if err != nil {
		return nil, err
	}
	defer runtime.KeepAlive(key)
	d := etw.PropertyDataDescriptor{PropertyName: uint64(uintptr(unsafe.Pointer(&key[0]))), ArrayIndex: ^uint32(0)}
	var size uint32
	if err := etw.TdhGetPropertySize(r, 0, nil, 1, &d, &size); err != nil {
		return nil, fmt.Errorf("process event field %s: %w", name, err)
	}
	if size == 0 || size > 131072 {
		return nil, fmt.Errorf("invalid process event field size: %s", name)
	}
	data := make([]byte, size)
	if err := etw.TdhGetProperty(r, 0, nil, 1, &d, size, &data[0]); err != nil {
		return nil, err
	}
	return data, nil
}
func eventNumber(r *etw.EventRecord, name string) (uint64, error) {
	b, err := eventProperty(r, name)
	if err != nil {
		return 0, err
	}
	switch len(b) {
	case 4:
		return uint64(binary.LittleEndian.Uint32(b)), nil
	case 8:
		return binary.LittleEndian.Uint64(b), nil
	default:
		return 0, fmt.Errorf("invalid numeric process event field %s", name)
	}
}
func (s *processEvents) consume(r *etw.EventRecord) {
	if !r.EventHeader.ProviderId.Equals(processProvider) || s.closing.Load() {
		return
	}
	id := r.EventHeader.EventDescriptor.Id
	if id != 1 && id != 2 {
		return
	}
	pid, err := eventNumber(r, "ProcessID")
	if err != nil {
		s.fail(err)
		return
	}
	if pid <= 4 || pid > uint64(^uint32(0)) {
		return
	}
	created, err := eventNumber(r, "CreateTime")
	if err != nil {
		s.fail(err)
		return
	}
	if id == 2 {
		exited, err := eventNumber(r, "ExitTime")
		if err != nil {
			s.fail(err)
			return
		}
		s.graph.Exited(uint32(pid), created, exited)
		return
	}
	parent, err := eventNumber(r, "ParentProcessID")
	if err != nil {
		s.fail(err)
		return
	}
	if parent > uint64(^uint32(0)) {
		s.fail(errors.New("invalid process event parent"))
		return
	}
	b, err := eventProperty(r, "ImageName")
	if err != nil {
		s.fail(err)
		return
	}
	if len(b)%2 != 0 {
		s.fail(errors.New("invalid process image name"))
		return
	}
	chars := make([]uint16, len(b)/2)
	for i := range chars {
		chars[i] = binary.LittleEndian.Uint16(b[2*i:])
	}
	p := Process{PID: uint32(pid), Parent: uint32(parent), Created: created, Path: dosEventPath(windows.UTF16ToString(chars))}
	if live, err := readProcess(p.PID); err == nil && live.Created == p.Created {
		p.Path = live.Path
	}
	if p.Path == "" {
		return
	} // Unknown volume/path is not an executable-name guess.
	at := clockTicks()
	var alive []Process
	child := p
	for len(alive) < maxDepth-1 && child.Parent > 4 && child.Parent != child.PID {
		parent, err := readProcess(child.Parent)
		if err != nil || parent.Created >= child.Created {
			break
		}
		alive = append(alive, parent)
		child = parent
	}
	s.graph.Observe(alive, at)
	s.graph.Started(p)
}

func dosEventPath(path string) string {
	if len(path) >= 3 && path[1] == ':' && path[2] == '\\' {
		return path
	}
	if strings.HasPrefix(path, `\??\UNC\`) {
		return `\\` + strings.TrimPrefix(path, `\??\UNC\`)
	}
	if strings.HasPrefix(path, `\??\`) {
		return dosEventPath(strings.TrimPrefix(path, `\??\`))
	}
	if strings.HasPrefix(path, `\Device\Mup\`) {
		return `\\` + strings.TrimPrefix(path, `\Device\Mup\`)
	}
	drives, err := windows.GetLogicalDrives()
	if err != nil {
		return ""
	}
	for i := 0; i < 26; i++ {
		if drives&(1<<i) == 0 {
			continue
		}
		drive := fmt.Sprintf("%c:", 'A'+i)
		name, _ := windows.UTF16PtrFromString(drive)
		buf := make([]uint16, 32768)
		if _, err := windows.QueryDosDevice(name, &buf[0], uint32(len(buf))); err != nil {
			continue
		}
		device := windows.UTF16ToString(buf)
		if len(path) > len(device) && strings.EqualFold(path[:len(device)], device) && path[len(device)] == '\\' {
			return drive + path[len(device):]
		}
	}
	return ""
}
