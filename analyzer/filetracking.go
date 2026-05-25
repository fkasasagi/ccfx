package analyzer

import (
	"encoding/json"
	"time"

	"github.com/fkasasagi/ccfx/model"
)

func buildFileChanges(raw *model.RawData, projectMap map[string]string, opts *Options) []model.FileChange {
	var changes []model.FileChange

	for _, ts := range raw.Transcripts {
		if !matchesFilter(ts, projectMap, opts) {
			continue
		}

		for _, msg := range ts.Messages {
			if msg.Role != "assistant" {
				continue
			}
			if !matchesDateFilter(msg.Timestamp, opts) {
				continue
			}

			for _, tc := range msg.ToolCalls {
				fc := extractFileChange(tc, msg.Timestamp, ts.SessionID)
				if fc != nil {
					changes = append(changes, *fc)
				}
			}
		}
	}

	return changes
}

func extractFileChange(tc model.ToolCall, ts time.Time, sessionID string) *model.FileChange {
	switch tc.ToolName {
	case "Edit":
		return parseToolChange(tc, ts, sessionID, "edit")
	case "Write":
		return parseToolChange(tc, ts, sessionID, "create")
	default:
		return nil
	}
}

func parseToolChange(tc model.ToolCall, ts time.Time, sessionID, operation string) *model.FileChange {
	var input struct {
		FilePath string `json:"file_path"`
	}
	if err := json.Unmarshal([]byte(tc.Input), &input); err != nil || input.FilePath == "" {
		return nil
	}
	return &model.FileChange{
		Timestamp: ts,
		SessionID: sessionID,
		FilePath:  input.FilePath,
		ToolName:  tc.ToolName,
		Operation: operation,
	}
}
