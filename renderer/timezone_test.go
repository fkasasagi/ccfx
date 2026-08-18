package renderer

import (
	"testing"
	"time"

	"github.com/fkasasagi/ccfx/model"
)

// Section 14 (injection) is the part of the report an investigator correlates
// against other evidence, so its timestamps must land in the same zone as every
// other timestamp in report.json. convertTimezones previously skipped the whole
// InjectionReport, leaving section 14 in UTC while the rest of the same file was
// converted — and disagreeing with injection_events.csv, which does convert.
func TestConvertTimezonesConvertsInjection(t *testing.T) {
	jst := time.FixedZone("JST", 9*3600)
	base := time.Date(2024, 5, 12, 13, 0, 10, 0, time.UTC)

	report := &model.ForensicReport{
		Injection: model.InjectionReport{
			Events: []model.InjectionEvent{
				{Timestamp: base, SessionID: "s1", Category: model.CatFileIngress},
			},
			Findings: []model.InjectionFinding{
				{
					Rule:     "r1",
					Severity: "high",
					Evidence: []model.InjectionEvent{
						{Timestamp: base, SessionID: "s1", Category: model.CatEgress},
					},
				},
			},
			Sessions: []model.SessionTriage{
				{SessionID: "s1", StartedAt: base},
			},
		},
	}

	got := convertTimezones(report, jst)

	checks := []struct {
		name string
		ts   time.Time
	}{
		{"Events[0].Timestamp", got.Injection.Events[0].Timestamp},
		{"Findings[0].Evidence[0].Timestamp", got.Injection.Findings[0].Evidence[0].Timestamp},
		{"Sessions[0].StartedAt", got.Injection.Sessions[0].StartedAt},
	}
	for _, c := range checks {
		if c.ts.Location() != jst {
			t.Errorf("%s: Location = %v, want JST", c.name, c.ts.Location())
		}
		if want := "2024-05-12 22:00:10"; c.ts.Format("2006-01-02 15:04:05") != want {
			t.Errorf("%s: got %s, want %s", c.name, c.ts.Format("2006-01-02 15:04:05"), want)
		}
	}

	// The input must not be mutated: writeJSON renders from a converted copy
	// while the CSV/Markdown/HTML writers still read the original report.
	if report.Injection.Events[0].Timestamp.Location() != time.UTC {
		t.Errorf("source report was mutated: Location = %v, want UTC",
			report.Injection.Events[0].Timestamp.Location())
	}
}

// report.json must not depend on the collectors having remembered to normalize.
// If any timestamp still carries a local zone, rendering with the default UTC
// setting has to bring it back to UTC rather than stamp the analyst machine's
// offset onto the evidence.
func TestConvertTimezonesNormalizesLocalTimesUnderUTC(t *testing.T) {
	jst := time.FixedZone("JST", 9*3600)
	local := time.Date(2024, 5, 12, 22, 0, 10, 0, jst) // 13:00:10 UTC

	report := &model.ForensicReport{
		Meta: model.ReportMeta{GeneratedAt: local},
		Sessions: []model.Session{
			{SessionID: "s1", StartedAt: local},
		},
		CommandHistory: []model.HistoryEntry{
			{Display: "cmd", Timestamp: local},
		},
		Injection: model.InjectionReport{
			Events: []model.InjectionEvent{{Timestamp: local, SessionID: "s1"}},
		},
	}

	got := convertTimezones(report, time.UTC)

	checks := []struct {
		name string
		ts   time.Time
	}{
		{"Meta.GeneratedAt", got.Meta.GeneratedAt},
		{"Sessions[0].StartedAt", got.Sessions[0].StartedAt},
		{"CommandHistory[0].Timestamp", got.CommandHistory[0].Timestamp},
		{"Injection.Events[0].Timestamp", got.Injection.Events[0].Timestamp},
	}
	for _, c := range checks {
		if c.ts.Location() != time.UTC {
			t.Errorf("%s: Location = %v, want UTC", c.name, c.ts.Location())
		}
		if want := "2024-05-12T13:00:10Z"; c.ts.Format(time.RFC3339) != want {
			t.Errorf("%s: got %s, want %s", c.name, c.ts.Format(time.RFC3339), want)
		}
	}
}

// A zero timestamp must stay zero so it keeps serializing as null rather than
// being shifted into a bogus year-1 local time.
func TestConvertTimezonesKeepsZeroTimes(t *testing.T) {
	jst := time.FixedZone("JST", 9*3600)

	report := &model.ForensicReport{
		Injection: model.InjectionReport{
			Sessions: []model.SessionTriage{{SessionID: "s1"}},
		},
	}

	got := convertTimezones(report, jst)

	if !got.Injection.Sessions[0].StartedAt.IsZero() {
		t.Errorf("zero StartedAt became %v", got.Injection.Sessions[0].StartedAt)
	}
}
