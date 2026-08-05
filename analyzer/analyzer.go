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

	// Priority order, highest first. The exact per-line cwd wins because it is
	// the native, non-lossy working directory Claude recorded (backslashes on
	// Windows), so it stays consistent with the raw paths in history.jsonl —
	// the backup file stores the same project in a mix of '/' and '\' forms and
	// must not shadow it.

	// 1. The exact per-line cwd. First transcript to name a project wins, so a
	//    project with several sessions resolves the same way every run
	//    (raw.Transcripts is directory-ordered, hence stable).
	for _, ts := range raw.Transcripts {
		if ts.CWD == "" {
			continue
		}
		if _, ok := pm[ts.EncodedProject]; !ok {
			pm[ts.EncodedProject] = ts.CWD
		}
	}

	// 2. The backup file's authoritative path, for projects no cwd covered.
	//    Iterate in sorted order: encodeProjectPath is not injective, so two
	//    backup paths can collide on one encoded key; a map's random iteration
	//    would pick the winner non-deterministically, and a forensic report
	//    must be byte-reproducible.
	if raw.BackupData != nil {
		projPaths := make([]string, 0, len(raw.BackupData.Projects))
		for projPath := range raw.BackupData.Projects {
			projPaths = append(projPaths, projPath)
		}
		sort.Strings(projPaths)
		for _, projPath := range projPaths {
			encoded := encodeProjectPath(projPath)
			if _, ok := pm[encoded]; !ok {
				pm[encoded] = projPath
			}
		}
	}

	// 3. Last resort: lossy decode of the encoded directory name.
	for _, ts := range raw.Transcripts {
		if _, ok := pm[ts.EncodedProject]; !ok {
			pm[ts.EncodedProject] = decodeProjectPath(ts.EncodedProject)
		}
	}

	return pm
}

// encodeProjectPath turns a filesystem path into the directory name Claude uses
// under ~/.claude/projects/. Claude replaces every non-alphanumeric character
// with '-' (so "/home/u/p" -> "-home-u-p" and, on Windows, "C:\Users\u\p" ->
// "C--Users-u-p"), one '-' per rune. Mirroring that exactly is what lets a
// backup's real project path match a transcript's encoded directory name on
// every platform — replacing only '/' left every Windows path (and any path
// with '.', spaces, or non-ASCII) unmatched.
func encodeProjectPath(path string) string {
	return strings.Map(func(r rune) rune {
		if isASCIIAlnum(r) {
			return r
		}
		return '-'
	}, path)
}

func isASCIIAlnum(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

// decodeProjectPath reconstructs a filesystem path from a Claude project
// directory name (an absolute cwd with every '/', '\' and ':' collapsed to
// '-'). The collapse is not invertible — a '-' in the result may have been any
// of those, or a literal '-' in a path segment — so this is a best-effort
// LAST RESORT. Callers should prefer the exact per-line cwd (TranscriptSession.CWD)
// and only fall back here when it is absent. Input is assumed to be a collapsed
// ABSOLUTE path: Unix absolute (leading '-') and Windows drive-letter ("X--")
// forms are recognized; anything else uses the generic '-' -> '/' mapping.
func decodeProjectPath(encoded string) string {
	if encoded == "" {
		return ""
	}
	// Windows drive-letter form: Claude encodes "C:\Users\x" as "C--Users-x"
	// because both ':' and '\' collapse to '-'. Reconstruct the drive letter and
	// backslash separators instead of the Unix-only "C//Users/x" mangling.
	// A Unix absolute path always starts with '-' (from its leading '/'), so the
	// "letter + '--'" prefix cannot collide with one.
	if len(encoded) >= 3 && isASCIILetter(encoded[0]) && encoded[1] == '-' && encoded[2] == '-' {
		return string(encoded[0]) + `:\` + strings.ReplaceAll(encoded[3:], "-", `\`)
	}
	if encoded[0] == '-' {
		return "/" + strings.ReplaceAll(encoded[1:], "-", "/")
	}
	return strings.ReplaceAll(encoded, "-", "/")
}

func isASCIILetter(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
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
