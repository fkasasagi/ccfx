package renderer

import (
	"os"

	"github.com/fkasasagi/ccfx/model"
)

type Config struct {
	Report  *model.ForensicReport
	OutDir  string
	Formats []string
	Lang    string
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

	if wantFmt["json"] {
		files, err := writeJSON(cfg.Report, cfg.OutDir)
		if err != nil {
			return nil, err
		}
		result.Files = append(result.Files, files...)
	}

	if wantFmt["csv"] {
		files, err := writeCSV(cfg.Report, cfg.OutDir, dict)
		if err != nil {
			return nil, err
		}
		result.Files = append(result.Files, files...)
	}

	if wantFmt["md"] {
		files, err := writeMarkdown(cfg.Report, cfg.OutDir, dict)
		if err != nil {
			return nil, err
		}
		result.Files = append(result.Files, files...)
	}

	if wantFmt["html"] {
		files, err := writeHTML(cfg.Report, cfg.OutDir, dict)
		if err != nil {
			return nil, err
		}
		result.Files = append(result.Files, files...)
	}

	return result, nil
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}
