package rules_test

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/app"
	_ "github.com/admiravelhomemdasneves/DocsMakerCLI/internal/lang/java"
	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/rules"
)

// TestEvaluateRealModel measures rule extraction against the answer key with
// a real local model. It is skipped unless DOCSMAKER_EVAL_MODEL is set, because
// it needs a running Ollama and takes minutes:
//
//	DOCSMAKER_EVAL_MODEL=qwen2.5-coder:1.5b go test ./internal/rules -run TestEvaluateRealModel -v -timeout 30m
//
// Optional: DOCSMAKER_EVAL_RUNS (default 3) repeats the run on a fresh copy of
// the fixture to show variance, DOCSMAKER_EVAL_LANG (default "en") picks the
// output language, DOCSMAKER_EVAL_OUT is a directory that receives the raw
// JSON of each run and a markdown summary.
//
// It asserts nothing about quality (that varies by model and by run); it
// reports. It does assert that the selector never sends plumbing.
func TestEvaluateRealModel(t *testing.T) {
	model := os.Getenv("DOCSMAKER_EVAL_MODEL")
	if model == "" {
		t.Skip("set DOCSMAKER_EVAL_MODEL to run against a real local model")
	}
	runs := 3
	if v := os.Getenv("DOCSMAKER_EVAL_RUNS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			t.Fatalf("DOCSMAKER_EVAL_RUNS=%q", v)
		}
		runs = n
	}
	lang := os.Getenv("DOCSMAKER_EVAL_LANG")
	if lang == "" {
		lang = "en"
	}
	outDir := os.Getenv("DOCSMAKER_EVAL_OUT")
	if outDir != "" {
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	key, err := rules.LoadGabarito(filepath.Join("..", "..", "testdata", "billing-expected-rules.json"))
	if err != nil {
		t.Fatal(err)
	}

	var summary strings.Builder
	for run := 1; run <= runs; run++ {
		dir := copyFixture(t)
		res, err := app.ExtractRules(context.Background(), app.ExtractRulesOptions{
			ProjectPath: dir, Model: model, OutputLang: lang,
			Secrets: app.SecretsStrict, NonInteractive: true,
			Logf: func(f string, a ...any) { t.Logf("run %d: "+f, append([]any{run}, a...)...) },
		})
		if err != nil {
			t.Fatalf("run %d: %v", run, err)
		}

		var outcomes []rules.Outcome
		for _, r := range res.Results {
			outcomes = append(outcomes, rules.Outcome{
				Target: r.Target, Status: string(r.Status), Rule: r.Rule, EvidenceDropped: r.EvidenceDropped,
			})
		}
		rep := rules.Evaluate(key, outcomes)
		if len(rep.PlumbingSent) > 0 {
			t.Errorf("run %d: plumbing reached the model: %v", run, rep.PlumbingSent)
		}

		md := rep.Markdown()
		t.Logf("model %s, run %d/%d (%s)\n%s", model, run, runs, lang, md)
		fmtRun := "### Run " + strconv.Itoa(run) + "\n\n" + md + "\n"
		summary.WriteString(fmtRun)

		if outDir != "" {
			raw, _ := json.MarshalIndent(res, "", "  ")
			name := strings.NewReplacer(":", "_", "/", "_").Replace(model)
			_ = os.WriteFile(filepath.Join(outDir, name+"-run"+strconv.Itoa(run)+".json"), raw, 0o644)
			_ = os.WriteFile(filepath.Join(outDir, name+"-run"+strconv.Itoa(run)+"-report.md"), []byte(res.Markdown()), 0o644)
		}
	}
	if outDir != "" {
		name := strings.NewReplacer(":", "_", "/", "_").Replace(model)
		_ = os.WriteFile(filepath.Join(outDir, name+"-summary.md"), []byte(summary.String()), 0o644)
	}
}

// copyFixture copies the sample project into a fresh temp dir, so every run
// starts with an empty cache and the repository is never written to.
func copyFixture(t *testing.T) string {
	t.Helper()
	src := filepath.Join("..", "..", "testdata", "sample-project")
	dst := t.TempDir()
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if rel == "." {
			return nil
		}
		if top := strings.Split(rel, string(filepath.Separator))[0]; top == ".docsmaker" || top == "target" {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dst
}
