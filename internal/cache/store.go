// Package cache provides a content-hash-keyed store for LLM outputs, so
// re-running docsmaker over an unchanged project doesn't re-summarize code
// that hasn't changed. The key is derived from the exact request sent to the
// model (model name + prompt text), so any change to the source, the
// prompt template, or the chosen model naturally invalidates the cache —
// no separate staleness tracking needed.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
)

// Store is the cache port docgen depends on.
type Store interface {
	Get(key string) (string, bool)
	Set(key string, value string) error
}

// Key derives a cache key from the pieces that make an LLM request unique.
func Key(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte{0}) // separator, so ("ab","c") and ("a","bc") don't collide
		h.Write([]byte(p))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// FileStore persists cached values as one file per key under Dir (typically
// <project>/.docsmaker/cache — gitignored by `docsmaker init`).
type FileStore struct {
	Dir string
}

// NewFileStore returns a FileStore rooted at dir. The directory is created
// lazily on first Set, not here.
func NewFileStore(dir string) *FileStore {
	return &FileStore{Dir: dir}
}

func (s *FileStore) path(key string) string {
	return filepath.Join(s.Dir, key+".txt")
}

func (s *FileStore) Get(key string) (string, bool) {
	data, err := os.ReadFile(s.path(key))
	if err != nil {
		return "", false
	}
	return string(data), true
}

func (s *FileStore) Set(key string, value string) error {
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.path(key), []byte(value), 0o644)
}
