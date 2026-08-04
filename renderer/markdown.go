package renderer

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fkasasagi/ccfx/model"
)

func writeMarkdown(report *model.ForensicReport, outDir string, dict Dict, tz *time.Location) ([]OutputFile, error) {
	path := filepath.Join(outDir, "report.md")
	ft := func(t time.Time) string { return formatTimeIn(t, tz) }
	fd := func(t time.Time) string { return formatDateIn(t, tz) }

	var b strings.Builder

	b.WriteString(fmt.Sprintf("# %s\n\n", dict["title"]))
	b.WriteString(fmt.Sprintf("- **%s**: %s\n", dict["generated_at"], report.Meta.GeneratedAt.In(tz).Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("- **%s**: %s\n", dict["source_path"], report.Meta.SourcePath))
	b.WriteString(fmt.Sprintf("- **%s**: %s\n", dict["tool_version"], report.Meta.ToolVersion))
	b.WriteString(fmt.Sprintf("- **%s**: %s\n", dict["platform"], report.Meta.Platform))
	b.WriteString(fmt.Sprintf("- **%s**: %d\n", dict["total_sessions"], report.Meta.TotalSessions))
	b.WriteString(fmt.Sprintf("- **%s**: %d\n", dict["total_projects"], report.Meta.TotalProjects))
	if !report.Meta.DateRange.Earliest.IsZero() {
		b.WriteString(fmt.Sprintf("- **%s**: %s ~ %s\n",
			dict["date_range"],
			report.Meta.DateRange.Earliest.In(tz).Format("2006-01-02"),
			report.Meta.DateRange.Latest.In(tz).Format("2006-01-02")))
	}
	b.WriteString("\n---\n\n")

	// User Identity
	b.WriteString(fmt.Sprintf("## 1. %s\n\n", dict["user_identity"]))
	ui := report.UserIdentity
	writeField(&b, dict["email"], ui.Email)
	writeField(&b, dict["account_uuid"], ui.AccountUUID)
	writeField(&b, dict["org_uuid"], ui.OrganizationUUID)
	writeField(&b, dict["org_name"], ui.OrganizationName)
	writeField(&b, dict["org_type"], ui.OrganizationType)
	writeField(&b, dict["org_role"], ui.OrganizationRole)
	writeField(&b, dict["rate_limit"], ui.RateLimitTier)
	writeField(&b, dict["cc_version"], ui.ClaudeCodeVersion)
	b.WriteString("\n")

	// Sessions
	b.WriteString(fmt.Sprintf("## 2. %s\n\n", dict["sessions"]))
	b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | %s |\n",
		dict["session_id"], dict["project"], dict["started_at"],
		dict["duration_min"], dict["model"], dict["message_count"]))
	b.WriteString("|---|---|---|---|---|---|\n")
	for _, s := range report.Sessions {
		b.WriteString(fmt.Sprintf("| `%.8s` | %s | %s | %.1f | %s | %d |\n",
			s.SessionID, s.Project, ft(s.StartedAt),
			s.DurationSec/60, s.Model, s.MessageCount))
	}
	b.WriteString("\n")

	// Timeline (last 50)
	b.WriteString(fmt.Sprintf("## 3. %s\n\n", dict["timeline"]))
	tl := report.Timeline
	if len(tl) > 50 {
		b.WriteString(fmt.Sprintf("*(%d entries total, showing last 50)*\n\n", len(tl)))
		tl = tl[len(tl)-50:]
	}
	b.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n",
		dict["timestamp"], dict["event_type"], dict["project"], dict["summary"]))
	b.WriteString("|---|---|---|---|\n")
	for _, e := range tl {
		summary := strings.ReplaceAll(e.Summary, "|", "\\|")
		summary = strings.ReplaceAll(summary, "\n", " ")
		b.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n",
			ft(e.Timestamp), e.EventType, e.Project, summary))
	}
	b.WriteString("\n")

	// Projects
	b.WriteString(fmt.Sprintf("## 4. %s\n\n", dict["projects"]))
	b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s |\n",
		dict["project"], dict["session_count"], dict["first_seen"],
		dict["last_seen"], dict["total_messages"]))
	b.WriteString("|---|---|---|---|---|\n")
	for _, p := range report.Projects {
		b.WriteString(fmt.Sprintf("| %s | %d | %s | %s | %d |\n",
			p.Path, p.SessionCount,
			fd(p.FirstSeen), fd(p.LastSeen),
			p.TotalMessages))
	}
	b.WriteString("\n")

	// Tool Usage
	b.WriteString(fmt.Sprintf("## 5. %s\n\n", dict["tool_usage"]))
	b.WriteString(fmt.Sprintf("| %s | %s | %s |\n",
		dict["tool_name"], dict["total_calls"], dict["session_count"]))
	b.WriteString("|---|---|---|\n")
	for _, t := range report.ToolUsage.TopTools {
		b.WriteString(fmt.Sprintf("| %s | %d | %d |\n",
			t.ToolName, t.TotalCalls, t.SessionCount))
	}
	b.WriteString("\n")

	// Token Usage
	b.WriteString(fmt.Sprintf("## 6. %s\n\n", dict["token_usage"]))
	tu := report.TokenUsage
	b.WriteString(fmt.Sprintf("- **%s**: %s\n", dict["input_tokens"], formatInt64(tu.TotalInput)))
	b.WriteString(fmt.Sprintf("- **%s**: %s\n", dict["output_tokens"], formatInt64(tu.TotalOutput)))
	b.WriteString(fmt.Sprintf("- **%s**: %s\n", dict["cache_creation"], formatInt64(tu.TotalCacheCreate)))
	b.WriteString(fmt.Sprintf("- **%s**: %s\n\n", dict["cache_read"], formatInt64(tu.TotalCacheRead)))

	if len(tu.ByModel) > 0 {
		b.WriteString(fmt.Sprintf("### %s\n\n", dict["by_model"]))
		b.WriteString(fmt.Sprintf("| %s | %s | %s |\n", dict["model"], dict["input_tokens"], dict["output_tokens"]))
		b.WriteString("|---|---|---|\n")
		for _, m := range sortedKeys(tu.ByModel) {
			t := tu.ByModel[m]
			b.WriteString(fmt.Sprintf("| %s | %s | %s |\n", m, formatInt64(t.InputTokens), formatInt64(t.OutputTokens)))
		}
		b.WriteString("\n")
	}

	if len(tu.ByDate) > 0 {
		b.WriteString(fmt.Sprintf("### %s\n\n", dict["by_date"]))
		dates := sortedKeys(tu.ByDate)
		b.WriteString(fmt.Sprintf("| %s | %s | %s |\n", dict["date"], dict["input_tokens"], dict["output_tokens"]))
		b.WriteString("|---|---|---|\n")
		for _, d := range dates {
			t := tu.ByDate[d]
			b.WriteString(fmt.Sprintf("| %s | %s | %s |\n", d, formatInt64(t.InputTokens), formatInt64(t.OutputTokens)))
		}
		b.WriteString("\n")
	}

	// File Changes
	if len(report.FileChanges) > 0 {
		b.WriteString(fmt.Sprintf("## 7. %s\n\n", dict["file_changes"]))
		b.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n",
			dict["timestamp"], dict["file_path"], dict["tool_name"], dict["operation"]))
		b.WriteString("|---|---|---|---|\n")
		changes := report.FileChanges
		if len(changes) > 100 {
			b.WriteString(fmt.Sprintf("*(%d entries total, showing last 100)*\n\n", len(changes)))
			changes = changes[len(changes)-100:]
		}
		for _, fc := range changes {
			b.WriteString(fmt.Sprintf("| %s | `%s` | %s | %s |\n",
				ft(fc.Timestamp), fc.FilePath, fc.ToolName, fc.Operation))
		}
		b.WriteString("\n")
	}

	// Permissions
	b.WriteString(fmt.Sprintf("## 8. %s\n\n", dict["permissions"]))
	if len(report.Permissions.GlobalDenyRules) > 0 {
		b.WriteString(fmt.Sprintf("### %s\n\n", dict["global_deny"]))
		for _, r := range report.Permissions.GlobalDenyRules {
			b.WriteString(fmt.Sprintf("- `%s`\n", r))
		}
		b.WriteString("\n")
	}
	if len(report.Permissions.GlobalAllowRules) > 0 {
		b.WriteString(fmt.Sprintf("### %s\n\n", dict["global_allow"]))
		for _, r := range report.Permissions.GlobalAllowRules {
			b.WriteString(fmt.Sprintf("- `%s`\n", r))
		}
		b.WriteString("\n")
	}
	if len(report.Permissions.HooksDefined) > 0 {
		b.WriteString(fmt.Sprintf("### %s\n\n", dict["hooks"]))
		for _, h := range report.Permissions.HooksDefined {
			b.WriteString(fmt.Sprintf("- **%s** [%s]: `%s`\n", h.Event, h.Matcher, h.Command))
		}
		b.WriteString("\n")
	}

	// Credentials
	b.WriteString(fmt.Sprintf("## 9. %s\n\n", dict["credentials"]))
	c := report.Credentials
	b.WriteString(fmt.Sprintf("- **%s**: %s\n", dict["file_exists"], boolStr(c.FileExists, dict)))
	if c.FileExists {
		b.WriteString(fmt.Sprintf("- **%s**: %s\n", dict["file_modified"], ft(c.FileModified)))
		b.WriteString(fmt.Sprintf("- **%s**: %d\n", dict["file_size"], c.FileSizeBytes))
		b.WriteString(fmt.Sprintf("- **%s**: %s\n", dict["token_detected"], boolStr(c.TokenDetected, dict)))
	}
	b.WriteString("\n")

	// Command History
	if len(report.CommandHistory) > 0 {
		b.WriteString(fmt.Sprintf("## 10. %s\n\n", dict["command_history"]))
		hist := report.CommandHistory
		if len(hist) > 100 {
			b.WriteString(fmt.Sprintf("*(%d entries total, showing last 100)*\n\n", len(hist)))
			hist = hist[len(hist)-100:]
		}
		b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s |\n",
			dict["timestamp"], dict["session_id"], dict["project"], dict["shell_command"], dict["display"]))
		b.WriteString("|---|---|---|---|---|\n")
		for _, h := range hist {
			cmd := h.Display
			if len(cmd) > 80 {
				cmd = clip(cmd, 80)
			}
			cmd = strings.ReplaceAll(cmd, "|", "\\|")
			cmd = strings.ReplaceAll(cmd, "\n", " ")
			shell := ""
			if h.IsShellCommand {
				shell = "✓"
			}
			b.WriteString(fmt.Sprintf("| %s | `%.8s` | %s | %s | %s |\n",
				ft(h.Timestamp), h.SessionID, h.Project, shell, cmd))
		}
		b.WriteString("\n")
	}

	// File History & Misc Stats
	b.WriteString(fmt.Sprintf("## 11. %s\n\n", dict["file_history"]))
	b.WriteString(fmt.Sprintf("- **%s**: %d\n", dict["file_hist_sessions"], report.FileHistoryStats.SessionCount))
	b.WriteString(fmt.Sprintf("- **%s**: %d\n\n", dict["file_hist_versions"], report.FileHistoryStats.TotalFileVersions))

	b.WriteString(fmt.Sprintf("## 12. %s\n\n", dict["misc_stats"]))
	b.WriteString(fmt.Sprintf("- **%s**: %d\n", dict["shell_snapshots"], report.MiscStats.ShellSnapshots))
	b.WriteString(fmt.Sprintf("- **%s**: %d\n", dict["paste_cache"], report.MiscStats.PasteCacheFiles))
	b.WriteString(fmt.Sprintf("- **%s**: %d\n", dict["task_sessions"], report.MiscStats.TaskSessions))
	b.WriteString(fmt.Sprintf("- **%s**: %d\n", dict["plan_files"], report.MiscStats.PlanFiles))
	b.WriteString(fmt.Sprintf("- **%s**: %d\n\n", dict["custom_commands"], report.MiscStats.CustomCommands))

	// Conversations
	if len(report.Conversations) > 0 {
		b.WriteString(fmt.Sprintf("## 13. %s\n\n", dict["conversations"]))
		for _, conv := range report.Conversations {
			title := conv.Title
			if title == "" {
				title = conv.SessionID[:8]
			}
			b.WriteString(fmt.Sprintf("### %s (%s)\n\n", title, conv.Project))
			for _, msg := range conv.Messages {
				prefix := "**User**: "
				if msg.Role == "assistant" {
					prefix = "**Assistant**: "
				}
				content := msg.Content
				if len(content) > 500 {
					content = clip(content, 500)
				}
				content = strings.ReplaceAll(content, "\n", " ")
				b.WriteString(fmt.Sprintf("- [%s] %s%s\n", ft(msg.Timestamp), prefix, content))
			}
			b.WriteString("\n")
		}
	}

	writeInjectionMarkdown(&b, report, dict, ft)

	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return nil, err
	}

	return []OutputFile{{Path: path, Size: fileSize(path)}}, nil
}

// writeInjectionMarkdown renders the triage smallest-first: the findings a human
// should open, then the sessions they live in, then the flagged content itself.
// The full inventory stays in the CSV — it runs to thousands of rows.
func writeInjectionMarkdown(b *strings.Builder, report *model.ForensicReport, dict Dict, ft func(time.Time) string) {
	inj := report.Injection

	b.WriteString(fmt.Sprintf("## 14. %s\n\n", dict["injection"]))
	b.WriteString(dict["inj_intro"] + "\n\n")
	b.WriteString(fmt.Sprintf("- **%s**: %d\n", dict["inj_scanned"], inj.ScannedResults))
	if inj.SignalsDropped > 0 {
		b.WriteString(fmt.Sprintf("- **%s**: %d\n", dict["inj_dropped"], inj.SignalsDropped))
	}
	b.WriteString("\n")

	if len(inj.Findings) == 0 && len(inj.Sessions) == 0 {
		b.WriteString(dict["inj_none"] + "\n\n")
		return
	}
	b.WriteString("> " + dict["inj_caveat"] + "\n\n")

	if len(inj.Findings) > 0 {
		b.WriteString(fmt.Sprintf("### %s\n\n", dict["inj_findings"]))
		b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s |\n",
			dict["severity"], dict["rule"], dict["session_id"], dict["project"], dict["summary"]))
		b.WriteString("|---|---|---|---|---|\n")
		for _, f := range inj.Findings {
			b.WriteString(fmt.Sprintf("| %s | %s | `%.8s` | %s | %s |\n",
				strings.ToUpper(f.Severity), f.Rule, f.SessionID, f.Project, mdCell(f.Summary)))
		}
		b.WriteString("\n")
	}

	if reviewable := reviewableSessions(inj.Sessions); len(reviewable) > 0 {
		b.WriteString(fmt.Sprintf("### %s\n\n", dict["inj_sessions"]))
		b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | %s | %s | %s | %s |\n",
			dict["session_id"], dict["project"], dict["started_at"], dict["net_ingress"],
			dict["file_in"], dict["ctx_injection"], dict["egress"], dict["config_changes"], dict["top_severity"]))
		b.WriteString("|---|---|---|---|---|---|---|---|---|\n")
		for _, s := range reviewable {
			b.WriteString(fmt.Sprintf("| `%.8s` | %s | %s | %d | %d | %d | %d | %d | %s |\n",
				s.SessionID, s.Project, ft(s.StartedAt), s.NetworkIngress, s.FileIngress,
				s.ContextInjection, s.Egress, s.ConfigChanges, strings.ToUpper(s.TopSeverity)))
		}
		b.WriteString("\n")
	}

	flagged := flaggedEvents(inj.Events)
	if len(flagged) > 0 {
		b.WriteString(fmt.Sprintf("### %s\n\n", dict["inj_flagged"]))
		b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | %s |\n",
			dict["timestamp"], dict["session_id"], dict["category"], dict["detail"], dict["rule"], dict["excerpt"]))
		b.WriteString("|---|---|---|---|---|---|\n")
		for _, ev := range flagged {
			for _, sig := range ev.Signals {
				b.WriteString(fmt.Sprintf("| %s | `%.8s` | %s | %s | %s (%s) | %s |\n",
					ft(ev.Timestamp), ev.SessionID, ev.Category, mdCell(ev.Detail),
					sig.Rule, sig.Severity, mdCell(sig.Excerpt)))
			}
		}
		b.WriteString("\n")
	}

	b.WriteString(dict["inj_full_inventory"] + "\n\n")
}

// reviewableSessions drops the sessions with nothing to look at: on a real
// machine they are the overwhelming majority.
func reviewableSessions(sessions []model.SessionTriage) []model.SessionTriage {
	var out []model.SessionTriage
	for _, s := range sessions {
		if s.SignalCount > 0 || s.Egress > 0 {
			out = append(out, s)
		}
	}
	return out
}

func flaggedEvents(events []model.InjectionEvent) []model.InjectionEvent {
	var out []model.InjectionEvent
	for _, ev := range events {
		if len(ev.Signals) > 0 {
			out = append(out, ev)
		}
	}
	return out
}

// clip cuts to a byte budget without splitting a rune. Slicing a string at an
// arbitrary byte offset produces invalid UTF-8, which any non-ASCII report will
// hit — and a broken byte makes the whole HTML file unparseable.
func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && s[n]&0xC0 == 0x80 {
		n--
	}
	return s[:n] + "..."
}

func mdCell(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	return strings.ReplaceAll(s, "\n", " ")
}

func writeField(b *strings.Builder, label, value string) {
	if value != "" {
		b.WriteString(fmt.Sprintf("- **%s**: %s\n", label, value))
	}
}

func boolStr(v bool, dict Dict) string {
	if v {
		return dict["yes"]
	}
	return dict["no"]
}

func formatInt64(n int64) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var parts []string
	for len(s) > 3 {
		parts = append([]string{s[len(s)-3:]}, parts...)
		s = s[:len(s)-3]
	}
	parts = append([]string{s}, parts...)
	return strings.Join(parts, ",")
}

func formatTimeIn(t time.Time, tz *time.Location) string {
	if t.IsZero() {
		return "-"
	}
	return t.In(tz).Format("2006-01-02 15:04:05")
}

func formatDateIn(t time.Time, tz *time.Location) string {
	if t.IsZero() {
		return "-"
	}
	return t.In(tz).Format("2006-01-02")
}

func sortedKeys(m map[string]model.TokenSummary) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
