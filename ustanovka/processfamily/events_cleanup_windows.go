package afforyprocess

import (
	"errors"
	"fmt"
	"regexp"
	"runtime"
	"strconv"
	"syscall"
	"unsafe"

	"github.com/0xrawsec/golang-etw/etw"
	"golang.org/x/sys/windows"
)

var queryAllTracesNative = windows.NewLazySystemDLL("advapi32.dll").NewProc("QueryAllTracesW")
var ownedTraceName = regexp.MustCompile(`^Affory-Processes-([0-9]+)-([0-9a-f]{16})-[A-Z2-7]{26,64}$`)

// ETW sessions can survive a crashed controller. Reclaim only our exact
// namespace and only after checking that its PID+creation-time owner is gone.
func cleanupOrphanProcessTraces() error {
	capacity := 128
	for attempt := 0; attempt < 2; attempt++ {
		buffers := make([][]byte, capacity)
		properties := make([]*etw.EventTraceProperties, capacity)
		for i := range buffers {
			buffers[i] = make([]byte, int(unsafe.Sizeof(etw.EventTraceProperties{}))+2048)
			properties[i] = (*etw.EventTraceProperties)(unsafe.Pointer(&buffers[i][0]))
			properties[i].Wnode.BufferSize = uint32(len(buffers[i]))
		}
		var count uint32
		code, _, _ := queryAllTracesNative.Call(uintptr(unsafe.Pointer(&properties[0])), uintptr(capacity), uintptr(unsafe.Pointer(&count)))
		runtime.KeepAlive(buffers)
		if code == uintptr(windows.ERROR_MORE_DATA) && count <= 1024 {
			capacity = int(count)
			continue
		}
		if code != 0 {
			return syscall.Errno(code)
		}
		if count > uint32(len(properties)) {
			return errors.New("invalid trace session count")
		}
		for i := 0; i < int(count); i++ {
			p := properties[i]
			offset := int(p.LoggerNameOffset)
			if offset < int(unsafe.Sizeof(*p)) || offset >= len(buffers[i]) || offset%2 != 0 {
				continue
			}
			name := windows.UTF16ToString(unsafe.Slice((*uint16)(unsafe.Pointer(&buffers[i][offset])), (len(buffers[i])-offset)/2))
			parts := ownedTraceName.FindStringSubmatch(name)
			if parts == nil {
				continue
			}
			pid, err := strconv.ParseUint(parts[1], 10, 32)
			if err != nil || pid <= 4 {
				continue
			}
			created, err := strconv.ParseUint(parts[2], 16, 64)
			if err != nil || created == 0 {
				continue
			}
			owner, readErr := readProcess(uint32(pid))
			if readErr == nil && owner.Created == created {
				continue
			}
			// Access denied and other inconclusive failures are not proof of exit.
			if readErr != nil && !errors.Is(readErr, windows.ERROR_INVALID_PARAMETER) && !errors.Is(readErr, errProcessExited) {
				continue
			}
			namePtr, err := windows.UTF16PtrFromString(name)
			if err != nil {
				return err
			}
			if err := etw.ControlTrace(0, namePtr, p, etw.EVENT_TRACE_CONTROL_STOP); err != nil && !errors.Is(err, windows.ERROR_WMI_INSTANCE_NOT_FOUND) {
				return fmt.Errorf("stop orphaned process tracking: %w", err)
			}
		}
		return nil
	}
	return errors.New("trace session list changed repeatedly")
}
