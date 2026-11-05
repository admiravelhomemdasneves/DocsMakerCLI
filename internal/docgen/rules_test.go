package docgen

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/cache"
	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/langmodel"
	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/llm"
	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/secrets"
)

// rulesClient records whole requests (the shared fakes keep only prompts) and
// answers through reply.
type rulesClient struct {
	reqs  []llm.Request
	reply func(req llm.Request) string
}

func (c *rulesClient) Generate(_ context.Context, req llm.Request) (llm.Response, error) {
	c.reqs = append(c.reqs, req)
	return llm.Response{Text: c.reply(req)}, nil
}
func (c *rulesClient) Available(context.Context) bool { return true }

const (
	discountEvidence = "if (qty >= 100) {"
	goodDiscount     = `{"hasRule": true, "statement": "Orders of 100 units or more get 15% off.",
		"conditions": ["qty >= 100"], "rationale": "Probably a bulk-purchase incentive.",
		"evidence": ["if (qty >= 100) {"], "confidence": "high", "openQuestions": []}`
	noRule = `{"hasRule": false}`
)

// buildRulesModel writes a class with three methods and returns a hand-built
// CodeModel with Signals set as the Java analyzer would: discount and check
// are logic (candidates), getName is plumbing. check also holds a synthetic
// credential, for the gate tests.
func buildRulesModel(t *testing.T) *langmodel.CodeModel {
	t.Helper()
	src := "package com.example;\n\npublic class Pricing {\n" +
		"    private static final double CAP = 500.0;\n\n" +
		"    public double discount(int qty) {\n        if (qty >= 100) {\n            return 0.15;\n        }\n        return 0.0;\n    }\n\n" +
		"    public String getName() {\n        return name;\n    }\n\n" +
		"    public void check(int v) {\n        String u = \"" + secretURL + "\";\n        if (v > CAP) {\n            throw new IllegalStateException(u);\n        }\n    }\n}\n"
	path := filepath.Join(t.TempDir(), "Pricing.java")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	span := func(startMarker string, line int) langmodel.Range {
		s := strings.Index(src, startMarker)
		e := s + strings.Index(src[s:], "\n    }\n") + len("\n    }")
		return langmodel.Range{File: path, StartByte: s, EndByte: e, StartLine: line}
	}
	capStart := strings.Index(src, "private static final")
	capEnd := capStart + strings.Index(src[capStart:], ";") + 1

	decl := langmodel.TypeDecl{
		Kind: langmodel.KindClass, Name: "Pricing", Package: "com.example",
		Fields: []langmodel.Field{{
			Name: "CAP", Type: langmodel.TypeRef{Name: "double"},
			SourceRange: langmodel.Range{File: path, StartByte: capStart, EndByte: capEnd, StartLine: 4},
		}},
		Methods: []langmodel.Method{
			{Name: "discount", SourceRange: span("public double discount", 6),
				Signals: &langmodel.Signals{Statements: 3, Branches: 1, Comparisons: 1, Literals: 2}},
			{Name: "getName", SourceRange: span("public String getName", 12),
				Signals: &langmodel.Signals{Statements: 1}},
			{Name: "check", SourceRange: span("public void check", 16),
				Signals: &langmodel.Signals{Statements: 3, Branches: 1, Throws: 1, Comparisons: 1, Literals: 1}},
		},
	}
	return &langmodel.CodeModel{Language: "java",
		Packages: []langmodel.Package{{Name: "com.example", Types: []langmodel.TypeDecl{decl}}}}
}

// answerByMethod replies with a canned answer chosen by which method the
// prompt is about.
func answerByMethod(discount, check string) func(llm.Request) string {
	return func(req llm.Request) string {
		if strings.Contains(req.Prompt, "discount(") {
			return discount
		}
		return check
	}
}

func extract(t *testing.T, p *Pipeline, model *langmodel.CodeModel, opts RulesOptions) []RuleResult {
	t.Helper()
	opts.MinScore = 3
	res, err := p.ExtractRules(context.Background(), model, opts)
	if err != nil {
		t.Fatalf("ExtractRules: %v", err)
	}
	return res
}

func TestExtractRules_FindsRulesAndSkipsPlumbing(t *testing.T) {
	client := &rulesClient{reply: answerByMethod(goodDiscount, noRule)}
	p := New(client, nil, "m")

	res := extract(t, p, buildRulesModel(t), RulesOptions{Lang: "en"})

	if len(res) != 2 || res[0].Target != "com.example.Pricing#discount" || res[1].Target != "com.example.Pricing#check" {
		t.Fatalf("results = %+v, want discount then check (getName is plumbing)", res)
	}
	if len(client.reqs) != 2 {
		t.Errorf("model called %d times, want 2 (one per candidate)", len(client.reqs))
	}
	for _, req := range client.reqs {
		if req.Format != "json" {
			t.Errorf("request Format = %q, want json", req.Format)
		}
	}
	if !strings.Contains(client.reqs[0].Prompt, "private static final double CAP = 500.0;") {
		t.Errorf("field declarations missing from the prompt:\n%s", client.reqs[0].Prompt)
	}
	if !strings.Contains(client.reqs[0].System, "English") {
		t.Error("the requested language did not reach the system prompt")
	}

	d := res[0]
	if d.Status != RuleFound || d.Rule == nil {
		t.Fatalf("discount: %+v, want a found rule", d)
	}
	if d.Rule.ID != "com.example.Pricing#discount@L6" || d.Rule.Target != d.Target || d.Rule.Range != d.Range {
		t.Errorf("rule location fields wrong: %+v", d.Rule)
	}
	if d.Rule.Confidence != "high" || len(d.Rule.Evidence) != 1 || d.EvidenceDropped != 0 {
		t.Errorf("verified evidence should keep high confidence: %+v dropped=%d", d.Rule, d.EvidenceDropped)
	}
	if res[1].Status != RuleNone || res[1].Rule != nil {
		t.Errorf("check: %+v, want status none", res[1])
	}
}

func TestExtractRules_InventedEvidenceIsDroppedAndConfidenceCapped(t *testing.T) {
	invented := `{"hasRule": true, "statement": "Big orders get a discount.", "evidence": ["if (qty > 9999) {"], "confidence": "high"}`
	p := New(&rulesClient{reply: answerByMethod(invented, noRule)}, nil, "m")

	d := extract(t, p, buildRulesModel(t), RulesOptions{})[0]
	if d.Status != RuleFound || d.EvidenceDropped != 1 || len(d.Rule.Evidence) != 0 || d.Rule.Confidence != "low" {
		t.Errorf("got %+v dropped=%d, want a low-confidence rule with its fake evidence removed", d.Rule, d.EvidenceDropped)
	}
}

func TestExtractRules_RetriesOnceWithTheProblemShown(t *testing.T) {
	client := &rulesClient{reply: func(req llm.Request) string {
		if strings.Contains(req.Prompt, "could not be used") {
			return goodDiscount
		}
		return "Sure, the method gives a discount for big orders."
	}}
	p := New(client, nil, "m")

	res := extract(t, p, buildRulesModel(t), RulesOptions{Scope: "com.example.Pricing#discount"})

	if len(client.reqs) != 2 {
		t.Fatalf("model called %d times, want 2 (answer + retry)", len(client.reqs))
	}
	retry := client.reqs[1].Prompt
	if !strings.Contains(retry, "no JSON object") || !strings.Contains(retry, "Sure, the method gives") {
		t.Errorf("retry prompt should show the problem and the bad answer:\n%s", retry)
	}
	if len(res) != 1 || res[0].Status != RuleFound {
		t.Errorf("results = %+v, want the rule recovered by the retry", res)
	}
}

func TestExtractRules_UnusableTwiceFailsOnlyThatMethod(t *testing.T) {
	client := &rulesClient{reply: func(req llm.Request) string {
		if strings.Contains(req.Prompt, "discount(") {
			return "I cannot answer in JSON."
		}
		return noRule
	}}
	p := New(client, nil, "m")

	res := extract(t, p, buildRulesModel(t), RulesOptions{})
	if len(res) != 2 {
		t.Fatalf("results = %+v, want both candidates reported", res)
	}
	if res[0].Status != RuleFailed || !strings.Contains(res[0].Reason, "after retry") {
		t.Errorf("discount: %+v, want failed after retry", res[0])
	}
	if res[1].Status != RuleNone {
		t.Errorf("check: %+v: one bad method must not stop the run", res[1])
	}
}

func TestExtractRules_UnreadableSourceFailsThatMethod(t *testing.T) {
	model := buildRulesModel(t)
	model.Packages[0].Types[0].Methods[0].SourceRange.File = filepath.Join(t.TempDir(), "gone.java")
	client := &rulesClient{reply: answerByMethod(goodDiscount, noRule)}

	res := extract(t, New(client, nil, "m"), model, RulesOptions{})
	if res[0].Status != RuleFailed || !strings.Contains(res[0].Reason, "source unavailable") {
		t.Errorf("discount: %+v, want failed (source unavailable)", res[0])
	}
	if len(client.reqs) != 1 {
		t.Errorf("model called %d times, want 1 (only for check)", len(client.reqs))
	}
}

func TestExtractRules_ScopeMaxMethodsAndProgress(t *testing.T) {
	client := &rulesClient{reply: answerByMethod(goodDiscount, noRule)}
	p := New(client, nil, "m")

	var seen []string
	opts := RulesOptions{Progress: func(n, total int, target string) {
		seen = append(seen, strings.Join([]string{string(rune('0' + n)), string(rune('0' + total)), target}, " "))
	}}
	extract(t, p, buildRulesModel(t), opts)
	want := []string{"1 2 com.example.Pricing#discount", "2 2 com.example.Pricing#check"}
	if strings.Join(seen, "|") != strings.Join(want, "|") {
		t.Errorf("progress = %v, want %v", seen, want)
	}

	res := extract(t, p, buildRulesModel(t), RulesOptions{MaxMethods: 1})
	if len(res) != 1 || res[0].Target != "com.example.Pricing#check" { // score 6 beats discount's 5
		t.Errorf("MaxMethods=1 kept %+v, want the highest-scoring method only", res)
	}
}

func TestExtractRules_CacheServesRepeatsAndNeverStoresBadAnswers(t *testing.T) {
	store := cache.NewFileStore(t.TempDir())
	model := buildRulesModel(t)
	scope := RulesOptions{Scope: "com.example.Pricing#discount"}

	// A good answer is cached: the second run does not call the model.
	good := &rulesClient{reply: answerByMethod(goodDiscount, noRule)}
	extract(t, New(good, store, "m"), model, scope)
	extract(t, New(good, store, "m"), model, scope)
	if len(good.reqs) != 1 {
		t.Errorf("model called %d times over two identical runs, want 1", len(good.reqs))
	}

	// The first attempt is unusable, the retry is fine. Only the retry may be
	// cached: the bad first answer must be asked for again on every run.
	flaky := &rulesClient{reply: func(req llm.Request) string {
		if strings.Contains(req.Prompt, "could not be used") {
			return goodDiscount
		}
		return "not json"
	}}
	store2 := cache.NewFileStore(t.TempDir())
	for run := 1; run <= 2; run++ {
		res := extract(t, New(flaky, store2, "m"), model, scope)
		if res[0].Status != RuleFound {
			t.Fatalf("run %d: %+v, want the retry's rule", run, res[0])
		}
	}
	if len(flaky.reqs) != 3 {
		t.Errorf("model called %d times over two runs, want 3 (first attempt twice, retry once)", len(flaky.reqs))
	}
}

func TestExtractRules_ModelErrorStopsTheRunKeepingProgress(t *testing.T) {
	calls := 0
	failing := &funcClient{fn: func(llm.Request) (llm.Response, error) {
		calls++
		if calls == 2 {
			return llm.Response{}, errors.New("ollama went away")
		}
		return llm.Response{Text: goodDiscount}, nil
	}}
	res, err := New(failing, nil, "m").ExtractRules(context.Background(), buildRulesModel(t), RulesOptions{MinScore: 3})
	if err == nil || !strings.Contains(err.Error(), "ollama went away") {
		t.Fatalf("err = %v, want the model error", err)
	}
	if len(res) != 1 || res[0].Status != RuleFound {
		t.Errorf("results = %+v, want the one method finished before the failure", res)
	}
}

func TestExtractRules_StopsWhenContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := &rulesClient{reply: answerByMethod(goodDiscount, noRule)}
	_, err := New(client, nil, "m").ExtractRules(ctx, buildRulesModel(t), RulesOptions{MinScore: 3})
	if !errors.Is(err, context.Canceled) || len(client.reqs) != 0 {
		t.Errorf("err = %v, calls = %d; want context.Canceled and no model calls", err, len(client.reqs))
	}
}

type funcClient struct {
	fn func(llm.Request) (llm.Response, error)
}

func (c *funcClient) Generate(_ context.Context, req llm.Request) (llm.Response, error) {
	return c.fn(req)
}
func (c *funcClient) Available(context.Context) bool { return true }

// --- secrets gates still apply to rule extraction -------------------------

var checkScope = RulesOptions{Scope: "com.example.Pricing#check"}

func TestExtractRules_GateRedactKeepsSecretsFromThePrompt(t *testing.T) {
	client := &rulesClient{reply: answerByMethod(noRule, noRule)}
	p := New(client, nil, "m")
	p.Gate = gateWith(fixedApprover{secrets.DecisionRedact})

	res := extract(t, p, buildRulesModel(t), checkScope)
	if len(client.reqs) != 1 || strings.Contains(client.reqs[0].Prompt, "hunter2") {
		t.Fatalf("prompt must reach the model exactly once and without the secret: %+v", client.reqs)
	}
	if !strings.Contains(client.reqs[0].Prompt, "[REDACTED:connection-string]") {
		t.Errorf("prompt should show the redaction placeholder:\n%s", client.reqs[0].Prompt)
	}
	if res[0].Status != RuleNone {
		t.Errorf("result = %+v", res[0])
	}
}

func TestExtractRules_GateSkipOmitsWithoutCallingTheModel(t *testing.T) {
	client := &rulesClient{reply: answerByMethod(noRule, noRule)}
	p := New(client, nil, "m")
	p.Gate = gateWith(fixedApprover{secrets.DecisionSkip})

	res := extract(t, p, buildRulesModel(t), checkScope)
	if len(client.reqs) != 0 {
		t.Errorf("model called %d times for a skipped method, want 0", len(client.reqs))
	}
	if res[0].Status != RuleOmitted || res[0].Reason == "" {
		t.Errorf("result = %+v, want omitted with a reason", res[0])
	}
}

func TestExtractRules_GateBlockStopsBeforeTheModel(t *testing.T) {
	client := &rulesClient{reply: answerByMethod(noRule, noRule)}
	p := New(client, nil, "m")
	p.Gate = gateWith(secrets.BlockApprover{})

	_, err := p.ExtractRules(context.Background(), buildRulesModel(t), RulesOptions{Scope: checkScope.Scope, MinScore: 3})
	var blocked *secrets.BlockedError
	if !errors.As(err, &blocked) || len(client.reqs) != 0 {
		t.Errorf("err = %v, calls = %d; want *secrets.BlockedError and no model calls", err, len(client.reqs))
	}
}

func TestExtractRules_OutputGateCatchesASecretInTheAnswer(t *testing.T) {
	leaky := `{"hasRule": true, "statement": "It connects with ` + secretURL + ` always.", "confidence": "low"}`
	model := buildRulesModel(t)
	// Discount's prompt is clean, so only the output gate can catch this.
	p := New(&rulesClient{reply: answerByMethod(leaky, noRule)}, nil, "m")
	p.Gate = gateWith(fixedApprover{secrets.DecisionRedact})

	d := extract(t, p, model, RulesOptions{Scope: "com.example.Pricing#discount"})[0]
	if d.Status != RuleFound || strings.Contains(d.Rule.Statement, "hunter2") {
		t.Errorf("the redacted rule still holds the secret: %+v", d.Rule)
	}

	p.Gate = gateWith(secrets.BlockApprover{})
	if _, err := p.ExtractRules(context.Background(), model, RulesOptions{Scope: "com.example.Pricing#discount", MinScore: 3}); err == nil {
		t.Error("blocking mode must fail when the answer contains a secret")
	}
}

// staleStore claims to hold a malformed answer for every key and records
// what is written, to pin both halves of the "never keep a bad answer" rule.
type staleStore struct{ sets map[string]string }

func (s *staleStore) Get(string) (string, bool) { return "stale garbage", true }
func (s *staleStore) Set(k, v string) error     { s.sets[k] = v; return nil }

func TestExtractRules_BadCachedAnswersAreIgnoredAndNeverWritten(t *testing.T) {
	store := &staleStore{sets: map[string]string{}}
	client := &rulesClient{reply: func(req llm.Request) string {
		if strings.Contains(req.Prompt, "could not be used") {
			return goodDiscount
		}
		return "still not json"
	}}

	res := extract(t, New(client, store, "m"), buildRulesModel(t), RulesOptions{Scope: "com.example.Pricing#discount"})

	if len(client.reqs) == 0 || res[0].Status != RuleFound {
		t.Fatalf("a malformed cache entry must be ignored: calls=%d result=%+v", len(client.reqs), res[0])
	}
	for k, v := range store.sets {
		if strings.Contains(v, "not json") || strings.Contains(v, "garbage") {
			t.Errorf("malformed answer written to the cache under %.8s: %q", k, v)
		}
	}
	if len(store.sets) != 1 {
		t.Errorf("cache writes = %d, want only the good retry answer", len(store.sets))
	}
}
