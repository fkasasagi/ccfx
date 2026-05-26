package analyzer

import (
	"testing"

	"github.com/fkasasagi/ccfx/collector"
)

const testDataDir = "../testdata/claude"

func TestAnalyze(t *testing.T) {
	raw, err := collector.Collect(testDataDir, false)
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	report := Analyze(raw, &Options{})

	// Meta
	if report.Meta.TotalSessions == 0 {
		t.Error("TotalSessions = 0")
	}
	if report.Meta.TotalProjects == 0 {
		t.Error("TotalProjects = 0")
	}

	// User Identity
	if report.UserIdentity.Email != "testuser@example.com" {
		t.Errorf("Email = %q, want %q", report.UserIdentity.Email, "testuser@example.com")
	}
	if report.UserIdentity.ClaudeCodeVersion != "2.1.0" {
		t.Errorf("ClaudeCodeVersion = %q, want %q", report.UserIdentity.ClaudeCodeVersion, "2.1.0")
	}

	// Sessions - no negative durations
	for _, s := range report.Sessions {
		if s.DurationSec < 0 {
			t.Errorf("session %s has negative duration: %f", s.SessionID, s.DurationSec)
		}
	}

	// Tool Usage
	if len(report.ToolUsage.TopTools) == 0 {
		t.Error("no tool usage recorded")
	}
	found := false
	for _, tr := range report.ToolUsage.TopTools {
		if tr.ToolName == "Bash" {
			found = true
			if tr.TotalCalls != 1 {
				t.Errorf("Bash TotalCalls = %d, want 1", tr.TotalCalls)
			}
		}
	}
	if !found {
		t.Error("Bash not found in tool usage")
	}

	// Token Usage
	if report.TokenUsage.TotalInput == 0 {
		t.Error("TotalInput = 0")
	}
	if report.TokenUsage.TotalOutput == 0 {
		t.Error("TotalOutput = 0")
	}

	// File Changes
	if len(report.FileChanges) == 0 {
		t.Error("no file changes recorded")
	}

	// Permissions
	if len(report.Permissions.GlobalDenyRules) != 1 {
		t.Errorf("GlobalDenyRules = %d, want 1", len(report.Permissions.GlobalDenyRules))
	}
	if len(report.Permissions.GlobalAllowRules) != 2 {
		t.Errorf("GlobalAllowRules = %d, want 2", len(report.Permissions.GlobalAllowRules))
	}

	// Command History
	if len(report.CommandHistory) != 3 {
		t.Errorf("CommandHistory = %d, want 3", len(report.CommandHistory))
	}

	// Timeline
	if len(report.Timeline) == 0 {
		t.Error("no timeline entries")
	}

	// Projects
	if len(report.Projects) == 0 {
		t.Error("no projects")
	}
}

func TestAnalyzeWithFilters(t *testing.T) {
	raw, err := collector.Collect(testDataDir, false)
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	// Session filter
	report := Analyze(raw, &Options{
		SessionFilter: "aaaa1111-2222-3333-4444-555566667777",
	})
	for _, s := range report.Sessions {
		if s.SessionID != "aaaa1111-2222-3333-4444-555566667777" {
			t.Errorf("session filter leaked: %s", s.SessionID)
		}
	}
	if len(report.CommandHistory) != 2 {
		t.Errorf("filtered CommandHistory = %d, want 2", len(report.CommandHistory))
	}
}

func TestRedactPII(t *testing.T) {
	raw, err := collector.Collect(testDataDir, false)
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	report := Analyze(raw, &Options{RedactPII: true})

	if report.UserIdentity.Email == "testuser@example.com" {
		t.Error("email not redacted")
	}
	if report.UserIdentity.Email != "te***@example.com" {
		t.Errorf("redacted email = %q, want %q", report.UserIdentity.Email, "te***@example.com")
	}
	if report.UserIdentity.AccountUUID == "a1b2c3d4-e5f6-7890-abcd-ef1234567890" {
		t.Error("UUID not redacted")
	}
	if report.UserIdentity.AccountUUID != "a1b2c3d4-****-****-****-************" {
		t.Errorf("redacted UUID = %q, want %q", report.UserIdentity.AccountUUID, "a1b2c3d4-****-****-****-************")
	}
}

func TestExtractConversations(t *testing.T) {
	raw, err := collector.Collect(testDataDir, false)
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	// Without flag
	report := Analyze(raw, &Options{})
	if len(report.Conversations) != 0 {
		t.Errorf("Conversations without flag = %d, want 0", len(report.Conversations))
	}

	// With flag
	report = Analyze(raw, &Options{ExtractConversations: true})
	if len(report.Conversations) == 0 {
		t.Error("Conversations with flag = 0")
	}
}
