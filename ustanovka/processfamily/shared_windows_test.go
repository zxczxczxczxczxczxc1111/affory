package afforyprocess

import (
	"context"
	"errors"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestSharedGraphSurvivesCoreClientsAndRejectsReusedPID(t *testing.T) {
	g := NewGraph()
	launcher := Process{PID: 10, Created: 10, Path: `C:\Launcher.exe`}
	app := Process{PID: 20, Parent: 10, Created: 20, Path: `C:\App.exe`}
	g.Observe([]Process{launcher, app}, 30)
	api, err := NewSharedServer(func(pid uint32, created uint64) ([]string, error) {
		if pid != app.PID || created != app.Created {
			return nil, errors.New("different process")
		}
		return g.Paths(app), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := api.Close(); err != nil {
			t.Error(err)
		}
	})
	for i := 0; i < 2; i++ {
		client, err := NewSharedClient(api.Address, api.Secret)
		if err != nil {
			t.Fatal(err)
		}
		paths, err := client.find(context.Background(), app)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(paths, []string{app.Path, launcher.Path}) {
			t.Fatal(paths)
		}
		client.Close()
		g.Observe([]Process{app}, 100)
	}
	client, err := NewSharedClient(api.Address, api.Secret)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	reused := Process{PID: app.PID, Created: 200, Path: `C:\Other.exe`}
	paths, err := client.find(context.Background(), reused)
	if err != nil || !reflect.DeepEqual(paths, []string{reused.Path}) {
		t.Fatalf("reused PID: %v %v", paths, err)
	}
}

func TestSharedAPIRequiresTokenAndValidIdentity(t *testing.T) {
	calls := 0
	api, err := NewSharedServer(func(uint32, uint64) ([]string, error) { calls++; return []string{`C:\App.exe`}, nil })
	if err != nil {
		t.Fatal(err)
	}
	defer api.Close()
	for _, tc := range []struct {
		body, token string
		status      int
	}{
		{`{"pid":1,"created":2}`, "wrong", 401},
		{`{"pid":0,"created":2}`, api.Secret, 400},
		{`{"pid":1,"created":2} {}`, api.Secret, 400},
		{strings.Repeat(" ", 129) + `{}`, api.Secret, 400},
	} {
		req, err := http.NewRequest("POST", api.Address+"/family", strings.NewReader(tc.body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+tc.token)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != tc.status {
			t.Fatalf("status=%d wanted=%d", res.StatusCode, tc.status)
		}
	}
	if calls != 0 {
		t.Fatal("invalid request reached tracker")
	}
}

func TestSharedFailureIsNotAnUnknownProcess(t *testing.T) {
	api, err := NewSharedServer(func(uint32, uint64) ([]string, error) { return []string{`C:\App.exe`}, nil })
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewSharedClient(api.Address, api.Secret)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := api.Close(); err != nil {
		t.Fatal(err)
	}
	if err := api.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = client.find(context.Background(), Process{PID: 1, Created: 2, Path: `C:\App.exe`})
	if !errors.Is(err, ErrSharedUnavailable) {
		t.Fatalf("tracker failure hidden: %v", err)
	}
}

func TestSharedEndpointCannotSendTokenOffMachine(t *testing.T) {
	secret := strings.Repeat("a", 64)
	for _, address := range []string{"http://example.org:123", "http://localhost:123", "https://127.0.0.1:123", "http://127.0.0.1:123/path", "http://user@127.0.0.1:123"} {
		if _, err := NewSharedClient(address, secret); err == nil {
			t.Fatalf("accepted %s", address)
		}
	}
}

func TestWatcherIdentityChecksCreationTime(t *testing.T) {
	w, err := NewWatcher(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	p, err := readProcess(uint32(os.Getpid()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.FindIdentity(p.PID, p.Created+1); err == nil {
		t.Fatal("mismatched creation time accepted")
	}
	paths, err := w.FindIdentity(p.PID, p.Created)
	if err != nil || len(paths) == 0 || paths[0] != p.Path {
		t.Fatalf("self not found: %v %v", paths, err)
	}
}
