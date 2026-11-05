package secrets

import (
	"math"
	"regexp"
	"strings"
)

// stringLiteralRe finds double-quoted literals of at least 20 characters;
// shorter ones can't hold a meaningfully random secret.
var stringLiteralRe = regexp.MustCompile(`"((?:[^"\\\n]|\\.){20,})"`)

var (
	uuidRe    = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	hexOnlyRe = regexp.MustCompile(`^[0-9a-fA-F]+$`)
)

// entropyFindings flags opaque, random-looking string literals that no
// regex rule recognises (rotated/custom secret formats). Thresholds are
// deliberately conservative to keep false positives (identifiers, URLs,
// paths, UUIDs) from drowning the confirmation prompt.
func entropyFindings(text string) []Finding {
	var out []Finding
	for _, loc := range stringLiteralRe.FindAllStringSubmatchIndex(text, -1) {
		start, end := loc[2], loc[3]
		content := text[start:end]
		if !looksRandom(content) {
			continue
		}
		out = append(out, Finding{Kind: KindHighEntropy, Match: content, StartByte: start, EndByte: end})
	}
	return out
}

func looksRandom(s string) bool {
	if strings.ContainsAny(s, " \t\r\n") || uuidRe.MatchString(s) {
		return false
	}
	// URLs, file paths and dotted class/property names are structured, not secret.
	if strings.Contains(s, "://") || strings.Count(s, "/") >= 3 || strings.Count(s, ".") >= 2 {
		return false
	}

	h := shannon(s)
	if hexOnlyRe.MatchString(s) {
		return len(s) >= 32 && h >= 3.0 // hex tops out at 4.0 bits/char
	}

	hasDigit, hasLetter := false, false
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			hasDigit = true
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
			hasLetter = true
		}
	}
	return hasDigit && hasLetter && h >= 4.0
}

// shannon returns the Shannon entropy of s in bits per byte.
func shannon(s string) float64 {
	if s == "" {
		return 0
	}
	var counts [256]int
	for i := 0; i < len(s); i++ {
		counts[s[i]]++
	}
	n := float64(len(s))
	var h float64
	for _, c := range counts {
		if c == 0 {
			continue
		}
		p := float64(c) / n
		h -= p * math.Log2(p)
	}
	return h
}
