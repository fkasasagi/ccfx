package analyzer

import (
	"testing"

	"github.com/fkasasagi/ccfx/model"
)

// TestDecodeProjectPath pins the reconstruction of a Claude project directory
// name back into a filesystem path. Claude encodes the project cwd by collapsing
// every path separator (and, on Windows, the drive-letter ':') to '-', which is
// lossy — so decoding is best-effort. These cases lock in the behavior that
// matters: Unix absolute paths must round-trip, and Windows drive-letter paths
// must reconstruct with a real drive letter and backslashes instead of the
// mangled "C//Users/..." the Unix-only decoder produced.
func TestDecodeProjectPath(t *testing.T) {
	cases := []struct {
		name    string
		encoded string
		want    string
	}{
		// Unix absolute paths — characterization of pre-existing behavior.
		// These must not regress when Windows handling is added.
		{"unix absolute", "-home-uff-tlvb", "/home/uff/tlvb"},
		{"unix root only", "-", "/"},
		{"unix single segment", "-tmp", "/tmp"},

		// Windows drive-letter paths — the bug being fixed. Claude writes
		// "C:\Users\takay\ccfx" as "C--Users-takay-ccfx" (':' and '\' both -> '-').
		{"windows drive nested", "C--Users-takay-ccfx", `C:\Users\takay\ccfx`},
		{"windows drive shallow", "C--Users-takay", `C:\Users\takay`},
		{"windows drive root", "C--", `C:\`},
		{"windows lowercase drive", "d--data-repo", `d:\data\repo`},

		// Boundaries.
		{"empty", "", ""},
		// A bare letter with a single dash is not the "X--" drive pattern; it
		// stays on the generic '-' -> '/' fallback (unchanged behavior).
		{"single dash after letter", "C-Users", "C/Users"},

		// Lossy class, documented and accepted: a literal '-' inside a segment
		// (dir "my-app") is indistinguishable from a separator, so it splits
		// wrongly here. buildProjectMap avoids this by preferring the exact cwd;
		// this only bites when no cwd was recorded.
		{"literal hyphen in segment (lossy)", "C--my-app-v2", `C:\my\app\v2`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := decodeProjectPath(c.encoded)
			if got != c.want {
				t.Errorf("decodeProjectPath(%q) = %q, want %q", c.encoded, got, c.want)
			}
		})
	}
}

// TestBuildProjectMapPrefersCWD locks in the primary fix: the exact per-line
// cwd is used verbatim when present, and the lossy decodeProjectPath is only a
// fallback. Without this end-to-end test the decode unit tests give false
// confidence that Windows paths resolve correctly.
func TestBuildProjectMapPrefersCWD(t *testing.T) {
	raw := &model.RawData{
		Transcripts: []model.TranscriptSession{
			// cwd present -> used verbatim, no decoding.
			{EncodedProject: "C--Users-takay-ccfx", CWD: `C:\Users\takay\ccfx`},
			// no cwd -> lossy decode fallback.
			{EncodedProject: "C--Users-takay-notes", CWD: ""},
			// Windows UNC path: only the verbatim cwd reconstructs it correctly;
			// the encoded-name decoder never could.
			{EncodedProject: "--srv-share-proj", CWD: `\\srv\share\proj`},
		},
	}

	pm := buildProjectMap(raw)

	if got, want := pm["C--Users-takay-ccfx"], `C:\Users\takay\ccfx`; got != want {
		t.Errorf("cwd present: got %q, want exact cwd %q", got, want)
	}
	if got, want := pm["C--Users-takay-notes"], `C:\Users\takay\notes`; got != want {
		t.Errorf("no cwd (fallback decode): got %q, want %q", got, want)
	}
	if got, want := pm["--srv-share-proj"], `\\srv\share\proj`; got != want {
		t.Errorf("UNC via cwd: got %q, want %q", got, want)
	}
}

// TestEncodeProjectPath pins encodeProjectPath to Claude's real encoding: every
// non-alphanumeric rune becomes one '-'. The Windows and non-ASCII cases are
// actual ~/.claude/projects/ directory names observed for the given real paths,
// so this is ground-truth, not a guess.
func TestEncodeProjectPath(t *testing.T) {
	cases := []struct {
		name string
		path string
		want string
	}{
		{"windows nested", `C:\Users\takay\ccfx`, "C--Users-takay-ccfx"},
		{"windows shallow", `C:\Users\takay`, "C--Users-takay"},
		// 履歴書と経歴書 is 7 runes -> 7 dashes (plus one for the '\'): per-rune.
		{"windows non-ascii segment", "C:\\Users\\takay\\claude\\履歴書と経歴書", "C--Users-takay-claude--------"},
		{"unix", "/home/uff/tlvb", "-home-uff-tlvb"},
		// A '.' segment also collapses now; the old '/'-only version left it as
		// "my.app" and so never matched Claude's real "-my-app" directory.
		{"unix dotted segment", "/home/u/my.app", "-home-u-my-app"},
		{"empty", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := encodeProjectPath(c.path); got != c.want {
				t.Errorf("encodeProjectPath(%q) = %q, want %q", c.path, got, c.want)
			}
		})
	}
}

// TestBuildProjectMapMatchesWindowsBackup is the regression this fix targets:
// a project's authoritative path from the backup file must match its transcript
// on Windows. Before the fix, encodeProjectPath only replaced '/', so the
// backup key "C:\Users\takay\ccfx" never matched the encoded dir name and the
// map fell through to the lossy decoder. Here the transcript carries no cwd, so
// only the backup match can supply the correct path.
func TestBuildProjectMapMatchesWindowsBackup(t *testing.T) {
	raw := &model.RawData{
		BackupData: &model.BackupData{
			Projects: map[string]model.BackupProjectInfo{
				`C:\Users\takay\ccfx`: {Path: `C:\Users\takay\ccfx`},
			},
		},
		Transcripts: []model.TranscriptSession{
			{EncodedProject: "C--Users-takay-ccfx", CWD: ""},
		},
	}

	pm := buildProjectMap(raw)

	if got, want := pm["C--Users-takay-ccfx"], `C:\Users\takay\ccfx`; got != want {
		t.Errorf("windows backup match: got %q, want %q", got, want)
	}
}

// TestBuildProjectMapCWDBeatsBackup guards the precedence: when a project has
// both a per-line cwd and a backup entry, the native cwd must win. The backup
// file stores Windows paths in a mix of '/' and '\' forms, so letting it shadow
// cwd made one project show up as both "C:\Users\x" (cwd/history) and
// "C:/Users/x" (backup) across the report.
func TestBuildProjectMapCWDBeatsBackup(t *testing.T) {
	raw := &model.RawData{
		BackupData: &model.BackupData{
			Projects: map[string]model.BackupProjectInfo{
				`C:/Users/takay/ccfx`: {Path: `C:/Users/takay/ccfx`}, // forward-slash backup form
			},
		},
		Transcripts: []model.TranscriptSession{
			{EncodedProject: "C--Users-takay-ccfx", CWD: `C:\Users\takay\ccfx`}, // native cwd
		},
	}

	pm := buildProjectMap(raw)

	if got, want := pm["C--Users-takay-ccfx"], `C:\Users\takay\ccfx`; got != want {
		t.Errorf("cwd should win over backup: got %q, want %q", got, want)
	}
}

// TestBuildProjectMapBackupBeatsDecode covers the middle tier: with no cwd, the
// backup's real path is used instead of the lossy decoder, even when the two
// disagree. decode("-home-u-my-app") splits "my.app" wrongly into "my/app".
func TestBuildProjectMapBackupBeatsDecode(t *testing.T) {
	raw := &model.RawData{
		BackupData: &model.BackupData{
			Projects: map[string]model.BackupProjectInfo{
				"/home/u/my.app": {Path: "/home/u/my.app"},
			},
		},
		Transcripts: []model.TranscriptSession{
			{EncodedProject: "-home-u-my-app", CWD: ""},
		},
	}

	pm := buildProjectMap(raw)

	if got, want := pm["-home-u-my-app"], "/home/u/my.app"; got != want {
		t.Errorf("backup should beat lossy decode: got %q, want %q", got, want)
	}
}

// TestBuildProjectMapCWDFirstWins guards against a first-write vs last-write
// regression: when one project has several transcripts, the first in stable
// directory order wins — a later session's differing cwd must not overwrite it.
func TestBuildProjectMapCWDFirstWins(t *testing.T) {
	raw := &model.RawData{
		Transcripts: []model.TranscriptSession{
			{EncodedProject: "C--Users-takay-ccfx", CWD: `C:\Users\takay\ccfx`},
			{EncodedProject: "C--Users-takay-ccfx", CWD: `C:\Users\takay\CCFX`},
		},
	}

	pm := buildProjectMap(raw)

	if got, want := pm["C--Users-takay-ccfx"], `C:\Users\takay\ccfx`; got != want {
		t.Errorf("first transcript cwd should win: got %q, want %q", got, want)
	}
}

// TestBuildProjectMapBackupCollisionDeterministic locks in byte-reproducibility:
// encodeProjectPath is not injective, so two backup paths can collide on one
// encoded key. The winner must be the sorted-first path every run, not whatever
// Go's randomized map iteration happens to yield. "/a-b" and "/a/b" both encode
// to "-a-b"; "/a-b" sorts first.
func TestBuildProjectMapBackupCollisionDeterministic(t *testing.T) {
	raw := &model.RawData{
		BackupData: &model.BackupData{
			Projects: map[string]model.BackupProjectInfo{
				"/a/b": {Path: "/a/b"},
				"/a-b": {Path: "/a-b"},
			},
		},
		Transcripts: []model.TranscriptSession{
			{EncodedProject: "-a-b", CWD: ""},
		},
	}

	for i := 0; i < 50; i++ {
		pm := buildProjectMap(raw)
		if got, want := pm["-a-b"], "/a-b"; got != want {
			t.Fatalf("collision winner must be deterministic (sorted): got %q, want %q", got, want)
		}
	}
}
