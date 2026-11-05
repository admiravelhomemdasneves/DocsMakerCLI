package secrets

import (
	"fmt"
	"sort"
	"strings"
)

// Redact replaces each finding's span in text with a [REDACTED:<kind>]
// placeholder. Findings must refer to text (as returned by Scanner.Scan);
// they are applied back-to-front so earlier offsets stay valid.
func Redact(text string, findings []Finding) string {
	sorted := append([]Finding(nil), findings...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].StartByte > sorted[j].StartByte })

	for _, f := range sorted {
		if f.StartByte < 0 || f.EndByte > len(text) || f.StartByte > f.EndByte {
			continue
		}
		text = text[:f.StartByte] + fmt.Sprintf("[REDACTED:%s]", f.Kind) + text[f.EndByte:]
	}
	return text
}

// Mask shortens a sensitive value for display in prompts and the audit log,
// so neither ever holds the full secret: enough to recognise it, not to use it.
func Mask(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) <= 8 {
		return value[:min(1, len(value))] + "***"
	}
	return value[:3] + "***" + value[len(value)-2:] + fmt.Sprintf(" (%d chars)", len(value))
}
