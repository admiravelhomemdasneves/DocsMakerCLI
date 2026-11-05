package app

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/cache"
	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/docgen"
	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/llm"
	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/output"
	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/project"
	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/rules"
	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/secrets"
)

// ExtractRulesOptions configures a business-rule extraction run.
type ExtractRulesOptions struct {
	ProjectPath string
	Language    string // source language; empty = the only registered analyzer
	// Target limits the run: a package, "pkg.Type" or "pkg.Type#method". Empty
	// means the whole project.
	Target string
	// Mode is ModeConsole (default) or ModeSidecar; inline is not supported.
	Mode OutputMode
	// MinScore is the lowest structural score a method needs to be sent to the
	// model; 0 means rules.DefaultMinScore. A method with no decisions at all
	// scores 0 and is never sent.
	MinScore int
	// MaxMethods keeps only the N highest-scoring candidates (0 = no limit).
	MaxMethods int
	// MinConfidence hides rules less trustworthy than this (empty = show all).
	MinConfidence rules.Confidence
	// OutputLang is the language of the rules and of the report ("pt", "en", ...).
	OutputLang string

	Secrets        SecretsPolicy
	NonInteractive bool
	Model          string
	Logf           func(format string, args ...any)
	Approver       secrets.Approver
}

// ExtractRulesResult is the structured outcome of ExtractRules.
type ExtractRulesResult struct {
	Project string `json:"project"`
	// Lang is the report language the Markdown view uses.
	Lang string `json:"lang"`
	// Results has one entry per candidate method, in source order, minus the
	// rules hidden by MinConfidence.
	Results []docgen.RuleResult `json:"results"`
	Summary docgen.RulesSummary `json:"summary"`
	// Hidden counts rules left out because they were below MinConfidence.
	Hidden int `json:"hiddenByConfidence,omitempty"`
	// OutputDir is where --mode sidecar wrote the report.
	OutputDir string `json:"outputDir,omitempty"`
}

// Markdown renders the whole report; see docgen.BuildRulesCatalog.
func (r *ExtractRulesResult) Markdown() string {
	return docgen.BuildRulesCatalog(r.Project, r.Results, r.Lang)
}

// rulesDir is where --mode sidecar puts the report, next to .docsmaker/doc.
func rulesDir(projectPath string) string {
	return filepath.Join(projectPath, ".docsmaker", "rules")
}

// ExtractRules runs analyze -> select candidates -> secrets-gated model calls
// -> report, for `docsmaker rules`. It uses the same local model, cache and
// secrets gates as GenerateDocs, and never touches the source files.
func ExtractRules(ctx context.Context, opts ExtractRulesOptions) (*ExtractRulesResult, error) {
	switch opts.Mode {
	case "", ModeConsole, ModeSidecar:
	case ModeInline:
		return nil, fmt.Errorf("app: rules does not support --mode inline; use console or sidecar")
	default:
		return nil, fmt.Errorf("app: unknown --mode %q", opts.Mode)
	}
	if opts.MinConfidence != "" {
		if c, ok := rules.ParseConfidence(string(opts.MinConfidence)); !ok {
			return nil, fmt.Errorf("app: unknown --min-confidence %q, expected low|medium|high", opts.MinConfidence)
		} else {
			opts.MinConfidence = c
		}
	}

	gate, err := secretsGate(GenerateDocsOptions{
		ProjectPath: opts.ProjectPath, Secrets: opts.Secrets,
		NonInteractive: opts.NonInteractive, Approver: opts.Approver,
	})
	if err != nil {
		return nil, err
	}

	analyzer, err := resolveAnalyzer(opts.Language)
	if err != nil {
		return nil, err
	}
	files, err := project.DiscoverFiles(opts.ProjectPath, analyzer.FileExtensions())
	if err != nil {
		return nil, fmt.Errorf("app: discovering files under %s: %w", opts.ProjectPath, err)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("app: no %s files found under %s", analyzer.Language(), opts.ProjectPath)
	}
	model, _, err := analyzer.ParseProject(ctx, files)
	if err != nil {
		return nil, fmt.Errorf("app: parsing project: %w", err)
	}
	if opts.Target != "" && !rules.ScopeExists(model, opts.Target) {
		return nil, fmt.Errorf("app: --target %q matches no package, class or method (use pkg, pkg.Type or pkg.Type#method)", opts.Target)
	}

	modelName := opts.Model
	if modelName == "" {
		modelName = llm.DefaultModel
	}
	baseURL, err := ensureLocalModel(ctx, modelName, opts.Logf)
	if err != nil {
		return nil, err
	}

	minScore := opts.MinScore
	if minScore == 0 {
		minScore = rules.DefaultMinScore
	}
	pipeline := docgen.New(llm.NewOllamaClient(baseURL), cache.NewFileStore(filepath.Join(opts.ProjectPath, ".docsmaker", "cache")), modelName)
	pipeline.Gate = gate

	all, err := pipeline.ExtractRules(ctx, model, docgen.RulesOptions{
		Scope: opts.Target, MinScore: minScore, MaxMethods: opts.MaxMethods, Lang: opts.OutputLang,
		Progress: func(n, total int, target string) {
			if opts.Logf != nil {
				opts.Logf("[%d/%d] %s", n, total, target)
			}
		},
	})
	if err != nil {
		return nil, fmt.Errorf("app: extracting rules: %w", err)
	}

	res := &ExtractRulesResult{Project: project.DetectBuildSystem(opts.ProjectPath).Name, Lang: opts.OutputLang}
	for _, r := range all {
		if r.Status == docgen.RuleFound && opts.MinConfidence != "" && !r.Rule.Confidence.AtLeast(opts.MinConfidence) {
			res.Hidden++
			continue
		}
		res.Results = append(res.Results, r)
	}
	res.Summary = docgen.SummarizeRules(res.Results)

	if opts.Mode == ModeSidecar {
		if err := writeRulesSidecar(ctx, rulesDir(opts.ProjectPath), res); err != nil {
			return res, err
		}
		res.OutputDir = rulesDir(opts.ProjectPath)
	}
	return res, nil
}

// writeRulesSidecar writes RULES.md plus one file per class that has rules.
func writeRulesSidecar(ctx context.Context, dir string, res *ExtractRulesResult) error {
	w := output.NewSidecarWriter(dir)
	write := func(target, content string) error {
		// Level "" puts the file straight in dir rather than a subfolder.
		if err := w.Write(ctx, output.Artifact{Target: target, Content: content}); err != nil {
			return fmt.Errorf("app: writing rules report: %w", err)
		}
		return nil
	}
	if err := write("RULES", res.Markdown()); err != nil {
		return err
	}
	for _, owner := range docgen.RuleOwners(res.Results) {
		if err := write(owner, docgen.BuildClassRules(owner, res.Results, res.Lang)); err != nil {
			return err
		}
	}
	return nil
}
