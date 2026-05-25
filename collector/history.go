package collector

import (
	"bufio"
	"encoding/json"
	"os"
	"time"

	"github.com/fkasasagi/ccfx/model"
)

type rawHistoryEntry struct {
	Display   string `json:"display"`
	Timestamp int64  `json:"timestamp"`
	Project   string `json:"project"`
	SessionID string `json:"sessionId"`
}

func parseHistory(path string) ([]model.HistoryEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []model.HistoryEntry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var raw rawHistoryEntry
		if err := json.Unmarshal(line, &raw); err != nil {
			continue
		}
		var ts time.Time
		if raw.Timestamp > 0 {
			ts = time.UnixMilli(raw.Timestamp)
		}
		entries = append(entries, model.HistoryEntry{
			Display:   raw.Display,
			Timestamp: ts,
			Project:   raw.Project,
			SessionID: raw.SessionID,
		})
	}
	return entries, scanner.Err()
}
