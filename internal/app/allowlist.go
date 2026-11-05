package app

import (
	"path/filepath"

	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/secrets"
)

func allowlistPath(projectPath string) string {
	return filepath.Join(projectPath, ".docsmaker", "allowlist.json")
}

// AllowlistResult describes the secrets allowlist of a project.
type AllowlistResult struct {
	Path     string               `json:"path"`
	Entries  []secrets.AllowEntry `json:"entries"`
	Patterns []string             `json:"patterns"`
}

// AllowlistShow returns what is currently allowlisted. Entries hold only
// hashes of values, never the values.
func AllowlistShow(projectPath string) (*AllowlistResult, error) {
	al, err := secrets.LoadAllowlist(allowlistPath(projectPath))
	if err != nil {
		return nil, err
	}
	return &AllowlistResult{Path: al.Path, Entries: al.Entries(), Patterns: al.Patterns()}, nil
}

// AllowlistRemove deletes an entry (by sha256 prefix) or a pattern, and
// returns a description of what was removed.
func AllowlistRemove(projectPath, id string) (string, error) {
	al, err := secrets.LoadAllowlist(allowlistPath(projectPath))
	if err != nil {
		return "", err
	}
	return al.Remove(id)
}

// AllowlistAddPattern allows every value that fully matches the regular expression.
func AllowlistAddPattern(projectPath, pattern string) error {
	al, err := secrets.LoadAllowlist(allowlistPath(projectPath))
	if err != nil {
		return err
	}
	return al.AddPattern(pattern)
}
