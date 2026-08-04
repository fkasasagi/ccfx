package renderer

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fkasasagi/ccfx/model"
)

var bom = []byte{0xEF, 0xBB, 0xBF}

// csvOutputs is the single source of truth for the CSV files ccfx emits.
// KnownOutputFiles derives from it, so a new writer is automatically covered
// by the --force overwrite guard.
var csvOutputs = []struct {
	name string
	fn   func(*model.ForensicReport, string, Dict, *time.Location) error
}{
	{"sessions.csv", writeSessionsCSV},
	{"timeline.csv", writeTimelineCSV},
	{"tool_usage.csv", writeToolUsageCSV},
	{"file_changes.csv", writeFileChangesCSV},
	{"token_usage.csv", writeTokenUsageCSV},
	{"history.csv", writeHistoryCSV},
	{"conversations.csv", writeConversationsCSV},
	{"injection_events.csv", writeInjectionEventsCSV},
	{"injection_findings.csv", writeInjectionFindingsCSV},
}

func writeCSV(report *model.ForensicReport, outDir string, dict Dict, tz *time.Location) ([]OutputFile, error) {
	var files []OutputFile

	for _, w := range csvOutputs {
		path := filepath.Join(outDir, w.name)
		if err := w.fn(report, path, dict, tz); err != nil {
			return nil, fmt.Errorf("%s: %w", w.name, err)
		}
		files = append(files, OutputFile{Path: path, Size: fileSize(path)})
	}

	return files, nil
}

func newCSVFile(path string) (*os.File, *csv.Writer, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, nil, err
	}
	if _, err := f.Write(bom); err != nil {
		f.Close()
		return nil, nil, err
	}
	return f, csv.NewWriter(f), nil
}

func writeSessionsCSV(report *model.ForensicReport, path string, dict Dict, tz *time.Location) error {
	f, w, err := newCSVFile(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w.Write([]string{
		dict["session_id"], dict["project"], dict["started_at"],
		dict["duration_min"], dict["version"], dict["model"],
		dict["message_count"], dict["tool_use_count"], dict["git_branch"],
		dict["permission_mode"], dict["title_label"],
	})

	for _, s := range report.Sessions {
		w.Write([]string{
			s.SessionID,
			s.Project,
			formatTimeIn(s.StartedAt, tz),
			fmt.Sprintf("%.1f", s.DurationSec/60),
			s.Version,
			s.Model,
			strconv.Itoa(s.MessageCount),
			strconv.Itoa(s.ToolUseCount),
			s.GitBranch,
			s.PermissionMode,
			s.Title,
		})
	}
	w.Flush()
	return w.Error()
}

func writeTimelineCSV(report *model.ForensicReport, path string, dict Dict, tz *time.Location) error {
	f, w, err := newCSVFile(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w.Write([]string{
		dict["timestamp"], dict["session_id"], dict["project"],
		dict["event_type"], dict["summary"], dict["model"], dict["git_branch"],
	})

	for _, e := range report.Timeline {
		w.Write([]string{
			formatTimeIn(e.Timestamp, tz),
			e.SessionID,
			e.Project,
			e.EventType,
			e.Summary,
			e.Model,
			e.GitBranch,
		})
	}
	w.Flush()
	return w.Error()
}

func writeToolUsageCSV(report *model.ForensicReport, path string, dict Dict, tz *time.Location) error {
	f, w, err := newCSVFile(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w.Write([]string{dict["tool_name"], dict["total_calls"], dict["session_count"]})

	for _, t := range report.ToolUsage.TopTools {
		w.Write([]string{
			t.ToolName,
			strconv.Itoa(t.TotalCalls),
			strconv.Itoa(t.SessionCount),
		})
	}
	w.Flush()
	return w.Error()
}

func writeFileChangesCSV(report *model.ForensicReport, path string, dict Dict, tz *time.Location) error {
	f, w, err := newCSVFile(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w.Write([]string{
		dict["timestamp"], dict["session_id"], dict["file_path"],
		dict["tool_name"], dict["operation"],
	})

	for _, fc := range report.FileChanges {
		w.Write([]string{
			formatTimeIn(fc.Timestamp, tz),
			fc.SessionID,
			fc.FilePath,
			fc.ToolName,
			fc.Operation,
		})
	}
	w.Flush()
	return w.Error()
}

func writeTokenUsageCSV(report *model.ForensicReport, path string, dict Dict, tz *time.Location) error {
	f, w, err := newCSVFile(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w.Write([]string{
		dict["date"], dict["input_tokens"], dict["output_tokens"],
		dict["cache_creation"], dict["cache_read"],
	})

	dates := make([]string, 0, len(report.TokenUsage.ByDate))
	for d := range report.TokenUsage.ByDate {
		dates = append(dates, d)
	}
	sort.Strings(dates)

	for _, d := range dates {
		t := report.TokenUsage.ByDate[d]
		w.Write([]string{
			d,
			strconv.FormatInt(t.InputTokens, 10),
			strconv.FormatInt(t.OutputTokens, 10),
			strconv.FormatInt(t.CacheCreationTokens, 10),
			strconv.FormatInt(t.CacheReadTokens, 10),
		})
	}
	w.Flush()
	return w.Error()
}

func writeHistoryCSV(report *model.ForensicReport, path string, dict Dict, tz *time.Location) error {
	f, w, err := newCSVFile(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w.Write([]string{
		dict["timestamp"], dict["session_id"], dict["project"], dict["shell_command"], dict["display"],
	})

	for _, h := range report.CommandHistory {
		w.Write([]string{
			formatTimeIn(h.Timestamp, tz),
			h.SessionID,
			h.Project,
			strconv.FormatBool(h.IsShellCommand),
			h.Display,
		})
	}
	w.Flush()
	return w.Error()
}

// writeInjectionEventsCSV is the complete ingress/egress inventory. It is the
// one place everything appears — the rendered sections deliberately show only
// what carries a signal.
func writeInjectionEventsCSV(report *model.ForensicReport, path string, dict Dict, tz *time.Location) error {
	f, w, err := newCSVFile(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w.Write([]string{
		dict["timestamp"], dict["session_id"], dict["project"], dict["category"],
		dict["tool_name"], dict["detail"], dict["file_size"], dict["rule"],
		dict["severity"], dict["excerpt"], dict["preview"],
	})

	for _, ev := range report.Injection.Events {
		var rules, sevs, excerpts []string
		for _, s := range ev.Signals {
			rules = append(rules, s.Rule)
			sevs = append(sevs, s.Severity)
			excerpts = append(excerpts, s.Excerpt)
		}
		w.Write([]string{
			formatTimeIn(ev.Timestamp, tz),
			ev.SessionID,
			ev.Project,
			ev.Category,
			ev.ToolName,
			ev.Detail,
			strconv.FormatInt(ev.Bytes, 10),
			strings.Join(rules, " "),
			strings.Join(sevs, " "),
			strings.Join(excerpts, " ⏐ "),
			ev.Preview,
		})
	}
	w.Flush()
	return w.Error()
}

func writeInjectionFindingsCSV(report *model.ForensicReport, path string, dict Dict, tz *time.Location) error {
	f, w, err := newCSVFile(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w.Write([]string{
		dict["severity"], dict["rule"], dict["session_id"], dict["project"],
		dict["timestamp"], dict["summary"], dict["detail"],
	})

	for _, fi := range report.Injection.Findings {
		var when time.Time
		var chain []string
		for _, ev := range fi.Evidence {
			if when.IsZero() {
				when = ev.Timestamp
			}
			chain = append(chain, ev.Category+": "+ev.Detail)
		}
		w.Write([]string{
			fi.Severity,
			fi.Rule,
			fi.SessionID,
			fi.Project,
			formatTimeIn(when, tz),
			fi.Summary,
			strings.Join(chain, " → "),
		})
	}
	w.Flush()
	return w.Error()
}

func writeConversationsCSV(report *model.ForensicReport, path string, dict Dict, tz *time.Location) error {
	f, w, err := newCSVFile(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w.Write([]string{
		dict["session_id"], dict["project"], dict["title_label"], dict["timestamp"],
		dict["role"], dict["msg_type"], dict["model"], dict["tool_name"], dict["content"],
	})

	for _, c := range report.Conversations {
		for _, m := range c.Messages {
			content := strings.ReplaceAll(m.Content, "\n", " ")
			w.Write([]string{
				c.SessionID, c.Project, c.Title,
				formatTimeIn(m.Timestamp, tz),
				m.Role, m.Type, m.Model, m.ToolName, content,
			})
		}
	}
	w.Flush()
	return w.Error()
}
