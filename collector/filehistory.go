package collector

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/fkasasagi/ccfx/model"
)

func scanFileHistory(dir string) model.FileHistoryStats {
	stats := model.FileHistoryStats{}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return stats
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		stats.SessionCount++

		sessionDir := filepath.Join(dir, e.Name())
		files, err := os.ReadDir(sessionDir)
		if err != nil {
			continue
		}
		for _, f := range files {
			if !f.IsDir() && strings.Contains(f.Name(), "@v") {
				stats.TotalFileVersions++
			}
		}
	}

	return stats
}
