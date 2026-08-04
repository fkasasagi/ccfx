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
