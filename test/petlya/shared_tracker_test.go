package petlya

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sagernet/sing-box/common/afforyprocess"
	"golang.org/x/sys/windows"
)

func TestSharedTrackerHelper(t *testing.T) {
	mode := os.Getenv("AFFORY_SHARED_MODE")
	if mode == "" {
		t.Skip("helper only")
	}
	if mode == "launcher" {
		cmd := exec.Command(os.Getenv("AFFORY_SHARED_LEAF"), "-test.run=^TestSharedTrackerHelper$", "-test.timeout=40s")
		for _, entry := range os.Environ() {
			if !strings.HasPrefix(entry, "AFFORY_SHARED_MODE=") {
				cmd.Env = append(cmd.Env, entry)
			}
		}
		cmd.Env = append(cmd.Env, "AFFORY_SHARED_MODE=leaf")
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		defer cmd.Process.Release()
		deadline := time.Now().Add(20 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(os.Getenv("AFFORY_SHARED_EXIT")); err == nil {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatal("launcher exit not requested")
	}
	conn, err := net.DialTimeout("tcp", os.Getenv("AFFORY_SHARED_CONTROL"), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(30 * time.Second))
	fmt.Fprintln(conn, os.Getpid())
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		parts := strings.SplitN(scanner.Text(), " ", 2)
		if len(parts) != 2 {
			t.Fatal("bad control command")
		}
		proxy, err := url.Parse(parts[0])
		if err != nil {
			t.Fatal(err)
		}
		transport := &http.Transport{Proxy: http.ProxyURL(proxy), DisableKeepAlives: true}
		client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
		response, err := client.Get(parts[1])
		result := "DENIED"
		if err == nil {
			data, readErr := io.ReadAll(response.Body)
			response.Body.Close()
			if readErr == nil && string(data) == "shared-target" {
				result = "OK"
			}
		}
		transport.CloseIdleConnections()
		fmt.Fprintln(conn, result)
	}
}

func TestSharedTrackerKeepsRunningApplicationAcrossCoreRestart(t *testing.T) {
	for _, events := range []bool{false, true} {
		t.Run(fmt.Sprintf("events=%v", events), func(t *testing.T) { checkSharedTrackerRestart(t, events) })
	}
}

func checkSharedTrackerRestart(t *testing.T, events bool) {
	Storozhit(t)
	newWatcher := afforyprocess.NewWatcher
	if events {
		newWatcher = afforyprocess.NewEventWatcher
	}
	var reportsMu sync.Mutex
	var reports []error
	trackerReports := func() []error { reportsMu.Lock(); defer reportsMu.Unlock(); return slices.Clone(reports) }
	w, err := newWatcher(func(err error) { reportsMu.Lock(); reports = append(reports, err); reportsMu.Unlock() })
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	// Without an event session the watcher falls back to polling by design, and
	// this subtest would check polling instead of events. Windows refuses the
	// session to a non-elevated account; any other refusal is a defect.
	if failed := trackerReports(); events && len(failed) > 0 {
		if errors.Is(failed[0], windows.ERROR_ACCESS_DENIED) {
			t.Skipf("process events need an administrator: %v", failed[0])
		}
		t.Fatalf("process events not running: %v", failed)
	}
	api, err := afforyprocess.NewSharedServer(w.FindIdentity)
	if err != nil {
		t.Fatal(err)
	}
	defer api.Close()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	launcher, leaf, exitFile := filepath.Join(dir, "launcher.exe"), filepath.Join(dir, "app.exe"), filepath.Join(dir, "exit")
	for _, path := range []string{launcher, leaf} {
		if err := os.WriteFile(path, data, 0700); err != nil {
			t.Fatal(err)
		}
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	listener.(*net.TCPListener).SetDeadline(time.Now().Add(5 * time.Second))
	cmd := exec.Command(launcher, "-test.run=^TestSharedTrackerHelper$", "-test.timeout=30s")
	cmd.Env = append(os.Environ(), "AFFORY_SHARED_MODE=launcher", "AFFORY_SHARED_LEAF="+leaf, "AFFORY_SHARED_EXIT="+exitFile, "AFFORY_SHARED_CONTROL="+listener.Addr().String())
	if events {
		if err := os.WriteFile(exitFile, []byte("exit immediately"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	launcherExited := false
	defer func() {
		if !launcherExited {
			cmd.Process.Kill()
			cmd.Wait()
		}
	}()
	conn, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(25 * time.Second))
	reader := bufio.NewReader(conn)
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.ParseUint(strings.TrimSpace(line), 10, 32)
	if err != nil {
		t.Fatal(err)
	}
	if events {
		if err := cmd.Wait(); err != nil {
			t.Fatal(err)
		}
		launcherExited = true
	} else {
		// Polling-only control deliberately observes the launcher while alive.
		paths, err := w.Find(uint32(pid))
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, path := range paths {
			found = found || strings.EqualFold(path, launcher)
		}
		if !found {
			t.Fatalf("live chain not captured: %v", paths)
		}
	}
	target := NovayaMishen(t, "shared-target")
	startCore := func(shared bool, finalDirect bool) *Yadro {
		rules := []any{map[string]any{"process_path_tree": []string{launcher}, "outbound": "direct"}}
		if !finalDirect {
			rules = append(rules, map[string]any{"action": "reject"})
		}
		route := map[string]any{"rules": rules, "final": "direct"}
		if shared {
			route["process_family_endpoint"] = api.Address
			route["process_family_secret"] = api.Secret
		}
		return PodnyatYadro(t, map[string]any{"inbounds": []any{map[string]any{"type": "mixed", "listen": "127.0.0.1", "listen_port": SvobodnyyPort(t)}}, "outbounds": []any{map[string]any{"type": "direct", "tag": "direct"}}, "route": route})
	}
	query := func(y *Yadro, want string) {
		if _, err := fmt.Fprintf(conn, "http://127.0.0.1:%d %s\n", y.PortProksi, target.Adres); err != nil {
			t.Fatal(err)
		}
		got, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(got) != want {
			t.Fatalf("application returned %q want %q; tracker reports %v\n%s", got, want, trackerReports(), y.Zhurnal())
		}
	}
	first := startCore(true, false)
	query(first, "OK")
	first.Pogasit()
	if err := os.WriteFile(exitFile, []byte("exit"), 0600); err != nil {
		t.Fatal(err)
	}
	if !launcherExited {
		if err := cmd.Wait(); err != nil {
			t.Fatal(err)
		}
	}
	launcherExited = true
	var session uint32
	if err := windows.ProcessIdToSessionId(uint32(os.Getpid()), &session); err != nil {
		t.Fatal(err)
	}
	views, err := w.Views(session)
	if err != nil {
		t.Fatal(err)
	}
	foundLiveChild := false
	for _, view := range views {
		if view.PID == uint32(cmd.Process.Pid) {
			t.Fatal("exited launcher shown as running")
		}
		if view.PID == uint32(pid) {
			for _, path := range view.Paths {
				foundLiveChild = foundLiveChild || strings.EqualFold(path, launcher)
			}
		}
	}
	if !foundLiveChild {
		t.Fatal("scope snapshot lost the app after launcher exit")
	}
	second := startCore(true, false)
	query(second, "OK")
	second.Pogasit()
	// Control: the same app with a fresh local tracker loses its exited launcher.
	local := startCore(false, false)
	query(local, "DENIED")
	local.Pogasit()
	// A tracker outage must reject, even when the default route permits traffic.
	final := startCore(true, true)
	query(final, "OK")
	if err := api.Close(); err != nil {
		t.Fatal(err)
	}
	query(final, "DENIED")
	final.Pogasit()
}
