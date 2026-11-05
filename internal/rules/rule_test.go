package rules

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseConfidence(t *testing.T) {
	cases := map[string]Confidence{
		"low": ConfidenceLow, "LOW": ConfidenceLow, " baixa ": ConfidenceLow,
		"medium": ConfidenceMedium, "média": ConfidenceMedium, "Media": ConfidenceMedium,
		"high": ConfidenceHigh, "Alta": ConfidenceHigh,
	}
	for in, want := range cases {
		got, ok := ParseConfidence(in)
		if !ok || got != want {
			t.Errorf("ParseConfidence(%q) = %q, %v; want %q, true", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "certain", "5"} {
		if c, ok := ParseConfidence(in); ok {
			t.Errorf("ParseConfidence(%q) = %q, true; want not ok", in, c)
		}
	}
}

func TestConfidenceOrdering(t *testing.T) {
	if !ConfidenceHigh.AtLeast(ConfidenceMedium) || !ConfidenceMedium.AtLeast(ConfidenceMedium) {
		t.Error("high/medium should satisfy a medium minimum")
	}
	if ConfidenceLow.AtLeast(ConfidenceMedium) {
		t.Error("low must not satisfy a medium minimum")
	}
	if Confidence("").AtLeast(ConfidenceLow) {
		t.Error("an empty confidence must not satisfy any minimum")
	}
}

func TestRuleIDDistinguishesOverloads(t *testing.T) {
	a := RuleID("com.x", "Foo", "bar", 10)
	b := RuleID("com.x", "Foo", "bar", 25)
	if a == b {
		t.Fatalf("overloads of the same method share an ID: %s", a)
	}
	if a != "com.x.Foo#bar@L10" {
		t.Errorf("RuleID = %q, want com.x.Foo#bar@L10", a)
	}
	if got := Target("", "Foo", "bar"); got != "Foo#bar" {
		t.Errorf("Target without package = %q, want Foo#bar", got)
	}
}

func TestNormalize(t *testing.T) {
	r := &Rule{
		ID: "a", Target: "a", Statement: "  Discount of 10%.  ",
		Conditions:    []string{" qty >= 50 ", "", "  "},
		Evidence:      []string{""},
		OpenQuestions: nil,
		Confidence:    "Alta",
	}
	r.Normalize()
	if r.Statement != "Discount of 10%." {
		t.Errorf("Statement = %q", r.Statement)
	}
	if !reflect.DeepEqual(r.Conditions, []string{"qty >= 50"}) {
		t.Errorf("Conditions = %q", r.Conditions)
	}
	if r.Evidence != nil {
		t.Errorf("Evidence = %q, want nil", r.Evidence)
	}
	if r.Confidence != ConfidenceHigh {
		t.Errorf("Confidence = %q, want high", r.Confidence)
	}
}

func TestNormalizeUnknownConfidenceFallsBackToLow(t *testing.T) {
	for _, in := range []Confidence{"", "certain", "7"} {
		r := (&Rule{Confidence: in}).Normalize()
		if r.Confidence != ConfidenceLow {
			t.Errorf("Confidence %q normalized to %q, want low", in, r.Confidence)
		}
	}
}

func TestValidate(t *testing.T) {
	ok := Rule{ID: "i", Target: "t", Statement: "s"}
	if err := ok.Validate(); err != nil {
		t.Errorf("valid rule rejected: %v", err)
	}
	for name, r := range map[string]Rule{
		"no id":        {Target: "t", Statement: "s"},
		"no target":    {ID: "i", Statement: "s"},
		"no statement": {ID: "i", Target: "t"},
	} {
		if err := r.Validate(); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestRuleJSONContract(t *testing.T) {
	r := Rule{ID: "i", Target: "t", Statement: "s", Confidence: ConfidenceMedium}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	for _, want := range []string{`"id":"i"`, `"target":"t"`, `"statement":"s"`, `"confidence":"medium"`} {
		if !strings.Contains(got, want) {
			t.Errorf("JSON %s lacks %s", got, want)
		}
	}
	// Optional fields stay out of the output when empty.
	for _, absent := range []string{"conditions", "rationale", "evidence", "openQuestions"} {
		if strings.Contains(got, absent) {
			t.Errorf("JSON %s should omit empty %s", got, absent)
		}
	}
}

// expectedRules mirrors testdata/billing-expected-rules.json.
type expectedRules struct {
	Rules []struct {
		Target   string     `json:"target"`
		Summary  string     `json:"summary"`
		Keywords [][]string `json:"keywords"`
	} `json:"rules"`
	NoRule    []string `json:"noRule"`
	Technical []string `json:"technical"`
}

// TestBillingFixtureMatchesGabarito keeps the planted-rules answer key honest:
// every target it names must exist in the fixture sources, so a rename in
// either place fails here instead of silently skewing the quality evaluation.
func TestBillingFixtureMatchesGabarito(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "billing-expected-rules.json"))
	if err != nil {
		t.Fatal(err)
	}
	var exp expectedRules
	if err := json.Unmarshal(data, &exp); err != nil {
		t.Fatalf("gabarito is not valid JSON: %v", err)
	}
	if len(exp.Rules) < 5 || len(exp.NoRule) == 0 {
		t.Fatalf("gabarito too small: %d rules, %d noRule", len(exp.Rules), len(exp.NoRule))
	}

	dir := filepath.Join("..", "..", "testdata", "sample-project", "src", "main", "java")
	check := func(target string) {
		owner, method, ok := strings.Cut(target, "#")
		if !ok {
			t.Errorf("target %q has no #method", target)
			return
		}
		file := filepath.Join(dir, filepath.FromSlash(strings.ReplaceAll(owner, ".", "/"))+".java")
		src, err := os.ReadFile(file)
		if err != nil {
			t.Errorf("target %s: %v", target, err)
			return
		}
		if !strings.Contains(string(src), " "+method+"(") {
			t.Errorf("target %s: method %s not found in %s", target, method, file)
		}
	}
	for _, r := range exp.Rules {
		check(r.Target)
		if r.Summary == "" || len(r.Keywords) == 0 {
			t.Errorf("rule %s needs a summary and keywords", r.Target)
		}
	}
	for _, target := range append(exp.NoRule, exp.Technical...) {
		check(target)
	}
}
