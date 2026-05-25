package collector

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fkasasagi/ccfx/model"
)

type rawSessionFile struct {
	PID        int    `json:"pid"`
	SessionID  string `json:"sessionId"`
	CWD        string `json:"cwd"`
	StartedAt  int64  `json:"startedAt"`
	UpdatedAt  int64  `json:"updatedAt"`
	Version    string `json:"version"`
	Entrypoint string `json:"entrypoint"`
	Kind       string `json:"kind"`
	Status     string `json:"status"`
	Name       string `json:"name"`
}

func parseSessions(dir string) ([]model.SessionFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var sessions []model.SessionFile
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var raw rawSessionFile
		if err := json.Unmarshal(data, &raw); err != nil {
			continue
		}
		sessions = append(sessions, model.SessionFile{
			PID:        raw.PID,
			SessionID:  raw.SessionID,
			CWD:        raw.CWD,
			StartedAt:  time.UnixMilli(raw.StartedAt),
			UpdatedAt:  time.UnixMilli(raw.UpdatedAt),
			Version:    raw.Version,
			Entrypoint: raw.Entrypoint,
			Kind:       raw.Kind,
			Status:     raw.Status,
		})
	}
	return sessions, nil
}
