//go:build windows

package main

import (
	"os"
	"os/exec"
	"os/user"
	"strings"
	"testing"
)

// Windows cannot express 0600, so the archive is checked through its actual ACL:
// the inherited entries must be gone and the creating account must be the only
// one left with access. Asserting the mode bits here would pass on a file the
// whole machine can read.
func assertArchivePrivate(t *testing.T, path string, info os.FileInfo) {
	t.Helper()

	out, err := exec.Command("icacls", path).CombinedOutput()
	if err != nil {
		t.Fatalf("icacls %s failed: %v: %s", path, err, out)
	}
	acl := string(out)

	u, err := user.Current()
	if err != nil {
		t.Fatalf("user.Current: %v", err)
	}
	if !strings.Contains(acl, u.Username) {
		t.Errorf("the creating account %q is not in the ACL:\n%s", u.Username, acl)
	}

	// Groups a cleartext OAuth token must never be exposed to. They appear only
	// through the inherited ACEs that restrictToOwner is supposed to have removed.
	for _, principal := range []string{"BUILTIN\\Users", "Everyone", "AUTHENTICATED USERS"} {
		if strings.Contains(strings.ToUpper(acl), strings.ToUpper(principal)) {
			t.Errorf("%s still has access to the archive:\n%s", principal, acl)
		}
	}
}
