package main

import (
	"errors"
	"github.com/sagernet/sing-box/common/afforyprocess"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"os"
	"testing"
)

func TestProcessTrackerLivesUntilServiceShutdown(t *testing.T) {
	s := podstavnaya(t, nil)
	n, err := novyyNablyudatelPrilozheniy()
	if err != nil {
		t.Fatal(err)
	}
	s.processTracker = n
	t.Cleanup(s.Zavershit)
	traffic := &protokol.PravilaTrafika{Prilozheniya: []protokol.PraviloPrilozheniya{{Potomki: true}}}
	before, err := s.trackerDlyaKonfiga(traffic)
	if err != nil {
		t.Fatal(err)
	}
	client, err := afforyprocess.NewSharedClient(before.Endpoint, before.Secret)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	s.Disconnect()
	after, err := s.trackerDlyaKonfiga(traffic)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("disconnect replaced tracker")
	}
	if paths, err := client.Find(uint32(os.Getpid())); err != nil || len(paths) == 0 {
		t.Fatalf("tracker stopped with tunnel: %v %v", paths, err)
	}
	s.Zavershit()
	if _, err := client.Find(uint32(os.Getpid())); !errors.Is(err, afforyprocess.ErrSharedUnavailable) {
		t.Fatalf("tracker survived service: %v", err)
	}
}

func TestProcessTrackerStartupErrorDoesNotSilentlyDropFamilyRules(t *testing.T) {
	s := podstavnaya(t, nil)
	s.processTrackerErr = errors.New("test tracker failure")
	s.novyyTracker = func() (*nablyudatelPrilozheniy, error) { return nil, errors.New("test tracker failure again") }
	traffic := &protokol.PravilaTrafika{Prilozheniya: []protokol.PraviloPrilozheniya{{Potomki: true}}}
	if _, err := s.trackerDlyaKonfiga(traffic); err == nil {
		t.Fatal("family tracking error ignored")
	}
	traffic.Prilozheniya[0].Potomki = false
	if config, err := s.trackerDlyaKonfiga(traffic); err != nil || config.Endpoint != "" {
		t.Fatal("exact-only rules unnecessarily require tracker")
	}
}

// В3 аудита 1.8.0: отказ трекера на старте службы держался до её перезапуска.
func TestProcessTrackerStartupErrorIsRetried(t *testing.T) {
	s := podstavnaya(t, nil)
	t.Cleanup(s.Zavershit)
	s.processTrackerErr = errors.New("test tracker failure")
	traffic := &protokol.PravilaTrafika{Prilozheniya: []protokol.PraviloPrilozheniya{{Potomki: true}}}
	config, err := s.trackerDlyaKonfiga(traffic)
	if err != nil || config.Endpoint == "" {
		t.Fatalf("tracker was not started again: %+v %v", config, err)
	}
}

func TestStoppedProcessTrackerIsReplaced(t *testing.T) {
	s := podstavnaya(t, nil)
	t.Cleanup(s.Zavershit)
	n, err := novyyNablyudatelPrilozheniy()
	if err != nil {
		t.Fatal(err)
	}
	s.processTracker = n
	if err := n.api.Close(); err != nil {
		t.Fatal(err)
	}
	traffic := &protokol.PravilaTrafika{Prilozheniya: []protokol.PraviloPrilozheniya{{Potomki: true}}}
	config, err := s.trackerDlyaKonfiga(traffic)
	if err != nil {
		t.Fatal(err)
	}
	if config.Endpoint == "" || config.Endpoint == n.api.Address {
		t.Fatalf("stopped tracker handed to the core: %+v", config)
	}
	client, err := afforyprocess.NewSharedClient(config.Endpoint, config.Secret)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err := client.Find(uint32(os.Getpid())); err != nil {
		t.Fatalf("replacement tracker does not answer: %v", err)
	}
}

// L6 аудита 1.8.0: карточка сервиса с программами даёт дерево процессов, и
// трекер службы нужен ей так же, как ручному правилу с потомками.
func TestServiceProgramsUseServiceTracker(t *testing.T) {
	s := podstavnaya(t, nil)
	t.Cleanup(s.Zavershit)
	n, err := novyyNablyudatelPrilozheniy()
	if err != nil {
		t.Fatal(err)
	}
	s.processTracker = n
	traffic := &protokol.PravilaTrafika{Servisy: []protokol.PraviloServisa{{Id: "steam", Marshrut: protokol.TrafikPryamo, Programmy: []string{`C:\Steam\steam.exe`}}}}
	config, err := s.trackerDlyaKonfiga(traffic)
	if err != nil || config.Endpoint != n.api.Address {
		t.Fatalf("service programs left on core polling: %+v %v", config, err)
	}
}
