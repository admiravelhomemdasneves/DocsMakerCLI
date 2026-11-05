package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/docgen"
	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/inserter"
	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/langmodel"
)

// InlineOutcome reports what --mode inline did for one declaration.
type InlineOutcome struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Target string `json:"target"`
	Action string `json:"action"` // inserted|replaced|unchanged|skipped
	Reason string `json:"reason,omitempty"`
}

// InlineReport summarizes a --mode inline run.
type InlineReport struct {
	Outcomes     []InlineOutcome `json:"outcomes"`
	FilesChanged int             `json:"filesChanged"`
}

// applyInline writes the method- and class-level artifacts into the source
// files as sentinel-wrapped doc comments. Package and project artifacts have
// no declaration to attach to and are only returned to the driver.
func applyInline(ctx context.Context, analyzer langmodel.LanguageAnalyzer, artifacts []docgen.Artifact) (*InlineReport, error) {
	byFile := map[string][]inserter.Insertion{}
	for _, a := range artifacts {
		if (a.Level != docgen.LevelMethod && a.Level != docgen.LevelClass) || a.Source.File == "" {
			continue
		}
		if a.Content == "" || a.Content == docgen.OmittedSummary {
			continue
		}
		byFile[a.Source.File] = append(byFile[a.Source.File], inserter.Insertion{
			Target: a.Source, Name: a.Target, Content: a.Content,
		})
	}

	files := make([]string, 0, len(byFile))
	for f := range byFile {
		files = append(files, f)
	}
	sort.Strings(files)

	ins := inserter.BlockCommentInserter{Verify: parseVerifier(analyzer)}
	report := &InlineReport{}
	for _, file := range files {
		outcomes, err := ins.InsertDocs(ctx, file, byFile[file])
		if err != nil {
			return report, fmt.Errorf("app: inline insertion into %s: %w", file, err)
		}
		changed := false
		for _, o := range outcomes {
			report.Outcomes = append(report.Outcomes, InlineOutcome{
				File: file, Line: o.Target.StartLine, Target: o.Name, Action: string(o.Action), Reason: o.Reason,
			})
			if o.Action == inserter.ActionInserted || o.Action == inserter.ActionReplaced {
				changed = true
			}
		}
		if changed {
			report.FilesChanged++
		}
	}
	return report, nil
}

// parseVerifier checks, using the same analyzer that read the project, that
// an edited file still parses cleanly and declares exactly what it did
// before. It is the safety net behind every inline write.
func parseVerifier(analyzer langmodel.LanguageAnalyzer) func(context.Context, []byte, []byte) error {
	ext := analyzer.FileExtensions()[0]

	inspect := func(ctx context.Context, dir, name string, data []byte) (string, bool, error) {
		path := filepath.Join(dir, name+ext)
		if err := os.WriteFile(path, data, 0o600); err != nil {
			return "", false, err
		}
		model, diags, err := analyzer.ParseProject(ctx, []string{path})
		if err != nil {
			return "", false, err
		}
		broken := false
		for _, d := range diags {
			if strings.Contains(d.Message, "syntax error") || !d.Warning {
				broken = true
			}
		}
		return declarations(model), broken, nil
	}

	return func(ctx context.Context, before, after []byte) error {
		dir, err := os.MkdirTemp("", "docsmaker-verify-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(dir)

		wantDecls, wasBroken, err := inspect(ctx, dir, "before", before)
		if err != nil {
			return err
		}
		gotDecls, nowBroken, err := inspect(ctx, dir, "after", after)
		if err != nil {
			return err
		}
		if nowBroken && !wasBroken {
			return fmt.Errorf("the edited file has syntax errors")
		}
		if gotDecls != wantDecls {
			return fmt.Errorf("the edited file declares different types or members than the original")
		}
		return nil
	}
}

// declarations renders a model as a stable fingerprint of what it declares.
func declarations(m *langmodel.CodeModel) string {
	var parts []string
	for _, p := range m.Packages {
		for _, t := range p.Types {
			parts = append(parts, fmt.Sprintf("type %s.%s", p.Name, t.Name))
			for _, f := range t.Fields {
				parts = append(parts, fmt.Sprintf("field %s.%s", t.Name, f.Name))
			}
			for _, md := range t.Methods {
				parts = append(parts, fmt.Sprintf("method %s.%s/%d", t.Name, md.Name, len(md.Params)))
			}
		}
	}
	sort.Strings(parts)
	return strings.Join(parts, "\n")
}
