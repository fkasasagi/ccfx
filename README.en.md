[![English](https://img.shields.io/badge/README-English-blue)](README.en.md)
[![日本語](https://img.shields.io/badge/README-%E6%97%A5%E6%9C%AC%E8%AA%9E-lightgrey)](README.md)

# ccfx - Claude Code Forensics eXtractor

A digital forensics tool that analyzes the local artifacts Claude Code leaves behind (`~/.claude/`) and reports **who** used Claude Code, **when**, and **how**.

A single Go binary with zero external dependencies. **Runs on Linux / macOS / Windows.**

> **On Windows:** project paths (`C:\Users\...` and UNC paths) are reconstructed from the working directory Claude records, and `--timezone` IANA names (`Asia/Tokyo`, etc.) work (the timezone database is embedded in the binary). Only `-ac` archive acquisition needs Administrator or Developer Mode, to preserve symlinks.

> **⚠ Note:** This tool is experimental. It does not guarantee the accuracy of its output. Use it at your own risk.
>
> This tool is intended for forensic analysis, internal auditing, and security review by users with proper authorization. Do not use it to collect or analyze other people's data without consent.

---

## Analyzed Artifacts

Claude Code leaves the following files under `~/.claude/` (Windows: `%USERPROFILE%\.claude\`). ccfx reads them non-destructively and generates a report.

| File / Directory | Format | What ccfx Extracts |
|---|---|---|
| `history.jsonl` | JSONL | Command history entered by the user, timestamps, session IDs |
| `sessions/<pid>.json` | JSON | Session metadata (PID, CWD, start/end time, version, entrypoint) |
| `projects/<encoded-path>/<uuid>.jsonl` | JSONL | **Full conversation text**, tool calls (name and arguments), model, token usage, Git branch |
| `backups/.claude.json.backup.*` | JSON | User email, organization UUID, subscription info, per-project cost |
| `settings.json` / `settings.local.json` | JSON | Permissions (allow/deny rules), hook definitions, effort level |
| `.credentials.json` | JSON | **Existence detection only** for OAuth tokens (values are never read) |
| `file-history/<session>/<hash>@v<n>` | Various | Statistics on the number of file edit versions |
| `shell-snapshots/`, `paste-cache/`, `tasks/`, `plans/` | Various | Counts of auxiliary metadata |

## Installation

### Build from source (Go 1.22+)

```bash
git clone https://github.com/fkasasagi/ccfx.git
cd ccfx
go build -o ccfx .
```

### Cross-compilation

```bash
# Linux
GOOS=linux GOARCH=amd64 go build -o ccfx-linux .

# macOS (Apple Silicon)
GOOS=darwin GOARCH=arm64 go build -o ccfx-darwin .

# Windows
GOOS=windows GOARCH=amd64 go build -o ccfx.exe .
```

## Usage

```
ccfx [flags]
ccfx help [topic]
ccfx version
```

### Flags

| Flag | Default | Description |
|---|---|---|
| `--path PATH` | auto-detect (`~/.claude/`) | Path to the directory to analyze |
| `--format csv,json,md,html,all` | `all` | Output format (comma-separated for multiple, `all` for every format) |
| `--output DIR` | `./ccfx-output` | Output directory |
| `--language en\|ja` | `en` | Report language (English / Japanese) |
| `--extract-conversations` | on | Include the full conversation text in the report (`--extract-conversations=false` to disable) |
| `--session-filter UUID` | - | Analyze only a specific session |
| `--project-filter PATH` | - | Analyze only a specific project |
| `--date-from YYYY-MM-DD` | - | Limit to entries on or after this date |
| `--date-to YYYY-MM-DD` | - | Limit to entries on or before this date |
| `--timezone ZONE` | `UTC` | IANA timezone name (e.g. `Asia/Tokyo`, `America/New_York`) |
| `--redact-pii` | off | Mask emails and UUIDs |
| `-ac` | off | Also write `claude-acquisition.zip`, a verbatim zip of the source directory with file timestamps, empty directories, and symlinks preserved. **Contains `.credentials.json` in cleartext — see [Security Notes](#security-notes)** |
| `--force` | off | Overwrite existing files in the output directory (aborts if not specified) |
| `--verbose` | off | Emit debug logs |
| `--version` | - | Show version |
| `--help` | - | Show help |

### Help System

Run `ccfx help [topic]` to display detailed help for each topic.

| Command | Content |
|---|---|
| `ccfx help` | Main help (flag list, usage examples, topic list) |
| `ccfx help artifacts` | Location, format, and extracted information of analyzed files |
| `ccfx help formats` | Details of the CSV / JSON / Markdown / HTML formats |
| `ccfx help report` | Description of the report's 14 sections |
| `ccfx help injection` | How to tell whether a session was prompt-injected: what is inspected, the signal taxonomy, and the false positives to expect |
| `ccfx help security` | Handling of credentials, PII masking, read-only behavior |
| `ccfx help timezone` | List of available IANA timezone names |
| `ccfx help examples` | Workflow examples for IR, internal audit, security review, etc. |

### Basic Usage

```bash
# JSON report only (default)
./ccfx

# Output every format at once
./ccfx --format all

# Output every format in Japanese
./ccfx --format all --language ja

# Analyze Claude data from another machine (e.g. a mounted USB drive)
./ccfx --path /mnt/evidence/home/user/.claude --format json,html

# Report a specific period with PII masking
./ccfx --format html --date-from 2026-05-01 --date-to 2026-05-31 --redact-pii

# Output timestamps in Japan Standard Time (JST)
./ccfx --format all --timezone Asia/Tokyo --language ja

# Investigate a suspected prompt injection (report section 14)
./ccfx --format html
# -> open report.html section 14: findings, then sessions to review, then the excerpts

# Extract the full conversation of a specific session
./ccfx --format json --session-filter "a1b2c3d4-e5f6-7890-abcd-ef1234567890" --extract-conversations
```

When `--timezone` is specified, every timestamp in the output is converted and the column names are suffixed with the timezone abbreviation (e.g. `Started At (JST)`).

## Output Files

When you request every format with `--format all` (or `--format csv,json,md,html`):

```
ccfx-output/
├── report.json          # Full report (structured JSON)
├── report.md            # Markdown report (14 sections)
├── report.html          # Self-contained HTML (embedded CSS, dark theme)
├── sessions.csv         # Session list
├── timeline.csv         # Activity timeline
├── tool_usage.csv       # Tool usage statistics
├── file_changes.csv     # File change records
├── token_usage.csv      # Daily token consumption
├── history.csv          # Command input history
├── conversations.csv    # Full conversation messages (one row per message)
├── injection_events.csv     # Complete ingress/egress inventory
└── injection_findings.csv   # Correlated injection findings, ranked
```

CSV files include a UTF-8 BOM, so they open without garbled characters in Windows Excel.

With `-ac`, one more file is written alongside them:

```
└── claude-acquisition.zip   # Verbatim copy of the source directory (timestamps preserved)
```

## Output Examples

### JSON (excerpt)

```json
{
  "meta": {
    "generated_at": "2026-05-25T13:38:19Z",
    "source_path": "/home/user/.claude",
    "tool_version": "0.1.0",
    "platform": "linux/amd64",
    "total_sessions": 310,
    "total_projects": 6,
    "date_range": {
      "earliest": "2026-05-12T13:16:00Z",
      "latest": "2026-05-25T13:38:12Z"
    }
  },
  "user_identity": {
    "email": "user@example.com",
    "account_uuid": "a1b2c3d4-****-****-****-************",
    "organization_type": "claude_max",
    "organization_role": "admin",
    "rate_limit_tier": "default_claude_max_5x",
    "claude_code_version": "2.1.139"
  },
  "tool_usage": {
    "top_tools": [
      { "tool_name": "Bash",       "total_calls": 440, "session_count": 9 },
      { "tool_name": "Edit",       "total_calls": 149, "session_count": 6 },
      { "tool_name": "TaskUpdate", "total_calls": 144, "session_count": 4 },
      { "tool_name": "Write",      "total_calls": 139, "session_count": 7 },
      { "tool_name": "TaskCreate", "total_calls":  73, "session_count": 4 }
    ]
  },
  "token_usage": {
    "total_input": 115342,
    "total_output": 2383171,
    "total_cache_creation": 8404208,
    "total_cache_read": 262189086
  },
  "credentials": {
    "file_exists": true,
    "file_modified_at": "2026-05-25T20:54:23Z",
    "file_size_bytes": 470,
    "oauth_token_detected": true
  },
  "command_history": [
    { "display": "/plan", "timestamp": "2026-05-12T13:16:00Z", "project": "/home/user/myproject", "sessionId": "a1b2c3d4-..." }
  ],
  "file_history_stats": {
    "session_count": 12,
    "total_file_versions": 270
  },
  "misc_stats": {
    "shell_snapshots": 3,
    "paste_cache_files": 1,
    "task_sessions": 4,
    "plan_files": 4,
    "custom_commands": 1
  }
}
```

### CSV - tool_usage.csv

```
Tool Name,Total Calls,Session Count
Bash,440,9
Edit,149,6
TaskUpdate,144,4
Write,139,7
TaskCreate,73,4
WebSearch,59,8
Read,43,9
```

### CSV - token_usage.csv

```
Date,Input Tokens,Output Tokens,Cache Creation,Cache Read
2026-05-12,6877,598573,1322042,26582115
2026-05-13,308,182360,621051,10105482
2026-05-15,94002,667665,2808707,27135615
2026-05-16,4737,619171,1650754,167553266
2026-05-20,178,81111,1043877,16517713
```

### CSV - history.csv

```
Timestamp (UTC),Session ID,Project,Shell?,Command
2026-05-12 13:16:00,11112222-3333-4444-5555-666677778888,/home/user/myproject,false,/plan
2026-05-12 13:17:07,11112222-3333-4444-5555-666677778888,/home/user/myproject,false,/effort
2026-05-12 13:18:32,11112222-3333-4444-5555-666677778888,/home/user/myproject,false,fix the login bug
2026-05-13 09:42:15,c4d5e6f7-a8b9-0123-cdef-456789abcdef,/home/user/api,true,!git status
```

The `Shell?` column is `true` when the entry was run as a subshell command via the prompt's `!` bang-mode, and `false` for regular prompts and slash commands.

### Markdown (beginning)

```markdown
# Claude Code Forensic Analysis Report

- **Generated At**: 2026-05-25T13:38:19Z
- **Total Sessions**: 310
- **Total Projects**: 6
- **Date Range**: 2026-05-12 ~ 2026-05-25

## 1. User Identity

- **Email**: user@example.com
- **Organization Type**: claude_max
- **Rate Limit Tier**: default_claude_max_5x

## 2. Sessions
| Session ID | Project | Started At | Duration (min) | Model | Messages |
|---|---|---|---|---|---|
| `a1b2c3d4` | /home/user/myproject | 2026-05-12 13:16:00 | 47.2 | claude-opus-4-7 | 139 |
```

### HTML

A self-contained HTML file (embedded CSS and JS, no external resources) is generated. It features a dark theme, collapsible tables, CSS bar charts, and **per-column filter boxes** on the large tables (Sessions, Timeline, File Changes, Command History, Conversations) for live row filtering. It opens directly in a browser and is print-ready (`@media print`; filters are hidden when printing).

## Report Sections

The generated report includes the following sections:

| # | Section | Content |
|---|---|---|
| 1 | User Identity | User email, organization info, subscription tier |
| 2 | Sessions | List of all sessions (start time, duration, model, message count) |
| 3 | Activity Timeline | Chronological user actions, tool calls, and responses |
| 4 | Projects | Per-project summary (session count, first/last use) |
| 5 | Tool Usage Statistics | Call count and session count per tool |
| 6 | Token Consumption | Token consumption (by model, by project, by date) |
| 7 | File Modifications | File changes made by the Edit/Write tools |
| 8 | Permission & Security | deny/allow rules, hook definitions, per-session permission mode |
| 9 | Credential Discovery | Existence detection of `.credentials.json` (token values are not read) |
| 10 | Command History | Command input history from `history.jsonl` (timestamp, session, project, and a `Shell?` flag marking `!` bang-mode subshell commands) |
| 11 | File History Statistics | File edit session count and total version count from `file-history/` |
| 12 | Auxiliary Artifact Statistics | Counts of shell-snapshots, paste-cache, tasks, plans, custom-commands |
| 13 | Conversations | Full conversation text (included by default; exclude with `--extract-conversations=false`) |
| 14 | Prompt Injection Triage | What entered each session (fetched URLs, files read, hook-injected text), what the text looked like, what left afterwards, and what was changed — correlated and ranked. See `ccfx help injection` |

## Security Notes

- **Credential token values are never read.** For `.credentials.json`, only the file's existence, size, and modification time are recorded.
- **`-ac` is the one exception.** The acquisition archive is a byte-for-byte copy of the source directory, so it contains `.credentials.json` with the OAuth token in cleartext, and `--redact-pii` does not apply to it. Whoever holds the archive can authenticate as the user it came from. Encrypt it in transit and at rest, and omit `-ac` when the report alone answers the question.
- With `--redact-pii`, email addresses and UUIDs in the output report are masked (`us***@example.com`, `a1b2c3d4-****-****-****-************`).
- The contents of the input directory (`~/.claude/`) are never modified (read-only analysis).

## Project Structure

```
ccfx/
├── main.go              # CLI entry point
├── model/model.go       # Data model definitions
├── collector/           # Artifact collection (8 parsers)
│   ├── collector.go     #   Orchestrator
│   ├── history.go       #   history.jsonl
│   ├── sessions.go      #   sessions/<pid>.json
│   ├── transcripts.go   #   projects/<path>/<uuid>.jsonl
│   ├── backups.go       #   backups/.claude.json.backup.*
│   ├── settings.go      #   settings.json
│   ├── credentials.go   #   .credentials.json (existence detection only)
│   ├── filehistory.go   #   file-history/
│   └── misc.go          #   shell-snapshots, paste-cache, etc.
├── analyzer/            # Forensic analysis
│   ├── analyzer.go      #   RawData → ForensicReport conversion
│   ├── timeline.go      #   Chronological event construction
│   ├── toolusage.go     #   Tool usage statistics
│   ├── tokenusage.go    #   Token consumption analysis
│   ├── filetracking.go  #   File change tracking
│   └── permissions.go   #   Permission analysis
└── renderer/            # Report output
    ├── renderer.go      #   Format dispatch
    ├── json.go          #   JSON output
    ├── csv.go           #   CSV output (with UTF-8 BOM)
    ├── markdown.go      #   Markdown output
    ├── html.go          #   HTML output (self-contained)
    └── locale.go        #   English/Japanese bilingual dictionary
```

## Forensic Use Cases

### Incident Response
```bash
# Investigate Claude Code usage on a departed employee's PC (displayed in local time)
./ccfx --path /mnt/evidence/Users/suspect/.claude \
       --format json,html \
       --timezone Asia/Tokyo \
       --extract-conversations \
       --redact-pii
```

### Internal Audit
```bash
# Get this month's usage as CSV to import into a spreadsheet
./ccfx --format csv \
       --timezone Asia/Tokyo \
       --date-from 2026-05-01 --date-to 2026-05-31 \
       --language ja
```

### Security Review
```bash
# Check permission settings and credential status
./ccfx --format html --language ja
# → Review the "Permission & Security" and "Credential Discovery" sections in report.html
```

## Disclaimer

- This tool is experimental. It does not guarantee the accuracy or completeness of its output.
- The developer assumes no liability for any damage arising from its use.
- This tool is intended for authorized use only. Misuse for unauthorized data collection or analysis is prohibited.

## License

MIT
