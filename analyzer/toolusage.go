package analyzer

import (
	"sort"

	"github.com/fkasasagi/ccfx/model"
)

func buildToolUsage(raw *model.RawData, opts *Options) model.ToolUsageReport {
	byTool := make(map[string]int)
	byToolSession := make(map[string]map[string]bool)

	for _, ts := range raw.Transcripts {
		if opts.SessionFilter != "" && ts.SessionID != opts.SessionFilter {
			continue
		}
		for _, msg := range ts.Messages {
			for _, tc := range msg.ToolCalls {
				byTool[tc.ToolName]++
				if byToolSession[tc.ToolName] == nil {
					byToolSession[tc.ToolName] = make(map[string]bool)
				}
				byToolSession[tc.ToolName][ts.SessionID] = true
			}
		}
	}

	var rankings []model.ToolRanking
	for name, count := range byTool {
		rankings = append(rankings, model.ToolRanking{
			ToolName:     name,
			TotalCalls:   count,
			SessionCount: len(byToolSession[name]),
		})
	}
	sort.Slice(rankings, func(i, j int) bool {
		return rankings[i].TotalCalls > rankings[j].TotalCalls
	})

	return model.ToolUsageReport{
		ByTool:   byTool,
		TopTools: rankings,
	}
}
