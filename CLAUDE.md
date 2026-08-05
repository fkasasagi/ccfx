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

Every type lives in `model/model.go`. The three pipeline packages never import each other — only
`model` and `detect`, both leaves.

`detect/` holds the prompt-injection rule table and `Scan()`. It sits outside the pipeline because
both ends need it: `collector` scans tool-result bodies **while streaming** (they are far too large
to retain — that is the whole reason the scan lives there and not in `analyzer`), and only the
matches survive into `RawData`. `analyzer/injection.go` then correlates those signals with what
happened next. Rules are symptoms, never verdicts; see `ccfx help injection`.

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
- **New CSV output file**: add it to `csvOutputs` in `renderer/csv.go` — that table is the single
  source of truth and `KnownOutputFiles()` (which drives the `--force` overwrite guard) derives from
  it. Then update the two hand-written places that deliberately do *not* derive from it:
  `expectedFiles` in `renderer/renderer_test.go` (an independent oracle — do not derive it, that
  makes the assertion tautological) and the file table in `showFormatsHelp()` in `main.go`.
  User-visible output files are also listed in `README.md` and `README.ja.md`.
- **New non-CSV output file** (a second JSON file, an HTML asset, …): only the CSV names are
  derived — `report.json` / `report.md` / `report.html` are still hardcoded at the top of
  `KnownOutputFiles()`, so add yours there by hand or the `--force` guard will not cover it.
  A file produced outside `renderer/` (as `-ac` does) is instead registered in `existingOutputs()`
  in `main.go`, which is what actually feeds the guard.
- **New label**: add the key to **both** `dictEN` and `dictJA`. A missing key renders as an empty
  string, silently. If the label is a timestamp column, add its key to the `timeKeys` list in
  `appendTZ()` so it gets the ` (JST)`-style suffix.

## Non-obvious behaviors and gotchas

- **Report output must be byte-reproducible.** Two runs over the same `~/.claude/` must produce
  identical files (modulo `generated_at`) — a forensic report gets diffed and re-derived. Anything
  assembled from a Go map (`sessionIndex`, tool rankings, project accumulators) is therefore sorted
  to a *total* order in `analyzer/`, with an explicit tiebreak; sorts over an already-deterministic
  slice use `sort.SliceStable`. Adding a `for k := range someMap` that reaches the output without a
  sort silently reintroduces the bug. (`text/template` sorts map keys itself, so HTML is safe;
  Markdown is not — use `sortedKeys`.)
- **Scanner buffer**: transcript JSONL lines can be hundreds of KB (a `tool_result` may embed a
  whole file), so `parseTranscript` sets a 4 MB `scanner.Buffer`. The 64 KB default panics with
  `token too long`.
- **Polymorphic `message.content`**: it is either a plain string or an array of
  `{type: text|tool_use|tool_result}` items. `parseTranscriptMessage` tries the string unmarshal
  first and falls back to the array form — keep both paths when touching it.
- **Three timestamp encodings**: `history.jsonl` and `sessions/*.json` use Unix **milliseconds**
  (`safeUnixMilli` maps `<= 0` to the zero time), transcripts use RFC3339. Everything is normalized
  to `time.Time` in the collector, and only converted to a `--timezone` at render time.
- **`main.go` blank-imports `_ "time/tzdata"` on purpose** — do not drop it as an "unused import".
  Windows ships no system zoneinfo, so without the embedded database `time.LoadLocation` fails on
  every IANA name (`Asia/Tokyo`, …) and `--timezone` crashes on the Windows build. It costs ~450 KB.
- **Never slice a string at a byte offset to truncate it.** Use `clip()` in `renderer/markdown.go`
  (exposed to templates as `truncate`). A cut mid-rune emits invalid UTF-8, and one bad byte makes
  the entire `report.html` unparseable — which is silent until someone tries to read the file.
- **A tool call and its result arrive on different transcript lines**, joined by `tool_use_id`.
  `collector/transcripts.go` keeps a pending map to pair them; the result body is scanned and
  dropped, never stored. Tool inputs are kept up to 4 KB (they were once cut at 200 chars, which
  silently broke `file_path` extraction in `analyzer/filetracking.go`).
- **The richest injection evidence is in the line types the pipeline once ignored**: `toolUseResult`
  (WebFetch's `url`/`bytes`, Bash's `stdout`, Read's `file`) and `attachment` lines — especially
  `hook_additional_context`, which is arbitrary text a hook injects on every turn with no user in
  the loop.
- **Detector tuning is a signal-to-noise problem, not a coverage problem.** Every threshold in
  `analyzer/injection.go` exists because the naive version buried the real findings: permission-mode
  lines are written routinely so only *transitions* count, `git push` is not egress, a mention of a
  credential path is `low` (ccfx's own source trips it) while an instruction to fetch one is `high`,
  and one ingress yields at most one finding. Measure against a real `~/.claude` before loosening
  any of them.
- **Project paths: encode is exact, decode is lossy.** Claude names `projects/` directories by
  replacing every non-alphanumeric rune with `-` (`encodeProjectPath` mirrors this — `:` `\` `/`
  and non-ASCII all collapse, so it matches on Windows too). `buildProjectMap` resolves a
  transcript's path in priority order: the exact per-line `cwd`, then the authoritative real path
  from the backup file (matched via `encodeProjectPath`), then a last-resort `decodeProjectPath`.
  Only that last step is lossy (a `-` may have been any separator or a literal `-`); it recognizes
  Unix-absolute and Windows drive-letter forms.
- **Zero times in JSON**: `writeJSON` byte-replaces `"0001-01-01T00:00:00Z"` with `null` after
  marshaling. A test asserts no zero-time string survives.
- **CSV files start with a UTF-8 BOM** (`newCSVFile`) for Windows Excel; tests assert it, and any
  reader must skip 3 bytes before `csv.NewReader`.
- **Markdown section numbers 1–14 are hardcoded** in `markdown.go` and asserted by
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
- `README.md` (Japanese, the default GitHub landing) and `README.en.md` (English) are mirrors —
  user-visible changes (flags, output files, report sections) must be applied to both, and to the
  relevant `ccfx help <topic>` text in `main.go`.
- `docs/` holds Japanese design notes and is **gitignored** (local-only); binaries, `ccfx-*`,
  `checksums.txt`, and `ccfx-output/` are gitignored too.
