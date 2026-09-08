package main

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/set"
)

func TestInstallerDoesNotKillForeignVPNCore(t *testing.T) {
	b, err := os.ReadFile("../../ustanovka/affory.nsi")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(b)), "taskkill /im sing-box.exe") {
		t.Fatal("installer kills unrelated VPN engines by image name")
	}
}

func TestProtectionPreferenceWhileIdleDoesNotTouchNetwork(t *testing.T) {
	s := podstavnaya(t, nil)
	s.vklyuchitVes = func(set.Razreshyonnoe, bool) error { t.Error("idle firewall mutation"); return nil }
	s.vyklyuchitVes = func() error { t.Error("idle firewall restore"); return nil }
	if err := s.SetKillSwitch(true); err != nil {
		t.Fatal(err)
	}
	if !s.Status().KillSwitch {
		t.Fatal("preference was not saved")
	}
	if err := s.SetKillSwitch(false); err != nil {
		t.Fatal(err)
	}
}

func TestInstallerStartupDoesNotAutoConnect(t *testing.T) {
	if razreshenAvtopodyom([]string{imyaSluzhby, argumentUstanovki}) {
		t.Fatal("installation startup would reconnect a saved profile")
	}
	if !razreshenAvtopodyom([]string{imyaSluzhby}) {
		t.Fatal("normal startup lost the user's auto-connect preference")
	}
}

func TestExplicitDisconnectReleasesProtectionButRecoveryKeepsIt(t *testing.T) {
	s := podstavnaya(t, nil)
	if err := s.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.SetKillSwitch(true); err != nil {
		t.Fatal(err)
	}
	released := 0
	s.vyklyuchitVes = func() error { released++; return nil }
	s.Disconnect()
	if released != 0 {
		t.Fatal("internal recovery cleanup released protection")
	}
	s.Otklyuchit()
	if released != 1 {
		t.Fatalf("explicit disconnect released %d times", released)
	}
	if !s.Status().KillSwitch {
		t.Fatal("disconnect forgot protection preference")
	}
	s.Otklyuchit()
	if released != 1 {
		t.Fatal("idle disconnect changed firewall again")
	}
}

func TestDisconnectReportsFailedNetworkRestore(t *testing.T) {
	s := podstavnaya(t, nil)
	s.PomnitZapertuyu(true)
	s.vyklyuchitVes = func() error { return errors.New("restore denied") }
	s.Otklyuchit()
	st := s.Status()
	if st.Oshib == nil || st.Oshib.Kod != protokol.KodFirewallFailed {
		t.Fatalf("failed restore was hidden: %+v", st)
	}
}

func TestShutdownReleasesOwnedProtection(t *testing.T) {
	s := podstavnaya(t, nil)
	s.PomnitZapertuyu(true)
	released := 0
	s.vyklyuchitVes = func() error { released++; return nil }
	s.Zavershit()
	if released != 1 {
		t.Fatalf("shutdown released %d times", released)
	}
}
