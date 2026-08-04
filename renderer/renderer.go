package renderer

import (
	"os"
	"path/filepath"
	"time"

	"github.com/fkasasagi/ccfx/model"
)

type Config struct {
	Report   *model.ForensicReport
	OutDir   string
	Formats  []string
	Lang     string
	Timezone *time.Location
}

type Result struct {
	Files []OutputFile
}

type OutputFile struct {
	Path string
	Size int64
}

func Render(cfg Config) (*Result, error) {
	if err := os.MkdirAll(cfg.OutDir, 0o755); err != nil {
		return nil, err
	}

	result := &Result{}
	wantFmt := make(map[string]bool)
	for _, f := range cfg.Formats {
		wantFmt[f] = true
	}

	dict := getDict(cfg.Lang)
	tz := cfg.Timezone
	if tz == nil {
		tz = time.UTC
	}
	dict = appendTZ(dict, tz)

	if wantFmt["json"] {
		files, err := writeJSON(cfg.Report, cfg.OutDir, tz)
		if err != nil {
			return nil, err
		}
		result.Files = append(result.Files, files...)
	}

	if wantFmt["csv"] {
		files, err := writeCSV(cfg.Report, cfg.OutDir, dict, tz)
		if err != nil {
			return nil, err
		}
		result.Files = append(result.Files, files...)
	}

	if wantFmt["md"] {
		files, err := writeMarkdown(cfg.Report, cfg.OutDir, dict, tz)
		if err != nil {
			return nil, err
		}
		result.Files = append(result.Files, files...)
	}

	if wantFmt["html"] {
		files, err := writeHTML(cfg.Report, cfg.OutDir, dict, tz)
		if err != nil {
			return nil, err
		}
		result.Files = append(result.Files, files...)
	}

	return result, nil
}

// KnownOutputFiles lists every file ccfx may generate, across all formats.
// Used to detect a non-empty output directory before rendering.
func KnownOutputFiles(outDir string) []string {
	names := []string{
		"report.json",
		"report.md",
		"report.html",
	}
	for _, o := range csvOutputs {
		names = append(names, o.name)
	}
	paths := make([]string, len(names))
	for i, n := range names {
		paths[i] = filepath.Join(outDir, n)
	}
	return paths
}

func appendTZ(dict Dict, tz *time.Location) Dict {
	abbrev := time.Now().In(tz).Format("MST")
	out := make(Dict, len(dict))
	for k, v := range dict {
		out[k] = v
	}
	timeKeys := []string{"started_at", "timestamp", "first_seen", "last_seen", "generated_at", "file_modified"}
	for _, k := range timeKeys {
		if v, ok := out[k]; ok {
			out[k] = v + " (" + abbrev + ")"
		}
	}
	return out
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}
