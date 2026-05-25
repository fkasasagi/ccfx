package analyzer

import (
	"sort"

	"github.com/fkasasagi/ccfx/model"
)

func buildPermissions(raw *model.RawData) model.PermissionReport {
	report := model.PermissionReport{
		SessionModes: make(map[string]string),
	}

	if raw.GlobalSettings != nil {
		report.GlobalDenyRules = raw.GlobalSettings.DenyRules
		report.GlobalAllowRules = raw.GlobalSettings.AllowRules
		report.HooksDefined = raw.GlobalSettings.Hooks
	}

	if raw.LocalSettings != nil {
		report.LocalDenyRules = raw.LocalSettings.DenyRules
		report.LocalAllowRules = raw.LocalSettings.AllowRules
	}

	for _, ts := range raw.Transcripts {
		if ts.PermissionMode != "" {
			report.SessionModes[ts.SessionID] = ts.PermissionMode
		}
	}

	return report
}

func buildProjectSummaries(raw *model.RawData, projectMap map[string]string, sessions []model.Session) []model.ProjectSummary {
	type projAccum struct {
		path        string
		encoded     string
		sessions    map[string]bool
		firstSeen   interface{}
		lastSeen    interface{}
		messages    int
		toolUses    int
	}

	accum := make(map[string]*projAccum)

	for _, ts := range raw.Transcripts {
		proj := projectMap[ts.EncodedProject]
		a, ok := accum[proj]
		if !ok {
			a = &projAccum{
				path:     proj,
				encoded:  ts.EncodedProject,
				sessions: make(map[string]bool),
			}
			accum[proj] = a
		}
		a.sessions[ts.SessionID] = true
		a.messages += len(ts.Messages)
		for _, msg := range ts.Messages {
			a.toolUses += len(msg.ToolCalls)
		}
	}

	for _, s := range sessions {
		if a, ok := accum[s.Project]; ok {
			a.sessions[s.SessionID] = true
		}
	}

	var summaries []model.ProjectSummary
	for _, a := range accum {
		ps := model.ProjectSummary{
			Path:           a.path,
			EncodedDirName: a.encoded,
			SessionCount:   len(a.sessions),
			TotalMessages:  a.messages,
			TotalToolUses:  a.toolUses,
		}

		for _, s := range sessions {
			if s.Project == a.path {
				if ps.FirstSeen.IsZero() || (!s.StartedAt.IsZero() && s.StartedAt.Before(ps.FirstSeen)) {
					ps.FirstSeen = s.StartedAt
				}
				if ps.LastSeen.IsZero() || (!s.StartedAt.IsZero() && s.StartedAt.After(ps.LastSeen)) {
					ps.LastSeen = s.StartedAt
				}
			}
		}

		summaries = append(summaries, ps)
	}

	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].SessionCount > summaries[j].SessionCount
	})

	return summaries
}
