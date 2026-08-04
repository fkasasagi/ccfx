package collector

import (
	"bufio"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fkasasagi/ccfx/detect"
	"github.com/fkasasagi/ccfx/model"
)

// toolInputCap bounds what we keep of a tool_use input. It has to be generous
// enough that file_path and the head of a Bash command survive.
const toolInputCap = 4096

// sessionSignalCap stops one pathological transcript from filling memory with
// signals. Anything dropped is counted, never silently discarded.
const sessionSignalCap = 300

const previewLen = 160

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
	ToolUseResult  json.RawMessage `json:"toolUseResult"`
	Attachment     json.RawMessage `json:"attachment"`
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
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ID        string          `json:"id"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
}

// rawToolInput picks out the fields that say *what* a tool was pointed at.
type rawToolInput struct {
	Command  string `json:"command"`
	URL      string `json:"url"`
	FilePath string `json:"file_path"`
	Path     string `json:"path"`
	Query    string `json:"query"`
	Pattern  string `json:"pattern"`
	Prompt   string `json:"prompt"`
}

// rawToolUseResult is Claude Code's structured result record. Field names vary
// by tool; the zero values simply stay empty for tools that lack them.
type rawToolUseResult struct {
	URL      string          `json:"url"`
	Query    string          `json:"query"`
	Bytes    int64           `json:"bytes"`
	Stdout   string          `json:"stdout"`
	Stderr   string          `json:"stderr"`
	FilePath string          `json:"filePath"`
	File     json.RawMessage `json:"file"`
}

type rawAttachment struct {
	Type      string          `json:"type"`
	HookName  string          `json:"hookName"`
	HookEvent string          `json:"hookEvent"`
	Command   string          `json:"command"`
	Content   json.RawMessage `json:"content"`
	Stdout    string          `json:"stdout"`
	Stderr    string          `json:"stderr"`
	Prompt    string          `json:"prompt"`
	Snippet   string          `json:"snippet"`
	Filename  string          `json:"filename"`
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

// transcriptState carries the cross-line bookkeeping: a tool_use and its result
// arrive on different lines, and "how many user turns ago" only means something
// in sequence.
type transcriptState struct {
	pending     map[string]int // tool_use_id -> index into ts.ToolEvents
	signalCount int
	dropped     int
	scanned     int
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
	st := &transcriptState{pending: make(map[string]int)}

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
			recordToolActivity(ts, st, raw)
			if raw.GitBranch != "" && ts.GitBranch == "" {
				ts.GitBranch = raw.GitBranch
			}
			if raw.PermissionMode != "" {
				ts.PermissionMode = raw.PermissionMode
			}

		case "attachment":
			recordAttachment(ts, st, raw)

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
				ts.PermissionChanges = append(ts.PermissionChanges, model.PermissionChange{
					Timestamp: parseTranscriptTime(raw.Timestamp),
					Mode:      raw.PermissionMode,
				})
			}
		}
	}

	ts.ScannedResults = st.scanned
	ts.SignalsDropped = st.dropped
	return ts, scanner.Err()
}

// recordToolActivity pairs each tool_use with the tool_result that answers it.
func recordToolActivity(ts *model.TranscriptSession, st *transcriptState, raw rawTranscriptLine) {
	if raw.Message == nil {
		return
	}
	var rm rawMessage
	if err := json.Unmarshal(raw.Message, &rm); err != nil {
		return
	}

	var items []rawContentItem
	if err := json.Unmarshal(rm.Content, &items); err != nil {
		return
	}

	when := parseTranscriptTime(raw.Timestamp)

	for _, item := range items {
		switch item.Type {
		case "tool_use":
			ev := model.ToolEvent{
				Timestamp: when,
				ToolName:  item.Name,
				ToolUseID: item.ID,
				Input:     truncate(string(item.Input), toolInputCap),
			}
			applyToolInput(&ev, item.Input)
			ts.ToolEvents = append(ts.ToolEvents, ev)
			if item.ID != "" {
				st.pending[item.ID] = len(ts.ToolEvents) - 1
			}

		case "tool_result":
			idx, ok := st.pending[item.ToolUseID]
			if !ok {
				continue
			}
			delete(st.pending, item.ToolUseID)
			ev := &ts.ToolEvents[idx]
			ev.IsError = item.IsError

			body := contentText(item.Content)
			applyToolUseResult(ev, raw.ToolUseResult, &body)
			ev.Preview = preview(body)
			ev.Signals = takeSignals(st, detect.Scan(body))
			st.scanned++
		}
	}
}

// applyToolInput records what the tool was aimed at, from the call side.
func applyToolInput(ev *model.ToolEvent, input json.RawMessage) {
	if len(input) == 0 {
		return
	}
	var in rawToolInput
	if err := json.Unmarshal(input, &in); err != nil {
		return
	}
	ev.Command = in.Command
	ev.URL = in.URL
	ev.Query = in.Query
	switch {
	case in.FilePath != "":
		ev.FilePath = in.FilePath
	case in.Path != "":
		ev.FilePath = in.Path
	}
	if ev.Query == "" && in.Pattern != "" {
		ev.Query = in.Pattern
	}
}

// applyToolUseResult fills in what the tool actually returned, and supplies the
// scan body for tools whose tool_result block is empty.
func applyToolUseResult(ev *model.ToolEvent, raw json.RawMessage, body *string) {
	if len(raw) == 0 || raw[0] != '{' {
		return
	}
	var r rawToolUseResult
	if err := json.Unmarshal(raw, &r); err != nil {
		return
	}
	if r.URL != "" {
		ev.URL = r.URL
	}
	if r.Query != "" {
		ev.Query = r.Query
	}
	if r.Bytes > 0 {
		ev.ResultBytes = r.Bytes
	}
	if r.FilePath != "" {
		ev.FilePath = r.FilePath
	}
	if len(r.File) > 0 {
		var file struct {
			FilePath string `json:"filePath"`
			Content  string `json:"content"`
		}
		if err := json.Unmarshal(r.File, &file); err == nil {
			if file.FilePath != "" {
				ev.FilePath = file.FilePath
			}
			if *body == "" {
				*body = file.Content
			}
		}
	}
	if *body == "" {
		*body = strings.TrimSpace(r.Stdout + "\n" + r.Stderr)
	}
	if ev.ResultBytes == 0 {
		ev.ResultBytes = int64(len(*body))
	}
}

// recordAttachment captures text the harness injected into the conversation.
// hook_additional_context is the sharpest of these: a hook can put arbitrary
// text in front of the model on every turn.
func recordAttachment(ts *model.TranscriptSession, st *transcriptState, raw rawTranscriptLine) {
	if len(raw.Attachment) == 0 || raw.Attachment[0] != '{' {
		return
	}
	var a rawAttachment
	if err := json.Unmarshal(raw.Attachment, &a); err != nil {
		return
	}

	body := contentText(a.Content)
	for _, extra := range []string{a.Stdout, a.Stderr, a.Prompt, a.Snippet} {
		if body == "" {
			body = extra
		}
	}
	if body == "" && a.Command == "" && a.Filename == "" {
		return
	}

	ts.Attachments = append(ts.Attachments, model.AttachmentEvent{
		Timestamp: parseTranscriptTime(raw.Timestamp),
		Kind:      a.Type,
		HookName:  a.HookName,
		HookEvent: a.HookEvent,
		Command:   a.Command,
		Preview:   preview(body),
		Signals:   takeSignals(st, detect.Scan(body)),
	})
}

func takeSignals(st *transcriptState, signals []model.ContentSignal) []model.ContentSignal {
	if len(signals) == 0 {
		return nil
	}
	room := sessionSignalCap - st.signalCount
	if room <= 0 {
		st.dropped += len(signals)
		return nil
	}
	if len(signals) > room {
		st.dropped += len(signals) - room
		signals = signals[:room]
	}
	st.signalCount += len(signals)
	return signals
}

// contentText flattens the two shapes a content field takes: a bare string, or
// an array of blocks.
func contentText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var items []rawContentItem
	if err := json.Unmarshal(raw, &items); err != nil {
		return ""
	}
	var parts []string
	for _, it := range items {
		if it.Text != "" {
			parts = append(parts, it.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func parseTranscriptMessage(raw rawTranscriptLine) *model.TranscriptMessage {
	msg := &model.TranscriptMessage{
		Timestamp: parseTranscriptTime(raw.Timestamp),
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
				Input:    truncate(string(item.Input), toolInputCap),
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

func parseTranscriptTime(s string) time.Time {
	ts, _ := time.Parse(time.RFC3339Nano, s)
	if ts.IsZero() {
		ts, _ = time.Parse("2006-01-02T15:04:05.000Z", s)
	}
	return ts
}

// preview quotes the head of a tool result into the report, so it goes through
// the same secret masking as a signal excerpt.
func preview(s string) string {
	return truncate(detect.MaskSecrets(strings.Join(strings.Fields(s), " ")), previewLen)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && s[n]&0xC0 == 0x80 {
		n--
	}
	return s[:n] + "..."
}
