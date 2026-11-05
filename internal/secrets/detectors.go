package secrets

import (
	"regexp"
	"sort"
	"strings"
)

// detector is one regex-based rule. If useGroups is set, the sensitive span
// is the first participating capture group (so e.g. `password = "x"` redacts
// only the value, not the key); otherwise it is the whole match.
type detector struct {
	kind      Kind
	re        *regexp.Regexp
	useGroups bool
	// keep, if non-nil, can veto a candidate (used to drop placeholders).
	keep func(value string) bool
}

// detectors are ordered by specificity: when two rules flag overlapping
// spans, the earlier (more specific) one wins in dedupe.
var detectors = []detector{
	{
		kind: KindPrivateKey,
		re:   regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?(?:-----END [A-Z ]*PRIVATE KEY-----|$)`),
	},
	{
		kind: KindAWSKey,
		re:   regexp.MustCompile(`\b(?:AKIA|ASIA|AIDA|AROA)[0-9A-Z]{16}\b`),
	},
	{
		kind: KindJWT,
		re:   regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}`),
	},
	{
		// scheme://user:password@host — covers JDBC, Mongo, Postgres, Redis, AMQP, FTP, HTTP(S)...
		kind: KindConnString,
		re:   regexp.MustCompile(`(?i)\b(?:jdbc:[a-z0-9]+|mongodb(?:\+srv)?|postgres(?:ql)?|mysql|mariadb|rediss?|amqps?|ftps?|sftp|smtp|https?)://[^\s/:@"'<>]+:[^\s/@"'<>]+@[^\s"'<>]+`),
	},
	{
		// JDBC-style URLs carrying the password as a parameter.
		kind: KindConnString,
		re:   regexp.MustCompile(`(?i)\bjdbc:[a-z0-9]+:[^\s"']*?[?;&](?:password|pwd)=[^\s;&"']+`),
	},
	{
		kind: KindAPIToken,
		re:   regexp.MustCompile(`\b(?:gh[pousr]_[A-Za-z0-9]{36,}|github_pat_[A-Za-z0-9_]{22,}|xox[abprs]-[A-Za-z0-9-]{10,}|[sr]k_(?:live|test)_[A-Za-z0-9]{16,}|AIza[0-9A-Za-z_-]{35})`),
	},
	{
		kind:      KindGenericSecret,
		re:        regexp.MustCompile(`(?i)\bbearer\s+([a-z0-9._~+/=-]{20,})`),
		useGroups: true,
	},
	{
		// identifier ending in a sensitive word, assigned a quoted literal:
		//   String dbPassword = "hunter2";   "apiKey": "abc123..."
		// Anchoring the keyword at the END of the identifier keeps
		// `tokenType = "Bearer"` and `passwordPolicy = "strict"` out.
		kind:      KindGenericSecret,
		re:        regexp.MustCompile(`(?i)\b[a-z0-9_.-]*(?:password|passwd|pwd|passphrase|secret|secret[_-]?key|api[_-]?key|access[_-]?key|access[_-]?token|auth[_-]?token|private[_-]?key|client[_-]?secret|token)["']?\s*[:=]\s*(?:"([^"\\\s]{4,})"|'([^'\\\s]{4,})')`),
		useGroups: true,
		keep:      func(v string) bool { return !looksLikePlaceholder(v) },
	},
}

// DefaultScanner combines every regex detector with the entropy detector.
type DefaultScanner struct{}

var _ Scanner = DefaultScanner{}

// Scan implements Scanner.
func (DefaultScanner) Scan(text string) []Finding {
	type candidate struct {
		Finding
		priority int
	}
	var cands []candidate

	for prio, d := range detectors {
		for _, loc := range d.re.FindAllStringSubmatchIndex(text, -1) {
			start, end := loc[0], loc[1]
			if d.useGroups {
				start, end = -1, -1
				for g := 1; g*2+1 < len(loc); g++ {
					if loc[g*2] >= 0 {
						start, end = loc[g*2], loc[g*2+1]
						break
					}
				}
				if start < 0 {
					continue
				}
			}
			value := text[start:end]
			if d.keep != nil && !d.keep(value) {
				continue
			}
			cands = append(cands, candidate{
				Finding:  Finding{Kind: d.kind, Match: value, StartByte: start, EndByte: end},
				priority: prio,
			})
		}
	}

	for _, f := range entropyFindings(text) {
		cands = append(cands, candidate{Finding: f, priority: len(detectors)})
	}

	// Earliest start first; on a tie the more specific (lower priority
	// number) rule wins. Anything overlapping an already accepted finding
	// is dropped.
	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].StartByte != cands[j].StartByte {
			return cands[i].StartByte < cands[j].StartByte
		}
		return cands[i].priority < cands[j].priority
	})

	var out []Finding
	lastEnd := -1
	for _, c := range cands {
		if c.StartByte < lastEnd {
			continue
		}
		out = append(out, c.Finding)
		lastEnd = c.EndByte
	}
	return out
}

// looksLikePlaceholder reports whether a quoted value is obviously not a
// real credential: templating syntax, "changeme"-style stand-ins, masked
// values, or an ALL_CAPS name that refers to an env var / constant.
func looksLikePlaceholder(v string) bool {
	for _, prefix := range []string{"${", "{{", "<", "%", "$", "[redacted", "your_", "your-"} {
		if strings.HasPrefix(strings.ToLower(v), prefix) {
			return true
		}
	}
	lower := strings.ToLower(v)
	if strings.Contains(lower, "****") || strings.Contains(lower, "xxxx") {
		return true
	}
	switch lower {
	case "changeme", "change-me", "change_me", "password", "passwd", "secret",
		"example", "placeholder", "dummy", "todo", "null", "none", "undefined", "redacted":
		return true
	}
	return constantNameRe.MatchString(v)
}

var constantNameRe = regexp.MustCompile(`^[A-Z][A-Z0-9_]+$`)
