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
	traffic := &protokol.PravilaTrafika{Prilozheniya: []protokol.PraviloPrilozheniya{{Potomki: true}}}
	if _, err := s.trackerDlyaKonfiga(traffic); err == nil {
		t.Fatal("family tracking error ignored")
	}
	traffic.Prilozheniya[0].Potomki = false
	if config, err := s.trackerDlyaKonfiga(traffic); err != nil || config.Endpoint != "" {
		t.Fatal("exact-only rules unnecessarily require tracker")
	}
}
