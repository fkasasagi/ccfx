package analyzer

import (
	"testing"
)

// MessageCount was assigned per transcript while ToolUseCount and Tokens were
// accumulated across them, so one Session reported the message count of only the
// last transcript file alongside the tool count of all of them.
func TestSessionSpanningProjectsAccumulatesCounters(t *testing.T) {
	report := Analyze(spanningRawData(), &Options{})

	if len(report.Sessions) != 1 {
		t.Fatalf("Sessions = %d, want 1 (both transcripts are one session)", len(report.Sessions))
	}
	s := report.Sessions[0]

	if s.MessageCount != 8 {
		t.Errorf("MessageCount = %d, want 8 (3 in p1 + 5 in p2)", s.MessageCount)
	}
	if s.ToolUseCount != 3 {
		t.Errorf("ToolUseCount = %d, want 3 (1 in p1 + 2 in p2)", s.ToolUseCount)
	}

	// The projects section counts every transcript, so its totals are the
	// reference the session counters have to match.
	var projMessages, projToolUses int
	for _, p := range report.Projects {
		projMessages += p.TotalMessages
		projToolUses += p.TotalToolUses
	}
	if projMessages != s.MessageCount {
		t.Errorf("projects total_messages = %d but session MessageCount = %d; the two sections disagree",
			projMessages, s.MessageCount)
	}
	if projToolUses != s.ToolUseCount {
		t.Errorf("projects total_tool_uses = %d but session ToolUseCount = %d; the two sections disagree",
			projToolUses, s.ToolUseCount)
	}
}

// SessionCount is per project, so a session active in two projects is counted by
// each of them. Summing it across projects therefore may exceed the number of
// sessions — that is the relation being one-to-many, not a miscount, and the
// filter fix must not be read as promising equality.
func TestProjectSessionCountsArePerProject(t *testing.T) {
	report := Analyze(spanningRawData(), &Options{})

	if len(report.Projects) != 2 {
		t.Fatalf("Projects = %d, want 2", len(report.Projects))
	}
	for _, p := range report.Projects {
		if p.SessionCount != 1 {
			t.Errorf("project %s: SessionCount = %d, want 1", p.Path, p.SessionCount)
		}
	}
}
