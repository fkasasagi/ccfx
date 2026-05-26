package collector

import (
	"testing"
)

const testDataDir = "../testdata/claude"

func TestCollect(t *testing.T) {
	raw, err := Collect(testDataDir, false)
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	if raw.SourcePath != testDataDir {
		t.Errorf("SourcePath = %q, want %q", raw.SourcePath, testDataDir)
	}

	// History
	if len(raw.HistoryEntries) != 3 {
		t.Errorf("HistoryEntries = %d, want 3", len(raw.HistoryEntries))
	}
	if len(raw.HistoryEntries) > 0 && raw.HistoryEntries[0].Display != "hello world" {
		t.Errorf("HistoryEntries[0].Display = %q, want %q", raw.HistoryEntries[0].Display, "hello world")
	}

	// Sessions
	if len(raw.SessionFiles) != 2 {
		t.Errorf("SessionFiles = %d, want 2", len(raw.SessionFiles))
	}

	// Zero updatedAt should be treated as zero time
	for _, sf := range raw.SessionFiles {
		if sf.SessionID == "bbbb1111-2222-3333-4444-555566667777" {
			if !sf.UpdatedAt.IsZero() {
				t.Errorf("session bbbb should have zero UpdatedAt, got %v", sf.UpdatedAt)
			}
		}
	}

	// Transcripts
	if len(raw.Transcripts) != 2 {
		t.Errorf("Transcripts = %d, want 2", len(raw.Transcripts))
	}

	// Backup
	if raw.BackupData == nil {
		t.Fatal("BackupData is nil")
	}
	if raw.BackupData.Email != "testuser@example.com" {
		t.Errorf("Email = %q, want %q", raw.BackupData.Email, "testuser@example.com")
	}
	if raw.BackupData.OrganizationType != "claude_max" {
		t.Errorf("OrganizationType = %q, want %q", raw.BackupData.OrganizationType, "claude_max")
	}

	// Settings
	if raw.GlobalSettings == nil {
		t.Fatal("GlobalSettings is nil")
	}
	if len(raw.GlobalSettings.AllowRules) != 2 {
		t.Errorf("AllowRules = %d, want 2", len(raw.GlobalSettings.AllowRules))
	}
	if len(raw.GlobalSettings.DenyRules) != 1 {
		t.Errorf("DenyRules = %d, want 1", len(raw.GlobalSettings.DenyRules))
	}
	if len(raw.GlobalSettings.Hooks) != 1 {
		t.Errorf("Hooks = %d, want 1", len(raw.GlobalSettings.Hooks))
	}

	// Credentials
	if raw.CredentialInfo == nil {
		t.Fatal("CredentialInfo is nil")
	}
	if !raw.CredentialInfo.FileExists {
		t.Error("CredentialInfo.FileExists = false, want true")
	}
	if !raw.CredentialInfo.TokenDetected {
		t.Error("CredentialInfo.TokenDetected = false, want true")
	}

	// File history
	if raw.FileHistStats.SessionCount != 1 {
		t.Errorf("FileHistStats.SessionCount = %d, want 1", raw.FileHistStats.SessionCount)
	}
	if raw.FileHistStats.TotalFileVersions != 2 {
		t.Errorf("FileHistStats.TotalFileVersions = %d, want 2", raw.FileHistStats.TotalFileVersions)
	}

	// Misc
	if raw.MiscStats.ShellSnapshots != 1 {
		t.Errorf("ShellSnapshots = %d, want 1", raw.MiscStats.ShellSnapshots)
	}
	if raw.MiscStats.PasteCacheFiles != 1 {
		t.Errorf("PasteCacheFiles = %d, want 1", raw.MiscStats.PasteCacheFiles)
	}
	if raw.MiscStats.TaskSessions != 1 {
		t.Errorf("TaskSessions = %d, want 1", raw.MiscStats.TaskSessions)
	}
	if raw.MiscStats.PlanFiles != 1 {
		t.Errorf("PlanFiles = %d, want 1", raw.MiscStats.PlanFiles)
	}
	if raw.MiscStats.CustomCommands != 1 {
		t.Errorf("CustomCommands = %d, want 1", raw.MiscStats.CustomCommands)
	}
}
