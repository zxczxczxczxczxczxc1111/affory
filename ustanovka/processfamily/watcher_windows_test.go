package afforyprocess

import (
	"os"
	"runtime"
	"slices"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// The service polls four times a second. A fresh 1 MiB snapshot buffer on
// every poll was nearly all of its garbage.
func TestSnapshotReusesBuffer(t *testing.T) {
	w, err := NewWatcher(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	var session uint32
	if err := windows.ProcessIdToSessionId(uint32(os.Getpid()), &session); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Views(session); err != nil {
		t.Fatal(err)
	}
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	const n = 10
	for range n {
		if _, err := w.Views(session); err != nil {
			t.Fatal(err)
		}
	}
	runtime.ReadMemStats(&after)
	if per := (after.TotalAlloc - before.TotalAlloc) / n; per > 256<<10 {
		t.Fatalf("snapshot allocates %d KiB per call", per>>10)
	}
}

// Start and exit events keep the graph current; the poll only refreshes live
// processes well inside retentionTicks. Without events it is the only way to
// catch a short-lived launcher.
func TestPollIntervalFollowsEvents(t *testing.T) {
	if got := pollInterval(false); got != 250*time.Millisecond {
		t.Fatalf("poll without events every %v", got)
	}
	if got := pollInterval(true); got < time.Second || got > retentionTicks/10000000*time.Second/4 {
		t.Fatalf("poll with events every %v", got)
	}
}

func TestWindowsReadsRealProcessIdentityAndParent(t *testing.T) {
	p, err := readProcess(uint32(os.Getpid()))
	if err != nil {
		t.Fatal(err)
	}
	if p.Created == 0 || p.Parent != uint32(os.Getppid()) || p.Path == "" {
		t.Fatalf("invalid own identity: %+v", p)
	}
	w, err := NewWatcher(func(err error) { t.Errorf("snapshot: %v", err) })
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	paths, err := w.Find(p.PID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(paths, p.Path) {
		t.Fatalf("own process missing: %v", paths)
	}
	parent, err := readProcess(p.Parent)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(paths, parent.Path) {
		t.Fatalf("real parent missing: %v", paths)
	}
}
