//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strings"
)

// restrictToOwner keeps the archive readable only by the user who created it.
// It holds an OAuth token in cleartext, so this runs before any content is
// written rather than after.
//
// Windows has no POSIX mode bits: os.Chmod there only toggles the read-only
// attribute, so opening the file 0600 leaves it readable by every account the
// parent directory's inherited ACEs allow. icacls is used instead — it ships
// with Windows, so ccfx keeps its zero-module-dependency property.
func restrictToOwner(f *os.File, path string) error {
	u, err := user.Current()
	if err != nil {
		return fmt.Errorf("cannot determine the current user to restrict %s: %w", path, err)
	}

	// /inheritance:r drops the ACEs inherited from the parent directory, so the
	// archive does not stay readable by whoever that directory grants access to.
	// /grant:r then replaces any existing entry for this account with full access.
	cmd := exec.Command("icacls", path, "/inheritance:r", "/grant:r", u.Username+":F")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("icacls could not restrict %s to %s: %w: %s",
			path, u.Username, err, strings.TrimSpace(string(out)))
	}
	return nil
}
