package secrets

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Allowlist remembers values the developer has declared not to be secrets
// (a build id that looks random, a documented sample token...), so they are
// not asked about again.
type Allowlist interface {
	// Allows reports whether value was declared harmless.
	Allows(value string) bool
	// Add remembers value. where is the location it was found at.
	Add(kind Kind, value, where string) error
}

// AllowEntry is one allowlisted value. Only a hash of the value is stored,
// never the value: the file must not become a second place secrets live.
// The hash of a weak value can still be recovered by guessing, so the file is
// local to the machine (.docsmaker/ is excluded from git by `docsmaker init`)
// and must not be committed.
type AllowEntry struct {
	SHA256 string `json:"sha256"`
	Kind   string `json:"kind"`
	Where  string `json:"where"` // location, for review
	Added  string `json:"added"`
}

type allowlistFile struct {
	Version  int          `json:"version"`
	Entries  []AllowEntry `json:"entries"`
	Patterns []string     `json:"patterns,omitempty"`
}

// FileAllowlist is an Allowlist stored as JSON at Path.
type FileAllowlist struct {
	Path string

	mu       sync.Mutex
	data     allowlistFile
	compiled []*regexp.Regexp
}

// LoadAllowlist reads path; a missing file is an empty allowlist. A file
// that exists but cannot be understood is an error, so that a broken
// allowlist is noticed instead of silently ignored.
func LoadAllowlist(path string) (*FileAllowlist, error) {
	a := &FileAllowlist{Path: path, data: allowlistFile{Version: 1}}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return a, nil
	}
	if err != nil {
		return nil, fmt.Errorf("secrets: reading allowlist: %w", err)
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&a.data); err != nil {
		return nil, fmt.Errorf("secrets: allowlist %s is not valid: %w", path, err)
	}
	if a.data.Version != 1 {
		return nil, fmt.Errorf("secrets: allowlist %s has unsupported version %d", path, a.data.Version)
	}
	for _, p := range a.data.Patterns {
		re, err := compilePattern(p)
		if err != nil {
			return nil, fmt.Errorf("secrets: allowlist %s: %w", path, err)
		}
		a.compiled = append(a.compiled, re)
	}
	return a, nil
}

func hashValue(v string) string {
	sum := sha256.Sum256([]byte(v))
	return hex.EncodeToString(sum[:])
}

// Allows implements Allowlist.
func (a *FileAllowlist) Allows(value string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	h := hashValue(value)
	for _, e := range a.data.Entries {
		if e.SHA256 == h {
			return true
		}
	}
	for _, re := range a.compiled {
		if re.MatchString(value) {
			return true
		}
	}
	return false
}

// Add implements Allowlist and saves immediately.
func (a *FileAllowlist) Add(kind Kind, value, where string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	h := hashValue(value)
	for _, e := range a.data.Entries {
		if e.SHA256 == h {
			return nil
		}
	}
	a.data.Entries = append(a.data.Entries, AllowEntry{
		SHA256: h, Kind: string(kind), Where: where, Added: time.Now().UTC().Format("2006-01-02"),
	})
	return a.saveLocked()
}

// Entries returns a copy of the stored entries.
func (a *FileAllowlist) Entries() []AllowEntry {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]AllowEntry(nil), a.data.Entries...)
}

// Patterns returns a copy of the stored patterns.
func (a *FileAllowlist) Patterns() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.data.Patterns...)
}

// Remove deletes the entry whose hash starts with idPrefix (at least 6
// characters, and unambiguous) or the pattern equal to idPrefix.
func (a *FileAllowlist) Remove(idPrefix string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	for i, p := range a.data.Patterns {
		if p == idPrefix {
			a.data.Patterns = append(a.data.Patterns[:i], a.data.Patterns[i+1:]...)
			a.compiled = append(a.compiled[:i], a.compiled[i+1:]...)
			return "pattern " + p, a.saveLocked()
		}
	}
	if len(idPrefix) < 6 {
		return "", errors.New("secrets: give at least 6 characters of the entry's sha256 (see `docsmaker allowlist list`)")
	}
	match := -1
	for i, e := range a.data.Entries {
		if strings.HasPrefix(e.SHA256, strings.ToLower(idPrefix)) {
			if match >= 0 {
				return "", fmt.Errorf("secrets: %q matches more than one entry; give more characters", idPrefix)
			}
			match = i
		}
	}
	if match < 0 {
		return "", fmt.Errorf("secrets: no allowlist entry or pattern matches %q", idPrefix)
	}
	id := a.data.Entries[match].SHA256[:12]
	a.data.Entries = append(a.data.Entries[:match], a.data.Entries[match+1:]...)
	return "entry " + id, a.saveLocked()
}

// AddPattern allows every value that matches the regular expression in full.
func (a *FileAllowlist) AddPattern(p string) error {
	re, err := compilePattern(p)
	if err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, existing := range a.data.Patterns {
		if existing == p {
			return nil
		}
	}
	a.data.Patterns = append(a.data.Patterns, p)
	a.compiled = append(a.compiled, re)
	return a.saveLocked()
}

// compilePattern anchors p to the whole value and refuses patterns so broad
// that they would switch detection off (".*", ".+", "[A-Za-z0-9]+"...).
func compilePattern(p string) (*regexp.Regexp, error) {
	re, err := regexp.Compile(`^(?:` + p + `)$`)
	if err != nil {
		return nil, fmt.Errorf("invalid allowlist pattern %q: %w", p, err)
	}
	for _, sample := range []string{"", "AKIAIOSFODNN7EXAMPLE", "hunter2", "x"} {
		if re.MatchString(sample) {
			return nil, fmt.Errorf("allowlist pattern %q is too broad: it would also allow %q", p, sample)
		}
	}
	return re, nil
}

func (a *FileAllowlist) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(a.Path), 0o755); err != nil {
		return err
	}
	out, err := json.MarshalIndent(a.data, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(a.Path), ".allowlist-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	_, werr := tmp.Write(append(out, '\n'))
	cerr := tmp.Close()
	if err := errors.Join(werr, cerr); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, a.Path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}
