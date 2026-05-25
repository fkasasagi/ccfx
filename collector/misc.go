package collector

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/fkasasagi/ccfx/model"
)

func scanMisc(claudeDir string) model.MiscStats {
	stats := model.MiscStats{}
	stats.ShellSnapshots = countFiles(filepath.Join(claudeDir, "shell-snapshots"), ".sh")
	stats.PasteCacheFiles = countFiles(filepath.Join(claudeDir, "paste-cache"), "")
	stats.TaskSessions = countDirs(filepath.Join(claudeDir, "tasks"))
	stats.PlanFiles = countFiles(filepath.Join(claudeDir, "plans"), ".md")
	stats.CustomCommands = countFiles(filepath.Join(claudeDir, "commands"), ".md")
	return stats
}

func countFiles(dir, suffix string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	count := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if suffix == "" || strings.HasSuffix(e.Name(), suffix) {
			count++
		}
	}
	return count
}

func countDirs(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	count := 0
	for _, e := range entries {
		if e.IsDir() {
			count++
		}
	}
	return count
}
