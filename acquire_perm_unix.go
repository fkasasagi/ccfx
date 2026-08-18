//go:build !windows

package main

import "os"

// restrictToOwner keeps the archive readable only by the user who created it.
// It holds an OAuth token in cleartext, so this runs before any content is
// written rather than after.
//
// The file was already opened 0600, but O_TRUNC reuses an existing file's mode,
// so a stale world-readable archive would otherwise stay world-readable.
func restrictToOwner(f *os.File, path string) error {
	return f.Chmod(0o600)
}
