package main

import (
	"errors"
	"fmt"
	"path/filepath"

	"golang.org/x/sys/windows/registry"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/sostoyanie"
)

// putInterfeysa is where the interface binary lives after install: next to
// the service, in the program directory. Not next to whatever is running
// now: during an update the running binary may be the old one.
func putInterfeysa() string {
	return filepath.Join(sostoyanie.KatalogProgrammy(), "affory-ui.exe")
}

// Autostart of the INTERFACE at logon (spec §9.2 default: on). Not the
// tunnel: "connect on start" is a separate flag with the opposite default and
// arrives in 4.8. Conflating them is the most unpleasant surprise a VPN can
// deliver, so the two never share a switch.
//
// The truth lives in the registry and nowhere else. Status() reads it back
// every time instead of caching a flag: a cached flag and a Run key edited by
// hand would disagree, and the screen would show the wrong one.
//
// HKLM, not HKCU: the service runs as LocalSystem and has no user hive to
// speak of, and a machine-wide install starts the interface for whoever logs
// in. Writing HKLM needs admin, which the service has and the UI does not,
// which is exactly why this is a service command and not a UI setting.

const (
	putKlyuchaRun     = `SOFTWARE\Microsoft\Windows\CurrentVersion\Run`
	imyaZnacheniyaRun = "Affory"
)

// klyuchAvtozapuska is the seam the tests swap for an HKCU key. Production
// opens HKLM with write access; reads go through the same handle.
var klyuchAvtozapuska = func() (registry.Key, error) {
	return registry.OpenKey(registry.LOCAL_MACHINE, putKlyuchaRun, registry.QUERY_VALUE|registry.SET_VALUE)
}

func vklyuchitAvtozapusk(putUI string) error {
	k, err := klyuchAvtozapuska()
	if err != nil {
		return fmt.Errorf("ключ автозапуска не открылся: %w", err)
	}
	defer k.Close()
	// Quoted, always. "C:\Program Files\..." without quotes launches
	// C:\Program.exe at logon, and Windows reports nothing.
	// The tray flag is spelled the same in cmd/affory-ui/main.go (flagTrey);
	// the two binaries share no package, so the string is repeated here.
	if err := k.SetStringValue(imyaZnacheniyaRun, `"`+putUI+`" --trey`); err != nil {
		return fmt.Errorf("запись автозапуска не удалась: %w", err)
	}
	return nil
}

func vyklyuchitAvtozapusk() error {
	k, err := klyuchAvtozapuska()
	if err != nil {
		return fmt.Errorf("ключ автозапуска не открылся: %w", err)
	}
	defer k.Close()
	err = k.DeleteValue(imyaZnacheniyaRun)
	// Already absent is the state we wanted. A second press of the switch is
	// not an error, and neither is uninstalling a never-registered install.
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("снятие автозапуска не удалось: %w", err)
	}
	return nil
}

func avtozapuskVklyuchen() bool {
	k, err := klyuchAvtozapuska()
	if err != nil {
		return false
	}
	defer k.Close()
	_, _, err = k.GetStringValue(imyaZnacheniyaRun)
	return err == nil
}
