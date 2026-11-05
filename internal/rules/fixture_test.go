package rules_test

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/lang/java"
	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/rules"
)

// TestSelectCandidatesOnBillingFixture runs the real Java analyzer over the
// fixture and checks the selector against the answer key: every planted rule
// and every logic-heavy technical method must be a candidate (the model is the
// one to say there is no business rule in the latter), and no accessor may be.
func TestSelectCandidatesOnBillingFixture(t *testing.T) {
	root := filepath.Join("..", "..", "testdata")
	java_ := filepath.Join(root, "sample-project", "src", "main", "java", "com", "example")

	var files []string
	for _, pkg := range []string{"billing", "util"} {
		entries, err := os.ReadDir(filepath.Join(java_, pkg))
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			files = append(files, filepath.Join(java_, pkg, e.Name()))
		}
	}

	model, _, err := (&java.Analyzer{}).ParseProject(context.Background(), files)
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]int{}
	for _, c := range rules.SelectCandidates(model, rules.SelectOptions{MinScore: rules.DefaultMinScore}) {
		got[c.Target()] = c.Score
	}

	exp, err := rules.LoadGabarito(filepath.Join(root, "billing-expected-rules.json"))
	if err != nil {
		t.Fatal(err)
	}

	for _, r := range exp.Rules {
		if _, ok := got[r.Target]; !ok {
			t.Errorf("planted rule %s was not selected", r.Target)
		}
	}
	for _, target := range exp.Technical {
		if _, ok := got[target]; !ok {
			t.Errorf("technical method %s should be a candidate (it is the false-positive probe)", target)
		}
	}
	for _, target := range exp.NoRule {
		if score, ok := got[target]; ok {
			t.Errorf("accessor %s was selected (score %d)", target, score)
		}
	}

	var names []string
	for target := range got {
		names = append(names, target)
	}
	sort.Strings(names)
	if want := len(exp.Rules) + len(exp.Technical); len(names) != want {
		t.Errorf("selected %d methods %v, want exactly the %d in the key", len(names), names, want)
	}
}
