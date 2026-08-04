package main

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// buildAcquisitionFixture creates a miniature .claude tree with the awkward
// cases: a known mtime, an empty directory, a symlink, and a credentials file.
func buildAcquisitionFixture(t *testing.T) (root string, want time.Time) {
	t.Helper()
	root = filepath.Join(t.TempDir(), ".claude")

	if err := os.MkdirAll(filepath.Join(root, "projects", "-home-user-x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "tasks", "task1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "history.jsonl"), []byte(`{"display":"hi"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".credentials.json"), []byte(`{"token":"secret"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/tmp/claude-0/task-output", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}

	want = time.Date(2026, 5, 12, 13, 16, 0, 0, time.UTC)
	if err := os.Chtimes(filepath.Join(root, "history.jsonl"), want, want); err != nil {
		t.Fatal(err)
	}
	return root, want
}

func openAcquisition(t *testing.T, path string) map[string]*zip.File {
	t.Helper()
	r, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("cannot open archive: %v", err)
	}
	t.Cleanup(func() { r.Close() })

	entries := make(map[string]*zip.File, len(r.File))
	for _, f := range r.File {
		entries[f.Name] = f
	}
	return entries
}

func TestAcquirePreservesTimestamps(t *testing.T) {
	root, want := buildAcquisitionFixture(t)
	outDir := t.TempDir()

	res, err := acquire(root, outDir)
	if err != nil {
		t.Fatalf("acquire failed: %v", err)
	}
	if len(res.Skipped) != 0 {
		t.Errorf("skipped entries: %v", res.Skipped)
	}

	entries := openAcquisition(t, res.Path)
	h, ok := entries[".claude/history.jsonl"]
	if !ok {
		t.Fatalf("history.jsonl missing from archive: got %v", keys(entries))
	}
	if got := h.Modified.UTC().Truncate(time.Second); !got.Equal(want) {
		t.Errorf("mtime = %s, want %s", got, want)
	}
}

func TestAcquireKeepsEmptyDirsAndSymlinks(t *testing.T) {
	root, _ := buildAcquisitionFixture(t)
	outDir := t.TempDir()

	res, err := acquire(root, outDir)
	if err != nil {
		t.Fatalf("acquire failed: %v", err)
	}
	entries := openAcquisition(t, res.Path)

	// An empty directory is invisible unless it gets its own entry, and
	// tasks/<session>/ is exactly how ccfx counts task sessions.
	if _, ok := entries[".claude/tasks/task1/"]; !ok {
		t.Errorf("empty directory entry missing: got %v", keys(entries))
	}

	link, ok := entries[".claude/link"]
	if !ok {
		t.Fatalf("symlink entry missing: got %v", keys(entries))
	}
	if link.Mode()&os.ModeSymlink == 0 {
		t.Errorf("link stored as mode %s, want a symlink", link.Mode())
	}
	rc, err := link.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	target, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	if string(target) != "/tmp/claude-0/task-output" {
		t.Errorf("link target = %q, want the path unfollowed", target)
	}
	if res.Symlinks != 1 {
		t.Errorf("Symlinks = %d, want 1", res.Symlinks)
	}
}

// The whole point of the -ac warning: the archive is verbatim, credentials and all.
func TestAcquireIncludesCredentials(t *testing.T) {
	root, _ := buildAcquisitionFixture(t)
	outDir := t.TempDir()

	res, err := acquire(root, outDir)
	if err != nil {
		t.Fatalf("acquire failed: %v", err)
	}
	if _, ok := openAcquisition(t, res.Path)[".claude/.credentials.json"]; !ok {
		t.Error(".credentials.json missing: the archive must be verbatim")
	}
}

// The archive holds an OAuth token in cleartext, so it must not be readable by
// other users on the machine. os.Create would have left it at 0666&^umask.
func TestAcquireArchiveIsPrivate(t *testing.T) {
	root, _ := buildAcquisitionFixture(t)
	outDir := t.TempDir()

	// A leftover world-readable archive must not keep its mode across a re-run.
	stale := filepath.Join(outDir, acquisitionName)
	if err := os.WriteFile(stale, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := acquire(root, outDir)
	if err != nil {
		t.Fatalf("acquire failed: %v", err)
	}
	info, err := os.Stat(res.Path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("archive mode = %04o, want 0600 — it contains a cleartext token", mode)
	}
}

func TestAcquireExcludesTheOutputDirectory(t *testing.T) {
	root, _ := buildAcquisitionFixture(t)
	outDir := filepath.Join(root, "ccfx-output")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A previous run's report sitting inside the tree is ccfx's own contamination,
	// not evidence.
	if err := os.WriteFile(filepath.Join(outDir, "report.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := acquire(root, outDir)
	if err != nil {
		t.Fatalf("acquire failed: %v", err)
	}
	for name := range openAcquisition(t, res.Path) {
		if filepath.Base(name) == acquisitionName {
			t.Errorf("archive contains itself as %s", name)
		}
		if filepath.Base(name) == "report.json" {
			t.Errorf("archive swept up a previous run's output: %s", name)
		}
	}
	// Silent omission would misrepresent the acquisition as complete.
	if len(res.Skipped) != 1 || !strings.Contains(res.Skipped[0], "output directory") {
		t.Errorf("Skipped = %v, want the excluded output directory recorded", res.Skipped)
	}
}

func TestAcquireLeavesSourceUntouched(t *testing.T) {
	root, _ := buildAcquisitionFixture(t)

	before := snapshot(t, root)
	if _, err := acquire(root, t.TempDir()); err != nil {
		t.Fatalf("acquire failed: %v", err)
	}
	after := snapshot(t, root)

	if len(before) != len(after) {
		t.Fatalf("entry count changed: %d -> %d", len(before), len(after))
	}
	for path, mod := range before {
		if !after[path].Equal(mod) {
			t.Errorf("%s mtime changed: %s -> %s", path, mod, after[path])
		}
	}
}

func snapshot(t *testing.T, root string) map[string]time.Time {
	t.Helper()
	out := make(map[string]time.Time)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		out[path] = info.ModTime()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func keys(m map[string]*zip.File) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
