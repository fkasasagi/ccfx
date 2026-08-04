package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/fkasasagi/ccfx/analyzer"
	"github.com/fkasasagi/ccfx/collector"
	"github.com/fkasasagi/ccfx/renderer"
)

const version = "0.7.0"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "help":
			if len(os.Args) > 2 {
				showTopicHelp(os.Args[2])
			} else {
				showHelp()
			}
			return
		case "version":
			fmt.Printf("ccfx v%s (%s/%s)\n", version, runtime.GOOS, runtime.GOARCH)
			return
		}
	}

	fs := flag.NewFlagSet("ccfx", flag.ExitOnError)
	path := fs.String("path", "", "Path to ~/.claude/ directory (auto-detect if omitted)")
	formatStr := fs.String("format", "all", "Output formats: csv,json,md,html,all (comma-separated)")
	outDir := fs.String("output", "./ccfx-output", "Output directory")
	lang := fs.String("language", "en", "Report language: en or ja")
	extractConv := fs.Bool("extract-conversations", true, "Include full conversation content")
	sessionFilter := fs.String("session-filter", "", "Limit to specific session ID")
	projectFilter := fs.String("project-filter", "", "Limit to specific project path")
	dateFrom := fs.String("date-from", "", "Filter by date range start (YYYY-MM-DD)")
	dateTo := fs.String("date-to", "", "Filter by date range end (YYYY-MM-DD)")
	timezone := fs.String("timezone", "", "Timezone for timestamps: e.g. Asia/Tokyo, America/New_York (default: UTC)")
	redactPII := fs.Bool("redact-pii", false, "Redact email addresses and UUIDs")
	acquireAll := fs.Bool("ac", false, "Also write "+acquisitionName+", a verbatim copy of the source directory (WARNING: includes credentials)")
	force := fs.Bool("force", false, "Overwrite existing output (otherwise ccfx refuses if any exists)")
	verbose := fs.Bool("verbose", false, "Enable debug logging")
	showVersion := fs.Bool("version", false, "Print version and exit")
	showHelpFlag := fs.Bool("help", false, "Show help")

	fs.Usage = func() { showHelp() }

	if err := fs.Parse(os.Args[1:]); err != nil {
		os.Exit(1)
	}

	if *showHelpFlag {
		showHelp()
		return
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

	if !*force {
		if existing := existingOutputs(*outDir, *acquireAll); len(existing) > 0 {
			fmt.Fprintf(os.Stderr, "Output already exists in %s:\n", *outDir)
			for _, e := range existing {
				fmt.Fprintf(os.Stderr, "  %s\n", e)
			}
			fmt.Fprintln(os.Stderr, "\nRefusing to overwrite — old reports left in place can be stale and mix with new output.")
			fmt.Fprintln(os.Stderr, "Re-run with --force to regenerate, or pass --output DIR to write to a fresh directory.")
			os.Exit(1)
		}
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

	var tz *time.Location
	if *timezone != "" {
		loc, err := time.LoadLocation(*timezone)
		if err != nil {
			log.Fatalf("invalid --timezone %q: %v\nRun 'ccfx help timezone' for available values.", *timezone, err)
		}
		tz = loc
	}

	if *verbose {
		log.Printf("ccfx v%s starting", version)
		log.Printf("source: %s", claudeDir)
		log.Printf("formats: %v", formats)
		if tz != nil {
			log.Printf("timezone: %s", tz)
		}
	}

	// Acquire before analyzing: if anything downstream fails, the evidence copy
	// has already been taken.
	var acq *acquisitionResult
	if *acquireAll {
		// Warn before writing, not after: by the time the archive exists, the
		// token is already on disk.
		fmt.Fprintf(os.Stderr,
			"WARNING: -ac writes %s, a verbatim copy of %s.\n"+
				"It includes .credentials.json — your OAuth token, in cleartext — if present,\n"+
				"and --redact-pii does not apply to it. The archive is created 0600; keep it\n"+
				"that way, encrypt it before sending it anywhere, and delete it when done.\n\n",
			acquisitionName, claudeDir)

		a, err := acquire(claudeDir, *outDir)
		if err != nil {
			log.Fatalf("acquisition failed: %v", err)
		}
		acq = a
		for _, s := range a.Skipped {
			fmt.Fprintf(os.Stderr, "acquisition skipped %s\n", s)
		}
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
		Report:   report,
		OutDir:   *outDir,
		Formats:  formats,
		Lang:     *lang,
		Timezone: tz,
	}

	result, err := renderer.Render(cfg)
	if err != nil {
		log.Fatalf("rendering failed: %v", err)
	}

	for _, f := range result.Files {
		fmt.Printf("  %s (%s)\n", f.Path, formatBytes(f.Size))
	}
	written := len(result.Files)
	if acq != nil {
		fmt.Printf("  %s (%s) — %d files, %d dirs, %d symlinks\n",
			acq.Path, formatBytes(acq.Size), acq.Files, acq.Dirs, acq.Symlinks)
		written++
	}
	fmt.Printf("\n%d file(s) written to %s\n", written, *outDir)
}

func showHelp() {
	fmt.Print(`ccfx - Claude Code Forensics eXtractor v` + version + `

  Analyze Claude Code local artifacts (~/.claude/) and generate
  forensic reports revealing who used Claude Code, when, and how.

USAGE
  ccfx [flags]
  ccfx help [topic]
  ccfx version

FLAGS
  --path PATH              Path to ~/.claude/ directory (auto-detect if omitted)
  --format FORMATS         Output formats: csv,json,md,html,all (default: all)
  --output DIR             Output directory (default: ./ccfx-output)
  --language en|ja         Report language (default: en)
  --extract-conversations  Include full conversation content (default: on; disable with --extract-conversations=false)
  --session-filter UUID    Analyze only the specified session
  --project-filter PATH    Analyze only the specified project
  --date-from YYYY-MM-DD   Include only activity on or after this date
  --date-to YYYY-MM-DD     Include only activity on or before this date
  --timezone ZONE          Timezone for timestamps (default: UTC)
                           Use IANA names: e.g. Asia/Tokyo, US/Eastern
  --redact-pii             Mask email addresses and UUIDs in output
  -ac                      Acquire: also write ` + acquisitionName + `, a verbatim
                           zip of the source directory with file timestamps,
                           empty directories and symlinks preserved.
                           WARNING: verbatim includes .credentials.json, i.e.
                           your OAuth token in cleartext. --redact-pii does NOT
                           apply to the archive. Anyone who receives it can act
                           as you. Treat it as a secret, not as a report.
  --force                  Overwrite existing output (otherwise ccfx refuses)
  --verbose                Print debug information to stderr
  --version                Print version and exit
  --help                   Show this help

EXAMPLES
  ccfx                                          Analyze ~/.claude/, output all formats
  ccfx --format csv,json,md,html --language ja  All formats in Japanese
  ccfx --path /mnt/disk/.claude --format html   Analyze mounted evidence
  ccfx --date-from 2026-05-01 --redact-pii      Filter by date, mask PII
  ccfx --extract-conversations --format json     Include conversation text
  ccfx --timezone Asia/Tokyo --format html       Timestamps in JST

HELP TOPICS
  ccfx help artifacts      Files and directories analyzed by ccfx
  ccfx help formats        Output format details (CSV, JSON, Markdown, HTML)
  ccfx help report         Report sections and what they contain
  ccfx help injection      How to tell whether a session was prompt-injected
  ccfx help security       Security considerations and credential handling
  ccfx help timezone       Available timezone names (IANA Time Zone Database)
  ccfx help examples       More usage examples and forensic workflows
`)
}

func showTopicHelp(topic string) {
	switch strings.ToLower(topic) {
	case "artifacts":
		showArtifactsHelp()
	case "formats":
		showFormatsHelp()
	case "report":
		showReportHelp()
	case "security":
		showSecurityHelp()
	case "injection":
		showInjectionHelp()
	case "timezone", "timezones", "tz":
		showTimezoneHelp()
	case "examples":
		showExamplesHelp()
	default:
		fmt.Fprintf(os.Stderr, "Unknown help topic: %q\n\n", topic)
		fmt.Fprintf(os.Stderr, "Available topics: artifacts, formats, report, injection, security, timezone, examples\n")
		os.Exit(1)
	}
}

func showArtifactsHelp() {
	fmt.Print(`ARTIFACTS — Files and directories analyzed by ccfx

  Claude Code stores data under ~/.claude/ (Windows: USERPROFILE\.claude\).
  ccfx reads these files in read-only mode. Nothing is modified.

  File / Directory                             What ccfx extracts
  ─────────────────────────────────────────────────────────────────────────
  history.jsonl                                Command history, timestamps,
                                               session IDs

  sessions/<pid>.json                          Session metadata: PID, CWD,
                                               start/end time, version,
                                               entrypoint (cli/web/etc)

  projects/<encoded-path>/<uuid>.jsonl         Conversation transcripts,
                                               tool calls (name + args),
                                               model, token usage, git branch
                                               ** Most important artifact **

  backups/.claude.json.backup.<timestamp>      User email, org UUID,
                                               subscription tier, per-project
                                               cost & token stats

  settings.json / settings.local.json          Permission rules (allow/deny),
                                               hook definitions, effort level

  .credentials.json                            Existence detection ONLY
                                               (values are never read)

  file-history/<session>/<hash>@v<n>           File edit version counts

  shell-snapshots/                             Shell environment snapshot count
  paste-cache/                                 Pasted content file count
  tasks/                                       Task session count
  plans/                                       Plan file count

  The project path encoding replaces / with -, so /home/user/myproject
  becomes -home-user-myproject in the directory name. ccfx decodes this
  automatically and cross-references with backup data to resolve ambiguity.
`)
}

func showFormatsHelp() {
	fmt.Print(`FORMATS — Output format details

  Specify one or more formats with --format (comma-separated).
  All files are written to the --output directory.

  FORMAT   FILES GENERATED          NOTES
  ─────────────────────────────────────────────────────────────────────────
  json     report.json              Complete ForensicReport as pretty-printed
                                    JSON (2-space indent). Suitable for
                                    programmatic consumption with jq, Python,
                                    or other tools.

  csv      sessions.csv             One row per session (ID, project, time,
                                    duration, model, message/tool counts)
           timeline.csv             Chronological events across all sessions
           tool_usage.csv           Tool name, call count, session count
           file_changes.csv         Files modified via Edit/Write tools
           token_usage.csv          Daily token consumption breakdown
           history.csv              User command inputs from history.jsonl
           conversations.csv        Full conversation text, one row per message
           injection_events.csv     Complete ingress/egress inventory
           injection_findings.csv   Correlated injection findings, ranked
                                    All CSV files include UTF-8 BOM for
                                    Windows Excel compatibility.

  md       report.md                14-section Markdown report. Suitable for
                                    viewing on GitHub, in editors, or
                                    converting to PDF via pandoc.

  html     report.html              Self-contained HTML with embedded CSS.
                                    Dark theme, scrollable tables, CSS bar
                                    charts for tool usage. No external
                                    resources needed. Print-friendly via
                                    @media print rules.

  Column headers respect --language (en/ja).

EXAMPLES
  ccfx --format all                  All formats (csv,json,md,html) (default)
  ccfx --format json                 JSON only
  ccfx --format csv,html             CSV tables + visual HTML report
`)
}

func showReportHelp() {
	fmt.Print(`REPORT — Report sections and what they contain

  #   SECTION                    CONTENT
  ─────────────────────────────────────────────────────────────────────────
  1   User Identity              Email, account UUID, organization info,
                                 subscription tier, Claude Code version.
                                 Extracted from backup files.

  2   Sessions                   All sessions with start time, duration,
                                 model used, message/tool-use counts,
                                 git branch, permission mode, and title.

  3   Activity Timeline          Chronological list of user messages,
                                 tool invocations, and assistant responses
                                 across all sessions. Sorted by timestamp.

  4   Projects                   Per-project summary: session count,
                                 first/last seen dates, total messages
                                 and tool uses.

  5   Tool Usage Statistics      Ranking of tools by call count (Bash,
                                 Edit, Write, Read, Agent, WebSearch, etc.)
                                 with session count per tool.

  6   Token Consumption          Total input/output/cache tokens.
                                 Breakdown by model, project, and date.

  7   File Modifications         Files created or edited via Write/Edit
                                 tools, with timestamps and session IDs.

  8   Permission & Security      Global/local deny and allow rules,
                                 hook definitions, per-session permission
                                 modes (default, plan, etc.)

  9   Credential Discovery       Whether .credentials.json exists, its
                                 size and modification date. Token values
                                 are NEVER read or included.

  10  Command History            User-typed commands from history.jsonl.
                                 Timestamp, session ID, project, and the
                                 command string. Independent corroborating
                                 record of user activity.

  11  File History Statistics    Number of sessions with file edits and
                                 total file versions stored in file-history/.
                                 Indicates scope of code modifications.

  12  Auxiliary Artifact Stats   Counts of shell-snapshots, paste-cache,
                                 tasks, plans, and custom-commands.
                                 Shows breadth of Claude Code usage.

  13  Conversations              Full conversation text (user + assistant).
                                 Included by default; disable with
                                 --extract-conversations=false.
                                 Can produce very large output.

  14  Prompt Injection Triage    What entered each session, what the text
                                 looked like, what left afterwards, and what
                                 was changed. See 'ccfx help injection'.
`)
}

func showInjectionHelp() {
	fmt.Print(`INJECTION — How to tell whether a session was prompt-injected

  A prompt injection is untrusted text that reaches the model and steers it.
  Deciding whether one landed is a question about *provenance*: what entered
  the context, what it said, and what the agent did next. Section 14 of the
  report lays those out; this topic explains what to look at and why.

  WHAT CCFX LOOKS AT

  1. INGRESS — what entered the context
     network_ingress    WebFetch (the URL and how many bytes came back),
                        WebSearch queries, MCP tool results, and Bash
                        commands that pull from the network (curl, wget,
                        git clone, package installs).
     file_ingress       Read/Grep/Glob, and Bash commands that read
                        credential paths.
     context_injection  Text the harness itself put in front of the model:
                        hook output (a hook can inject arbitrary text on
                        every turn), attached and pasted files, and the
                        return value of a subagent.

  2. SIGNALS — what the text looked like
     Each ingress body is scanned while streaming; only matches are kept.

     instruction-override       "ignore previous instructions", "your new
                                task is"
     authority-spoof            content posing as the harness, e.g. a
                                <system-reminder> tag inside a fetched page
     tool-call-injection        "<invoke name=..." — faking a tool call
     secrecy-request            "do not tell the user"
     credential-request         being told to cat ~/.ssh/id_ed25519 or .env
     exfiltration-instruction   webhook.site, requestbin, "post the
                                contents to"
     permission-escalation      "--dangerously-skip-permissions"
     hidden-characters          zero-width and bidi-override runs, which
                                hide text from a human but not the model
     scroll-hiding              a wall of blank lines pushing text below
                                the fold
     agent-directed-imperative  "if you are an AI reading this file..."
     credential-target          a mention of a secret path (weak on its own)
     encoded-payload            a long base64 run

  3. EGRESS — what left
     Bash commands that upload (curl with a body, wget --post, scp,
     rsync to a remote, nc, /dev/tcp), pipes into a shell, and WebFetch
     to an unusually long URL. ` + "`git push`" + ` is deliberately NOT treated as
     egress: it is the most common write-out in ordinary work.

  4. PERSISTENCE — what was left changed
     Writes to settings.json, settings.local.json, CLAUDE.md, .mcp.json or
     anything under .claude/, plus git remote changes. These outlive the
     session that made them, so they are how an injection becomes durable.

  5. CORRELATION — the part that matters
     A signal on its own is a curiosity. A signal followed, within 30 tool
     calls in the same session, by an upload or a config change is a lead.
     Those become findings:

       ingress-then-egress          tainted content in, data out
       sensitive-read-then-egress   a secret read, then an upload
       ingress-then-config-change   tainted content in, settings changed
       signal-in-injected-context   a signal in hook-injected text, which
                                    is potent because no user is in the loop
       permission-mode-escalation   the session moved to bypassPermissions
       high-signal-ingress          a strong signal with nothing after it

  HOW TO READ IT

    ccfx --format html
    # open report.html, section 14:
    #   Findings           -> what to open first, worst first
    #   Sessions to Review -> which sessions carry signals or outbound calls
    #   Flagged Content    -> the excerpt itself, in context
    # injection_events.csv holds the complete inventory, findings or not.

  FALSE POSITIVES ARE EXPECTED

    These rules are symptoms, not verdicts. A session that *discusses*
    prompt injection matches nearly all of them. So does reading source
    code that mentions credential paths — ccfx's own code trips
    credential-target. Always read the excerpt before concluding anything.

  WHAT CCFX CANNOT SEE

    Only what is on disk under the analyzed directory. Tool results that
    predate the transcript format, content the model saw but the harness
    did not record, and anything already deleted are all invisible.

    The signal rules are a list of known-bad phrasings, in English and
    Japanese. That is a blocklist, and a blocklist only recognises what
    it already knows: an injection written in another language, or in
    wording nobody has catalogued, passes it without a mark. Only the
    first 1 MB of any single tool result is scanned.

    An absence of findings is therefore not evidence that nothing
    happened. The ingress inventory in injection_events.csv is the part
    that does not depend on recognising the attack: read it when you
    need to know what a session was exposed to, findings or not.
`)
}

func showSecurityHelp() {
	fmt.Print(`SECURITY — Security considerations and credential handling

  READ-ONLY ANALYSIS
    ccfx never modifies any file in the target ~/.claude/ directory.
    All access is read-only. The tool can safely be run against live
    installations or mounted forensic images.

  CREDENTIAL HANDLING
    The .credentials.json file contains OAuth tokens for the Claude API.
    ccfx checks ONLY for the file's existence, size, and modification
    date. Token values are never read, parsed, or included in output.

    THE ONE EXCEPTION IS -ac. The acquisition archive is a byte-for-byte
    copy of the source directory, so it contains .credentials.json with
    the token in cleartext. PII redaction does not apply to it. Whoever
    holds the archive can authenticate as the user it came from, so
    encrypt it in transit and at rest, and prefer omitting -ac when the
    report alone answers the question.

  PII REDACTION (--redact-pii)
    When enabled, email addresses and UUIDs in the output are masked:
      user@example.com  →  us***@example.com
      a1b2c3d4-e5f6-... →  a1b2c3d4-****-****-****-************
    This applies to the user_identity section of the report.

  OUTPUT SENSITIVITY
    Generated reports may contain sensitive information:
    - User email and organization details
    - Conversation content (with --extract-conversations)
    - File paths and project structures
    - Tool commands executed (Bash commands, file edits)
    Treat output files with appropriate access controls.

  CROSS-MACHINE ANALYSIS
    ccfx can analyze Claude Code data from another machine by pointing
    --path to a mounted disk or copied directory. No network access is
    required. The tool makes no API calls and has no internet dependency.
`)
}

func showTimezoneHelp() {
	fmt.Print(`TIMEZONE — Available timezone names (IANA Time Zone Database)

  ccfx uses the IANA Time Zone Database (also known as the Olson database
  or tz database). Specify a timezone with --timezone to convert all
  timestamps in the output.

  Default: UTC (no conversion)

  COMMON TIMEZONES

  Asia
  ────────────────────────────────────────────────
  Asia/Tokyo              JST  (UTC+9)    Japan
  Asia/Shanghai           CST  (UTC+8)    China
  Asia/Kolkata            IST  (UTC+5:30) India
  Asia/Singapore          SGT  (UTC+8)    Singapore
  Asia/Seoul              KST  (UTC+9)    South Korea
  Asia/Taipei             CST  (UTC+8)    Taiwan
  Asia/Hong_Kong          HKT  (UTC+8)    Hong Kong
  Asia/Bangkok            ICT  (UTC+7)    Thailand
  Asia/Dubai              GST  (UTC+4)    UAE
  Asia/Jerusalem          IST  (UTC+2/3)  Israel

  Americas
  ────────────────────────────────────────────────
  America/New_York        EST/EDT (UTC-5/-4)  US Eastern
  America/Chicago         CST/CDT (UTC-6/-5)  US Central
  America/Denver          MST/MDT (UTC-7/-6)  US Mountain
  America/Los_Angeles     PST/PDT (UTC-8/-7)  US Pacific
  America/Toronto         EST/EDT (UTC-5/-4)  Canada Eastern
  America/Vancouver       PST/PDT (UTC-8/-7)  Canada Pacific
  America/Sao_Paulo       BRT     (UTC-3)     Brazil

  Europe
  ────────────────────────────────────────────────
  Europe/London           GMT/BST (UTC+0/+1)  UK
  Europe/Berlin           CET/CEST(UTC+1/+2)  Germany
  Europe/Paris            CET/CEST(UTC+1/+2)  France
  Europe/Moscow           MSK     (UTC+3)     Russia
  Europe/Zurich           CET/CEST(UTC+1/+2)  Switzerland

  Oceania
  ────────────────────────────────────────────────
  Australia/Sydney        AEST/AEDT(UTC+10/+11)  Australia Eastern
  Australia/Perth         AWST     (UTC+8)       Australia Western
  Pacific/Auckland        NZST/NZDT(UTC+12/+13)  New Zealand

  Shortcuts
  ────────────────────────────────────────────────
  UTC                     Coordinated Universal Time
  US/Eastern              Alias for America/New_York
  US/Central              Alias for America/Chicago
  US/Mountain             Alias for America/Denver
  US/Pacific              Alias for America/Los_Angeles

  EXAMPLES
    ccfx --timezone Asia/Tokyo --format all
    ccfx --timezone America/New_York --format html
    ccfx --timezone UTC                              (default, explicit)

  NOTES
  - Daylight saving time is handled automatically.
  - The full list depends on your OS. On Linux, see /usr/share/zoneinfo/.
  - On Windows, Go embeds timezone data so all names work regardless of OS.
  - If a timezone name is invalid, ccfx exits with an error message.
`)
}

func showExamplesHelp() {
	fmt.Print(`EXAMPLES — Usage examples and forensic workflows

  BASIC USAGE
    ccfx                                     Default: analyze ~/.claude/, JSON
    ccfx --format html                       Visual HTML report
    ccfx --format csv,json,md,html           All formats at once

  INCIDENT RESPONSE
    # Analyze a suspect's Claude Code data from mounted evidence
    ccfx --path /mnt/evidence/home/user/.claude \
         --format json,html \
         --extract-conversations \
         --redact-pii

  INTERNAL AUDIT
    # Monthly usage report in Japanese, CSV for spreadsheet import
    ccfx --format csv \
         --date-from 2026-05-01 \
         --date-to 2026-05-31 \
         --language ja

  SECURITY REVIEW
    # Check permission settings and credential status
    ccfx --format html --language ja
    # Open report.html → sections 8 (Permissions) and 9 (Credentials)

  SPECIFIC SESSION INVESTIGATION
    # Extract full conversation from a known session
    ccfx --format json \
         --session-filter "a1b2c3d4-e5f6-7890-abcd-ef1234567890" \
         --extract-conversations

  PROJECT-FOCUSED ANALYSIS
    # Analyze only activity in a specific project
    ccfx --format md \
         --project-filter "/home/user/myproject"

  COMBINING FILTERS
    # Specific project, date range, with PII masking
    ccfx --format json,html \
         --project-filter "/home/user/myproject" \
         --date-from 2026-05-01 \
         --date-to 2026-05-15 \
         --redact-pii

  PROGRAMMATIC USE
    # Parse JSON output with jq
    ccfx --format json --output /tmp/ccfx
    jq '.tool_usage.top_tools[:5]' /tmp/ccfx/report.json
    jq '.sessions[] | select(.message_count > 50)' /tmp/ccfx/report.json

  CUSTOM OUTPUT DIRECTORY
    ccfx --format csv,html --output ./reports/2026-05
`)
}

func existingOutputs(outDir string, withAcquisition bool) []string {
	known := renderer.KnownOutputFiles(outDir)
	if withAcquisition {
		known = append(known, filepath.Join(outDir, acquisitionName))
	}

	var existing []string
	for _, p := range known {
		if info, err := os.Stat(p); err == nil {
			existing = append(existing, fmt.Sprintf("%s  (modified %s)", p, info.ModTime().Format("2006-01-02 15:04:05")))
		}
	}
	return existing
}

func parseFormats(s string) []string {
	if strings.TrimSpace(strings.ToLower(s)) == "all" {
		return []string{"csv", "json", "md", "html"}
	}
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
