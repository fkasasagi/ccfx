package renderer

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/fkasasagi/ccfx/model"
)

var zeroTimeJSON = []byte(`"0001-01-01T00:00:00Z"`)
var nullJSON = []byte(`null`)

func writeJSON(report *model.ForensicReport, outDir string, tz *time.Location) ([]OutputFile, error) {
	path := filepath.Join(outDir, "report.json")

	converted := convertTimezones(report, tz)

	data, err := json.MarshalIndent(converted, "", "  ")
	if err != nil {
		return nil, err
	}

	data = bytes.ReplaceAll(data, zeroTimeJSON, nullJSON)

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return nil, err
	}

	return []OutputFile{{Path: path, Size: fileSize(path)}}, nil
}

func convertTimezones(report *model.ForensicReport, tz *time.Location) *model.ForensicReport {
	if tz == time.UTC {
		return report
	}

	r := *report

	r.Meta.GeneratedAt = r.Meta.GeneratedAt.In(tz)
	if !r.Meta.DateRange.Earliest.IsZero() {
		r.Meta.DateRange.Earliest = r.Meta.DateRange.Earliest.In(tz)
	}
	if !r.Meta.DateRange.Latest.IsZero() {
		r.Meta.DateRange.Latest = r.Meta.DateRange.Latest.In(tz)
	}

	sessions := make([]model.Session, len(r.Sessions))
	for i, s := range r.Sessions {
		if !s.StartedAt.IsZero() {
			s.StartedAt = s.StartedAt.In(tz)
		}
		if !s.UpdatedAt.IsZero() {
			s.UpdatedAt = s.UpdatedAt.In(tz)
		}
		sessions[i] = s
	}
	r.Sessions = sessions

	timeline := make([]model.TimelineEntry, len(r.Timeline))
	for i, t := range r.Timeline {
		if !t.Timestamp.IsZero() {
			t.Timestamp = t.Timestamp.In(tz)
		}
		timeline[i] = t
	}
	r.Timeline = timeline

	fileChanges := make([]model.FileChange, len(r.FileChanges))
	for i, fc := range r.FileChanges {
		if !fc.Timestamp.IsZero() {
			fc.Timestamp = fc.Timestamp.In(tz)
		}
		fileChanges[i] = fc
	}
	r.FileChanges = fileChanges

	if !r.Credentials.FileModified.IsZero() {
		r.Credentials.FileModified = r.Credentials.FileModified.In(tz)
	}

	projects := make([]model.ProjectSummary, len(r.Projects))
	for i, p := range r.Projects {
		if !p.FirstSeen.IsZero() {
			p.FirstSeen = p.FirstSeen.In(tz)
		}
		if !p.LastSeen.IsZero() {
			p.LastSeen = p.LastSeen.In(tz)
		}
		projects[i] = p
	}
	r.Projects = projects

	if len(r.CommandHistory) > 0 {
		hist := make([]model.HistoryEntry, len(r.CommandHistory))
		for i, h := range r.CommandHistory {
			if !h.Timestamp.IsZero() {
				h.Timestamp = h.Timestamp.In(tz)
			}
			hist[i] = h
		}
		r.CommandHistory = hist
	}

	if len(r.Conversations) > 0 {
		convs := make([]model.Conversation, len(r.Conversations))
		for i, c := range r.Conversations {
			msgs := make([]model.ConversationMsg, len(c.Messages))
			for j, m := range c.Messages {
				m.Timestamp = m.Timestamp.In(tz)
				msgs[j] = m
			}
			c.Messages = msgs
			convs[i] = c
		}
		r.Conversations = convs
	}

	return &r
}
