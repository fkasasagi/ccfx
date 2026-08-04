# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`ccfx` (Claude Code Forensics eXtractor) is a single Go binary that reads Claude Code's local
artifacts (`~/.claude/`) **read-only** and emits forensic reports in JSON / CSV / Markdown / HTML,
bilingual (en/ja).

Two hard constraints that shape every decision:

- **Zero external dependencies.** `go.mod` has no `require` block and must stay that way — the tool
  is meant to build and run on an air-gapped forensic workstation. Anything CSV/JSON/HTML/Markdown
  is done with stdlib (`encoding/csv`, `encoding/json`, `text/template`).
- **Never mutate or read secrets from the target directory.** `collector/credentials.go` only stats
  `.credentials.json` (existence / size / mtime); token values are never parsed or emitted.

## Commands

```bash
go build -o ccfx .                 # build
go vet ./...
go test ./...                     # all tests (uses testdata/claude fixture)
go test ./renderer/ -run TestRenderCSV -v     # single test
go test ./collector/ -v

# manual run against the fixture, into a throwaway dir
./ccfx --path ./testdata/claude --format all --output /tmp/ccfx-test --force

# cross-compile (release artifacts are gitignored)
GOOS=linux GOARCH=amd64 go build -o ccfx-linux-amd64 .
```

`ccfx` refuses to write into a directory that already holds report files unless `--force` is passed,
so use a fresh `--output` dir or `--force` when iterating.

## Architecture: three stages, one struct between each

```
~/.claude/ → collector.Collect() → *model.RawData → analyzer.Analyze() → *model.ForensicReport → renderer.Render() → files
```

Every type lives in `model/model.go`; the three packages never import each other, only `model`.

- **collector/** — one file per artifact type, each returning into `RawData`. Failures are
  *non-fatal by design*: `Collect()` logs them only under `--verbose` and continues (a forensic
  image is often partial). Do not turn parse errors into hard failures.
- **analyzer/** — `Analyze()` calls one `buildXxx()` per report section, applies the
  session/project/date filters, and optionally redacts PII. It also merges the two independent views
  of a session: `sessions/<pid>.json` (metadata) and `projects/**/<uuid>.jsonl` (transcript), keyed
  by `sessionId`; transcript-only sessions get synthesized entries.
- **renderer/** — one file per format, all driven by the same `Dict` label table in `locale.go`.

### Adding things — the multi-file touch points

- **New artifact parser**: `collector/<name>.go` → new field on `RawData` → call it from
  `Collect()` → surface it in `analyzer` → add a section to *each* renderer.
- **New output format**: `renderer/<fmt>.go` with `write<Fmt>(report, outDir, dict, tz)` → branch in
  `Render()` → add the key to `parseFormats()`'s `valid` map **and** to the `"all"` list in
  `main.go` → update `showFormatsHelp()`.
- **New output file**: add it to the writers list in `csv.go` (or equivalent), to
  `renderer.KnownOutputFiles()` (this drives the `--force` overwrite guard), and to
  `expectedFiles` in `renderer/renderer_test.go`. All three must agree — a file missing from
  `KnownOutputFiles` is silently overwritten without `--force`.
- **New label**: add the key to **both** `dictEN` and `dictJA`. A missing key renders as an empty
  string, silently. If the label is a timestamp column, add its key to the `timeKeys` list in
  `appendTZ()` so it gets the ` (JST)`-style suffix.

## Non-obvious behaviors and gotchas

- **Scanner buffer**: transcript JSONL lines can be hundreds of KB (a `tool_result` may embed a
  whole file), so `parseTranscript` sets a 4 MB `scanner.Buffer`. The 64 KB default panics with
  `token too long`.
- **Polymorphic `message.content`**: it is either a plain string or an array of
  `{type: text|tool_use|tool_result}` items. `parseTranscriptMessage` tries the string unmarshal
  first and falls back to the array form — keep both paths when touching it.
- **Three timestamp encodings**: `history.jsonl` and `sessions/*.json` use Unix **milliseconds**
  (`safeUnixMilli` maps `<= 0` to the zero time), transcripts use RFC3339. Everything is normalized
  to `time.Time` in the collector, and only converted to a `--timezone` at render time.
- **Tool input is truncated to 200 chars** in `collector/transcripts.go`. `analyzer/filetracking.go`
  parses `file_path` out of that truncated JSON, so file-change tracking only works while
  `file_path` appears early in the tool input. Widen the truncation if that assumption breaks.
- **Project path decoding is lossy**: directory names encode `/` as `-`. `buildProjectMap` prefers
  the authoritative real paths from the backup file and only falls back to naive `-` → `/`
  substitution, which mangles paths that legitimately contain `-`.
- **Zero times in JSON**: `writeJSON` byte-replaces `"0001-01-01T00:00:00Z"` with `null` after
  marshaling. A test asserts no zero-time string survives.
- **CSV files start with a UTF-8 BOM** (`newCSVFile`) for Windows Excel; tests assert it, and any
  reader must skip 3 bytes before `csv.NewReader`.
- **Markdown section numbers 1–13 are hardcoded** in `markdown.go` and asserted by
  `TestRenderMarkdown`; renumber deliberately.
- **HTML is one big `text/template` string** with a `FuncMap` supplying the arithmetic Go templates
  lack (`toFloat`/`mulFloat`/`divFloat`/`barWidth`/`maxToolCalls`). Output must stay self-contained —
  no external CSS/JS/fonts.

## Tests

`testdata/claude/` is a miniature `~/.claude/` tree. `collector_test.go` asserts **exact counts**
(3 history entries, 2 sessions, 2 transcripts, 1 shell snapshot, …), so adding a fixture file will
break tests until the expected numbers are updated — that is intentional.

## Release conventions

- Version lives in one place: `const version` in `main.go`. Bump it in its own
  `chore: bump version to X.Y.Z` commit, after the feature commits.
- Commit messages: `feat:` / `fix:` / `docs:` / `chore:`, imperative, English.
- `README.md` (English, canonical) and `README.ja.md` are mirrors — user-visible changes (flags,
  output files, report sections) must be applied to both, and to the relevant `ccfx help <topic>`
  text in `main.go`.
- `docs/` holds Japanese design notes and is **gitignored** (local-only); binaries, `ccfx-*`,
  `checksums.txt`, and `ccfx-output/` are gitignored too.
