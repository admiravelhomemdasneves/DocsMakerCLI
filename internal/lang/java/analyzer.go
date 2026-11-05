package java

import (
	"context"
	"fmt"

	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/langmodel"
)

func init() {
	langmodel.Register("java", func() langmodel.LanguageAnalyzer { return &Analyzer{} })
}

// Analyzer implements langmodel.LanguageAnalyzer for Java.
type Analyzer struct{}

func (a *Analyzer) Language() string { return "java" }

func (a *Analyzer) FileExtensions() []string { return []string{".java"} }

// fileResult is one file's Pass 1 output, kept until Resolve (Pass 2) has
// seen every file in the project — cross-file resolution needs the whole
// symbol table built first.
type fileResult struct {
	path  string
	pkg   string
	types []*rawType
}

// ParseProject runs Pass 1 (per-file symbol extraction) over every file,
// then Pass 2 (Resolve) to cross-reference extends/implements/calls across
// the whole project. A single file failing to parse is recorded as a
// Diagnostic and skipped, not fatal to the run.
func (a *Analyzer) ParseProject(ctx context.Context, files []string) (*langmodel.CodeModel, []langmodel.Diagnostic, error) {
	var results []fileResult
	var diags []langmodel.Diagnostic

	for _, path := range files {
		select {
		case <-ctx.Done():
			return nil, diags, ctx.Err()
		default:
		}

		pkg, types, fileDiags, err := parseFile(ctx, path)
		diags = append(diags, fileDiags...)
		if err != nil {
			diags = append(diags, langmodel.Diagnostic{
				File:    path,
				Message: fmt.Sprintf("skipped: %v", err),
				Warning: true,
			})
			continue
		}
		results = append(results, fileResult{path: path, pkg: pkg, types: types})
	}

	return Resolve(results), diags, nil
}
