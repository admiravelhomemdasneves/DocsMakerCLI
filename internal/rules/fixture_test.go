package rules_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/lang/java"
	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/rules"
)

// TestSelectCandidatesOnBillingFixture runs the real Java analyzer over the
// planted-rules fixture and checks the selector against the answer key: every
// planted rule must be a candidate and no accessor may be.
func TestSelectCandidatesOnBillingFixture(t *testing.T) {
	root := filepath.Join("..", "..", "testdata")
	billing := filepath.Join(root, "sample-project", "src", "main", "java", "com", "example", "billing")
	entries, err := os.ReadDir(billing)
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, e := range entries {
		files = append(files, filepath.Join(billing, e.Name()))
	}

	model, _, err := (&java.Analyzer{}).ParseProject(context.Background(), files)
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]int{}
	for _, c := range rules.SelectCandidates(model, rules.SelectOptions{MinScore: rules.DefaultMinScore}) {
		got[c.Target()] = c.Score
	}

	data, err := os.ReadFile(filepath.Join(root, "billing-expected-rules.json"))
	if err != nil {
		t.Fatal(err)
	}
	var exp struct {
		Rules  []struct{ Target string } `json:"rules"`
		NoRule []string                  `json:"noRule"`
	}
	if err := json.Unmarshal(data, &exp); err != nil {
		t.Fatal(err)
	}

	for _, r := range exp.Rules {
		if _, ok := got[r.Target]; !ok {
			t.Errorf("planted rule %s was not selected", r.Target)
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
	if len(names) != len(exp.Rules) {
		t.Errorf("selected %d methods %v, want exactly the %d planted rules", len(names), names, len(exp.Rules))
	}
}
