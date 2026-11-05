// Package rules models the business rules the "rules" feature extracts from
// source code: a natural-language statement of what a method enforces, the
// code evidence it was read from, and how much to trust it.
//
// A model only ever sees the code, so everything it says about *why* a rule
// exists is an inference. The types here keep that distinction explicit:
// Statement describes what the code does, Rationale is a hypothesis, and
// Evidence plus Confidence let a reader judge it.
//
// Nothing in this package talks to an LLM or to a source language; it is
// plain data and validation, usable by the pipeline, the app layer and every
// renderer.
package rules

import (
	"fmt"
	"strings"

	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/langmodel"
)

// Confidence is how much a reader should trust an extracted rule.
type Confidence string

const (
	ConfidenceLow    Confidence = "low"
	ConfidenceMedium Confidence = "medium"
	ConfidenceHigh   Confidence = "high"
)

// Rank orders confidences (low < medium < high); an unknown value ranks 0.
func (c Confidence) Rank() int {
	switch c {
	case ConfidenceLow:
		return 1
	case ConfidenceMedium:
		return 2
	case ConfidenceHigh:
		return 3
	default:
		return 0
	}
}

// AtLeast reports whether c is at least as confident as min.
func (c Confidence) AtLeast(min Confidence) bool { return c.Rank() >= min.Rank() }

// ParseConfidence accepts the canonical values plus the Portuguese ones a
// model may answer with, case-insensitively. ok is false for anything else.
func ParseConfidence(s string) (c Confidence, ok bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "low", "baixa":
		return ConfidenceLow, true
	case "medium", "média", "media", "médio", "medio":
		return ConfidenceMedium, true
	case "high", "alta", "alto":
		return ConfidenceHigh, true
	default:
		return "", false
	}
}

// Rule is one business rule found in one method.
type Rule struct {
	// ID is stable for a given method location; see RuleID.
	ID string `json:"id"`
	// Target is "pkg.Type#method".
	Target string `json:"target"`
	// Range locates the method the rule was read from.
	Range langmodel.Range `json:"range"`
	// Statement is the rule itself, in one or two sentences: what the code
	// enforces, with the concrete values it uses.
	Statement string `json:"statement"`
	// Conditions lists the individual conditions/thresholds that make up the rule.
	Conditions []string `json:"conditions,omitempty"`
	// Rationale is the model's guess at why the rule exists. It is always an
	// inference, never a fact, and renderers must present it as such.
	Rationale string `json:"rationale,omitempty"`
	// Evidence quotes the lines of code the statement rests on.
	Evidence []string `json:"evidence,omitempty"`
	// Confidence is how much to trust the extraction.
	Confidence Confidence `json:"confidence"`
	// OpenQuestions are things the code alone cannot answer and a person who
	// knows the domain should confirm.
	OpenQuestions []string `json:"openQuestions,omitempty"`
}

// Target builds the "pkg.Type#method" name used across the CLI.
func Target(pkg, typeName, method string) string {
	owner := typeName
	if pkg != "" {
		owner = pkg + "." + typeName
	}
	return owner + "#" + method
}

// RuleID builds an identifier from the method's start line as well as its
// name: the code model finds methods by name only, so overloads share a
// Target, while their start lines never coincide.
func RuleID(pkg, typeName, method string, startLine int) string {
	return fmt.Sprintf("%s@L%d", Target(pkg, typeName, method), startLine)
}

// Normalize trims every text field, drops empty list entries and replaces a
// missing or unrecognised Confidence with ConfidenceLow (an unsure answer
// must never read as a sure one). It returns r for chaining.
func (r *Rule) Normalize() *Rule {
	r.Statement = strings.TrimSpace(r.Statement)
	r.Rationale = strings.TrimSpace(r.Rationale)
	r.Conditions = cleanList(r.Conditions)
	r.Evidence = cleanList(r.Evidence)
	r.OpenQuestions = cleanList(r.OpenQuestions)
	if c, ok := ParseConfidence(string(r.Confidence)); ok {
		r.Confidence = c
	} else {
		r.Confidence = ConfidenceLow
	}
	return r
}

// Validate reports why r is unusable, or nil. Call it after Normalize.
func (r *Rule) Validate() error {
	switch {
	case r.ID == "":
		return fmt.Errorf("rules: rule has no ID")
	case r.Target == "":
		return fmt.Errorf("rules: rule %s has no target", r.ID)
	case r.Statement == "":
		return fmt.Errorf("rules: rule %s has an empty statement", r.ID)
	}
	return nil
}

func cleanList(in []string) []string {
	var out []string
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}
