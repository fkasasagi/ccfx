package renderer

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fkasasagi/ccfx/model"
)

type htmlData struct {
	Report *model.ForensicReport
	Dict   Dict
}

func writeHTML(report *model.ForensicReport, outDir string, dict Dict, tz *time.Location) ([]OutputFile, error) {
	path := filepath.Join(outDir, "report.html")

	funcMap := template.FuncMap{
		"fmtTime":  func(t time.Time) string { return formatTimeIn(t, tz) },
		"fmtDate":  func(t time.Time) string { return formatDateIn(t, tz) },
		"fmtInt64": formatInt64,
		"fmtDur": func(sec float64) string {
			return fmt.Sprintf("%.1f", sec/60)
		},
		"boolYN": func(v bool) string {
			return boolStr(v, dict)
		},
		"truncate": func(s string, n int) string {
			if len(s) <= n {
				return s
			}
			return s[:n] + "..."
		},
		"sanitize": func(s string) string {
			return strings.ReplaceAll(s, "\n", " ")
		},
		"sortedDateKeys": func(m map[string]model.TokenSummary) []string {
			return sortedKeys(m)
		},
		"getToken": func(m map[string]model.TokenSummary, key string) model.TokenSummary {
			return m[key]
		},
		"toFloat": func(n int) float64 {
			return float64(n)
		},
		"mulFloat": func(a, b float64) float64 {
			return a * b
		},
		"divFloat": func(a, b float64) float64 {
			if b == 0 {
				return 0
			}
			return a / b
		},
		"maxToolCalls": func(tools []model.ToolRanking) int {
			m := 0
			for _, t := range tools {
				if t.TotalCalls > m {
					m = t.TotalCalls
				}
			}
			return m
		},
		"barWidth": func(calls, max int) float64 {
			if max == 0 {
				return 0
			}
			return float64(calls) * 200.0 / float64(max)
		},
	}

	tmpl, err := template.New("report").Funcs(funcMap).Parse(htmlTemplate)
	if err != nil {
		return nil, fmt.Errorf("template parse: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	data := htmlData{Report: report, Dict: dict}
	if err := tmpl.Execute(f, data); err != nil {
		return nil, fmt.Errorf("template execute: %w", err)
	}

	return []OutputFile{{Path: path, Size: fileSize(path)}}, nil
}

const htmlTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>{{.Dict.title}}</title>
<style>
:root{--bg:#0d1117;--card:#161b22;--border:#30363d;--text:#e6edf3;--muted:#8b949e;--accent:#58a6ff;--green:#3fb950;--red:#f85149;--orange:#d29922}
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Helvetica,Arial,sans-serif;background:var(--bg);color:var(--text);line-height:1.6;padding:2rem;max-width:1400px;margin:0 auto}
h1{color:var(--accent);margin-bottom:.5rem;font-size:1.8rem}
h2{color:var(--accent);margin:2rem 0 1rem;padding-bottom:.5rem;border-bottom:1px solid var(--border);font-size:1.3rem}
h3{color:var(--text);margin:1.5rem 0 .5rem;font-size:1.1rem}
.meta{color:var(--muted);font-size:.9rem;margin-bottom:2rem}
.meta span{margin-right:2rem}
.card{background:var(--card);border:1px solid var(--border);border-radius:6px;padding:1rem;margin:.5rem 0}
table{width:100%;border-collapse:collapse;font-size:.85rem;margin:.5rem 0}
th{background:var(--card);color:var(--accent);text-align:left;padding:8px 12px;border-bottom:2px solid var(--border);position:sticky;top:0}
td{padding:6px 12px;border-bottom:1px solid var(--border);vertical-align:top}
tr:hover{background:rgba(88,166,255,.05)}
.field{display:flex;gap:.5rem;padding:4px 0}
.field-label{color:var(--muted);min-width:180px}
.field-value{color:var(--text)}
.badge{display:inline-block;padding:2px 8px;border-radius:12px;font-size:.75rem;font-weight:600}
.badge-green{background:rgba(63,185,80,.15);color:var(--green)}
.badge-red{background:rgba(248,81,73,.15);color:var(--red)}
.bar{height:20px;background:var(--accent);border-radius:3px;min-width:2px}
.bar-container{display:flex;align-items:center;gap:8px}
.bar-label{font-size:.8rem;color:var(--muted);min-width:60px;text-align:right}
details{margin:.5rem 0}
details>summary{cursor:pointer;color:var(--accent);font-weight:600;padding:4px 0}
code{background:var(--card);padding:2px 6px;border-radius:3px;font-size:.85rem;color:var(--orange)}
.rule-list{list-style:none;padding:0}
.rule-list li{padding:4px 0;font-family:monospace;font-size:.85rem;color:var(--muted)}
.stat-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(200px,1fr));gap:1rem;margin:1rem 0}
.stat-box{background:var(--card);border:1px solid var(--border);border-radius:6px;padding:1rem;text-align:center}
.stat-value{font-size:1.8rem;font-weight:700;color:var(--accent)}
.stat-label{font-size:.85rem;color:var(--muted)}
.scroll-table{max-height:500px;overflow-y:auto;border:1px solid var(--border);border-radius:6px}
@media print{body{background:#fff;color:#000;padding:1rem}h1,h2,h3,.stat-value{color:#000}.card,.stat-box{border-color:#ddd;background:#f8f8f8}th{background:#eee;color:#000}td{border-color:#ddd}tr:hover{background:transparent}.scroll-table{max-height:none;overflow:visible}}
</style>
</head>
<body>

<h1>{{.Dict.title}}</h1>
<div class="meta">
<span>{{.Dict.generated_at}}: {{fmtTime .Report.Meta.GeneratedAt}}</span>
<span>{{.Dict.source_path}}: {{.Report.Meta.SourcePath}}</span>
<span>{{.Dict.tool_version}}: {{.Report.Meta.ToolVersion}}</span>
<span>{{.Dict.platform}}: {{.Report.Meta.Platform}}</span>
</div>

<div class="stat-grid">
<div class="stat-box"><div class="stat-value">{{.Report.Meta.TotalSessions}}</div><div class="stat-label">{{.Dict.total_sessions}}</div></div>
<div class="stat-box"><div class="stat-value">{{.Report.Meta.TotalProjects}}</div><div class="stat-label">{{.Dict.total_projects}}</div></div>
<div class="stat-box"><div class="stat-value">{{fmtInt64 .Report.TokenUsage.TotalInput}}</div><div class="stat-label">{{.Dict.input_tokens}}</div></div>
<div class="stat-box"><div class="stat-value">{{fmtInt64 .Report.TokenUsage.TotalOutput}}</div><div class="stat-label">{{.Dict.output_tokens}}</div></div>
</div>

<h2>1. {{.Dict.user_identity}}</h2>
<div class="card">
{{if .Report.UserIdentity.Email}}<div class="field"><span class="field-label">{{.Dict.email}}</span><span class="field-value">{{.Report.UserIdentity.Email}}</span></div>{{end}}
{{if .Report.UserIdentity.AccountUUID}}<div class="field"><span class="field-label">{{.Dict.account_uuid}}</span><span class="field-value"><code>{{.Report.UserIdentity.AccountUUID}}</code></span></div>{{end}}
{{if .Report.UserIdentity.OrganizationUUID}}<div class="field"><span class="field-label">{{.Dict.org_uuid}}</span><span class="field-value"><code>{{.Report.UserIdentity.OrganizationUUID}}</code></span></div>{{end}}
{{if .Report.UserIdentity.OrganizationName}}<div class="field"><span class="field-label">{{.Dict.org_name}}</span><span class="field-value">{{.Report.UserIdentity.OrganizationName}}</span></div>{{end}}
{{if .Report.UserIdentity.OrganizationType}}<div class="field"><span class="field-label">{{.Dict.org_type}}</span><span class="field-value">{{.Report.UserIdentity.OrganizationType}}</span></div>{{end}}
{{if .Report.UserIdentity.OrganizationRole}}<div class="field"><span class="field-label">{{.Dict.org_role}}</span><span class="field-value">{{.Report.UserIdentity.OrganizationRole}}</span></div>{{end}}
{{if .Report.UserIdentity.RateLimitTier}}<div class="field"><span class="field-label">{{.Dict.rate_limit}}</span><span class="field-value">{{.Report.UserIdentity.RateLimitTier}}</span></div>{{end}}
{{if .Report.UserIdentity.ClaudeCodeVersion}}<div class="field"><span class="field-label">{{.Dict.cc_version}}</span><span class="field-value">{{.Report.UserIdentity.ClaudeCodeVersion}}</span></div>{{end}}
</div>

<h2>2. {{.Dict.sessions}}</h2>
<div class="scroll-table">
<table>
<thead><tr>
<th>{{.Dict.session_id}}</th><th>{{.Dict.project}}</th><th>{{.Dict.started_at}}</th>
<th>{{.Dict.duration_min}}</th><th>{{.Dict.model}}</th><th>{{.Dict.message_count}}</th>
<th>{{.Dict.tool_use_count}}</th><th>{{.Dict.git_branch}}</th><th>{{.Dict.title_label}}</th>
</tr></thead>
<tbody>
{{range .Report.Sessions}}<tr>
<td><code>{{.SessionID}}</code></td><td>{{.Project}}</td><td>{{fmtTime .StartedAt}}</td>
<td>{{fmtDur .DurationSec}}</td><td>{{.Model}}</td><td>{{.MessageCount}}</td>
<td>{{.ToolUseCount}}</td><td>{{.GitBranch}}</td><td>{{.Title}}</td>
</tr>{{end}}
</tbody>
</table>
</div>

<h2>3. {{.Dict.timeline}}</h2>
<div class="scroll-table">
<table>
<thead><tr>
<th>{{.Dict.timestamp}}</th><th>{{.Dict.event_type}}</th><th>{{.Dict.project}}</th>
<th>{{.Dict.summary}}</th><th>{{.Dict.model}}</th>
</tr></thead>
<tbody>
{{range .Report.Timeline}}<tr>
<td>{{fmtTime .Timestamp}}</td><td>{{.EventType}}</td><td>{{.Project}}</td>
<td>{{truncate (sanitize .Summary) 120}}</td><td>{{.Model}}</td>
</tr>{{end}}
</tbody>
</table>
</div>

<h2>4. {{.Dict.projects}}</h2>
<table>
<thead><tr>
<th>{{.Dict.project}}</th><th>{{.Dict.session_count}}</th><th>{{.Dict.first_seen}}</th>
<th>{{.Dict.last_seen}}</th><th>{{.Dict.total_messages}}</th><th>{{.Dict.total_tool_uses}}</th>
</tr></thead>
<tbody>
{{range .Report.Projects}}<tr>
<td>{{.Path}}</td><td>{{.SessionCount}}</td><td>{{fmtDate .FirstSeen}}</td>
<td>{{fmtDate .LastSeen}}</td><td>{{.TotalMessages}}</td><td>{{.TotalToolUses}}</td>
</tr>{{end}}
</tbody>
</table>

<h2>5. {{.Dict.tool_usage}}</h2>
{{$max := maxToolCalls .Report.ToolUsage.TopTools}}
<table>
<thead><tr><th>{{.Dict.tool_name}}</th><th>{{.Dict.total_calls}}</th><th>{{.Dict.session_count}}</th><th></th></tr></thead>
<tbody>
{{range .Report.ToolUsage.TopTools}}<tr>
<td><strong>{{.ToolName}}</strong></td><td>{{.TotalCalls}}</td><td>{{.SessionCount}}</td>
<td><div class="bar-container"><div class="bar" style="width:{{printf "%.0f" (barWidth .TotalCalls $max)}}px"></div></div></td>
</tr>{{end}}
</tbody>
</table>

<h2>6. {{.Dict.token_usage}}</h2>
<div class="stat-grid">
<div class="stat-box"><div class="stat-value">{{fmtInt64 .Report.TokenUsage.TotalInput}}</div><div class="stat-label">{{.Dict.input_tokens}}</div></div>
<div class="stat-box"><div class="stat-value">{{fmtInt64 .Report.TokenUsage.TotalOutput}}</div><div class="stat-label">{{.Dict.output_tokens}}</div></div>
<div class="stat-box"><div class="stat-value">{{fmtInt64 .Report.TokenUsage.TotalCacheCreate}}</div><div class="stat-label">{{.Dict.cache_creation}}</div></div>
<div class="stat-box"><div class="stat-value">{{fmtInt64 .Report.TokenUsage.TotalCacheRead}}</div><div class="stat-label">{{.Dict.cache_read}}</div></div>
</div>

{{if .Report.TokenUsage.ByModel}}
<h3>{{.Dict.by_model}}</h3>
<table>
<thead><tr><th>{{.Dict.model}}</th><th>{{.Dict.input_tokens}}</th><th>{{.Dict.output_tokens}}</th></tr></thead>
<tbody>
{{range $model, $tokens := .Report.TokenUsage.ByModel}}<tr>
<td>{{$model}}</td><td>{{fmtInt64 $tokens.InputTokens}}</td><td>{{fmtInt64 $tokens.OutputTokens}}</td>
</tr>{{end}}
</tbody>
</table>
{{end}}

{{if .Report.TokenUsage.ByDate}}
<h3>{{.Dict.by_date}}</h3>
<table>
<thead><tr><th>{{.Dict.date}}</th><th>{{.Dict.input_tokens}}</th><th>{{.Dict.output_tokens}}</th></tr></thead>
<tbody>
{{range $date := sortedDateKeys .Report.TokenUsage.ByDate}}{{$t := getToken $.Report.TokenUsage.ByDate $date}}<tr>
<td>{{$date}}</td><td>{{fmtInt64 $t.InputTokens}}</td><td>{{fmtInt64 $t.OutputTokens}}</td>
</tr>{{end}}
</tbody>
</table>
{{end}}

{{if .Report.FileChanges}}
<h2>7. {{.Dict.file_changes}}</h2>
<div class="scroll-table">
<table>
<thead><tr><th>{{.Dict.timestamp}}</th><th>{{.Dict.file_path}}</th><th>{{.Dict.tool_name}}</th><th>{{.Dict.operation}}</th></tr></thead>
<tbody>
{{range .Report.FileChanges}}<tr>
<td>{{fmtTime .Timestamp}}</td><td><code>{{.FilePath}}</code></td><td>{{.ToolName}}</td><td>{{.Operation}}</td>
</tr>{{end}}
</tbody>
</table>
</div>
{{end}}

<h2>8. {{.Dict.permissions}}</h2>
<div class="card">
{{if .Report.Permissions.GlobalDenyRules}}
<h3>{{.Dict.global_deny}}</h3>
<ul class="rule-list">{{range .Report.Permissions.GlobalDenyRules}}<li><code>{{.}}</code></li>{{end}}</ul>
{{end}}
{{if .Report.Permissions.GlobalAllowRules}}
<h3>{{.Dict.global_allow}}</h3>
<ul class="rule-list">{{range .Report.Permissions.GlobalAllowRules}}<li><code>{{.}}</code></li>{{end}}</ul>
{{end}}
{{if .Report.Permissions.HooksDefined}}
<h3>{{.Dict.hooks}}</h3>
<table><thead><tr><th>{{.Dict.event}}</th><th>{{.Dict.matcher}}</th><th>{{.Dict.command}}</th></tr></thead>
<tbody>{{range .Report.Permissions.HooksDefined}}<tr><td>{{.Event}}</td><td>{{.Matcher}}</td><td><code>{{.Command}}</code></td></tr>{{end}}</tbody></table>
{{end}}
</div>

<h2>9. {{.Dict.credentials}}</h2>
<div class="card">
<div class="field"><span class="field-label">{{.Dict.file_exists}}</span><span class="field-value">{{if .Report.Credentials.FileExists}}<span class="badge badge-red">{{boolYN true}}</span>{{else}}<span class="badge badge-green">{{boolYN false}}</span>{{end}}</span></div>
{{if .Report.Credentials.FileExists}}
<div class="field"><span class="field-label">{{.Dict.file_modified}}</span><span class="field-value">{{fmtTime .Report.Credentials.FileModified}}</span></div>
<div class="field"><span class="field-label">{{.Dict.file_size}}</span><span class="field-value">{{.Report.Credentials.FileSizeBytes}}</span></div>
<div class="field"><span class="field-label">{{.Dict.token_detected}}</span><span class="field-value">{{if .Report.Credentials.TokenDetected}}<span class="badge badge-red">{{boolYN true}}</span>{{else}}<span class="badge badge-green">{{boolYN false}}</span>{{end}}</span></div>
{{end}}
</div>

{{if .Report.Conversations}}
<h2>10. {{.Dict.conversations}}</h2>
{{range .Report.Conversations}}
<details>
<summary>{{if .Title}}{{.Title}}{{else}}{{.SessionID}}{{end}} ({{.Project}})</summary>
<table>
<thead><tr><th>{{$.Dict.timestamp}}</th><th>Role</th><th>Content</th></tr></thead>
<tbody>
{{range .Messages}}<tr>
<td>{{fmtTime .Timestamp}}</td><td>{{.Role}}</td><td>{{truncate (sanitize .Content) 300}}</td>
</tr>{{end}}
</tbody>
</table>
</details>
{{end}}
{{end}}

</body>
</html>`
