package renderer

import (
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fkasasagi/ccfx/analyzer"
	"github.com/fkasasagi/ccfx/collector"
	"github.com/fkasasagi/ccfx/model"
)

const testDataDir = "../testdata/claude"

func buildTestReport(t *testing.T) *model.ForensicReport {
	t.Helper()
	raw, err := collector.Collect(testDataDir, false)
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}
	return analyzer.Analyze(raw, &analyzer.Options{ExtractConversations: true})
}

func TestRenderJSON(t *testing.T) {
	report := buildTestReport(t)
	outDir := t.TempDir()

	result, err := Render(Config{
		Report:  report,
		OutDir:  outDir,
		Formats: []string{"json"},
		Lang:    "en",
	})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if len(result.Files) != 1 {
		t.Fatalf("files = %d, want 1", len(result.Files))
	}

	data, err := os.ReadFile(filepath.Join(outDir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}

	// No zero-time values
	if strings.Contains(string(data), `"0001-01-01T00:00:00Z"`) {
		t.Error("JSON contains zero-time string, should be null")
	}

	// Valid JSON
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
}

func TestRenderCSV(t *testing.T) {
	report := buildTestReport(t)
	outDir := t.TempDir()

	result, err := Render(Config{
		Report:  report,
		OutDir:  outDir,
		Formats: []string{"csv"},
		Lang:    "en",
	})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	expectedFiles := []string{"sessions.csv", "timeline.csv", "tool_usage.csv", "file_changes.csv", "token_usage.csv", "history.csv", "conversations.csv"}
	if len(result.Files) != len(expectedFiles) {
		t.Fatalf("files = %d, want %d", len(result.Files), len(expectedFiles))
	}

	// Check UTF-8 BOM
	for _, name := range expectedFiles {
		data, err := os.ReadFile(filepath.Join(outDir, name))
		if err != nil {
			t.Errorf("cannot read %s: %v", name, err)
			continue
		}
		if len(data) < 3 || data[0] != 0xEF || data[1] != 0xBB || data[2] != 0xBF {
			t.Errorf("%s missing UTF-8 BOM", name)
		}
	}

	// Validate history.csv has rows
	f, err := os.Open(filepath.Join(outDir, "history.csv"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	// Skip BOM
	bom := make([]byte, 3)
	f.Read(bom)
	reader := csv.NewReader(f)
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("csv parse error: %v", err)
	}
	if len(records) < 2 { // header + at least 1 row
		t.Errorf("history.csv has %d rows, want >= 2", len(records))
	}
}

func TestRenderMarkdown(t *testing.T) {
	report := buildTestReport(t)
	outDir := t.TempDir()

	_, err := Render(Config{
		Report:  report,
		OutDir:  outDir,
		Formats: []string{"md"},
		Lang:    "ja",
	})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(outDir, "report.md"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	// Japanese title
	if !strings.Contains(content, "フォレンジック分析レポート") {
		t.Error("missing Japanese title")
	}

	// Section numbers
	for _, section := range []string{"## 1.", "## 2.", "## 5.", "## 10.", "## 11.", "## 12."} {
		if !strings.Contains(content, section) {
			t.Errorf("missing section %s", section)
		}
	}
}

func TestRenderHTML(t *testing.T) {
	report := buildTestReport(t)
	outDir := t.TempDir()

	_, err := Render(Config{
		Report:  report,
		OutDir:  outDir,
		Formats: []string{"html"},
		Lang:    "en",
	})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(outDir, "report.html"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	if !strings.Contains(content, "<!DOCTYPE html>") {
		t.Error("missing DOCTYPE")
	}
	if !strings.Contains(content, "<style>") {
		t.Error("missing embedded CSS")
	}
	if !strings.Contains(content, "Command History") {
		t.Error("missing Command History section")
	}
}

func TestRenderTimezone(t *testing.T) {
	report := buildTestReport(t)
	outDir := t.TempDir()

	tokyo, _ := time.LoadLocation("Asia/Tokyo")
	_, err := Render(Config{
		Report:   report,
		OutDir:   outDir,
		Formats:  []string{"csv"},
		Lang:     "en",
		Timezone: tokyo,
	})
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(outDir, "sessions.csv"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	// Header should include JST
	if !strings.Contains(content, "(JST)") {
		t.Error("timezone abbreviation not in CSV header")
	}
}
