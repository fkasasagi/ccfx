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
	// os.Stat reports mtime in time.Local; normalize so the report does not
	// depend on the examining machine's timezone.
	report.FileModified = info.ModTime().UTC()
	report.FileSizeBytes = info.Size()

	if info.Size() > 0 {
		report.TokenDetected = true
	}

	return report
}
