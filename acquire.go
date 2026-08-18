package main

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// acquisitionName is the archive written by -ac.
const acquisitionName = "claude-acquisition.zip"

type acquisitionResult struct {
	Path     string
	Size     int64
	Files    int
	Dirs     int
	Symlinks int
	Skipped  []string
}

// acquire writes a verbatim zip copy of srcDir into outDir, preserving each
// entry's modification time. Symlinks are stored as their target string rather
// than followed, so the archive cannot escape the tree or loop. srcDir is only
// ever read.
//
// Unreadable entries are recorded in Skipped instead of aborting the run: a
// partial acquisition of a forensic image is worth more than none.
func acquire(srcDir, outDir string) (*acquisitionResult, error) {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}

	root := filepath.Clean(srcDir)
	zipPath := filepath.Join(outDir, acquisitionName)

	// outDir may sit inside the tree being captured. Excluding the whole output
	// directory keeps ccfx's own reports — and the archive itself — out of a
	// "verbatim" copy of the source.
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	zipAbs, err := filepath.Abs(zipPath)
	if err != nil {
		return nil, err
	}
	outAbs, err := filepath.Abs(outDir)
	if err != nil {
		return nil, err
	}
	excludedDir := ""
	if strings.HasPrefix(outAbs, rootAbs+string(filepath.Separator)) {
		excludedDir = outAbs
	}

	// The archive carries credentials in cleartext, so other users must not be
	// able to read it. O_TRUNC alone would leave an existing file's mode as-is.
	f, err := os.OpenFile(zipPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	// Before the token reaches the disk, not after. On failure the (still empty)
	// archive is removed rather than left behind unprotected.
	if err := restrictToOwner(f, zipPath); err != nil {
		f.Close()
		os.Remove(zipPath)
		return nil, err
	}

	zw := zip.NewWriter(f)
	res := &acquisitionResult{Path: zipPath}
	prefix := filepath.Base(root)

	walkErr := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			res.Skipped = append(res.Skipped, fmt.Sprintf("%s: %v", path, err))
			return nil
		}

		if abs, err := filepath.Abs(path); err == nil {
			if abs == zipAbs {
				return nil
			}
			if excludedDir != "" && abs == excludedDir {
				res.Skipped = append(res.Skipped, fmt.Sprintf("%s: ccfx output directory, excluded from the archive", path))
				return filepath.SkipDir
			}
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			res.Skipped = append(res.Skipped, fmt.Sprintf("%s: %v", path, err))
			return nil
		}
		if rel == "." {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			res.Skipped = append(res.Skipped, fmt.Sprintf("%s: %v", path, err))
			return nil
		}

		// Classify once: sockets, fifos and devices have no meaningful zip
		// representation and are recorded rather than written.
		isDir := d.IsDir()
		isSymlink := info.Mode()&os.ModeSymlink != 0
		if !isDir && !isSymlink && !info.Mode().IsRegular() {
			res.Skipped = append(res.Skipped, fmt.Sprintf("%s: unsupported file type %s", path, info.Mode().Type()))
			return nil
		}

		// FileInfoHeader carries info.ModTime() into the entry, which is what
		// preserves the timestamps.
		h, err := zip.FileInfoHeader(info)
		if err != nil {
			res.Skipped = append(res.Skipped, fmt.Sprintf("%s: %v", path, err))
			return nil
		}
		h.Name = filepath.ToSlash(filepath.Join(prefix, rel))
		if isDir {
			h.Name += "/"
		} else if !isSymlink {
			h.Method = zip.Deflate
		}

		w, err := zw.CreateHeader(h)
		if err != nil {
			return err
		}

		switch {
		case isDir:
			res.Dirs++
		case isSymlink:
			target, err := os.Readlink(path)
			if err != nil {
				res.Skipped = append(res.Skipped, fmt.Sprintf("%s: %v", path, err))
				return nil
			}
			if _, err := io.WriteString(w, target); err != nil {
				return err
			}
			res.Symlinks++
		default:
			src, err := os.Open(path)
			if err != nil {
				res.Skipped = append(res.Skipped, fmt.Sprintf("%s: %v", path, err))
				return nil
			}
			_, copyErr := io.Copy(w, src)
			src.Close()
			if copyErr != nil {
				return copyErr
			}
			res.Files++
		}
		return nil
	})
	if walkErr != nil {
		zw.Close()
		return nil, walkErr
	}

	if err := zw.Close(); err != nil {
		return nil, err
	}
	if info, err := f.Stat(); err == nil {
		res.Size = info.Size()
	}
	return res, nil
}
