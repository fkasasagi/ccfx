package collector

import (
	"os"

	"github.com/fkasasagi/ccfx/model"
)

func detectCredentials(path string) *model.CredentialReport {
	report := &model.CredentialReport{}

	info, err := os.Stat(path)
	if err != nil {
		return report
	}

	report.FileExists = true
	report.FileModified = info.ModTime()
	report.FileSizeBytes = info.Size()

	if info.Size() > 0 {
		report.TokenDetected = true
	}

	return report
}
