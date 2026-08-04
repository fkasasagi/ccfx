package analyzer

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/fkasasagi/ccfx/detect"
	"github.com/fkasasagi/ccfx/model"
)

// correlationWindow is how many tool calls later an outbound action still counts
// as "shortly after" the content that may have prompted it. Injections act
// promptly — they have to, before the context moves on.
const correlationWindow = 30

// detailLen keeps a URL or command readable in a table without wrapping the page.
const detailLen = 200

// maxEvidence bounds a finding: enough to see the chain, not the whole session.
const maxEvidence = 4

var (
	// Commands that pull material from outside the machine into the context.
	networkCmd = regexp.MustCompile(`(?i)\b(curl|wget|git\s+(clone|fetch|pull)|npm\s+(i|install|ci)|pnpm\s+add|yarn\s+add|pip3?\s+install|go\s+get|brew\s+install|apt(-get)?\s+install|gh\s+\w+\s+view)\b`)

	// Commands that push material out, or hand control to something fetched.
	// `git push` is deliberately absent: it is the most common write-out in a
	// normal session, and pushing to the user's own remote is not exfiltration.
	// Adding a *new* remote is the interesting act, and lands in configCmd.
	egressCmd = regexp.MustCompile(`(?i)(curl[^|;&]*(-d\s|--data|--form|-F\s|-T\s|--upload-file|-X\s*(POST|PUT|PATCH))|wget[^|;&]*--post|\bnc\s+\S+\s+\d+|/dev/tcp/|\|\s*(sudo\s+)?(ba)?sh\b|\bscp\s|\brsync\s+[^|;&]*\S+@|\bmail\s+-s)`)

	// Commands that change where this machine trusts or sends things.
	configCmd = regexp.MustCompile(`(?i)(git\s+remote\s+(add|set-url)|npm\s+config\s+set|git\s+config\s+.*url\.)`)

	// Reading these is how a session turns into a credential leak.
	sensitivePath = regexp.MustCompile(`(?i)(\.env(\.|$|\s)|id_rsa|id_ed25519|\.credentials\.json|\.aws/credentials|\.ssh/|\.netrc|\.pypirc|\.npmrc)`)

	// Changes here outlive the session that made them.
	configPath = regexp.MustCompile(`(?i)(settings\.json|settings\.local\.json|CLAUDE\.md|/\.claude/|\.mcp\.json|/hooks?/)`)

	// Commands that write, rather than read, a file.
	writeCmd = regexp.MustCompile(`(?i)(>{1,2}\s*\S|\btee\b|\bsed\s+-i|\bmv\b|\bcp\b|\bchmod\b)`)
)

var readTools = map[string]bool{
	"Read": true, "Grep": true, "Glob": true, "NotebookRead": true,
}

var writeTools = map[string]bool{
	"Edit": true, "Write": true, "NotebookEdit": true,
}

// buildInjection assembles the triage: what entered each session, what the text
// looked like, what left, and what was changed. It ranks, it never concludes.
func buildInjection(raw *model.RawData, projectMap map[string]string, opts *Options) model.InjectionReport {
	report := model.InjectionReport{}

	for _, ts := range raw.Transcripts {
		if !matchesFilter(ts, projectMap, opts) {
			continue
		}
		project := projectMap[ts.EncodedProject]

		events := sessionEvents(ts, project, opts)
		if len(events) == 0 {
			continue
		}

		report.ScannedResults += ts.ScannedResults
		report.SignalsDropped += ts.SignalsDropped
		report.Events = append(report.Events, events...)
		report.Findings = append(report.Findings, correlate(events)...)
		report.Sessions = append(report.Sessions, triage(ts, project, events))
	}

	sort.SliceStable(report.Events, func(i, j int) bool {
		return report.Events[i].Timestamp.Before(report.Events[j].Timestamp)
	})
	sort.SliceStable(report.Findings, func(i, j int) bool {
		a, b := report.Findings[i], report.Findings[j]
		if a.Severity != b.Severity {
			return severityRank(a.Severity) > severityRank(b.Severity)
		}
		return a.SessionID < b.SessionID
	})
	sort.SliceStable(report.Sessions, func(i, j int) bool {
		a, b := report.Sessions[i], report.Sessions[j]
		if a.TopSeverity != b.TopSeverity {
			return severityRank(a.TopSeverity) > severityRank(b.TopSeverity)
		}
		if !a.StartedAt.Equal(b.StartedAt) {
			return a.StartedAt.Before(b.StartedAt)
		}
		return a.SessionID < b.SessionID
	})

	return report
}

// sessionEvents turns one transcript into the flat event list everything else
// works from. Order follows the transcript, which is what makes "afterwards"
// meaningful.
func sessionEvents(ts model.TranscriptSession, project string, opts *Options) []model.InjectionEvent {
	var events []model.InjectionEvent

	for i, te := range ts.ToolEvents {
		if !matchesDateFilter(te.Timestamp, opts) {
			continue
		}
		// Older transcripts omit tool_use ids; position is then the identity.
		source := te.ToolUseID
		if source == "" {
			source = fmt.Sprintf("%s#%d", ts.SessionID, i)
		}
		for _, cat := range categorize(te) {
			events = append(events, model.InjectionEvent{
				Timestamp: te.Timestamp,
				SessionID: ts.SessionID,
				SourceID:  source,
				Project:   project,
				Category:  cat,
				ToolName:  te.ToolName,
				Detail:    detailOf(te),
				Bytes:     te.ResultBytes,
				IsError:   te.IsError,
				Preview:   te.Preview,
				Signals:   te.Signals,
			})
		}
	}

	for _, at := range ts.Attachments {
		if !matchesDateFilter(at.Timestamp, opts) {
			continue
		}
		// Only harness text that actually carries content is interesting; the
		// bookkeeping attachments (task reminders, listings) are noise here.
		if !injectedKind(at.Kind) {
			continue
		}
		detail := at.Kind
		if at.HookName != "" {
			detail = fmt.Sprintf("%s (%s %s)", at.Kind, at.HookName, at.HookEvent)
		}
		events = append(events, model.InjectionEvent{
			Timestamp: at.Timestamp,
			SessionID: ts.SessionID,
			Project:   project,
			Category:  model.CatContextInjection,
			ToolName:  at.Command,
			Detail:    truncateRunes(detail, detailLen),
			Preview:   at.Preview,
			Signals:   at.Signals,
		})
	}

	// Claude Code writes a permission-mode line routinely, not only when the mode
	// actually moves. Only transitions are events.
	previousMode := ""
	for _, pc := range ts.PermissionChanges {
		mode := pc.Mode
		if mode == previousMode {
			continue
		}
		previousMode = mode
		if !matchesDateFilter(pc.Timestamp, opts) {
			continue
		}
		if mode == "" || mode == "default" {
			continue
		}
		events = append(events, model.InjectionEvent{
			Timestamp: pc.Timestamp,
			SessionID: ts.SessionID,
			Project:   project,
			Category:  model.CatPermissionChange,
			Detail:    pc.Mode,
		})
	}

	sort.SliceStable(events, func(i, j int) bool {
		return events[i].Timestamp.Before(events[j].Timestamp)
	})
	return events
}

// categorize can return more than one category: `curl https://x | sh` is both
// how the material arrived and how it got executed.
func categorize(te model.ToolEvent) []string {
	var cats []string

	switch {
	case te.ToolName == "WebFetch", te.ToolName == "WebSearch":
		cats = append(cats, model.CatNetworkIngress)
		// A URL long enough to carry a payload is a documented exfiltration path.
		if len(te.URL) > 300 {
			cats = append(cats, model.CatEgress)
		}
	case strings.HasPrefix(te.ToolName, "mcp__"):
		cats = append(cats, model.CatNetworkIngress)
	case te.ToolName == "Agent", te.ToolName == "Task":
		// A subagent's answer is text the parent did not write.
		cats = append(cats, model.CatContextInjection)
	case readTools[te.ToolName]:
		cats = append(cats, model.CatFileIngress)
	case writeTools[te.ToolName]:
		if configPath.MatchString(te.FilePath) {
			cats = append(cats, model.CatConfigChange)
		}
	case te.ToolName == "Bash":
		if networkCmd.MatchString(te.Command) {
			cats = append(cats, model.CatNetworkIngress)
		}
		if egressCmd.MatchString(te.Command) {
			cats = append(cats, model.CatEgress)
		}
		if configCmd.MatchString(te.Command) ||
			(configPath.MatchString(te.Command) && writeCmd.MatchString(te.Command)) {
			cats = append(cats, model.CatConfigChange)
		}
		if sensitivePath.MatchString(te.Command) && len(cats) == 0 {
			cats = append(cats, model.CatFileIngress)
		}
	}

	return cats
}

func detailOf(te model.ToolEvent) string {
	switch {
	case te.URL != "":
		return truncateRunes(te.URL, detailLen)
	case te.Command != "":
		return truncateRunes(strings.Join(strings.Fields(te.Command), " "), detailLen)
	case te.FilePath != "":
		return truncateRunes(te.FilePath, detailLen)
	case te.Query != "":
		return truncateRunes(te.Query, detailLen)
	}
	return ""
}

// injectedKind lists the attachment kinds that put text in front of the model.
func injectedKind(kind string) bool {
	switch kind {
	case "hook_additional_context", "hook_success", "hook_system_message",
		"file", "edited_text_file", "queued_command", "compact_file_reference":
		return true
	}
	return false
}

// correlate is the part that matters: a signal on its own is a curiosity, a
// signal followed by an outbound action is a lead.
func correlate(events []model.InjectionEvent) []model.InjectionFinding {
	var findings []model.InjectionFinding
	covered := make(map[int]bool)

	// The window is measured in actions, not events. One action can produce two
	// events (ingress + egress), and that must not eat the budget twice.
	ordinal := make(map[string]int, len(events))
	order := make([]int, len(events))
	for i, ev := range events {
		key := ev.SourceID
		if key == "" {
			key = fmt.Sprintf("#%d", i)
		}
		if _, seen := ordinal[key]; !seen {
			ordinal[key] = len(ordinal)
		}
		order[i] = ordinal[key]
	}

	for i, ev := range events {
		if !isIngress(ev.Category) {
			continue
		}
		sev := detect.Severity(ev.Signals)
		sensitive := sensitivePath.MatchString(ev.Detail)
		if sev == "" && !sensitive {
			continue
		}

		// One finding per ingress: the first thing that followed is the lead. The
		// remaining twenty config writes in the window are the same story told
		// twenty times.
	search:
		for j := i + 1; j < len(events); j++ {
			if order[j]-order[i] > correlationWindow {
				break
			}
			follow := events[j]
			// `curl -d @.env https://…` is both ingress and egress, and is split
			// into two events. It must not be its own smoking gun.
			if follow.SourceID != "" && follow.SourceID == ev.SourceID {
				continue
			}
			switch follow.Category {
			case model.CatEgress:
				rule, summary := "ingress-then-egress",
					fmt.Sprintf("%s content entered the session, then %s went out shortly after",
						ingressLabel(ev), follow.Detail)
				if sensitive {
					rule = "sensitive-read-then-egress"
					summary = fmt.Sprintf("%s was read, then %s went out shortly after", ev.Detail, follow.Detail)
				}
				findings = append(findings, model.InjectionFinding{
					Rule:      rule,
					Severity:  "high",
					SessionID: ev.SessionID,
					Project:   ev.Project,
					Summary:   summary,
					Evidence:  evidence(ev, follow),
				})
				covered[i] = true
				break search

			case model.CatConfigChange, model.CatPermissionChange:
				if sev != "high" {
					continue
				}
				findings = append(findings, model.InjectionFinding{
					Rule:      "ingress-then-config-change",
					Severity:  "high",
					SessionID: ev.SessionID,
					Project:   ev.Project,
					Summary: fmt.Sprintf("%s content entered the session, then %s changed shortly after",
						ingressLabel(ev), follow.Detail),
					Evidence: evidence(ev, follow),
				})
				covered[i] = true
				break search
			}
		}
	}

	for i, ev := range events {
		if covered[i] {
			continue
		}
		sev := detect.Severity(ev.Signals)
		if sev == "" {
			continue
		}

		switch {
		case ev.Category == model.CatContextInjection && sev != "low":
			// Hook output is injected automatically, every turn, with no user in
			// the loop — the same text is worth more here than in a fetched page.
			findings = append(findings, model.InjectionFinding{
				Rule:      "signal-in-injected-context",
				Severity:  escalate(sev),
				SessionID: ev.SessionID,
				Project:   ev.Project,
				Summary:   fmt.Sprintf("harness-injected context (%s) matched %s", ev.Detail, ruleList(ev.Signals)),
				Evidence:  evidence(ev),
			})
		case isIngress(ev.Category) && sev == "high":
			findings = append(findings, model.InjectionFinding{
				Rule:      "high-signal-ingress",
				Severity:  "med",
				SessionID: ev.SessionID,
				Project:   ev.Project,
				Summary:   fmt.Sprintf("%s matched %s", ingressLabel(ev), ruleList(ev.Signals)),
				Evidence:  evidence(ev),
			})
		}
	}

	// Only the mode that removes the human from the loop is worth a finding on its
	// own; plan and acceptEdits are ordinary working modes.
	for _, ev := range events {
		if ev.Category != model.CatPermissionChange || !strings.EqualFold(ev.Detail, "bypassPermissions") {
			continue
		}
		findings = append(findings, model.InjectionFinding{
			Rule:      "permission-mode-escalation",
			Severity:  "high",
			SessionID: ev.SessionID,
			Project:   ev.Project,
			Summary:   "permission mode changed to bypassPermissions",
			Evidence:  evidence(ev),
		})
	}

	return findings
}

func isIngress(cat string) bool {
	return cat == model.CatNetworkIngress || cat == model.CatFileIngress || cat == model.CatContextInjection
}

func ingressLabel(ev model.InjectionEvent) string {
	if ev.Detail != "" {
		return ev.Detail
	}
	if ev.ToolName != "" {
		return ev.ToolName
	}
	return ev.Category
}

func ruleList(signals []model.ContentSignal) string {
	var names []string
	for _, s := range signals {
		names = append(names, s.Rule)
	}
	return strings.Join(names, ", ")
}

func escalate(sev string) string {
	if sev == "med" {
		return "high"
	}
	return sev
}

func evidence(events ...model.InjectionEvent) []model.InjectionEvent {
	if len(events) > maxEvidence {
		events = events[:maxEvidence]
	}
	return events
}

func triage(ts model.TranscriptSession, project string, events []model.InjectionEvent) model.SessionTriage {
	t := model.SessionTriage{SessionID: ts.SessionID, Project: project}
	if len(events) > 0 {
		t.StartedAt = events[0].Timestamp
	}
	// One action split across two categories still carries one set of signals.
	counted := make(map[string]bool)
	for _, ev := range events {
		switch ev.Category {
		case model.CatNetworkIngress:
			t.NetworkIngress++
		case model.CatFileIngress:
			t.FileIngress++
		case model.CatContextInjection:
			t.ContextInjection++
		case model.CatEgress:
			t.Egress++
		case model.CatConfigChange:
			t.ConfigChanges++
		}
		if ev.SourceID == "" || !counted[ev.SourceID] {
			counted[ev.SourceID] = true
			t.SignalCount += len(ev.Signals)
		}
		if sev := detect.Severity(ev.Signals); severityRank(sev) > severityRank(t.TopSeverity) {
			t.TopSeverity = sev
		}
	}
	return t
}

func severityRank(sev string) int {
	switch sev {
	case "high":
		return 3
	case "med":
		return 2
	case "low":
		return 1
	}
	return 0
}

func truncateRunes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && s[n]&0xC0 == 0x80 {
		n--
	}
	return s[:n] + "..."
}
