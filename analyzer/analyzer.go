package analyzer

import (
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/fkasasagi/ccfx/model"
)

type Options struct {
	ExtractConversations bool
	SessionFilter        string
	ProjectFilter        string
	DateFrom             *time.Time
	DateTo               *time.Time
	RedactPII            bool
}

func Analyze(raw *model.RawData, opts *Options) *model.ForensicReport {
	report := &model.ForensicReport{}
	report.Meta.SourcePath = raw.SourcePath
	report.Meta.GeneratedAt = time.Now().UTC()

	projectMap := buildProjectMap(raw)

	report.Sessions = buildSessions(raw, projectMap, opts)
	report.Meta.TotalSessions = len(report.Sessions)

	report.Timeline = buildTimeline(raw, projectMap, opts)
	report.Projects = buildProjectSummaries(raw, projectMap, report.Sessions)
	report.Meta.TotalProjects = len(report.Projects)

	report.ToolUsage = buildToolUsage(raw, opts)
	report.TokenUsage = buildTokenUsage(raw, projectMap, opts)
	report.FileChanges = buildFileChanges(raw, projectMap, opts)
	report.Permissions = buildPermissions(raw)
	report.UserIdentity = buildUserIdentity(raw)

	if raw.CredentialInfo != nil {
		report.Credentials = *raw.CredentialInfo
	}

	report.CommandHistory = filterHistory(raw.HistoryEntries, opts)
	report.FileHistoryStats = raw.FileHistStats
	report.MiscStats = raw.MiscStats
	report.Injection = buildInjection(raw, projectMap, opts)

	if opts.ExtractConversations {
		report.Conversations = extractConversations(raw, projectMap, opts)
	}

	report.Meta.DateRange = computeDateRange(report.Sessions, report.Timeline)

	if opts.RedactPII {
		redact(report)
	}

	return report
}

func buildProjectMap(raw *model.RawData) map[string]string {
	pm := make(map[string]string)

	if raw.BackupData != nil {
		for projPath := range raw.BackupData.Projects {
			encoded := encodeProjectPath(projPath)
			pm[encoded] = projPath
		}
	}

	for _, ts := range raw.Transcripts {
		if _, ok := pm[ts.EncodedProject]; !ok {
			pm[ts.EncodedProject] = decodeProjectPath(ts.EncodedProject)
		}
	}

	return pm
}

func encodeProjectPath(path string) string {
	return strings.ReplaceAll(path, "/", "-")
}

func decodeProjectPath(encoded string) string {
	if encoded == "" {
		return ""
	}
	if encoded[0] == '-' {
		return "/" + strings.ReplaceAll(encoded[1:], "-", "/")
	}
	return strings.ReplaceAll(encoded, "-", "/")
}

func computeDateRange(sessions []model.Session, timeline []model.TimelineEntry) model.DateRange {
	dr := model.DateRange{}
	var earliest, latest time.Time

	for _, s := range sessions {
		if !s.StartedAt.IsZero() {
			if earliest.IsZero() || s.StartedAt.Before(earliest) {
				earliest = s.StartedAt
			}
			if latest.IsZero() || s.StartedAt.After(latest) {
				latest = s.StartedAt
			}
		}
	}
	for _, t := range timeline {
		if !t.Timestamp.IsZero() {
			if earliest.IsZero() || t.Timestamp.Before(earliest) {
				earliest = t.Timestamp
			}
			if latest.IsZero() || t.Timestamp.After(latest) {
				latest = t.Timestamp
			}
		}
	}

	dr.Earliest = earliest
	dr.Latest = latest
	return dr
}

var emailRegex = regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)
var uuidRegex = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

func redact(report *model.ForensicReport) {
	report.UserIdentity.Email = redactEmail(report.UserIdentity.Email)
	report.UserIdentity.AccountUUID = redactUUID(report.UserIdentity.AccountUUID)
	report.UserIdentity.OrganizationUUID = redactUUID(report.UserIdentity.OrganizationUUID)

	// Injection previews and excerpts are verbatim quotes of whatever entered the
	// session, so they are exactly where stray identifiers turn up.
	for i := range report.Injection.Events {
		redactEvent(&report.Injection.Events[i])
	}
	for i := range report.Injection.Findings {
		f := &report.Injection.Findings[i]
		f.Summary = redactText(f.Summary)
		for j := range f.Evidence {
			redactEvent(&f.Evidence[j])
		}
	}
}

func redactEvent(ev *model.InjectionEvent) {
	ev.Detail = redactText(ev.Detail)
	ev.Preview = redactText(ev.Preview)
	for i := range ev.Signals {
		ev.Signals[i].Excerpt = redactText(ev.Signals[i].Excerpt)
	}
}

func redactText(s string) string {
	if s == "" {
		return ""
	}
	s = emailRegex.ReplaceAllStringFunc(s, redactEmail)
	return uuidRegex.ReplaceAllStringFunc(s, redactUUID)
}

func redactEmail(email string) string {
	if email == "" {
		return ""
	}
	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 {
		return "***"
	}
	user := parts[0]
	if len(user) > 2 {
		return user[:2] + "***@" + parts[1]
	}
	return "***@" + parts[1]
}

func redactUUID(uuid string) string {
	if uuid == "" {
		return ""
	}
	if len(uuid) > 8 {
		return uuid[:8] + "-****-****-****-************"
	}
	return "***"
}

func filterHistory(entries []model.HistoryEntry, opts *Options) []model.HistoryEntry {
	if len(entries) == 0 {
		return nil
	}
	var result []model.HistoryEntry
	for _, e := range entries {
		if opts.SessionFilter != "" && e.SessionID != opts.SessionFilter {
			continue
		}
		if opts.ProjectFilter != "" && e.Project != opts.ProjectFilter {
			continue
		}
		if !matchesDateFilter(e.Timestamp, opts) {
			continue
		}
		result = append(result, e)
	}
	return result
}

func matchesFilter(ts model.TranscriptSession, projectMap map[string]string, opts *Options) bool {
	if opts.SessionFilter != "" && ts.SessionID != opts.SessionFilter {
		return false
	}
	if opts.ProjectFilter != "" {
		proj := projectMap[ts.EncodedProject]
		if proj != opts.ProjectFilter {
			return false
		}
	}
	return true
}

func matchesDateFilter(t time.Time, opts *Options) bool {
	if t.IsZero() {
		return true
	}
	if opts.DateFrom != nil && t.Before(*opts.DateFrom) {
		return false
	}
	if opts.DateTo != nil && t.After(*opts.DateTo) {
		return false
	}
	return true
}

func buildUserIdentity(raw *model.RawData) model.UserIdentity {
	ui := model.UserIdentity{}
	if raw.BackupData == nil {
		return ui
	}
	ui.Email = raw.BackupData.Email
	ui.AccountUUID = raw.BackupData.AccountUUID
	ui.OrganizationUUID = raw.BackupData.OrganizationUUID
	ui.OrganizationName = raw.BackupData.OrganizationName
	ui.OrganizationType = raw.BackupData.OrganizationType
	ui.OrganizationRole = raw.BackupData.OrganizationRole
	ui.RateLimitTier = raw.BackupData.RateLimitTier

	for _, sf := range raw.SessionFiles {
		if sf.Version != "" {
			ui.ClaudeCodeVersion = sf.Version
			break
		}
	}
	return ui
}

func buildSessions(raw *model.RawData, projectMap map[string]string, opts *Options) []model.Session {
	sessionIndex := make(map[string]*model.Session)

	for _, sf := range raw.SessionFiles {
		s := &model.Session{
			SessionID:  sf.SessionID,
			PID:        sf.PID,
			CWD:        sf.CWD,
			StartedAt:  sf.StartedAt,
			UpdatedAt:  sf.UpdatedAt,
			Version:    sf.Version,
			Entrypoint: sf.Entrypoint,
			Kind:       sf.Kind,
			Status:     sf.Status,
		}
		if !sf.UpdatedAt.IsZero() && !sf.StartedAt.IsZero() {
			dur := sf.UpdatedAt.Sub(sf.StartedAt).Seconds()
			if dur > 0 {
				s.DurationSec = dur
			}
		}
		sessionIndex[sf.SessionID] = s
	}

	for _, ts := range raw.Transcripts {
		proj := projectMap[ts.EncodedProject]

		s, ok := sessionIndex[ts.SessionID]
		if !ok {
			s = &model.Session{SessionID: ts.SessionID}
			sessionIndex[ts.SessionID] = s
		}
		s.Project = proj
		s.Title = ts.Title
		s.Model = ts.Model
		s.GitBranch = ts.GitBranch
		s.PermissionMode = ts.PermissionMode
		s.MessageCount = len(ts.Messages)

		for _, msg := range ts.Messages {
			s.ToolUseCount += len(msg.ToolCalls)
			if msg.Tokens != nil {
				s.Tokens.InputTokens += msg.Tokens.InputTokens
				s.Tokens.OutputTokens += msg.Tokens.OutputTokens
				s.Tokens.CacheCreationTokens += msg.Tokens.CacheCreationTokens
				s.Tokens.CacheReadTokens += msg.Tokens.CacheReadTokens
			}
			if s.StartedAt.IsZero() && !msg.Timestamp.IsZero() {
				s.StartedAt = msg.Timestamp
			}
			if !msg.Timestamp.IsZero() {
				s.UpdatedAt = msg.Timestamp
			}
		}
		if s.DurationSec == 0 && !s.UpdatedAt.IsZero() && !s.StartedAt.IsZero() {
			dur := s.UpdatedAt.Sub(s.StartedAt).Seconds()
			if dur > 0 {
				s.DurationSec = dur
			}
		}
	}

	var sessions []model.Session
	for _, s := range sessionIndex {
		if opts.SessionFilter != "" && s.SessionID != opts.SessionFilter {
			continue
		}
		if opts.ProjectFilter != "" && s.Project != opts.ProjectFilter {
			continue
		}
		if !matchesDateFilter(s.StartedAt, opts) {
			continue
		}
		sessions = append(sessions, *s)
	}

	// sessionIndex is a map, so iteration order is random. Sort to a total order
	// (SessionID breaks ties) — a forensic report must be byte-reproducible.
	sort.Slice(sessions, func(i, j int) bool {
		if !sessions[i].StartedAt.Equal(sessions[j].StartedAt) {
			return sessions[i].StartedAt.Before(sessions[j].StartedAt)
		}
		return sessions[i].SessionID < sessions[j].SessionID
	})

	return sessions
}

func extractConversations(raw *model.RawData, projectMap map[string]string, opts *Options) []model.Conversation {
	var convs []model.Conversation

	for _, ts := range raw.Transcripts {
		if !matchesFilter(ts, projectMap, opts) {
			continue
		}

		conv := model.Conversation{
			SessionID: ts.SessionID,
			Project:   projectMap[ts.EncodedProject],
			Title:     ts.Title,
		}

		for _, msg := range ts.Messages {
			if !matchesDateFilter(msg.Timestamp, opts) {
				continue
			}
			cm := model.ConversationMsg{
				Timestamp: msg.Timestamp,
				Role:      msg.Role,
				Type:      msg.Type,
				Content:   msg.Content,
				Model:     msg.Model,
			}
			if len(msg.ToolCalls) > 0 {
				cm.ToolName = msg.ToolCalls[0].ToolName
			}
			conv.Messages = append(conv.Messages, cm)
		}

		if len(conv.Messages) > 0 {
			convs = append(convs, conv)
		}
	}

	return convs
}
