package analyzer

import (
	"fmt"
	"sort"

	"github.com/fkasasagi/ccfx/model"
)

func buildTimeline(raw *model.RawData, projectMap map[string]string, opts *Options) []model.TimelineEntry {
	var entries []model.TimelineEntry

	for _, ts := range raw.Transcripts {
		if !matchesFilter(ts, projectMap, opts) {
			continue
		}
		proj := projectMap[ts.EncodedProject]

		for _, msg := range ts.Messages {
			if !matchesDateFilter(msg.Timestamp, opts) {
				continue
			}

			switch msg.Role {
			case "user":
				entries = append(entries, model.TimelineEntry{
					Timestamp: msg.Timestamp,
					SessionID: ts.SessionID,
					Project:   proj,
					EventType: "user_message",
					Summary:   msg.Content,
					GitBranch: ts.GitBranch,
				})

			case "assistant":
				for _, tc := range msg.ToolCalls {
					entries = append(entries, model.TimelineEntry{
						Timestamp: msg.Timestamp,
						SessionID: ts.SessionID,
						Project:   proj,
						EventType: "tool_use",
						Summary:   fmt.Sprintf("Tool: %s", tc.ToolName),
						Model:     msg.Model,
						GitBranch: ts.GitBranch,
					})
				}
				if len(msg.ToolCalls) == 0 {
					entries = append(entries, model.TimelineEntry{
						Timestamp: msg.Timestamp,
						SessionID: ts.SessionID,
						Project:   proj,
						EventType: "assistant_response",
						Summary:   msg.Content,
						Model:     msg.Model,
						GitBranch: ts.GitBranch,
					})
				}
			}
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp.Before(entries[j].Timestamp)
	})

	return entries
}
