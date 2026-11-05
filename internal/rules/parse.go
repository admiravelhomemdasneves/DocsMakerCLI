package rules

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/langmodel"
)

// Extraction is what the model said about one method, before it is tied to a
// location and judged. HasRule is false when the model found nothing.
type Extraction struct {
	HasRule       bool
	Statement     string
	Conditions    []string
	Rationale     string
	Evidence      []string
	Confidence    Confidence
	OpenQuestions []string
}

// Parse reads a model answer. Local models are sloppy, so it accepts what a
// sensible person would: JSON wrapped in prose or code fences, a bare "NONE",
// lists given as a single string, a missing hasRule when a statement is
// present, and Portuguese confidence words. What it refuses is returned as an
// error whose text is fit to show the model on a retry.
func Parse(text string) (*Extraction, error) {
	t := strings.TrimSpace(text)
	if t == "" {
		return nil, errors.New("the answer was empty")
	}
	if strings.EqualFold(strings.Trim(t, " \t\r\n.\"'`*"), "none") {
		return &Extraction{}, nil
	}

	obj, err := firstJSONObject(t)
	if err != nil {
		return nil, err
	}

	var raw struct {
		HasRule       json.RawMessage `json:"hasRule"`
		Statement     json.RawMessage `json:"statement"`
		Conditions    json.RawMessage `json:"conditions"`
		Rationale     json.RawMessage `json:"rationale"`
		Evidence      json.RawMessage `json:"evidence"`
		Confidence    json.RawMessage `json:"confidence"`
		OpenQuestions json.RawMessage `json:"openQuestions"`
	}
	if err := json.Unmarshal([]byte(obj), &raw); err != nil {
		return nil, fmt.Errorf("the JSON is invalid: %v", err)
	}

	e := &Extraction{
		Conditions:    stringList(raw.Conditions),
		Evidence:      stringList(raw.Evidence),
		OpenQuestions: stringList(raw.OpenQuestions),
		Statement:     strings.Join(stringList(raw.Statement), " "),
		Rationale:     strings.Join(stringList(raw.Rationale), " "),
	}
	if c, ok := ParseConfidence(strings.Join(stringList(raw.Confidence), "")); ok {
		e.Confidence = c
	}

	has, known := boolValue(raw.HasRule)
	if !known {
		has = strings.TrimSpace(e.Statement) != ""
	}
	if !has {
		return &Extraction{}, nil
	}
	e.HasRule = true
	if strings.TrimSpace(e.Statement) == "" {
		return nil, errors.New(`"hasRule" is true but "statement" is missing or empty`)
	}
	return e, nil
}

// Rule turns a positive extraction into a Rule for the method at rng. The
// result is normalized; the caller should still run CheckEvidence against the
// method source. It returns nil when the extraction found no rule.
func (e *Extraction) Rule(id, target string, rng langmodel.Range) *Rule {
	if e == nil || !e.HasRule {
		return nil
	}
	r := &Rule{
		ID: id, Target: target, Range: rng,
		Statement: e.Statement, Conditions: e.Conditions, Rationale: e.Rationale,
		Evidence: e.Evidence, Confidence: e.Confidence, OpenQuestions: e.OpenQuestions,
	}
	return r.Normalize()
}

// CheckEvidence enforces the rule that every claim must point at real code:
// evidence lines that do not appear in source (ignoring whitespace) are
// dropped as invented, and a rule left with no verified evidence is capped at
// low confidence, whatever the model claimed. It returns how many lines were
// dropped.
func (r *Rule) CheckEvidence(source string) (dropped int) {
	haystack := collapseSpace(source)
	var kept []string
	for _, line := range r.Evidence {
		if needle := collapseSpace(line); needle != "" && strings.Contains(haystack, needle) {
			kept = append(kept, line)
		} else {
			dropped++
		}
	}
	r.Evidence = kept
	if len(kept) == 0 {
		r.Confidence = ConfidenceLow
	}
	return dropped
}

func collapseSpace(s string) string { return strings.Join(strings.Fields(s), " ") }

// firstJSONObject returns the first complete {...} in s, honouring strings
// and escapes so braces inside text do not confuse it.
func firstJSONObject(s string) (string, error) {
	start := strings.IndexByte(s, '{')
	if start < 0 {
		return "", errors.New("no JSON object was found in the answer")
	}
	depth, inString, escaped := 0, false, false
	for i := start; i < len(s); i++ {
		c := s[i]
		switch {
		case escaped:
			escaped = false
		case inString:
			switch c {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
		case c == '"':
			inString = true
		case c == '{':
			depth++
		case c == '}':
			depth--
			if depth == 0 {
				return s[start : i+1], nil
			}
		}
	}
	return "", errors.New("the JSON object is incomplete (the answer was cut off)")
}

// boolValue reads a JSON bool, or the strings "true"/"false"; known is false
// when the field is absent or unreadable.
func boolValue(raw json.RawMessage) (value, known bool) {
	if len(raw) == 0 {
		return false, false
	}
	var b bool
	if json.Unmarshal(raw, &b) == nil {
		return b, true
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		switch strings.ToLower(strings.TrimSpace(s)) {
		case "true", "yes", "sim":
			return true, true
		case "false", "no", "não", "nao":
			return false, true
		}
	}
	return false, false
}

// stringList reads a field the model may have written as a string, a list of
// strings, a number, or null, and returns the non-empty texts.
func stringList(raw json.RawMessage) []string {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return nil
	}
	var out []string
	var add func(any)
	add = func(x any) {
		switch t := x.(type) {
		case string:
			if t = strings.TrimSpace(t); t != "" {
				out = append(out, t)
			}
		case float64:
			out = append(out, strconv.FormatFloat(t, 'f', -1, 64))
		case []any:
			for _, item := range t {
				add(item)
			}
		}
	}
	add(v)
	return out
}
