// Package prompts builds the system/user prompt pairs for each level of the
// docgen pipeline. Every function here is pure string assembly — no file
// I/O, no LLM calls — so it's trivial to unit test and to tune independently
// of the pipeline that calls it.
//
// Higher levels deliberately consume the summaries produced by the level
// below rather than raw source, to stay inside a local model's context
// window (see plan: sumarização hierárquica bottom-up).
package prompts

import (
	"fmt"
	"sort"
	"strings"

	"github.com/andresantos/docsmakercli/internal/langmodel"
)

const methodSystem = "You are a terse code documentation assistant. Given a Java method's " +
	"signature and source, write a concise summary (2-4 sentences) of what it does and why, " +
	"followed by one @param line per parameter and one @return line if it returns a value. " +
	"Plain text only: no markdown, no code fences, no restating the signature."

const classSystem = "You are a terse code documentation assistant. Given a Java class's " +
	"signature, fields, relationships, and the already-written summaries of its methods, " +
	"write a short summary (3-6 sentences) of the class's responsibility and how it collaborates " +
	"with the types it extends, implements, or references. Plain text only, no markdown."

const packageSystem = "You are a terse code documentation assistant. Given a Java package's " +
	"types and their already-written summaries, write a short paragraph (3-5 sentences) " +
	"describing what this package is responsible for. Plain text only, no markdown."

const projectSystem = "You are a terse code documentation assistant writing a project README " +
	"overview section. Given a project name and the summaries of its packages, write 1-2 short " +
	"paragraphs describing what the project does as a whole. Plain text only, no markdown, no headings."

// Method builds the prompt for a single method, given its owning type,
// resolved signature, and raw source text.
func Method(pkg, ownerType string, m *langmodel.Method, source string) (system, user string) {
	var b strings.Builder
	fmt.Fprintf(&b, "Class: %s.%s\n", pkg, ownerType)
	fmt.Fprintf(&b, "Signature: %s\n\n", methodSignature(m))
	fmt.Fprintf(&b, "Source:\n%s\n", source)
	return methodSystem, b.String()
}

// Class builds the prompt for a class/interface, given its resolved
// declaration, the previously generated summary for each of its methods
// (keyed by method name), and, optionally, each field's declaration as
// written in source (keyed by field name) so defaults and constants can
// inform the summary. Fields without an entry fall back to "name: type".
func Class(decl *langmodel.TypeDecl, methodSummaries, fieldDecls map[string]string) (system, user string) {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s.%s%s\n", decl.Kind, decl.Package, decl.Name, extendsImplementsClause(decl))

	if len(decl.Fields) > 0 {
		b.WriteString("Fields:\n")
		for _, f := range decl.Fields {
			if src := fieldDecls[f.Name]; src != "" {
				fmt.Fprintf(&b, "  - %s\n", src)
			} else {
				fmt.Fprintf(&b, "  - %s: %s\n", f.Name, f.Type.Name)
			}
		}
	}

	if len(decl.Methods) > 0 {
		b.WriteString("Methods:\n")
		for _, m := range decl.Methods {
			summary := methodSummaries[m.Name]
			if summary == "" {
				summary = "(no summary available)"
			}
			fmt.Fprintf(&b, "  - %s: %s\n", methodSignature(&m), summary)
		}
	}

	return classSystem, b.String()
}

// Package builds the prompt for a package, given the previously generated
// summary for each type it declares (keyed by type name) and the
// relationships involving those types.
func Package(pkgName string, types []langmodel.TypeDecl, classSummaries map[string]string, rels []langmodel.Relationship) (system, user string) {
	var b strings.Builder
	fmt.Fprintf(&b, "Package: %s\n\n", pkgName)

	b.WriteString("Types:\n")
	for _, t := range types {
		summary := classSummaries[t.Name]
		if summary == "" {
			summary = "(no summary available)"
		}
		fmt.Fprintf(&b, "  - %s %s: %s\n", t.Kind, t.Name, summary)
	}

	if len(rels) > 0 {
		b.WriteString("\nRelationships:\n")
		for _, r := range rels {
			fmt.Fprintf(&b, "  - %s %s %s\n", r.From.Name, r.Kind, r.To.Name)
		}
	}

	return packageSystem, b.String()
}

// Project builds the README overview prompt, given the previously generated
// summary for each package (keyed by package name).
func Project(projectName string, packageSummaries map[string]string) (system, user string) {
	var b strings.Builder
	fmt.Fprintf(&b, "Project: %s\n\n", projectName)
	b.WriteString("Packages:\n")
	// Sorted: the prompt is a cache key, and map order is random in Go.
	names := make([]string, 0, len(packageSummaries))
	for name := range packageSummaries {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Fprintf(&b, "  - %s: %s\n", name, packageSummaries[name])
	}
	return projectSystem, b.String()
}

func methodSignature(m *langmodel.Method) string {
	params := make([]string, len(m.Params))
	for i, p := range m.Params {
		params[i] = p.Type.Name + " " + p.Name
	}
	ret := m.ReturnType.Name
	if ret == "" {
		ret = "void"
	}
	return fmt.Sprintf("%s %s(%s)", ret, m.Name, strings.Join(params, ", "))
}

func extendsImplementsClause(decl *langmodel.TypeDecl) string {
	var parts []string
	if len(decl.Extends) > 0 {
		parts = append(parts, "extends "+joinNames(decl.Extends))
	}
	if len(decl.Implements) > 0 {
		parts = append(parts, "implements "+joinNames(decl.Implements))
	}
	if len(parts) == 0 {
		return ""
	}
	return " " + strings.Join(parts, " ")
}

func joinNames(refs []langmodel.TypeRef) string {
	names := make([]string, len(refs))
	for i, r := range refs {
		names[i] = r.Name
	}
	return strings.Join(names, ", ")
}
