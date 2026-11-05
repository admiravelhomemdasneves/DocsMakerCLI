package gitfilter

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// FilterName is the git filter driver name wired by `docsmaker init`.
const FilterName = "docsmaker-ai-doc"

var (
	// ErrGitNotFound means the git executable is not on PATH.
	ErrGitNotFound = errors.New("git executable not found in PATH")
	// ErrNotARepo means the path is not inside a git working tree.
	ErrNotARepo = errors.New("not inside a git repository")
)

type gitError struct {
	args []string
	msg  string
	err  error
}

func (e *gitError) Error() string {
	return fmt.Sprintf("git %s: %v: %s", strings.Join(e.args, " "), e.err, e.msg)
}

func (e *gitError) Unwrap() error { return e.err }

// runGit runs git in dir and returns trimmed stdout.
func runGit(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return "", ErrGitNotFound
		}
		msg := strings.TrimSpace(stderr.String())
		if strings.Contains(msg, "not a git repository") {
			return "", ErrNotARepo
		}
		return "", &gitError{args: args, msg: msg, err: err}
	}
	return strings.TrimSpace(stdout.String()), nil
}

// configGet returns the local value of key, or "" when it is unset.
func configGet(ctx context.Context, dir, key string) (string, error) {
	out, err := runGit(ctx, dir, "config", "--local", "--get", key)
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return "", nil
	}
	return out, err
}

// SetupOptions configures Setup.
type SetupOptions struct {
	RepoPath string
	// Executable is what git runs for the filter; it is recorded in the
	// repository's local config as an absolute path so it works even when
	// docsmaker is not on PATH.
	Executable string
	// Patterns are the attribute patterns to filter, e.g. "*.java".
	Patterns []string
	// Shared writes the attributes to the tracked .gitattributes instead of
	// the local-only .git/info/attributes.
	Shared bool
	// Force replaces conflicting existing configuration.
	Force bool
}

// SetupResult reports what Setup did.
type SetupResult struct {
	RepoRoot          string
	AttributesFile    string
	AttributesChanged bool
	ConfigChanged     bool
	ExcludeChanged    bool
}

// Setup wires the clean filter into the repository containing opts.RepoPath.
// It is idempotent: a second run changes nothing.
//
// Only a clean filter is configured. Smudge would be the identity transform,
// so registering it would just cost a process spawn per checkout. The filter
// is marked required, so if git cannot run it the add/commit fails instead of
// silently committing the AI lines.
func Setup(ctx context.Context, opts SetupOptions) (*SetupResult, error) {
	if opts.Executable == "" {
		return nil, errors.New("gitfilter: Setup needs an Executable")
	}
	if len(opts.Patterns) == 0 {
		return nil, errors.New("gitfilter: Setup needs at least one pattern")
	}

	root, err := runGit(ctx, opts.RepoPath, "rev-parse", "--show-toplevel")
	switch {
	case errors.Is(err, ErrNotARepo):
		return nil, fmt.Errorf("%s is not a git repository, nothing to wire", opts.RepoPath)
	case err != nil:
		return nil, err
	}
	res := &SetupResult{RepoRoot: root}

	attrs := filepath.Join(root, ".gitattributes")
	if !opts.Shared {
		if attrs, err = gitPath(ctx, root, "info/attributes"); err != nil {
			return nil, err
		}
	}
	res.AttributesFile = attrs
	if res.AttributesChanged, err = ensureAttributes(attrs, opts.Patterns, opts.Force); err != nil {
		return nil, err
	}

	if res.ConfigChanged, err = ensureConfig(ctx, root, opts); err != nil {
		return nil, err
	}

	exclude, err := gitPath(ctx, root, "info/exclude")
	if err != nil {
		return nil, err
	}
	if res.ExcludeChanged, err = appendLineIfMissing(exclude, ".docsmaker/"); err != nil {
		return nil, err
	}
	return res, nil
}

// gitPath resolves a path inside the git dir (correct for worktrees too).
func gitPath(ctx context.Context, root, rel string) (string, error) {
	p, err := runGit(ctx, root, "rev-parse", "--git-path", rel)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(root, p)
	}
	return p, nil
}

func ensureAttributes(path string, patterns []string, force bool) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	eol := "\n"
	if bytes.Contains(data, []byte("\r\n")) {
		eol = "\r\n"
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	for i := range lines {
		lines[i] = strings.TrimSuffix(lines[i], "\r")
	}

	want := "filter=" + FilterName
	changed := false
	for _, pattern := range patterns {
		found := false
		for i, line := range lines {
			fields := strings.Fields(line)
			if len(fields) == 0 || fields[0] != pattern {
				continue
			}
			ours, other := false, false
			for _, f := range fields[1:] {
				if f == want {
					ours = true
				} else if strings.HasPrefix(f, "filter=") {
					other = true
				}
			}
			switch {
			case ours:
				found = true
			case other:
				if !force {
					return false, fmt.Errorf("%s already assigns another filter to %s (%q); re-run with --force to replace it", path, pattern, line)
				}
				lines[i] = pattern + " " + want
				found, changed = true, true
			}
		}
		if !found {
			lines = append(lines, pattern+" "+want)
			changed = true
		}
	}
	if !changed {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	return true, os.WriteFile(path, []byte(strings.Join(lines, eol)+eol), 0o644)
}

func ensureConfig(ctx context.Context, root string, opts SetupOptions) (bool, error) {
	key := "filter." + FilterName + ".clean"
	exe := strings.ReplaceAll(opts.Executable, `\`, "/")
	want := `"` + exe + `" git-clean-filter clean`

	have, err := configGet(ctx, root, key)
	if err != nil {
		return false, err
	}
	changed := false
	if have != want {
		// A differing value that is still one of ours (binary moved or
		// upgraded) is refreshed silently; anything else needs --force.
		if have != "" && !strings.Contains(have, "git-clean-filter") && !opts.Force {
			return false, fmt.Errorf("git config %s is already set to %q; re-run with --force to replace it", key, have)
		}
		if _, err := runGit(ctx, root, "config", "--local", key, want); err != nil {
			return false, err
		}
		changed = true
	}

	reqKey := "filter." + FilterName + ".required"
	req, err := configGet(ctx, root, reqKey)
	if err != nil {
		return false, err
	}
	if req != "true" {
		if _, err := runGit(ctx, root, "config", "--local", reqKey, "true"); err != nil {
			return false, err
		}
		changed = true
	}
	return changed, nil
}

func appendLineIfMissing(path, line string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	for _, l := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(l) == line {
			return false, nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	prefix := ""
	if len(data) > 0 && !bytes.HasSuffix(data, []byte("\n")) {
		prefix = "\n"
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return false, err
	}
	defer f.Close()
	_, err = f.WriteString(prefix + line + "\n")
	return true, err
}

// EnsureWired fails unless every file in files would pass through the
// docsmaker clean filter when committed. Call it before writing AI content
// into a working tree. A project that is not inside a git repository passes:
// there is nothing to leak into.
func EnsureWired(ctx context.Context, projectPath string, files []string) error {
	root, err := runGit(ctx, projectPath, "rev-parse", "--show-toplevel")
	switch {
	case errors.Is(err, ErrNotARepo):
		return nil
	case errors.Is(err, ErrGitNotFound):
		if insideGitDir(projectPath) {
			return fmt.Errorf("%w: cannot verify that AI-generated lines will stay out of commits", err)
		}
		return nil
	case err != nil:
		return err
	}

	clean, err := configGet(ctx, root, "filter."+FilterName+".clean")
	if err != nil {
		return err
	}
	if clean == "" {
		return fmt.Errorf("the docsmaker git filter is not configured in %s; run `docsmaker init` first so AI-generated lines cannot be committed", root)
	}

	byDir := map[string][]string{}
	for _, f := range files {
		byDir[filepath.Dir(f)] = append(byDir[filepath.Dir(f)], filepath.Base(f))
	}
	var unfiltered []string
	for dir, names := range byDir {
		for start := 0; start < len(names); start += 100 {
			end := min(start+100, len(names))
			out, err := runGit(ctx, dir, append([]string{"check-attr", "-z", "filter", "--"}, names[start:end]...)...)
			if err != nil {
				return err
			}
			parts := strings.Split(out, "\x00")
			for i := 0; i+2 < len(parts); i += 3 {
				if parts[i+2] != FilterName {
					unfiltered = append(unfiltered, filepath.Join(dir, parts[i]))
				}
			}
		}
	}
	if len(unfiltered) > 0 {
		return fmt.Errorf("%d file(s) are not covered by the docsmaker git filter (first: %s); run `docsmaker init`", len(unfiltered), unfiltered[0])
	}
	return nil
}

func insideGitDir(path string) bool {
	dir, err := filepath.Abs(path)
	if err != nil {
		return true // can't tell: be conservative
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false
		}
		dir = parent
	}
}
