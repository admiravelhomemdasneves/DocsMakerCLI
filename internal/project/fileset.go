// Package project locates the files a LanguageAnalyzer should parse and
// sniffs lightweight build-system metadata (Maven/Gradle) used later for
// README generation. It knows nothing about any specific language.
package project

import (
	"os"
	"path/filepath"
	"strings"

	gitignore "github.com/sabhiram/go-gitignore"
)

// alwaysIgnoredDirs are skipped even without a .gitignore entry — build
// output and VCS metadata are never source to document.
var alwaysIgnoredDirs = map[string]bool{
	".git":         true,
	"target":       true, // Maven
	"build":        true, // Gradle
	"out":          true,
	"node_modules": true,
	".docsmaker":   true,
}

// DiscoverFiles walks root and returns every file whose extension is in
// extensions (e.g. []string{".java"}), skipping anything matched by a
// .gitignore file found at root (nested .gitignore files are out of scope
// for MVP — a single root-level file covers the common case) and the
// always-ignored build/VCS directories above.
func DiscoverFiles(root string, extensions []string) ([]string, error) {
	extSet := make(map[string]bool, len(extensions))
	for _, e := range extensions {
		extSet[strings.ToLower(e)] = true
	}

	ignorer := loadGitignore(root)

	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}
		relSlash := filepath.ToSlash(rel)

		if d.IsDir() {
			if path != root && (alwaysIgnoredDirs[d.Name()] || (ignorer != nil && ignorer.MatchesPath(relSlash))) {
				return filepath.SkipDir
			}
			return nil
		}

		if ignorer != nil && ignorer.MatchesPath(relSlash) {
			return nil
		}

		if extSet[strings.ToLower(filepath.Ext(path))] {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return files, nil
}

// loadGitignore reads root/.gitignore if present. A missing file is not an
// error — plenty of valid projects don't have one.
func loadGitignore(root string) *gitignore.GitIgnore {
	path := filepath.Join(root, ".gitignore")
	if _, err := os.Stat(path); err != nil {
		return nil
	}
	ign, err := gitignore.CompileIgnoreFile(path)
	if err != nil {
		return nil
	}
	return ign
}
