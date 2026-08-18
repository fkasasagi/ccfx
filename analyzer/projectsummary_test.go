package analyzer

import (
	"testing"
	"time"

	"github.com/fkasasagi/ccfx/collector"
	"github.com/fkasasagi/ccfx/model"
)

func analyzeFixture(t *testing.T, opts *Options) *model.ForensicReport {
	t.Helper()
	raw, err := collector.Collect(testDataDir, false)
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}
	return Analyze(raw, opts)
}

// totalsOf reduces the project section to the numbers an investigator reads off
// it, so a mismatch reports the whole picture rather than one field at a time.
func totalsOf(projects []model.ProjectSummary) (sessions, messages, toolUses int) {
	for _, p := range projects {
		sessions += p.SessionCount
		messages += p.TotalMessages
		toolUses += p.TotalToolUses
	}
	return
}

// The project section must describe the sessions the report actually contains.
// It was built from every transcript on disk regardless of --session-filter,
// --project-filter and --date-*, so a filtered report claimed more sessions and
// messages under "projects" than it listed under "sessions" — two sections of
// one report contradicting each other.
func TestProjectSummariesRespectFilters(t *testing.T) {
	const sessionA = "aaaa1111-2222-3333-4444-555566667777"

	full := analyzeFixture(t, &Options{})
	fullSessions, fullMessages, _ := totalsOf(full.Projects)
	if fullSessions != len(full.Sessions) {
		t.Fatalf("unfiltered baseline is already inconsistent: projects claim %d sessions, report lists %d",
			fullSessions, len(full.Sessions))
	}

	t.Run("session filter", func(t *testing.T) {
		r := analyzeFixture(t, &Options{SessionFilter: sessionA})
		if len(r.Sessions) != 1 {
			t.Fatalf("fixture precondition: got %d sessions, want 1", len(r.Sessions))
		}

		gotSessions, gotMessages, _ := totalsOf(r.Projects)
		if gotSessions != len(r.Sessions) {
			t.Errorf("projects claim %d sessions, report lists %d", gotSessions, len(r.Sessions))
		}
		if gotMessages >= fullMessages {
			t.Errorf("TotalMessages = %d with one session filtered in, want fewer than the unfiltered %d",
				gotMessages, fullMessages)
		}
		if gotMessages != r.Sessions[0].MessageCount {
			t.Errorf("TotalMessages = %d, want %d (the one session's MessageCount)",
				gotMessages, r.Sessions[0].MessageCount)
		}
	})

	t.Run("project filter excluding everything", func(t *testing.T) {
		r := analyzeFixture(t, &Options{ProjectFilter: "/nonexistent"})
		if len(r.Sessions) != 0 {
			t.Fatalf("fixture precondition: got %d sessions, want 0", len(r.Sessions))
		}
		if len(r.Projects) != 0 {
			t.Errorf("Projects = %d entries, want 0 when no session survives the filter: %+v",
				len(r.Projects), r.Projects)
		}
		if r.Meta.TotalProjects != 0 {
			t.Errorf("Meta.TotalProjects = %d, want 0", r.Meta.TotalProjects)
		}
	})

	t.Run("date filter excluding everything", func(t *testing.T) {
		cutoff := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
		r := analyzeFixture(t, &Options{DateFrom: &cutoff})
		if len(r.Sessions) != 0 {
			t.Fatalf("fixture precondition: got %d sessions, want 0", len(r.Sessions))
		}
		if len(r.Projects) != 0 {
			t.Errorf("Projects = %d entries, want 0 when every session is out of range: %+v",
				len(r.Projects), r.Projects)
		}
	})
}

// Filtering must not change the unfiltered report: the fix narrows what is
// summarized, it does not re-define how the totals are counted.
func TestProjectSummariesUnfilteredUnchanged(t *testing.T) {
	r := analyzeFixture(t, &Options{})

	if len(r.Projects) != 1 {
		t.Fatalf("Projects = %d, want 1", len(r.Projects))
	}
	p := r.Projects[0]
	if p.Path != "/home/testuser/myproject" {
		t.Errorf("Path = %q", p.Path)
	}
	if p.EncodedDirName != "-home-testuser-myproject" {
		t.Errorf("EncodedDirName = %q", p.EncodedDirName)
	}
	if p.SessionCount != 2 {
		t.Errorf("SessionCount = %d, want 2", p.SessionCount)
	}
	if p.TotalMessages != 6 {
		t.Errorf("TotalMessages = %d, want 6", p.TotalMessages)
	}
	if p.TotalToolUses != 4 {
		t.Errorf("TotalToolUses = %d, want 4", p.TotalToolUses)
	}
}

// A session is not guaranteed to stay inside one project directory: resuming in
// a different working directory writes further transcript lines for the same
// session ID under a second encoded project. buildSessions folds those files
// into one Session, so the per-session counters must all agree on what "one
// session" totals.
func spanningRawData() *model.RawData {
	const sid = "dddd1111-2222-3333-4444-555566667777"
	msg := func(min int, tools int) model.TranscriptMessage {
		m := model.TranscriptMessage{
			Timestamp: time.Date(2024, 5, 12, 13, min, 0, 0, time.UTC),
			Role:      "user",
			Type:      "user",
		}
		for i := 0; i < tools; i++ {
			m.ToolCalls = append(m.ToolCalls, model.ToolCall{ToolName: "Bash"})
		}
		return m
	}

	return &model.RawData{
		SourcePath: "memory",
		Transcripts: []model.TranscriptSession{
			{
				SessionID:      sid,
				EncodedProject: "-home-t-p1",
				CWD:            "/home/t/p1",
				Messages:       []model.TranscriptMessage{msg(0, 1), msg(1, 0), msg(2, 0)},
			},
			{
				SessionID:      sid,
				EncodedProject: "-home-t-p2",
				CWD:            "/home/t/p2",
				Messages:       []model.TranscriptMessage{msg(10, 1), msg(11, 1), msg(12, 0), msg(13, 0), msg(14, 0)},
			},
		},
	}
}

// A session file with no transcript has no project to belong to. The project
// section is accumulated from transcripts, so such a session must simply not
// appear there — it must not be attached to some other project's bucket.
func TestSessionWithoutTranscriptJoinsNoProject(t *testing.T) {
	raw := spanningRawData()
	raw.SessionFiles = []model.SessionFile{
		{
			SessionID: "eeee1111-2222-3333-4444-555566667777",
			PID:       4242,
			StartedAt: time.Date(2024, 5, 12, 15, 0, 0, 0, time.UTC),
			Version:   "2.1.0",
		},
	}

	report := Analyze(raw, &Options{})

	var orphan *model.Session
	for i := range report.Sessions {
		if report.Sessions[i].SessionID == "eeee1111-2222-3333-4444-555566667777" {
			orphan = &report.Sessions[i]
		}
	}
	if orphan == nil {
		t.Fatal("the transcript-less session is missing from Sessions")
	}
	if orphan.Project != "" {
		t.Errorf("Project = %q, want empty (no transcript names one)", orphan.Project)
	}

	var counted int
	for _, p := range report.Projects {
		if p.Path == "" {
			t.Errorf("Projects contains an empty-path entry: %+v", p)
		}
		counted += p.SessionCount
	}
	// Only the spanning session's two project memberships may be counted.
	if counted != 2 {
		t.Errorf("project SessionCount total = %d, want 2 (the orphan must not be counted)", counted)
	}
}
