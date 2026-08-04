// Package detect scans text that entered a Claude Code conversation for the
// hallmarks of a prompt injection. It is a leaf package: collector scans while
// streaming (tool results are far too large to retain), analyzer scans the
// smaller texts it already holds.
//
// Every rule here is a *symptom*, not a verdict. Legitimate content trips these
// patterns routinely — documentation about prompt injection trips almost all of
// them. The output is meant to be read by a human deciding where to look.
package detect

import (
	"regexp"
	"strings"

	"github.com/fkasasagi/ccfx/model"
)

// maxScan bounds the work per result. Injected instructions are placed where a
// model will read them, and a model does not read 1 MB into a tool result
// either.
const maxScan = 1 << 20

const (
	excerptBefore = 60
	excerptAfter  = 140
)

type rule struct {
	id       string
	severity string
	// needles are matched case-insensitively against the lowercased text.
	needles []string
	// re is matched against the original text, for patterns that case folding
	// would destroy.
	re *regexp.Regexp
}

var rules = []rule{
	{
		id:       "instruction-override",
		severity: "high",
		needles: []string{
			"ignore previous instruction", "ignore all previous", "ignore the above",
			"disregard previous", "disregard the above", "disregard all prior",
			"forget everything", "forget all previous", "new instructions:",
			"updated instructions:", "override your instructions", "your new task is",
			"以前の指示を無視", "これまでの指示を無視", "上記の指示を無視",
		},
	},
	{
		// Claude Code delivers real out-of-band guidance in <system-reminder> tags,
		// so fetched or read content carrying one is impersonating the harness.
		id:       "authority-spoof",
		severity: "high",
		needles: []string{
			"<system-reminder", "</system-reminder", "<system>", "[system]",
			"system prompt:", "you are now", "from now on you must",
			"as the system administrator", "this is a message from anthropic",
		},
	},
	{
		id:       "tool-call-injection",
		severity: "high",
		needles: []string{
			"<invoke", "<function_calls>", "<invoke name=", "</function_calls",
		},
	},
	{
		id:       "secrecy-request",
		severity: "high",
		needles: []string{
			"do not tell the user", "don't tell the user", "without telling the user",
			"do not mention this", "don't mention this", "without informing",
			"do not inform the user", "keep this secret", "silently and without",
			"ユーザーには言わない", "ユーザーに知らせず",
		},
	},
	{
		// A mention of a credential path is weak on its own — any tool that reads
		// its own source trips it. It earns weight only next to an egress event,
		// which the correlation pass supplies.
		id:       "credential-target",
		severity: "low",
		needles: []string{
			"id_rsa", "id_ed25519", ".credentials.json", "anthropic_api_key",
			"aws_secret_access_key", "~/.ssh/", "/.aws/credentials", ".npmrc",
		},
	},
	{
		// An instruction to go get a secret, as opposed to a mention of one.
		id:       "credential-request",
		severity: "high",
		needles: []string{
			"cat ~/.ssh", "cat .env", "print the contents of .env", "read the .env",
			"show me your api key", "send the api key", "reveal the token",
			"contents of ~/.aws", "export the credentials",
		},
	},
	{
		id:       "exfiltration-instruction",
		severity: "high",
		needles: []string{
			"webhook.site", "requestbin", "pipedream.net", "ngrok.io", "burpcollaborator",
			"send the contents to", "send it to http", "post the contents", "upload the file to",
			"exfiltrat",
		},
	},
	{
		id:       "permission-escalation",
		severity: "high",
		needles: []string{
			"--dangerously-skip-permissions", "bypasspermissions",
			"add this to the allow list", "disable the hook", "remove the deny rule",
		},
	},
	{
		// Zero-width and bidirectional-override characters hide text from a human
		// reviewer while leaving it perfectly legible to the model.
		id:       "hidden-characters",
		severity: "med",
		re:       regexp.MustCompile(`[\x{200b}-\x{200f}\x{202a}-\x{202e}\x{2060}-\x{2064}\x{2066}-\x{2069}\x{feff}]`),
	},
	{
		// A wall of blank lines is the classic way to push instructions below the
		// fold of anything that previews the first screenful.
		id:       "scroll-hiding",
		severity: "med",
		re:       regexp.MustCompile(`\n[ \t]*(?:\n[ \t]*){14,}\S`),
	},
	{
		id:       "agent-directed-imperative",
		severity: "med",
		needles: []string{
			"if you are an ai", "if you are a language model", "attention: ai agent",
			"note to the assistant", "note to claude", "dear claude", "hey claude,",
			"when you read this file", "ai agents reading this",
		},
	},
	{
		id:       "encoded-payload",
		severity: "low",
		re:       regexp.MustCompile(`[A-Za-z0-9+/]{300,}={0,2}`),
	},
}

// Scan reports at most one hit per rule, in rule order. One hit is enough to
// send a human to the source; a hundred would bury the next finding.
func Scan(text string) []model.ContentSignal {
	if text == "" {
		return nil
	}
	scanned := text
	if len(scanned) > maxScan {
		scanned = scanned[:maxScan]
	}
	lower := strings.ToLower(scanned)

	var signals []model.ContentSignal
	for _, r := range rules {
		if idx, matched := r.find(scanned, lower); matched {
			signals = append(signals, model.ContentSignal{
				Rule:     r.id,
				Severity: r.severity,
				Offset:   idx,
				Excerpt:  excerpt(scanned, idx),
			})
		}
	}
	return signals
}

func (r rule) find(text, lower string) (int, bool) {
	best := -1
	for _, n := range r.needles {
		if i := strings.Index(lower, n); i >= 0 && (best < 0 || i < best) {
			best = i
		}
	}
	if r.re != nil {
		if loc := r.re.FindStringIndex(text); loc != nil && (best < 0 || loc[0] < best) {
			best = loc[0]
		}
	}
	return best, best >= 0
}

// Severity returns the worst severity among signals, or "" for none.
func Severity(signals []model.ContentSignal) string {
	worst := ""
	for _, s := range signals {
		if rank(s.Severity) > rank(worst) {
			worst = s.Severity
		}
	}
	return worst
}

func rank(sev string) int {
	switch sev {
	case "high":
		return 3
	case "med":
		return 2
	case "low":
		return 1
	}
	return 0
}

var whitespace = regexp.MustCompile(`\s+`)

// The detector deliberately hunts for text near credentials, so the excerpt it
// quotes is exactly where a real secret would end up in the report. Masking
// happens at the point the quote is produced — not behind --redact-pii — because
// no caller should have to remember to ask.
var secretPatterns = []struct {
	re   *regexp.Regexp
	with string
}{
	{regexp.MustCompile(`-----BEGIN[A-Z ]*PRIVATE KEY-----[\s\S]*?(?:-----END[A-Z ]*PRIVATE KEY-----|$)`), "[REDACTED PRIVATE KEY]"},
	{regexp.MustCompile(`\b(?:sk-ant-|sk-|ghp_|gho_|ghs_|github_pat_|glpat-|xox[baprs]-)[A-Za-z0-9_\-]{10,}`), "[REDACTED TOKEN]"},
	{regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`), "[REDACTED KEY ID]"},
	{regexp.MustCompile(`(?i)\b(bearer)\s+[A-Za-z0-9._\-]{16,}`), "$1 [REDACTED]"},
	{regexp.MustCompile(`(?i)"(access_?token|refresh_?token|api_?key|client_secret|secret|password)"\s*:\s*"[^"]{6,}"`), `"$1": "[REDACTED]"`},
	{regexp.MustCompile(`(?i)\b([A-Z0-9_]*(?:TOKEN|SECRET|API_?KEY|PASSWORD))\s*=\s*\S{6,}`), "$1=[REDACTED]"},
}

// MaskSecrets removes credential-shaped substrings from text that is about to be
// written into a report. It is deliberately shape-based and conservative: it can
// only catch what it recognises, so it is a second line of defence, not a
// guarantee.
func MaskSecrets(s string) string {
	for _, p := range secretPatterns {
		s = p.re.ReplaceAllString(s, p.with)
	}
	return s
}

func excerpt(text string, idx int) string {
	start := idx - excerptBefore
	if start < 0 {
		start = 0
	}
	end := idx + excerptAfter
	if end > len(text) {
		end = len(text)
	}
	// Do not slice a rune in half.
	for start > 0 && !isRuneStart(text[start]) {
		start--
	}
	for end < len(text) && !isRuneStart(text[end]) {
		end++
	}

	out := whitespace.ReplaceAllString(text[start:end], " ")
	out = MaskSecrets(strings.TrimSpace(out))
	if start > 0 {
		out = "…" + out
	}
	if end < len(text) {
		out += "…"
	}
	return out
}

func isRuneStart(b byte) bool { return b&0xC0 != 0x80 }
