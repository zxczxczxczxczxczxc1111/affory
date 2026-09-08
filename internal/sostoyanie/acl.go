package sostoyanie

import (
	"fmt"
	"os"
	"os/exec"
)

// ZavestiKatalogDannyh creates the data directory and cuts it off from whatever
// C:\ProgramData hands down. Everything in here is either a secret (DPAPI blobs,
// the subscription URL) or a measuring instrument the acceptance thresholds are
// read from, and neither should be writable by a normal account.
//
// SIDs, not names. On a Russian Windows the group is "Администраторы" and the
// account is "СИСТЕМА"; icacls with English names fails there with a message
// about an invalid parameter, and the failure looks like a bug in our code.
func ZavestiKatalogDannyh() error {
	k := KatalogDannyh()
	if err := os.MkdirAll(k, 0o700); err != nil {
		return fmt.Errorf("каталог данных не создан: %w", err)
	}
	// /inheritance:r drops inherited entries, /grant:r replaces rather than adds.
	// Without :r on the grant, a second run stacks duplicate entries forever.
	out, err := exec.Command("icacls", k,
		"/inheritance:r",
		"/grant:r", "*S-1-5-18:(OI)(CI)F", // LocalSystem
		"/grant:r", "*S-1-5-32-544:(OI)(CI)F", // BUILTIN\Administrators
	).CombinedOutput()
	if err != nil {
		return fmt.Errorf("права каталога данных не выставлены: %w: %s", err, out)
	}
	return nil
}
