// Package docgen implements the bottom-up hierarchical summarization
// pipeline: method summaries feed class summaries, class summaries feed
// package summaries, package summaries feed the project README. Every level
// is generated (and cached) independently, so a run only pays for LLM calls
// on nodes whose input actually changed.
package docgen

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/cache"
	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/langmodel"
	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/llm"
	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/llm/prompts"
	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/secrets"
)

// Level selects how much of the project a Run covers.
type Level string

const (
	LevelMethod  Level = "method"
	LevelClass   Level = "class"
	LevelPackage Level = "package"
	LevelProject Level = "project"
)

// Options configures a single pipeline run. Target's shape depends on
// Level: "pkg.Type#method" for LevelMethod, "pkg.Type" for LevelClass,
// "pkg" for LevelPackage, and unused for LevelProject.
type Options struct {
	Level       Level
	Target      string
	ProjectName string
}

// Artifact is one generated documentation unit. A single Run typically
// returns more than one Artifact — every intermediate level consumed to
// build the requested one is included, bottom-up, so a caller (e.g. the
// console renderer) can show the whole chain, not just the top result.
type Artifact struct {
	Level   Level
	Target  string
	Content string
	// Source locates the declaration a method/class artifact describes, so
	// --mode inline can place it without guessing from Target (which is
	// ambiguous for overloads). Zero for package/project artifacts.
	Source langmodel.Range
}

// Pipeline ties a local LLM client and an optional cache together to run
// the summarization levels. Client is the only thing here that talks to
// Ollama — everything else is prompt assembly and CodeModel traversal.
type Pipeline struct {
	Client llm.Client
	Cache  cache.Store
	Model  string
	// Gate screens every prompt and every generated output for sensitive
	// information. nil disables screening (only for trusted/synthetic code).
	Gate *secrets.Gate
}

// New builds a Pipeline. store may be nil to disable caching.
func New(client llm.Client, store cache.Store, model string) *Pipeline {
	return &Pipeline{Client: client, Cache: store, Model: model}
}

// Run executes opts.Level against model.
func (p *Pipeline) Run(ctx context.Context, model *langmodel.CodeModel, opts Options) ([]Artifact, error) {
	switch opts.Level {
	case LevelMethod:
		return p.runMethodTarget(ctx, model, opts.Target)
	case LevelClass:
		return p.runClassTarget(ctx, model, opts.Target)
	case LevelPackage:
		return p.runPackageTarget(ctx, model, opts.Target)
	case LevelProject, "":
		return p.runProject(ctx, model, opts.ProjectName)
	default:
		return nil, fmt.Errorf("docgen: unknown level %q", opts.Level)
	}
}

func (p *Pipeline) runMethodTarget(ctx context.Context, model *langmodel.CodeModel, target string) ([]Artifact, error) {
	fqcn, methodName := splitTarget(target)
	if methodName == "" {
		return nil, fmt.Errorf("docgen: --level method requires --target pkg.Type#method")
	}
	pkg, typeName := splitFqcn(fqcn)
	decl, ok := findType(model, pkg, typeName)
	if !ok {
		return nil, fmt.Errorf("docgen: type %q not found in package %q", typeName, pkg)
	}
	m, ok := findMethod(decl, methodName)
	if !ok {
		return nil, fmt.Errorf("docgen: method %q not found on %s.%s", methodName, pkg, typeName)
	}

	summary, err := p.summarizeMethod(ctx, decl, m)
	if err != nil {
		return nil, err
	}
	return []Artifact{{Level: LevelMethod, Target: target, Content: summary, Source: m.SourceRange}}, nil
}

func (p *Pipeline) runClassTarget(ctx context.Context, model *langmodel.CodeModel, target string) ([]Artifact, error) {
	pkg, typeName := splitFqcn(target)
	decl, ok := findType(model, pkg, typeName)
	if !ok {
		return nil, fmt.Errorf("docgen: type %q not found in package %q", typeName, pkg)
	}
	artifacts, _, err := p.summarizeClass(ctx, decl)
	return artifacts, err
}

func (p *Pipeline) runPackageTarget(ctx context.Context, model *langmodel.CodeModel, target string) ([]Artifact, error) {
	pkg, ok := findPackage(model, target)
	if !ok {
		return nil, fmt.Errorf("docgen: package %q not found", target)
	}
	artifacts, _, err := p.summarizePackage(ctx, pkg, model.Relationships)
	return artifacts, err
}

func (p *Pipeline) runProject(ctx context.Context, model *langmodel.CodeModel, projectName string) ([]Artifact, error) {
	var artifacts []Artifact
	packageSummaries := map[string]string{}

	for i := range model.Packages {
		pkg := &model.Packages[i]
		pkgArtifacts, summary, err := p.summarizePackage(ctx, pkg, model.Relationships)
		if err != nil {
			return artifacts, err
		}
		artifacts = append(artifacts, pkgArtifacts...)
		packageSummaries[pkg.Name] = summary
	}

	system, user := prompts.Project(projectName, packageSummaries)
	overview, err := p.generate(ctx, "project "+projectName, system, user)
	if err != nil {
		return artifacts, fmt.Errorf("docgen: summarizing project: %w", err)
	}

	readme := BuildReadme(projectName, overview, model)
	artifacts = append(artifacts, Artifact{Level: LevelProject, Target: projectName, Content: readme})

	return artifacts, nil
}

func (p *Pipeline) summarizeMethod(ctx context.Context, decl *langmodel.TypeDecl, m *langmodel.Method) (string, error) {
	source, err := readSpan(m.SourceRange)
	if err != nil {
		source = "(source unavailable)"
	}
	system, user := prompts.Method(decl.Package, decl.Name, m, source)
	where := fmt.Sprintf("method %s.%s#%s (%s)", decl.Package, decl.Name, m.Name, location(m.SourceRange))
	summary, err := p.generate(ctx, where, system, user)
	if err != nil {
		return "", fmt.Errorf("docgen: summarizing %s.%s#%s: %w", decl.Package, decl.Name, m.Name, err)
	}
	return summary, nil
}

// summarizeClass returns every artifact generated to describe decl (its
// methods, then the class itself) plus the class-level summary text alone,
// for reuse by summarizePackage.
func (p *Pipeline) summarizeClass(ctx context.Context, decl *langmodel.TypeDecl) ([]Artifact, string, error) {
	var artifacts []Artifact
	methodSummaries := map[string]string{}

	for i := range decl.Methods {
		m := &decl.Methods[i]
		summary, err := p.summarizeMethod(ctx, decl, m)
		if err != nil {
			return artifacts, "", err
		}
		methodSummaries[m.Name] = summary
		artifacts = append(artifacts, Artifact{
			Level:   LevelMethod,
			Target:  fmt.Sprintf("%s.%s#%s", decl.Package, decl.Name, m.Name),
			Content: summary,
			Source:  m.SourceRange,
		})
	}

	fieldDecls := map[string]string{}
	for _, f := range decl.Fields {
		if src, err := readSpan(f.SourceRange); err == nil {
			fieldDecls[f.Name] = strings.Join(strings.Fields(src), " ")
		}
	}

	system, user := prompts.Class(decl, methodSummaries, fieldDecls)
	where := fmt.Sprintf("class %s.%s (%s)", decl.Package, decl.Name, location(decl.SourceRange))
	classSummary, err := p.generate(ctx, where, system, user)
	if err != nil {
		return artifacts, "", fmt.Errorf("docgen: summarizing class %s.%s: %w", decl.Package, decl.Name, err)
	}

	artifacts = append(artifacts, Artifact{
		Level:   LevelClass,
		Target:  decl.Package + "." + decl.Name,
		Content: classSummary,
		Source:  decl.SourceRange,
	})

	return artifacts, classSummary, nil
}

// summarizePackage mirrors summarizeClass one level up: every artifact for
// pkg's types, then the package itself, plus the package-level summary text
// alone for reuse by runProject.
func (p *Pipeline) summarizePackage(ctx context.Context, pkg *langmodel.Package, allRels []langmodel.Relationship) ([]Artifact, string, error) {
	var artifacts []Artifact
	classSummaries := map[string]string{}

	for i := range pkg.Types {
		decl := &pkg.Types[i]
		classArtifacts, summary, err := p.summarizeClass(ctx, decl)
		if err != nil {
			return artifacts, "", err
		}
		artifacts = append(artifacts, classArtifacts...)
		classSummaries[decl.Name] = summary
	}

	system, user := prompts.Package(pkg.Name, pkg.Types, classSummaries, relationshipsWithin(pkg, allRels))
	pkgSummary, err := p.generate(ctx, "package "+pkg.Name, system, user)
	if err != nil {
		return artifacts, "", fmt.Errorf("docgen: summarizing package %s: %w", pkg.Name, err)
	}

	artifacts = append(artifacts, Artifact{Level: LevelPackage, Target: pkg.Name, Content: pkgSummary})
	return artifacts, pkgSummary, nil
}

// OmittedSummary stands in for any unit the developer chose to skip at a
// sensitive-information prompt; higher levels then summarize around it.
const OmittedSummary = "(documentation omitted: sensitive content)"

// generate is the single point every level calls through, so the secret
// gates and the cache are applied uniformly:
//
//  1. gate on the prompt, before anything reaches the model;
//  2. cache lookup keyed by the exact (post-gate) request, so an unchanged
//     method/class/package hits the cache instead of calling Ollama again;
//  3. gate on the output, before it is kept, cached or returned. This also
//     runs on cache hits, so a stale cache can't bypass the second gate.
//
// where describes what is being generated; it is shown in confirmation
// prompts and recorded in the audit log.
func (p *Pipeline) generate(ctx context.Context, where, system, user string) (string, error) {
	if p.Gate != nil {
		screened, err := p.Gate.Screen(ctx, "prompt for "+where, user)
		if err != nil {
			return "", err
		}
		if screened.Skip {
			return OmittedSummary, nil
		}
		user = screened.Text
	}

	key := cache.Key(p.Model, system, user)
	text, cached := "", false
	if p.Cache != nil {
		text, cached = p.Cache.Get(key)
	}
	if !cached {
		resp, err := p.Client.Generate(ctx, llm.Request{Model: p.Model, System: system, Prompt: user})
		if err != nil {
			return "", err
		}
		text = strings.TrimSpace(resp.Text)
	}

	if p.Gate != nil {
		screened, err := p.Gate.Screen(ctx, "generated output for "+where, text)
		if err != nil {
			return "", err
		}
		if screened.Skip {
			return OmittedSummary, nil
		}
		text = screened.Text
	}

	if p.Cache != nil && !cached {
		_ = p.Cache.Set(key, text)
	}
	return text, nil
}

// location renders a source range as "file:line" for prompts and logs.
func location(r langmodel.Range) string {
	if r.File == "" {
		return "unknown location"
	}
	return fmt.Sprintf("%s:%d", r.File, r.StartLine)
}

func relationshipsWithin(pkg *langmodel.Package, all []langmodel.Relationship) []langmodel.Relationship {
	names := make(map[string]bool, len(pkg.Types))
	for _, t := range pkg.Types {
		names[t.Name] = true
	}
	var out []langmodel.Relationship
	for _, r := range all {
		if names[r.From.Name] {
			out = append(out, r)
		}
	}
	return out
}

func findPackage(model *langmodel.CodeModel, name string) (*langmodel.Package, bool) {
	for i := range model.Packages {
		if model.Packages[i].Name == name {
			return &model.Packages[i], true
		}
	}
	return nil, false
}

func findType(model *langmodel.CodeModel, pkg, name string) (*langmodel.TypeDecl, bool) {
	p, ok := findPackage(model, pkg)
	if !ok {
		return nil, false
	}
	for i := range p.Types {
		if p.Types[i].Name == name {
			return &p.Types[i], true
		}
	}
	return nil, false
}

func findMethod(decl *langmodel.TypeDecl, name string) (*langmodel.Method, bool) {
	for i := range decl.Methods {
		if decl.Methods[i].Name == name {
			return &decl.Methods[i], true
		}
	}
	return nil, false
}

// splitTarget splits "pkg.Type#method" into ("pkg.Type", "method"); if
// there is no "#", method is "".
func splitTarget(target string) (fqcn, method string) {
	if idx := strings.Index(target, "#"); idx >= 0 {
		return target[:idx], target[idx+1:]
	}
	return target, ""
}

// splitFqcn splits "pkg.Type" into ("pkg", "Type"); a name with no dot is
// treated as a type in the default package.
func splitFqcn(fqcn string) (pkg, name string) {
	if idx := strings.LastIndex(fqcn, "."); idx >= 0 {
		return fqcn[:idx], fqcn[idx+1:]
	}
	return "", fqcn
}

// readSpan reads the exact source text a langmodel.Range points at. The
// CodeModel only stores byte offsets, not source text, to stay small; this
// is the one place docgen goes back to disk to recover it.
func readSpan(rng langmodel.Range) (string, error) {
	data, err := os.ReadFile(rng.File)
	if err != nil {
		return "", err
	}
	if rng.StartByte < 0 || rng.EndByte > len(data) || rng.StartByte > rng.EndByte {
		return "", fmt.Errorf("docgen: invalid source range for %s", rng.File)
	}
	return string(data[rng.StartByte:rng.EndByte]), nil
}
