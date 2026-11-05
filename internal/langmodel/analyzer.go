package langmodel

import "context"

// LanguageAnalyzer is the seam that makes DocsMakerCli language-pluggable:
// every supported language implements this once (internal/lang/<name>) and
// nothing else in the codebase needs to change to add a new one.
type LanguageAnalyzer interface {
	// Language returns the canonical language identifier, e.g. "java".
	Language() string
	// FileExtensions lists the extensions this analyzer claims, e.g. []string{".java"}.
	FileExtensions() []string
	// ParseProject parses the given files into a single CodeModel, resolving
	// cross-file references (extends/implements/calls) where possible.
	// Files that fail to parse are reported as Diagnostics, not errors —
	// a single broken file should not abort analysis of the rest of the project.
	ParseProject(ctx context.Context, files []string) (*CodeModel, []Diagnostic, error)
}

// registry holds the analyzers available to the CLI, keyed by Language().
var registry = map[string]func() LanguageAnalyzer{}

// Register makes a LanguageAnalyzer factory available under its language name.
// Called from each lang/<name> package's init().
func Register(language string, factory func() LanguageAnalyzer) {
	registry[language] = factory
}

// Lookup returns the analyzer factory registered for language, if any.
func Lookup(language string) (func() LanguageAnalyzer, bool) {
	factory, ok := registry[language]
	return factory, ok
}

// SupportedLanguages lists every currently registered language.
func SupportedLanguages() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	return names
}
