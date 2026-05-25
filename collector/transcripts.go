package collector

import (
	"bufio"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fkasasagi/ccfx/model"
)

type rawTranscriptLine struct {
	Type           string          `json:"type"`
	Timestamp      string          `json:"timestamp"`
	SessionID      string          `json:"sessionId"`
	CWD            string          `json:"cwd"`
	Version        string          `json:"version"`
	GitBranch      string          `json:"gitBranch"`
	PermissionMode string          `json:"permissionMode"`
	Message        json.RawMessage `json:"message"`
	UUID           string          `json:"uuid"`
}

type rawMessage struct {
	Role    string          `json:"role"`
	Model   string          `json:"model"`
	Content json.RawMessage `json:"content"`
	Usage   *rawUsage       `json:"usage"`
}

type rawUsage struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
}

type rawContentItem struct {
	Type  string          `json:"type"`
	Text  string          `json:"text"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

func parseAllTranscripts(projectsDir string, verbose bool) ([]model.TranscriptSession, error) {
	projEntries, err := os.ReadDir(projectsDir)
	if err != nil {
		return nil, err
	}

	var all []model.TranscriptSession

	for _, pe := range projEntries {
		if !pe.IsDir() {
			continue
		}
		encodedProject := pe.Name()
		projPath := filepath.Join(projectsDir, encodedProject)

		entries, err := os.ReadDir(projPath)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
				continue
			}
			sessionID := strings.TrimSuffix(e.Name(), ".jsonl")
			filePath := filepath.Join(projPath, e.Name())

			ts, err := parseTranscript(filePath, sessionID, encodedProject)
			if err != nil {
				if verbose {
					log.Printf("transcript %s: %v", filePath, err)
				}
				continue
			}
			all = append(all, *ts)
		}
	}
	return all, nil
}

func parseTranscript(path, sessionID, encodedProject string) (*model.TranscriptSession, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	ts := &model.TranscriptSession{
		SessionID:      sessionID,
		EncodedProject: encodedProject,
		FilePath:       path,
	}

	scanner := bufio.NewScanner(f)
	buf := make([]byte, 4<<20)
	scanner.Buffer(buf, 4<<20)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var raw rawTranscriptLine
		if err := json.Unmarshal(line, &raw); err != nil {
			continue
		}

		switch raw.Type {
		case "user", "assistant":
			msg := parseTranscriptMessage(raw)
			if msg != nil {
				ts.Messages = append(ts.Messages, *msg)
				if raw.Type == "assistant" && msg.Model != "" && ts.Model == "" {
					ts.Model = msg.Model
				}
			}
			if raw.GitBranch != "" && ts.GitBranch == "" {
				ts.GitBranch = raw.GitBranch
			}
			if raw.PermissionMode != "" {
				ts.PermissionMode = raw.PermissionMode
			}

		case "ai-title":
			var titleData struct {
				Title string `json:"title"`
			}
			if raw.Message != nil {
				if err := json.Unmarshal(raw.Message, &titleData); err == nil && titleData.Title != "" {
					ts.Title = titleData.Title
				}
			}

		case "permission-mode":
			if raw.PermissionMode != "" {
				ts.PermissionMode = raw.PermissionMode
			}
		}
	}

	return ts, scanner.Err()
}

func parseTranscriptMessage(raw rawTranscriptLine) *model.TranscriptMessage {
	ts, _ := time.Parse(time.RFC3339Nano, raw.Timestamp)
	if ts.IsZero() {
		ts, _ = time.Parse("2006-01-02T15:04:05.000Z", raw.Timestamp)
	}

	msg := &model.TranscriptMessage{
		Timestamp: ts,
		Role:      raw.Type,
		Type:      "text",
	}

	if raw.Message == nil {
		return msg
	}

	var rm rawMessage
	if err := json.Unmarshal(raw.Message, &rm); err != nil {
		return msg
	}

	msg.Model = rm.Model

	if rm.Usage != nil {
		msg.Tokens = &model.TokenSummary{
			InputTokens:         rm.Usage.InputTokens,
			OutputTokens:        rm.Usage.OutputTokens,
			CacheCreationTokens: rm.Usage.CacheCreationInputTokens,
			CacheReadTokens:     rm.Usage.CacheReadInputTokens,
		}
	}

	if rm.Content == nil {
		return msg
	}

	// content can be a string or an array of objects
	var contentStr string
	if err := json.Unmarshal(rm.Content, &contentStr); err == nil {
		msg.Content = contentStr
		return msg
	}

	var items []rawContentItem
	if err := json.Unmarshal(rm.Content, &items); err != nil {
		return msg
	}

	var textParts []string
	for _, item := range items {
		switch item.Type {
		case "text":
			textParts = append(textParts, item.Text)
		case "tool_use":
			msg.ToolCalls = append(msg.ToolCalls, model.ToolCall{
				ToolName: item.Name,
				Input:    truncate(string(item.Input), 200),
			})
		case "tool_result":
			msg.Type = "tool_result"
		}
	}
	if len(textParts) > 0 {
		msg.Content = strings.Join(textParts, "\n")
	}

	return msg
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
