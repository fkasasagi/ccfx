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

// convertEventTimes copies events with their timestamps moved into tz. The same
// event list is reachable both directly and through a finding's evidence, so the
// copy keeps the caller's slice untouched.
func convertEventTimes(events []model.InjectionEvent, tz *time.Location) []model.InjectionEvent {
	if len(events) == 0 {
		return events
	}
	out := make([]model.InjectionEvent, len(events))
	for i, e := range events {
		if !e.Timestamp.IsZero() {
			e.Timestamp = e.Timestamp.In(tz)
		}
		out[i] = e
	}
	return out
}

// convertTimezones returns a copy of the report with every serialized timestamp
// moved into tz. It deliberately does not skip the tz == time.UTC case: doing so
// would make report.json correct only as long as every collector remembers to
// normalize, and a single source left in time.Local would then silently stamp
// the examining machine's zone onto the evidence. Converting unconditionally is
// a no-op when the values are already UTC, and one branch fewer to reason about.
func convertTimezones(report *model.ForensicReport, tz *time.Location) *model.ForensicReport {
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

	// Section 14. Investigators correlate these against the timeline and against
	// injection_events.csv, so they must not stay behind in UTC while the rest of
	// the report moves to the requested zone.
	r.Injection.Events = convertEventTimes(r.Injection.Events, tz)
	if len(r.Injection.Findings) > 0 {
		findings := make([]model.InjectionFinding, len(r.Injection.Findings))
		for i, f := range r.Injection.Findings {
			f.Evidence = convertEventTimes(f.Evidence, tz)
			findings[i] = f
		}
		r.Injection.Findings = findings
	}
	if len(r.Injection.Sessions) > 0 {
		triage := make([]model.SessionTriage, len(r.Injection.Sessions))
		for i, s := range r.Injection.Sessions {
			if !s.StartedAt.IsZero() {
				s.StartedAt = s.StartedAt.In(tz)
			}
			triage[i] = s
		}
		r.Injection.Sessions = triage
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
