package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/fkasasagi/ccfx/analyzer"
	"github.com/fkasasagi/ccfx/collector"
	"github.com/fkasasagi/ccfx/model"
	"github.com/fkasasagi/ccfx/renderer"
)

const version = "0.1.0"

func main() {
	fs := flag.NewFlagSet("ccfx", flag.ExitOnError)
	path := fs.String("path", "", "Path to ~/.claude/ directory (auto-detect if omitted)")
	formatStr := fs.String("format", "json", "Output formats: csv,json,md,html (comma-separated)")
	outDir := fs.String("output", "./ccfx-output", "Output directory")
	lang := fs.String("language", "en", "Report language: en or ja")
	extractConv := fs.Bool("extract-conversations", false, "Include full conversation content")
	sessionFilter := fs.String("session-filter", "", "Limit to specific session ID")
	projectFilter := fs.String("project-filter", "", "Limit to specific project path")
	dateFrom := fs.String("date-from", "", "Filter by date range start (YYYY-MM-DD)")
	dateTo := fs.String("date-to", "", "Filter by date range end (YYYY-MM-DD)")
	redactPII := fs.Bool("redact-pii", false, "Redact email addresses and UUIDs")
	verbose := fs.Bool("verbose", false, "Enable debug logging")
	showVersion := fs.Bool("version", false, "Print version and exit")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "ccfx - Claude Code Forensics eXtractor v%s\n\n", version)
		fmt.Fprintf(os.Stderr, "Usage: ccfx [flags]\n\nFlags:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(os.Args[1:]); err != nil {
		os.Exit(1)
	}

	if *showVersion {
		fmt.Printf("ccfx v%s (%s/%s)\n", version, runtime.GOOS, runtime.GOARCH)
		return
	}

	claudeDir := *path
	if claudeDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Fatalf("cannot detect home directory: %v", err)
		}
		claudeDir = home + "/.claude"
	}

	if _, err := os.Stat(claudeDir); os.IsNotExist(err) {
		log.Fatalf("claude directory not found: %s", claudeDir)
	}

	formats := parseFormats(*formatStr)
	if len(formats) == 0 {
		log.Fatal("no valid output format specified")
	}

	var dateFromT, dateToT *time.Time
	if *dateFrom != "" {
		t, err := time.Parse("2006-01-02", *dateFrom)
		if err != nil {
			log.Fatalf("invalid --date-from: %v", err)
		}
		dateFromT = &t
	}
	if *dateTo != "" {
		t, err := time.Parse("2006-01-02", *dateTo)
		if err != nil {
			log.Fatalf("invalid --date-to: %v", err)
		}
		t = t.Add(24*time.Hour - time.Nanosecond)
		dateToT = &t
	}

	if *verbose {
		log.Printf("ccfx v%s starting", version)
		log.Printf("source: %s", claudeDir)
		log.Printf("formats: %v", formats)
	}

	raw, err := collector.Collect(claudeDir, *verbose)
	if err != nil {
		log.Fatalf("collection failed: %v", err)
	}

	if *verbose {
		log.Printf("collected: %d history entries, %d sessions, %d transcripts",
			len(raw.HistoryEntries), len(raw.SessionFiles), len(raw.Transcripts))
	}

	report := analyzer.Analyze(raw, &analyzer.Options{
		ExtractConversations: *extractConv,
		SessionFilter:        *sessionFilter,
		ProjectFilter:        *projectFilter,
		DateFrom:             dateFromT,
		DateTo:               dateToT,
		RedactPII:            *redactPII,
	})

	report.Meta.ToolVersion = version
	report.Meta.Platform = runtime.GOOS + "/" + runtime.GOARCH

	cfg := renderer.Config{
		Report:  report,
		OutDir:  *outDir,
		Formats: formats,
		Lang:    *lang,
	}

	result, err := renderer.Render(cfg)
	if err != nil {
		log.Fatalf("rendering failed: %v", err)
	}

	for _, f := range result.Files {
		fmt.Printf("  %s (%s)\n", f.Path, formatBytes(f.Size))
	}
	fmt.Printf("\n%d file(s) written to %s\n", len(result.Files), *outDir)
}

func parseFormats(s string) []string {
	valid := map[string]bool{"csv": true, "json": true, "md": true, "html": true}
	var out []string
	for _, f := range strings.Split(s, ",") {
		f = strings.TrimSpace(strings.ToLower(f))
		if valid[f] {
			out = append(out, f)
		}
	}
	return out
}

func formatBytes(b int64) string {
	switch {
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

// Filter is used by the analyzer; defined here to keep model package clean.
type Filter struct {
	SessionID string
	Project   string
	DateFrom  *time.Time
	DateTo    *time.Time
}

func (f *Filter) MatchSession(s model.Session) bool {
	if f.SessionID != "" && s.SessionID != f.SessionID {
		return false
	}
	if f.Project != "" && s.Project != f.Project {
		return false
	}
	if f.DateFrom != nil && s.StartedAt.Before(*f.DateFrom) {
		return false
	}
	if f.DateTo != nil && s.StartedAt.After(*f.DateTo) {
		return false
	}
	return true
}
