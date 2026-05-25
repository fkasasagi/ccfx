package analyzer

import (
	"github.com/fkasasagi/ccfx/model"
)

func buildTokenUsage(raw *model.RawData, projectMap map[string]string, opts *Options) model.TokenUsageReport {
	report := model.TokenUsageReport{
		ByModel:   make(map[string]model.TokenSummary),
		ByProject: make(map[string]model.TokenSummary),
		ByDate:    make(map[string]model.TokenSummary),
	}

	for _, ts := range raw.Transcripts {
		if !matchesFilter(ts, projectMap, opts) {
			continue
		}
		proj := projectMap[ts.EncodedProject]

		for _, msg := range ts.Messages {
			if msg.Tokens == nil {
				continue
			}
			if !matchesDateFilter(msg.Timestamp, opts) {
				continue
			}

			t := msg.Tokens
			report.TotalInput += t.InputTokens
			report.TotalOutput += t.OutputTokens
			report.TotalCacheCreate += t.CacheCreationTokens
			report.TotalCacheRead += t.CacheReadTokens

			if msg.Model != "" {
				m := report.ByModel[msg.Model]
				m.InputTokens += t.InputTokens
				m.OutputTokens += t.OutputTokens
				m.CacheCreationTokens += t.CacheCreationTokens
				m.CacheReadTokens += t.CacheReadTokens
				report.ByModel[msg.Model] = m
			}

			if proj != "" {
				p := report.ByProject[proj]
				p.InputTokens += t.InputTokens
				p.OutputTokens += t.OutputTokens
				p.CacheCreationTokens += t.CacheCreationTokens
				p.CacheReadTokens += t.CacheReadTokens
				report.ByProject[proj] = p
			}

			if !msg.Timestamp.IsZero() {
				dateKey := msg.Timestamp.Format("2006-01-02")
				d := report.ByDate[dateKey]
				d.InputTokens += t.InputTokens
				d.OutputTokens += t.OutputTokens
				d.CacheCreationTokens += t.CacheCreationTokens
				d.CacheReadTokens += t.CacheReadTokens
				report.ByDate[dateKey] = d
			}
		}
	}

	return report
}
