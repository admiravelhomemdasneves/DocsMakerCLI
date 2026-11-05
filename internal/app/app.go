// Package app is the application layer: the single seam between drivers
// (the Cobra CLI today, potentially a "docsmaker serve" HTTP API or an
// embedding UI later) and the core pipeline. Every function here returns
// structured data, never pre-formatted text — rendering is the driver's job.
package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/andresantos/docsmakercli/internal/cache"
	"github.com/andresantos/docsmakercli/internal/docgen"
	"github.com/andresantos/docsmakercli/internal/gitfilter"
	"github.com/andresantos/docsmakercli/internal/langmodel"
	"github.com/andresantos/docsmakercli/internal/llm"
	"github.com/andresantos/docsmakercli/internal/output"
	"github.com/andresantos/docsmakercli/internal/project"
	"github.com/andresantos/docsmakercli/internal/secrets"
)

// AnalyzeOptions configures a structural analysis run.
type AnalyzeOptions struct {
	ProjectPath string
	Language    string // e.g. "java"; empty = auto-detect from file extensions found
}

// AnalyzeResult is the structured outcome of Analyze, independent of how a
// driver chooses to render it (text tree, JSON, a future UI's own widgets).
type AnalyzeResult struct {
	Model       *langmodel.CodeModel
	Diagnostics []langmodel.Diagnostic
}

// Analyze parses ProjectPath with the appropriate LanguageAnalyzer and
// returns the resulting CodeModel. It does not call the LLM — this is the
// "no AI" structural pass that backs `docsmaker analyze`.
func Analyze(ctx context.Context, opts AnalyzeOptions) (*AnalyzeResult, error) {
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

	model, diags, err := analyzer.ParseProject(ctx, files)
	if err != nil {
		return nil, fmt.Errorf("app: parsing project: %w", err)
	}

	return &AnalyzeResult{Model: model, Diagnostics: diags}, nil
}

// resolveAnalyzer picks the LanguageAnalyzer to use: the explicitly
// requested one, or — since only Java is registered for the MVP — the sole
// registered analyzer. Once a second language ships, an empty language with
// more than one candidate becomes ambiguous and must be asked for
// explicitly rather than guessed.
func resolveAnalyzer(language string) (langmodel.LanguageAnalyzer, error) {
	if language == "" {
		langs := langmodel.SupportedLanguages()
		switch len(langs) {
		case 0:
			return nil, errors.New("app: no language analyzers registered")
		case 1:
			language = langs[0]
		default:
			return nil, fmt.Errorf("app: multiple languages available (%v), specify one explicitly", langs)
		}
	}

	factory, ok := langmodel.Lookup(language)
	if !ok {
		return nil, fmt.Errorf("app: unsupported language %q", language)
	}
	return factory(), nil
}

// DocLevel selects how much of the project a GenerateDocs run covers.
type DocLevel string

const (
	LevelMethod  DocLevel = "method"
	LevelClass   DocLevel = "class"
	LevelPackage DocLevel = "package"
	LevelProject DocLevel = "project"
)

// OutputMode selects where generated documentation is written.
type OutputMode string

const (
	ModeConsole OutputMode = "console"
	ModeInline  OutputMode = "inline"
	ModeSidecar OutputMode = "sidecar"
)

// SecretsPolicy controls how the secret-detection gates behave.
type SecretsPolicy string

const (
	SecretsStrict SecretsPolicy = "strict" // never allow a bypass
	SecretsPrompt SecretsPolicy = "prompt" // interactive confirmation (default)
	SecretsOff    SecretsPolicy = "off"    // demo/synthetic-code only, always warns
)

// GenerateDocsOptions configures a documentation-generation run.
type GenerateDocsOptions struct {
	ProjectPath    string
	Language       string
	Level          DocLevel
	Target         string // fully-qualified name, required when Level == LevelMethod or LevelClass
	Mode           OutputMode
	Secrets        SecretsPolicy
	NonInteractive bool   // --yes: skip prompts, defaults to hard-block on findings rather than silent-allow
	Model          string // Ollama model name; empty uses llm.DefaultModel
	// Logf receives progress notes for the driver to show (may be nil).
	Logf func(format string, args ...any)
	// Approver resolves findings interactively under SecretsPrompt. The
	// driver supplies it (terminal prompt today, a UI modal tomorrow); if
	// nil, findings are hard-blocked rather than silently allowed.
	Approver secrets.Approver
}

// secretsGate builds the sensitive-information gate for a run, or nil when
// the developer explicitly turned screening off.
func secretsGate(opts GenerateDocsOptions) (*secrets.Gate, error) {
	var approver secrets.Approver
	switch opts.Secrets {
	case SecretsOff:
		return nil, nil
	case SecretsStrict:
		approver = secrets.BlockApprover{}
	case "", SecretsPrompt:
		approver = opts.Approver
		if opts.NonInteractive || approver == nil {
			approver = secrets.BlockApprover{}
		}
	default:
		return nil, fmt.Errorf("app: unknown --secrets %q, expected strict|prompt|off", opts.Secrets)
	}

	// Values the developer already declared harmless pass in every policy
	// (strict and --yes included); a broken allowlist file stops the run
	// rather than being ignored.
	allow, err := secrets.LoadAllowlist(allowlistPath(opts.ProjectPath))
	if err != nil {
		return nil, err
	}

	return &secrets.Gate{
		Scanner:  secrets.DefaultScanner{},
		Approver: approver,
		Audit:    &secrets.FileAuditLog{Path: filepath.Join(opts.ProjectPath, ".docsmaker", "audit.log")},
		Allow:    allow,
	}, nil
}

// DocArtifact is a single generated documentation unit ready for a driver to render.
type DocArtifact struct {
	Level   DocLevel `json:"level"`
	Target  string   `json:"target"`
	Content string   `json:"content"`
}

// GenerateDocsResult is the structured outcome of a GenerateDocs run.
type GenerateDocsResult struct {
	Artifacts []DocArtifact `json:"artifacts"`
	// Inline is set for --mode inline: what was written into the sources.
	Inline *InlineReport `json:"inline,omitempty"`
}

// GenerateDocs runs the full analyze -> secrets-scan -> summarize -> output
// pipeline (internal/docgen.Pipeline) according to opts.
//
// Two secrets gates guard the run (see opts.Secrets): one on every prompt
// before it reaches the model, one on every generated output before it is
// kept or written anywhere.
func GenerateDocs(ctx context.Context, opts GenerateDocsOptions) (*GenerateDocsResult, error) {
	gate, err := secretsGate(opts)
	if err != nil {
		return nil, err
	}

	switch opts.Mode {
	case "", ModeConsole, ModeSidecar:
		// handled below
	case ModeInline:
		// handled below, once the files are known
	default:
		return nil, fmt.Errorf("app: unknown --mode %q", opts.Mode)
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

	// Inline docs live in the working tree, so before generating anything
	// (it is slow) make sure git would strip them from every commit.
	if opts.Mode == ModeInline {
		if err := gitfilter.EnsureWired(ctx, opts.ProjectPath, files); err != nil {
			return nil, fmt.Errorf("app: refusing --mode inline: %w", err)
		}
	}

	model, _, err := analyzer.ParseProject(ctx, files)
	if err != nil {
		return nil, fmt.Errorf("app: parsing project: %w", err)
	}

	modelName := opts.Model
	if modelName == "" {
		modelName = llm.DefaultModel
	}

	baseURL, err := ensureLocalModel(ctx, modelName, opts.Logf)
	if err != nil {
		return nil, err
	}
	client := llm.NewOllamaClient(baseURL)

	buildInfo := project.DetectBuildSystem(opts.ProjectPath)
	store := cache.NewFileStore(filepath.Join(opts.ProjectPath, ".docsmaker", "cache"))
	pipeline := docgen.New(client, store, modelName)
	pipeline.Gate = gate

	level := opts.Level
	if level == "" {
		level = LevelProject
	}

	artifacts, err := pipeline.Run(ctx, model, docgen.Options{
		Level:       docgen.Level(level),
		Target:      opts.Target,
		ProjectName: buildInfo.Name,
	})
	if err != nil {
		return nil, fmt.Errorf("app: generating docs: %w", err)
	}

	result := &GenerateDocsResult{Artifacts: make([]DocArtifact, len(artifacts))}
	for i, a := range artifacts {
		result.Artifacts[i] = DocArtifact{Level: DocLevel(a.Level), Target: a.Target, Content: a.Content}
	}

	if opts.Mode == ModeInline {
		report, err := applyInline(ctx, analyzer, artifacts)
		result.Inline = report
		if err != nil {
			return result, err
		}
	}

	if opts.Mode == ModeSidecar {
		writer := output.NewSidecarWriter(filepath.Join(opts.ProjectPath, ".docsmaker", "doc"))
		for _, a := range artifacts {
			if err := writer.Write(ctx, output.Artifact{Level: string(a.Level), Target: a.Target, Content: a.Content}); err != nil {
				return result, fmt.Errorf("app: writing sidecar output: %w", err)
			}
		}
	}

	return result, nil
}

// InitOptions configures wiring the git clean/smudge filter into a repo.
type InitOptions struct {
	RepoPath   string
	Force      bool   // replace conflicting existing attributes/config
	Shared     bool   // write the tracked .gitattributes instead of the local .git/info/attributes
	Executable string // binary git should run; empty = this process's executable
}

// InitResult reports what Init changed, for the CLI to render.
type InitResult struct {
	RepoRoot          string   `json:"repoRoot"`
	AttributesFile    string   `json:"attributesFile"`
	Patterns          []string `json:"patterns"`
	Executable        string   `json:"executable"`
	AttributesChanged bool     `json:"attributesChanged"`
	ConfigChanged     bool     `json:"configChanged"`
	ExcludeChanged    bool     `json:"excludeChanged"`
}

// Init wires up internal/gitfilter in RepoPath so AI-inserted doc lines are
// stripped from anything staged/committed there (see docsmaker init). It is
// idempotent, and local to this clone unless opts.Shared is set.
func Init(ctx context.Context, opts InitOptions) (*InitResult, error) {
	exe := opts.Executable
	if exe == "" {
		var err error
		if exe, err = os.Executable(); err != nil {
			return nil, fmt.Errorf("app: locating the docsmaker executable: %w", err)
		}
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			exe = resolved
		}
	}

	// One pattern per extension of every registered language.
	seen := map[string]bool{}
	var patterns []string
	for _, lang := range langmodel.SupportedLanguages() {
		factory, _ := langmodel.Lookup(lang)
		for _, ext := range factory().FileExtensions() {
			if p := "*" + ext; !seen[p] {
				seen[p] = true
				patterns = append(patterns, p)
			}
		}
	}
	sort.Strings(patterns)

	res, err := gitfilter.Setup(ctx, gitfilter.SetupOptions{
		RepoPath: opts.RepoPath, Executable: exe, Patterns: patterns, Shared: opts.Shared, Force: opts.Force,
	})
	if err != nil {
		return nil, fmt.Errorf("app: init: %w", err)
	}
	return &InitResult{
		RepoRoot: res.RepoRoot, AttributesFile: res.AttributesFile, Patterns: patterns, Executable: exe,
		AttributesChanged: res.AttributesChanged, ConfigChanged: res.ConfigChanged, ExcludeChanged: res.ExcludeChanged,
	}, nil
}
