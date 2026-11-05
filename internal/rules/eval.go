package rules

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Gabarito is the answer key for a fixture project: which methods hold a
// planted business rule (and what a correct explanation must mention), which
// are logic-heavy but not business rules, and which are plain plumbing.
type Gabarito struct {
	Rules     []PlantedRule `json:"rules"`
	Technical []string      `json:"technical"`
	NoRule    []string      `json:"noRule"`
}

// PlantedRule is one rule the fixture is known to contain.
type PlantedRule struct {
	Target  string `json:"target"`
	Summary string `json:"summary"`
	// Keywords are groups of lowercase alternatives: an explanation covers a
	// group when it contains any one of them, and is complete when it covers
	// every group. A crude proxy for "mentions the right values", not a proof
	// of correctness: a person must still read the explanations.
	Keywords [][]string `json:"keywords"`
}

// LoadGabarito reads an answer key from a JSON file.
func LoadGabarito(path string) (*Gabarito, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var g Gabarito
	if err := json.Unmarshal(data, &g); err != nil {
		return nil, fmt.Errorf("rules: %s is not a valid answer key: %w", path, err)
	}
	return &g, nil
}

// Outcome is what an extraction run said about one method, reduced to what
// the evaluation needs. It mirrors docgen.RuleResult without importing it.
type Outcome struct {
	Target          string
	Status          string // "rule", "none", "omitted" or "failed"
	Rule            *Rule
	EvidenceDropped int
}

// Verdict grades one planted rule.
type Verdict string

const (
	VerdictComplete Verdict = "complete" // found, and every keyword group is covered
	VerdictPartial  Verdict = "partial"  // found, but only some groups are covered
	VerdictWrong    Verdict = "wrong"    // found, but no group is covered
	VerdictMissed   Verdict = "missed"   // not found (no rule, failed, omitted or not analysed)
)

// RuleEval is the grade of one planted rule.
type RuleEval struct {
	Target     string     `json:"target"`
	Verdict    Verdict    `json:"verdict"`
	Covered    int        `json:"covered"`
	Groups     int        `json:"groups"`
	Status     string     `json:"status"`
	Confidence Confidence `json:"confidence,omitempty"`
	// Overconfident: the rule is not complete yet the model (after the
	// evidence check) still rated it high.
	Overconfident   bool   `json:"overconfident,omitempty"`
	EvidenceDropped int    `json:"evidenceDropped,omitempty"`
	Statement       string `json:"statement,omitempty"`
	Rationale       string `json:"rationale,omitempty"`
}

// TechnicalEval is the grade of one logic-heavy, non-business method.
type TechnicalEval struct {
	Target        string `json:"target"`
	Status        string `json:"status"`
	FalsePositive bool   `json:"falsePositive"`
}

// EvalReport is the result of grading one run against a Gabarito.
type EvalReport struct {
	Rules     []RuleEval      `json:"rules"`
	Technical []TechnicalEval `json:"technical"`
	// Unlisted are targets the run analysed that the key does not mention.
	Unlisted []string `json:"unlisted,omitempty"`
	// PlumbingSent are noRule targets that reached the model (a selector bug).
	PlumbingSent []string `json:"plumbingSent,omitempty"`

	Complete, Partial, Wrong, Missed int
	FalsePositives                   int
	Overconfident                    int
	Failed                           int // analysed methods with no usable answer
	EvidenceDropped                  int
}

// Evaluate grades outcomes against the answer key.
func Evaluate(g *Gabarito, outcomes []Outcome) *EvalReport {
	byTarget := map[string]Outcome{}
	for _, o := range outcomes {
		byTarget[o.Target] = o
	}

	rep := &EvalReport{}
	known := map[string]bool{}

	for _, p := range g.Rules {
		known[p.Target] = true
		o, analysed := byTarget[p.Target]
		ev := RuleEval{Target: p.Target, Groups: len(p.Keywords), Verdict: VerdictMissed, Status: "not analysed"}
		if analysed {
			ev.Status = o.Status
		}
		if analysed && o.Status == "rule" && o.Rule != nil {
			text := strings.ToLower(o.Rule.Statement + " " + strings.Join(o.Rule.Conditions, " "))
			for _, group := range p.Keywords {
				for _, alt := range group {
					if strings.Contains(text, strings.ToLower(alt)) {
						ev.Covered++
						break
					}
				}
			}
			switch {
			case ev.Covered == ev.Groups:
				ev.Verdict = VerdictComplete
			case ev.Covered > 0:
				ev.Verdict = VerdictPartial
			default:
				ev.Verdict = VerdictWrong
			}
			ev.Confidence = o.Rule.Confidence
			ev.Overconfident = ev.Verdict != VerdictComplete && o.Rule.Confidence == ConfidenceHigh
			ev.EvidenceDropped = o.EvidenceDropped
			ev.Statement, ev.Rationale = o.Rule.Statement, o.Rule.Rationale
		}
		switch ev.Verdict {
		case VerdictComplete:
			rep.Complete++
		case VerdictPartial:
			rep.Partial++
		case VerdictWrong:
			rep.Wrong++
		default:
			rep.Missed++
		}
		if ev.Overconfident {
			rep.Overconfident++
		}
		rep.Rules = append(rep.Rules, ev)
	}

	for _, target := range g.Technical {
		known[target] = true
		o, analysed := byTarget[target]
		te := TechnicalEval{Target: target, Status: "not analysed"}
		if analysed {
			te.Status = o.Status
			te.FalsePositive = o.Status == "rule"
		}
		if te.FalsePositive {
			rep.FalsePositives++
		}
		rep.Technical = append(rep.Technical, te)
	}

	plumbing := map[string]bool{}
	for _, t := range g.NoRule {
		plumbing[t] = true
	}
	for _, o := range outcomes {
		rep.EvidenceDropped += o.EvidenceDropped
		if o.Status == "failed" {
			rep.Failed++
		}
		switch {
		case plumbing[o.Target]:
			rep.PlumbingSent = append(rep.PlumbingSent, o.Target)
		case !known[o.Target]:
			rep.Unlisted = append(rep.Unlisted, o.Target)
		}
	}
	return rep
}

// Markdown renders the report as a short table plus the totals.
func (r *EvalReport) Markdown() string {
	var b strings.Builder
	b.WriteString("| Planted rule | Verdict | Keywords | Confidence | Evidence dropped |\n|---|---|---|---|---|\n")
	for _, e := range r.Rules {
		conf := "-"
		if e.Confidence != "" {
			conf = string(e.Confidence)
			if e.Overconfident {
				conf += " (overconfident)"
			}
		}
		fmt.Fprintf(&b, "| `%s` | %s | %d/%d | %s | %d |\n", e.Target, e.Verdict, e.Covered, e.Groups, conf, e.EvidenceDropped)
	}
	b.WriteString("\n| Technical method (no business rule) | Model said | False positive |\n|---|---|---|\n")
	for _, e := range r.Technical {
		fmt.Fprintf(&b, "| `%s` | %s | %v |\n", e.Target, e.Status, e.FalsePositive)
	}
	fmt.Fprintf(&b, "\nPlanted rules: %d complete, %d partial, %d wrong, %d missed (of %d). "+
		"False positives: %d of %d. Overconfident: %d. Methods with no usable answer: %d. Evidence lines dropped: %d.\n",
		r.Complete, r.Partial, r.Wrong, r.Missed, len(r.Rules),
		r.FalsePositives, len(r.Technical), r.Overconfident, r.Failed, r.EvidenceDropped)
	if len(r.Unlisted) > 0 {
		fmt.Fprintf(&b, "Analysed but not in the key: %s.\n", strings.Join(r.Unlisted, ", "))
	}
	if len(r.PlumbingSent) > 0 {
		fmt.Fprintf(&b, "BUG: plumbing methods reached the model: %s.\n", strings.Join(r.PlumbingSent, ", "))
	}
	return b.String()
}
