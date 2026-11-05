package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// billingFake answers each billing method differently, covering every status
// the report can show: a verified rule, a rule with invented evidence, "no
// rule", and an answer that is never JSON.
func billingFake(t *testing.T) *fakeOllama {
	t.Helper()
	fake := startFakeOllama(t, "qwen2.5-coder:7b")
	fake.replyFn = func(prompt string) string {
		switch {
		case strings.Contains(prompt, "discountFor("):
			return `{"hasRule": true, "statement": "Orders under 20 get no discount; 100+ units get 15%.",
				"conditions": ["orderTotal < 20", "quantity >= 100"], "rationale": "Probably a bulk incentive.",
				"evidence": ["if (orderTotal < 20.0) {", "if (quantity >= 100) {"], "confidence": "high",
				"openQuestions": ["Is 20 before tax?"]}`
		case strings.Contains(prompt, "canExtendCredit("):
			return `{"hasRule": true, "statement": "Credit is refused after 90 days overdue.",
				"evidence": ["if (c.getDaysOverdue() > 999) {"], "confidence": "high"}`
		case strings.Contains(prompt, "canReturn("):
			return `{"hasRule": true, "statement": "Perishables are never returned.",
				"evidence": ["if (\"PERISHABLE\".equals(category)) {"], "confidence": "low"}`
		case strings.Contains(prompt, "vatRate("):
			return `{"hasRule": false}`
		default: // isValidNif
			return "I am sorry, I can only chat."
		}
	}
	return fake
}

type rulesJSON struct {
	Project string
	Summary struct{ Found, None, Omitted, Failed, Classes int }
	Hidden  int `json:"hiddenByConfidence"`
	Results []struct {
		Target, Status, Reason string
		Rule                   *struct {
			Statement, Confidence string
			Evidence              []string
		}
	}
}

func TestRules_ConsoleReport(t *testing.T) {
	fake := billingFake(t)
	dir := sampleProject(t)

	out, errOut, err := run(t, "", "rules", dir, "--target", "com.example.billing", "--output-lang", "en")
	if err != nil {
		t.Fatalf("rules: %v\n%s", err, errOut)
	}

	for _, want := range []string{
		"# Business rules",
		"3 rule(s) in 3 class(es); 1 method(s) without a rule, 0 omitted, 1 failed.",
		"## com.example.billing.DiscountPolicy",
		"**Rule:** Orders under 20 get no discount; 100+ units get 15%.",
		"discountFor (line 4) — Confidence: high",
		"**Probable reason (inferred):** Probably a bulk incentive.",
		"if (quantity >= 100) {",
		"- Is 20 before tax?",
		"## Methods without a result",
		"`com.example.billing.TaxIdValidator#isValidNif`",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report lacks %q\n%s", want, out)
		}
	}
	// The model quoted a line that is not in the code: it must be gone, the
	// reader told, and the confidence no longer "high".
	if strings.Contains(out, "999") {
		t.Error("invented evidence reached the report")
	}
	if !strings.Contains(out, "canExtendCredit (line 6) — Confidence: low") {
		t.Errorf("a rule without verified evidence must read as low confidence:\n%s", out)
	}

	if !strings.Contains(errOut, "[1/5]") || !strings.Contains(errOut, "[5/5]") {
		t.Errorf("progress should go to stderr:\n%s", errOut)
	}
	if strings.Contains(out, "[1/5]") {
		t.Error("progress leaked into stdout")
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	for _, f := range fake.formats {
		if f != "json" {
			t.Fatalf("a rules request went out with format %q, want json", f)
		}
	}
	if strings.Contains(strings.Join(fake.prompts, ""), "Signature: double getBalance") {
		t.Error("plumbing methods (getters) must not be sent to the model")
	}
}

func TestRules_JSONAndMinConfidence(t *testing.T) {
	billingFake(t)
	dir := sampleProject(t)
	base := []string{"rules", dir, "--target", "com.example.billing", "--format", "json"}

	out, _, err := run(t, "", base...)
	var all rulesJSON
	if err != nil || json.Unmarshal([]byte(out), &all) != nil {
		t.Fatalf("not a JSON document: %q %v", out, err)
	}
	if len(all.Results) != 5 || all.Summary.Found != 3 || all.Summary.None != 1 || all.Summary.Failed != 1 {
		t.Errorf("results=%d summary=%+v, want 5 results: 3 found, 1 none, 1 failed", len(all.Results), all.Summary)
	}
	for _, r := range all.Results {
		if r.Status == "failed" && !strings.Contains(r.Reason, "after retry") {
			t.Errorf("failed result has reason %q", r.Reason)
		}
	}

	out, _, err = run(t, "", append(base, "--min-confidence", "medium")...)
	var filtered rulesJSON
	if err != nil || json.Unmarshal([]byte(out), &filtered) != nil {
		t.Fatal(err, out)
	}
	// canExtendCredit (invented evidence) and canReturn (model said low) are both low.
	if filtered.Hidden != 2 || filtered.Summary.Found != 1 {
		t.Errorf("hidden=%d found=%d, want 2 hidden and only the high-confidence rule left", filtered.Hidden, filtered.Summary.Found)
	}

	// The text report says on stderr that rules were left out.
	textArgs := []string{"rules", dir, "--target", "com.example.billing", "--min-confidence", "medium"}
	_, errOut, err := run(t, "", textArgs...)
	if err != nil || !strings.Contains(errOut, "2 rule(s) hidden by --min-confidence") {
		t.Errorf("stderr should say rules were hidden: %q %v", errOut, err)
	}
}

func TestRules_SidecarWritesReportsAndLeavesSourcesAlone(t *testing.T) {
	billingFake(t)
	dir := sampleProject(t)
	src := filepath.Join(dir, "src/main/java/com/example/billing/DiscountPolicy.java")
	before, _ := os.ReadFile(src)

	if _, _, err := run(t, "", "rules", dir, "--target", "com.example.billing", "--mode", "sidecar"); err != nil {
		t.Fatal(err)
	}

	rulesDir := filepath.Join(dir, ".docsmaker", "rules")
	catalog, err := os.ReadFile(filepath.Join(rulesDir, "RULES.md"))
	if err != nil || !strings.Contains(string(catalog), "# Regras de negócio") {
		t.Fatalf("RULES.md missing or not in the default language (pt): %v\n%s", err, catalog)
	}
	class, err := os.ReadFile(filepath.Join(rulesDir, "com.example.billing.DiscountPolicy.md"))
	if err != nil || !strings.Contains(string(class), "Orders under 20") {
		t.Fatalf("per-class report missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rulesDir, "com.example.billing.TaxCalculator.md")); err == nil {
		t.Error("a class without rules must not get a file")
	}
	after, _ := os.ReadFile(src)
	if !bytes.Equal(before, after) {
		t.Fatal("rules must never touch the sources")
	}
}

func TestRules_SecondRunIsServedFromTheCache(t *testing.T) {
	fake := billingFake(t)
	dir := sampleProject(t)
	args := []string{"rules", dir, "--target", "com.example.billing"}

	if _, _, err := run(t, "", args...); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	first := len(fake.prompts)
	fake.mu.Unlock()
	if first != 6 { // 5 methods, plus the retry for the one that never answers in JSON
		t.Fatalf("first run made %d model calls, want 6", first)
	}

	if _, _, err := run(t, "", args...); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	second := len(fake.prompts) - first
	fake.mu.Unlock()
	// Good answers come from the cache; the bad one was never cached, so it is
	// asked once more (its retry prompt is the same, so that one is cached).
	if second != 2 {
		t.Errorf("second run made %d model calls, want 2", second)
	}
}

func TestRules_InputValidation(t *testing.T) {
	fake := billingFake(t)
	dir := sampleProject(t)

	for name, args := range map[string][]string{
		"unknown format":         {"rules", dir, "--format", "xml"},
		"unknown mode":           {"rules", dir, "--mode", "banana"},
		"inline is not offered":  {"rules", dir, "--mode", "inline"},
		"unknown min-confidence": {"rules", dir, "--min-confidence", "certain"},
		"unknown secrets policy": {"rules", dir, "--secrets", "maybe"},
		"target matches nothing": {"rules", dir, "--target", "no.such.Type#thing"},
		"folder without sources": {"rules", t.TempDir()},
	} {
		if _, _, err := run(t, "", args...); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	if n := len(fake.prompts); n != 0 {
		t.Errorf("the model was called %d times by invalid invocations, want 0", n)
	}
}

func TestRules_NothingToAnalyse(t *testing.T) {
	fake := billingFake(t)
	dir := sampleProject(t)

	// Greeter is a real class, but none of its methods hold decisions.
	out, errOut, err := run(t, "", "rules", dir, "--target", "com.example.greeting")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Nenhuma regra de negócio encontrada.") || !strings.Contains(errOut, "no candidate methods") {
		t.Errorf("stdout=%q stderr=%q: want the empty report and a hint to lower --min-score", out, errOut)
	}
	if len(fake.prompts) != 0 {
		t.Errorf("the model was called %d times with nothing to analyse", len(fake.prompts))
	}
}

// secretProject is a one-class project whose only decision method sits next
// to a field holding a (synthetic) credential, which the prompt includes.
func secretProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "src", "main", "java", "com", "acme")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	code := "package com.acme;\n\npublic class Gate {\n" +
		"    private final String url = \"jdbc:mysql://admin:hunter2@localhost:3306/db\";\n\n" +
		"    public boolean allowed(int age) {\n        if (age >= 18) {\n            return true;\n        }\n" +
		"        if (age < 0) {\n            throw new IllegalArgumentException(\"age\");\n        }\n        return false;\n    }\n}\n"
	if err := os.WriteFile(filepath.Join(src, "Gate.java"), []byte(code), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestRules_Secrets(t *testing.T) {
	fake := startFakeOllama(t, "qwen2.5-coder:7b")
	fake.reply = `{"hasRule": true, "statement": "Adults only.", "evidence": ["if (age >= 18) {"], "confidence": "high"}`
	dir := secretProject(t)

	t.Run("--yes blocks and the model is never called", func(t *testing.T) {
		_, _, err := run(t, "", "rules", dir, "--yes")
		if err == nil || strings.Contains(err.Error(), "hunter2") {
			t.Fatalf("want a blocking error without the secret, got %v", err)
		}
		if len(fake.prompts) != 0 {
			t.Fatalf("the model was called %d times", len(fake.prompts))
		}
	})

	t.Run("redact: the model sees a placeholder, never the secret", func(t *testing.T) {
		out, _, err := run(t, "r\n", "rules", dir)
		if err != nil || !strings.Contains(out, "Adults only.") {
			t.Fatalf("redacted run failed: %q %v", out, err)
		}
		if strings.Contains(fake.allPrompts(), "hunter2") || !strings.Contains(fake.allPrompts(), "[REDACTED:") {
			t.Fatalf("prompt should carry a placeholder instead of the secret:\n%s", fake.allPrompts())
		}
	})

	t.Run("skip: the method is reported as omitted", func(t *testing.T) {
		before := len(fake.prompts)
		out, _, err := run(t, "s\n", "rules", dir, "--model", "qwen2.5-coder:7b")
		if err != nil || !strings.Contains(out, "omitido") {
			t.Fatalf("skip run: %q %v", out, err)
		}
		if len(fake.prompts) != before {
			t.Error("a skipped method must not reach the model")
		}
	})
}
