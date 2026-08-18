//go:build !windows

package main

import (
	"os"
	"testing"
)

func assertArchivePrivate(t *testing.T, path string, info os.FileInfo) {
	t.Helper()
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("archive mode = %04o, want 0600 — it contains a cleartext token", mode)
	}
}
