// Package java implements langmodel.LanguageAnalyzer for Java using
// tree-sitter for syntax parsing plus a project-scoped symbol-resolution
// pass built on top (symbols.go, resolve.go). Deliberately not a compiler:
// no generics inference, no overload resolution, no full type-checking —
// "good enough for documentation" per the plan's stated scope.
package java

import (
	"context"

	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/langmodel"
	sitter "github.com/smacker/go-tree-sitter"
	tsjava "github.com/smacker/go-tree-sitter/java"
)

// parseSource parses a single Java file's contents into a tree-sitter tree.
// Callers must call tree.Close() when done.
func parseSource(ctx context.Context, source []byte) (*sitter.Tree, error) {
	parser := sitter.NewParser()
	parser.SetLanguage(tsjava.GetLanguage())
	return parser.ParseCtx(ctx, nil, source)
}

// text returns the exact source text spanned by n.
func text(n *sitter.Node, source []byte) string {
	if n == nil {
		return ""
	}
	return string(source[n.StartByte():n.EndByte()])
}

// childByFieldOrType looks up a child by its grammar field name first (the
// normal, precise way to navigate a tree-sitter tree), and falls back to
// scanning named children for a matching node type. The fallback exists
// because this codebase's confidence in a couple of tree-sitter-java's
// field names (superclass/extends on class/interface declarations) is
// empirical rather than certain — this keeps parsing resilient if a field
// name assumption turns out wrong for some grammar version.
func childByFieldOrType(n *sitter.Node, field, nodeType string) *sitter.Node {
	if n == nil {
		return nil
	}
	if field != "" {
		if c := n.ChildByFieldName(field); c != nil {
			return c
		}
	}
	for i := 0; i < int(n.NamedChildCount()); i++ {
		c := n.NamedChild(i)
		if c.Type() == nodeType {
			return c
		}
	}
	return nil
}

// modifiers reads the keyword modifiers (public, static, final, ...) from a
// `modifiers` node, skipping annotations — those are anonymous/unnamed
// tokens in the grammar, so a plain (non-named) child walk yields exactly
// the keyword strings.
func modifiers(n *sitter.Node) []string {
	if n == nil {
		return nil
	}
	var mods []string
	for i := 0; i < int(n.ChildCount()); i++ {
		c := n.Child(i)
		if !c.IsNamed() {
			mods = append(mods, c.Type())
		}
	}
	return mods
}

// rangeOf converts a tree-sitter node's position into a langmodel.Range.
func rangeOf(n *sitter.Node, file string) langmodel.Range {
	start := n.StartPoint()
	end := n.EndPoint()
	return langmodel.Range{
		File:      file,
		StartByte: int(n.StartByte()),
		EndByte:   int(n.EndByte()),
		StartLine: int(start.Row) + 1,
		EndLine:   int(end.Row) + 1,
	}
}
