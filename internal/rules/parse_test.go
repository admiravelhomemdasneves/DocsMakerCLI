package rules

import (
	"reflect"
	"strings"
	"testing"

	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/langmodel"
)

const goodAnswer = `{"hasRule": true, "statement": "Orders under 20 get no discount.", "conditions": ["total < 20"],
 "rationale": "Probably avoids discounting tiny orders.", "evidence": ["if (orderTotal < 20.0) {"],
 "confidence": "high", "openQuestions": ["Is 20 before or after tax?"]}`

func TestParseWellFormedAnswer(t *testing.T) {
	e, err := Parse(goodAnswer)
	if err != nil {
		t.Fatal(err)
	}
	want := &Extraction{
		HasRule:       true,
		Statement:     "Orders under 20 get no discount.",
		Conditions:    []string{"total < 20"},
		Rationale:     "Probably avoids discounting tiny orders.",
		Evidence:      []string{"if (orderTotal < 20.0) {"},
		Confidence:    ConfidenceHigh,
		OpenQuestions: []string{"Is 20 before or after tax?"},
	}
	if !reflect.DeepEqual(e, want) {
		t.Errorf("got %+v\nwant %+v", e, want)
	}
}

func TestParseNoRule(t *testing.T) {
	for _, in := range []string{
		`{"hasRule": false}`, `{"hasRule": "false"}`, "NONE", "none.", "`None`", "  **NONE**\n",
		`{"hasRule": false, "statement": "ignored when the flag says no"}`,
		`{"statement": "", "conditions": []}`,
	} {
		e, err := Parse(in)
		if err != nil || e.HasRule {
			t.Errorf("Parse(%q) = %+v, %v; want a clean no-rule result", in, e, err)
		}
	}
}

func TestParseToleratesWrappingAndSloppiness(t *testing.T) {
	cases := map[string]string{
		"code fence":        "```json\n" + goodAnswer + "\n```",
		"prose":             "Sure! Here is the analysis:\n" + goodAnswer + "\nHope that helps.",
		"braces in strings": `{"hasRule": true, "statement": "Uses {x} and \"y}\" literally", "confidence": "low"}`,
	}
	for name, in := range cases {
		e, err := Parse(in)
		if err != nil || !e.HasRule {
			t.Errorf("%s: got %+v, %v", name, e, err)
		}
	}
}

func TestParseCoercesFieldShapes(t *testing.T) {
	e, err := Parse(`{"statement": "Limit is 500.", "conditions": "balance <= limit", "evidence": null,
		"confidence": "Média", "openQuestions": ["a", "", 7], "rationale": ["Probably", "risk."]}`)
	if err != nil {
		t.Fatal(err)
	}
	if !e.HasRule {
		t.Error("a statement without hasRule should count as a rule")
	}
	if !reflect.DeepEqual(e.Conditions, []string{"balance <= limit"}) {
		t.Errorf("Conditions = %q", e.Conditions)
	}
	if e.Evidence != nil {
		t.Errorf("Evidence = %q, want nil", e.Evidence)
	}
	if e.Confidence != ConfidenceMedium {
		t.Errorf("Confidence = %q, want medium", e.Confidence)
	}
	if !reflect.DeepEqual(e.OpenQuestions, []string{"a", "7"}) {
		t.Errorf("OpenQuestions = %q", e.OpenQuestions)
	}
	if e.Rationale != "Probably risk." {
		t.Errorf("Rationale = %q", e.Rationale)
	}
}

func TestParseErrorsAreRetryable(t *testing.T) {
	cases := map[string]string{
		"":                                       "empty",
		"I think this method validates a tax id": "no JSON object",
		`{"hasRule": true, "statement": "cut of`: "incomplete",
		`{"hasRule": true}`:                      "statement",
		`{"hasRule": true, "statement": ,}`:      "invalid",
	}
	for in, wantFragment := range cases {
		_, err := Parse(in)
		if err == nil || !strings.Contains(err.Error(), wantFragment) {
			t.Errorf("Parse(%q) error = %v, want one mentioning %q", in, err, wantFragment)
		}
	}
}

func TestExtractionRule(t *testing.T) {
	e, _ := Parse(goodAnswer)
	rng := langmodel.Range{File: "D.java", StartLine: 4}
	r := e.Rule("p.D#discountFor@L4", "p.D#discountFor", rng)
	if r == nil || r.ID != "p.D#discountFor@L4" || r.Range != rng || r.Confidence != ConfidenceHigh {
		t.Fatalf("Rule = %+v", r)
	}
	if err := r.Validate(); err != nil {
		t.Errorf("rule from a good answer should validate: %v", err)
	}
	if (&Extraction{}).Rule("i", "t", rng) != nil {
		t.Error("a no-rule extraction must yield a nil Rule")
	}
}

const discountSource = `public double discountFor(int quantity, double orderTotal) {
        if (orderTotal < 20.0) {
            return 0.0;
        }
    }`

func TestCheckEvidenceKeepsRealAndDropsInvented(t *testing.T) {
	r := &Rule{
		Confidence: ConfidenceHigh,
		Evidence: []string{
			"if (orderTotal < 20.0) {",      // verbatim
			"if   (orderTotal  <  20.0)  {", // whitespace differs only
			"if (orderTotal < 25.0) {",      // invented value
			"",                              // empty
		},
	}
	dropped := r.CheckEvidence(discountSource)
	if dropped != 2 {
		t.Errorf("dropped = %d, want 2", dropped)
	}
	if len(r.Evidence) != 2 {
		t.Errorf("Evidence = %q, want the two genuine lines", r.Evidence)
	}
	if r.Confidence != ConfidenceHigh {
		t.Errorf("Confidence = %q: verified evidence must not lower it", r.Confidence)
	}
}

func TestCheckEvidenceCapsConfidenceWithoutProof(t *testing.T) {
	for name, evidence := range map[string][]string{
		"no evidence":  nil,
		"all invented": {"return 42;"},
	} {
		r := &Rule{Confidence: ConfidenceHigh, Evidence: evidence}
		r.CheckEvidence(discountSource)
		if r.Confidence != ConfidenceLow {
			t.Errorf("%s: Confidence = %q, want low", name, r.Confidence)
		}
	}
}
