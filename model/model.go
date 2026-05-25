package model

import "time"

type ForensicReport struct {
	Meta          ReportMeta         `json:"meta"`
	UserIdentity  UserIdentity       `json:"user_identity"`
	Sessions      []Session          `json:"sessions"`
	Timeline      []TimelineEntry    `json:"timeline"`
	Projects      []ProjectSummary   `json:"projects"`
	ToolUsage     ToolUsageReport    `json:"tool_usage"`
	TokenUsage    TokenUsageReport   `json:"token_usage"`
	FileChanges   []FileChange       `json:"file_changes"`
	Permissions   PermissionReport   `json:"permissions"`
	Credentials   CredentialReport   `json:"credentials"`
	CommandHistory []HistoryEntry    `json:"command_history,omitempty"`
	FileHistoryStats FileHistoryStats `json:"file_history_stats"`
	MiscStats     MiscStats          `json:"misc_stats"`
	Conversations []Conversation     `json:"conversations,omitempty"`
}

type ReportMeta struct {
	GeneratedAt   time.Time `json:"generated_at"`
	SourcePath    string    `json:"source_path"`
	ToolVersion   string    `json:"tool_version"`
	Platform      string    `json:"platform"`
	TotalSessions int       `json:"total_sessions"`
	TotalProjects int       `json:"total_projects"`
	DateRange     DateRange `json:"date_range"`
}

type DateRange struct {
	Earliest time.Time `json:"earliest"`
	Latest   time.Time `json:"latest"`
}

type UserIdentity struct {
	Email            string `json:"email,omitempty"`
	AccountUUID      string `json:"account_uuid,omitempty"`
	OrganizationUUID string `json:"organization_uuid,omitempty"`
	OrganizationName string `json:"organization_name,omitempty"`
	OrganizationType string `json:"organization_type,omitempty"`
	OrganizationRole string `json:"organization_role,omitempty"`
	RateLimitTier    string `json:"rate_limit_tier,omitempty"`
	ClaudeCodeVersion string `json:"claude_code_version,omitempty"`
}

type Session struct {
	SessionID      string       `json:"session_id"`
	PID            int          `json:"pid,omitempty"`
	Project        string       `json:"project"`
	CWD            string       `json:"cwd"`
	StartedAt      time.Time    `json:"started_at"`
	UpdatedAt      time.Time    `json:"updated_at,omitempty"`
	DurationSec    float64      `json:"duration_sec,omitempty"`
	Version        string       `json:"version"`
	Entrypoint     string       `json:"entrypoint"`
	Kind           string       `json:"kind"`
	Status         string       `json:"status"`
	Title          string       `json:"title,omitempty"`
	GitBranch      string       `json:"git_branch,omitempty"`
	Model          string       `json:"model,omitempty"`
	MessageCount   int          `json:"message_count"`
	ToolUseCount   int          `json:"tool_use_count"`
	PermissionMode string       `json:"permission_mode,omitempty"`
	Tokens         TokenSummary `json:"tokens,omitempty"`
}

type TokenSummary struct {
	InputTokens         int64 `json:"input_tokens"`
	OutputTokens        int64 `json:"output_tokens"`
	CacheCreationTokens int64 `json:"cache_creation_tokens"`
	CacheReadTokens     int64 `json:"cache_read_tokens"`
}

type TimelineEntry struct {
	Timestamp time.Time `json:"timestamp"`
	SessionID string    `json:"session_id"`
	Project   string    `json:"project"`
	EventType string    `json:"event_type"`
	Summary   string    `json:"summary"`
	Model     string    `json:"model,omitempty"`
	GitBranch string    `json:"git_branch,omitempty"`
}

type ProjectSummary struct {
	Path           string    `json:"path"`
	EncodedDirName string    `json:"encoded_dir_name"`
	SessionCount   int       `json:"session_count"`
	FirstSeen      time.Time `json:"first_seen"`
	LastSeen       time.Time `json:"last_seen"`
	TotalMessages  int       `json:"total_messages"`
	TotalToolUses  int       `json:"total_tool_uses"`
}

type ToolUsageReport struct {
	ByTool   map[string]int            `json:"by_tool"`
	TopTools []ToolRanking             `json:"top_tools"`
}

type ToolRanking struct {
	ToolName     string `json:"tool_name"`
	TotalCalls   int    `json:"total_calls"`
	SessionCount int    `json:"session_count"`
}

type TokenUsageReport struct {
	TotalInput       int64                      `json:"total_input"`
	TotalOutput      int64                      `json:"total_output"`
	TotalCacheCreate int64                      `json:"total_cache_creation"`
	TotalCacheRead   int64                      `json:"total_cache_read"`
	ByModel          map[string]TokenSummary    `json:"by_model"`
	ByProject        map[string]TokenSummary    `json:"by_project"`
	ByDate           map[string]TokenSummary    `json:"by_date"`
}

type FileChange struct {
	Timestamp time.Time `json:"timestamp"`
	SessionID string    `json:"session_id"`
	FilePath  string    `json:"file_path"`
	ToolName  string    `json:"tool_name"`
	Operation string    `json:"operation"`
}

type PermissionReport struct {
	GlobalDenyRules  []string          `json:"global_deny_rules,omitempty"`
	GlobalAllowRules []string          `json:"global_allow_rules,omitempty"`
	LocalDenyRules   []string          `json:"local_deny_rules,omitempty"`
	LocalAllowRules  []string          `json:"local_allow_rules,omitempty"`
	SessionModes     map[string]string `json:"session_modes,omitempty"`
	HooksDefined     []HookEntry       `json:"hooks_defined,omitempty"`
}

type HookEntry struct {
	Event   string `json:"event"`
	Matcher string `json:"matcher,omitempty"`
	Command string `json:"command"`
}

type CredentialReport struct {
	FileExists    bool      `json:"file_exists"`
	FileModified  time.Time `json:"file_modified_at,omitempty"`
	FileSizeBytes int64     `json:"file_size_bytes,omitempty"`
	TokenDetected bool      `json:"oauth_token_detected"`
}

type Conversation struct {
	SessionID string            `json:"session_id"`
	Project   string            `json:"project"`
	Title     string            `json:"title,omitempty"`
	Messages  []ConversationMsg `json:"messages"`
}

type ConversationMsg struct {
	Timestamp time.Time `json:"timestamp"`
	Role      string    `json:"role"`
	Type      string    `json:"type"`
	Content   string    `json:"content"`
	Model     string    `json:"model,omitempty"`
	ToolName  string    `json:"tool_name,omitempty"`
}

// RawData is the intermediate structure between collector and analyzer.
type RawData struct {
	SourcePath     string
	HistoryEntries []HistoryEntry
	SessionFiles   []SessionFile
	Transcripts    []TranscriptSession
	BackupData     *BackupData
	GlobalSettings *SettingsData
	LocalSettings  *SettingsData
	CredentialInfo *CredentialReport
	FileHistStats  FileHistoryStats
	MiscStats      MiscStats
}

type HistoryEntry struct {
	Display   string    `json:"display"`
	Timestamp time.Time `json:"timestamp"`
	Project   string    `json:"project"`
	SessionID string    `json:"sessionId"`
}

type SessionFile struct {
	PID        int       `json:"pid"`
	SessionID  string    `json:"sessionId"`
	CWD        string    `json:"cwd"`
	StartedAt  time.Time `json:"startedAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
	Version    string    `json:"version"`
	Entrypoint string    `json:"entrypoint"`
	Kind       string    `json:"kind"`
	Status     string    `json:"status"`
}

type TranscriptSession struct {
	SessionID      string
	EncodedProject string
	FilePath       string
	Messages       []TranscriptMessage
	Title          string
	Model          string
	GitBranch      string
	PermissionMode string
}

type TranscriptMessage struct {
	Timestamp time.Time
	Role      string
	Type      string
	Content   string
	Model     string
	ToolCalls []ToolCall
	Tokens    *TokenSummary
}

type ToolCall struct {
	ToolName string
	Input    string
}

type BackupData struct {
	Email            string
	AccountUUID      string
	OrganizationUUID string
	OrganizationName string
	OrganizationType string
	OrganizationRole string
	RateLimitTier    string
	Projects         map[string]BackupProjectInfo
}

type BackupProjectInfo struct {
	Path      string
	CostUSD   float64
	Tokens    TokenSummary
}

type SettingsData struct {
	AllowRules []string
	DenyRules  []string
	Hooks      []HookEntry
}

type FileHistoryStats struct {
	SessionCount      int `json:"session_count"`
	TotalFileVersions int `json:"total_file_versions"`
}

type MiscStats struct {
	ShellSnapshots  int `json:"shell_snapshots"`
	PasteCacheFiles int `json:"paste_cache_files"`
	TaskSessions    int `json:"task_sessions"`
	PlanFiles       int `json:"plan_files"`
	CustomCommands  int `json:"custom_commands"`
}
