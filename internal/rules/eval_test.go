package rules

import (
	"strings"
	"testing"
)

func testKey() *Gabarito {
	return &Gabarito{
		Rules: []PlantedRule{
			{Target: "p.A#discount", Keywords: [][]string{{"10", "50"}, {"15"}}},
			{Target: "p.A#credit", Keywords: [][]string{{"90"}, {"500"}}},
			{Target: "p.A#vat", Keywords: [][]string{{"23"}}},
			{Target: "p.A#nif", Keywords: [][]string{{"9"}}},
			{Target: "p.A#returns", Keywords: [][]string{{"14"}}},
		},
		Technical: []string{"p.U#pad", "p.U#cut"},
		NoRule:    []string{"p.A#getX"},
	}
}

func rule(statement string, conf Confidence, conds ...string) *Rule {
	return &Rule{Statement: statement, Conditions: conds, Confidence: conf}
}

func TestEvaluateGradesEachOutcome(t *testing.T) {
	rep := Evaluate(testKey(), []Outcome{
		{Target: "p.A#discount", Status: "rule", Rule: rule("Orders of 50 get 15% off.", ConfidenceHigh)},                // complete
		{Target: "p.A#credit", Status: "rule", Rule: rule("Refused after 90 days.", ConfidenceHigh), EvidenceDropped: 1}, // partial + overconfident
		{Target: "p.A#vat", Status: "rule", Rule: rule("A tax applies.", ConfidenceMedium)},                              // wrong
		{Target: "p.A#nif", Status: "none"}, // missed
		// p.A#returns is never analysed                                                                         // missed
		{Target: "p.U#pad", Status: "rule", Rule: rule("Pads text.", ConfidenceLow)}, // false positive
		{Target: "p.U#cut", Status: "none"},
		{Target: "p.Z#extra", Status: "failed"},
	})

	if rep.Complete != 1 || rep.Partial != 1 || rep.Wrong != 1 || rep.Missed != 2 {
		t.Errorf("verdicts = %d complete %d partial %d wrong %d missed, want 1/1/1/2", rep.Complete, rep.Partial, rep.Wrong, rep.Missed)
	}
	if rep.Overconfident != 1 || !rep.Rules[1].Overconfident {
		t.Errorf("only the partial rule rated high is overconfident: %+v", rep.Rules[1])
	}
	if rep.Rules[0].Overconfident {
		t.Error("a complete rule rated high is not overconfident")
	}
	if rep.FalsePositives != 1 || !rep.Technical[0].FalsePositive || rep.Technical[1].FalsePositive {
		t.Errorf("technical = %+v, want only padLeft flagged", rep.Technical)
	}
	if rep.Rules[4].Status != "not analysed" {
		t.Errorf("a planted rule the run never reached should read 'not analysed', got %q", rep.Rules[4].Status)
	}
	if rep.Failed != 1 || rep.EvidenceDropped != 1 {
		t.Errorf("failed=%d evidenceDropped=%d, want 1 and 1", rep.Failed, rep.EvidenceDropped)
	}
	if len(rep.Unlisted) != 1 || rep.Unlisted[0] != "p.Z#extra" {
		t.Errorf("Unlisted = %v", rep.Unlisted)
	}
}

func TestEvaluateMatchesStatementAndConditionsCaseInsensitively(t *testing.T) {
	g := &Gabarito{Rules: []PlantedRule{{Target: "t", Keywords: [][]string{{"madeira"}, {"22"}}}}}
	rep := Evaluate(g, []Outcome{{Target: "t", Status: "rule",
		Rule: rule("VAT depends on the region.", ConfidenceLow, "MADEIRA pays 22%")}})
	if rep.Complete != 1 {
		t.Errorf("keywords in the conditions should count, case-insensitively: %+v", rep.Rules[0])
	}

	// The "why" must not help a wrong statement look right.
	r := rule("A tax applies.", ConfidenceLow)
	r.Rationale = "Probably madeira 22"
	rep = Evaluate(g, []Outcome{{Target: "t", Status: "rule", Rule: r}})
	if rep.Wrong != 1 {
		t.Errorf("the rationale must not be graded as the rule: %+v", rep.Rules[0])
	}
}

func TestEvaluateFlagsPlumbingThatReachedTheModel(t *testing.T) {
	rep := Evaluate(testKey(), []Outcome{{Target: "p.A#getX", Status: "none"}})
	if len(rep.PlumbingSent) != 1 || !strings.Contains(rep.Markdown(), "BUG: plumbing") {
		t.Errorf("a getter reaching the model must be reported: %+v", rep.PlumbingSent)
	}
}

func TestEvaluateMarkdown(t *testing.T) {
	md := Evaluate(testKey(), []Outcome{
		{Target: "p.A#discount", Status: "rule", Rule: rule("50 units: 15%", ConfidenceHigh)},
	}).Markdown()
	for _, want := range []string{"`p.A#discount` | complete | 2/2 | high", "`p.A#nif` | missed | 0/1", "`p.U#pad` | not analysed | false", "1 complete, 0 partial, 0 wrong, 4 missed (of 5)"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown lacks %q:\n%s", want, md)
		}
	}
}

func TestLoadGabaritoReadsTheFixtureKey(t *testing.T) {
	g, err := LoadGabarito("../../testdata/billing-expected-rules.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Rules) != 5 || len(g.Technical) != 2 || len(g.NoRule) != 5 {
		t.Errorf("key = %d rules, %d technical, %d noRule; want 5, 2, 5", len(g.Rules), len(g.Technical), len(g.NoRule))
	}
	if _, err := LoadGabarito("nope.json"); err == nil {
		t.Error("a missing file must be an error")
	}
}
