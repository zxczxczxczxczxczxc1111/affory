package afforyprocess

import (
	"os"
	"slices"
	"testing"
)

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
