package renderer

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/fkasasagi/ccfx/model"
)

func writeJSON(report *model.ForensicReport, outDir string) ([]OutputFile, error) {
	path := filepath.Join(outDir, "report.json")

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return nil, err
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return nil, err
	}

	return []OutputFile{{Path: path, Size: fileSize(path)}}, nil
}
