package collector

import (
	"fmt"
	"testing"
	"time"
)

// withLocalZone moves the process-local timezone away from UTC for the duration
// of a test, so that any timestamp the collector forgets to normalize surfaces
// as a non-UTC Location instead of silently passing on a UTC machine.
//
// FixedZone is used rather than LoadLocation because it needs no system
// zoneinfo database: the test then behaves identically on Linux, macOS and
// Windows, which is exactly the portability property under test.
func withLocalZone(t *testing.T, offsetHours int) {
	t.Helper()
	prev := time.Local
	time.Local = time.FixedZone("TEST", offsetHours*3600)
	t.Cleanup(func() { time.Local = prev })
}

func mustBeUTC(t *testing.T, name string, ts time.Time) {
	t.Helper()
	if ts.IsZero() {
		return
	}
	if ts.Location() != time.UTC {
		t.Errorf("%s: Location = %v, want UTC (value %s)",
			name, ts.Location(), ts.Format(time.RFC3339Nano))
	}
}

// Every time.Time the collector puts into the model must be UTC. The renderer
// is the only layer allowed to apply --timezone, so a value that arrives still
// carrying the examining machine's local zone leaks that zone into report.json
// and makes the same evidence serialize differently on different analyst
// machines. Two of the three sources below (epoch millis, file mtime) default
// to time.Local in Go, so this is a real regression risk, not a theoretical one.
func TestCollectNormalizesTimesToUTC(t *testing.T) {
	withLocalZone(t, 9) // pretend the analyst machine is in JST

	raw, err := Collect(testDataDir, false)
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	for i, h := range raw.HistoryEntries {
		mustBeUTC(t, fmt.Sprintf("HistoryEntries[%d].Timestamp", i), h.Timestamp)
	}

	for i, s := range raw.SessionFiles {
		mustBeUTC(t, fmt.Sprintf("SessionFiles[%d].StartedAt", i), s.StartedAt)
		mustBeUTC(t, fmt.Sprintf("SessionFiles[%d].UpdatedAt", i), s.UpdatedAt)
	}

	if raw.CredentialInfo != nil {
		mustBeUTC(t, "CredentialInfo.FileModified", raw.CredentialInfo.FileModified)
	}

	for i, ts := range raw.Transcripts {
		for j, m := range ts.Messages {
			mustBeUTC(t, fmt.Sprintf("Transcripts[%d].Messages[%d].Timestamp", i, j), m.Timestamp)
		}
		for j, e := range ts.ToolEvents {
			mustBeUTC(t, fmt.Sprintf("Transcripts[%d].ToolEvents[%d].Timestamp", i, j), e.Timestamp)
		}
		for j, a := range ts.Attachments {
			mustBeUTC(t, fmt.Sprintf("Transcripts[%d].Attachments[%d].Timestamp", i, j), a.Timestamp)
		}
		for j, p := range ts.PermissionChanges {
			mustBeUTC(t, fmt.Sprintf("Transcripts[%d].PermissionChanges[%d].Timestamp", i, j), p.Timestamp)
		}
	}
}

// A transcript line may carry a numeric UTC offset instead of "Z". time.Parse
// preserves that offset as a fixed zone, which would break the UTC invariant
// even on a machine whose local zone is UTC.
func TestParseTranscriptTimeNormalizesOffsets(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string // instant, as RFC3339 in UTC
	}{
		{"zulu", "2024-05-12T13:00:00.000Z", "2024-05-12T13:00:00Z"},
		{"positive offset", "2024-05-12T22:00:00+09:00", "2024-05-12T13:00:00Z"},
		{"negative offset", "2024-05-12T09:00:00-04:00", "2024-05-12T13:00:00Z"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseTranscriptTime(tc.in)
			if got.Location() != time.UTC {
				t.Errorf("Location = %v, want UTC", got.Location())
			}
			if got.Format(time.RFC3339) != tc.want {
				t.Errorf("got %s, want %s", got.Format(time.RFC3339), tc.want)
			}
		})
	}
}
