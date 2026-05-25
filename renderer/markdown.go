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
		summary := e.Summary
		if len(summary) > 80 {
			summary = summary[:80] + "..."
		}
		summary = strings.ReplaceAll(summary, "|", "\\|")
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
		for m, t := range tu.ByModel {
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

	// Conversations
	if len(report.Conversations) > 0 {
		b.WriteString(fmt.Sprintf("## 10. %s\n\n", dict["conversations"]))
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
					content = content[:500] + "..."
				}
				content = strings.ReplaceAll(content, "\n", " ")
				b.WriteString(fmt.Sprintf("- [%s] %s%s\n", ft(msg.Timestamp), prefix, content))
			}
			b.WriteString("\n")
		}
	}

	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return nil, err
	}

	return []OutputFile{{Path: path, Size: fileSize(path)}}, nil
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
	return t.In(tz).Format("2006-01-02 15:04:05 MST")
}

func formatDateIn(t time.Time, tz *time.Location) string {
	if t.IsZero() {
		return "-"
	}
	return t.In(tz).Format("2006-01-02 MST")
}

func sortedKeys(m map[string]model.TokenSummary) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
