package gitfilter

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func newRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	return dir
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSetup_WiresFilterIdempotentlyAndEnsureWiredAgrees(t *testing.T) {
	repo := newRepo(t)
	java := filepath.Join(repo, "src", "A.java")
	writeFile(t, java, "class A {}\n")
	ctx := context.Background()

	if err := EnsureWired(ctx, repo, []string{java}); err == nil {
		t.Fatal("EnsureWired should fail before Setup")
	}

	opts := SetupOptions{RepoPath: repo, Executable: `C:\Program Files\docsmaker\docsmaker.exe`, Patterns: []string{"*.java"}}
	res, err := Setup(ctx, opts)
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if !res.AttributesChanged || !res.ConfigChanged || !res.ExcludeChanged {
		t.Fatalf("first run should change everything: %+v", res)
	}

	attrs, _ := os.ReadFile(res.AttributesFile)
	if !strings.Contains(string(attrs), "*.java filter="+FilterName) {
		t.Fatalf("attributes = %q", attrs)
	}
	if _, err := os.Stat(filepath.Join(repo, ".gitattributes")); err == nil {
		t.Fatal("default mode must not touch the tracked .gitattributes")
	}
	clean, _ := configGet(ctx, repo, "filter."+FilterName+".clean")
	if clean != `"C:/Program Files/docsmaker/docsmaker.exe" git-clean-filter clean` {
		t.Fatalf("clean command = %q", clean)
	}

	if err := EnsureWired(ctx, repo, []string{java}); err != nil {
		t.Fatalf("EnsureWired after Setup: %v", err)
	}
	other := filepath.Join(repo, "README.md")
	writeFile(t, other, "x")
	if err := EnsureWired(ctx, repo, []string{other}); err == nil {
		t.Fatal("a file outside the patterns must not count as covered")
	}

	res, err = Setup(ctx, opts)
	if err != nil {
		t.Fatalf("second Setup: %v", err)
	}
	if res.AttributesChanged || res.ConfigChanged || res.ExcludeChanged {
		t.Fatalf("second run should change nothing: %+v", res)
	}
}

func TestSetup_ConflictingFilterNeedsForce(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, filepath.Join(repo, ".git", "info", "attributes"), "*.java filter=other\n")
	opts := SetupOptions{RepoPath: repo, Executable: "/x/docsmaker", Patterns: []string{"*.java"}}

	if _, err := Setup(context.Background(), opts); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("err = %v, want a --force hint", err)
	}
	opts.Force = true
	res, err := Setup(context.Background(), opts)
	if err != nil {
		t.Fatalf("forced Setup: %v", err)
	}
	attrs, _ := os.ReadFile(res.AttributesFile)
	if strings.Contains(string(attrs), "other") || !strings.Contains(string(attrs), FilterName) {
		t.Fatalf("attributes = %q", attrs)
	}
}

func TestSetup_SharedWritesGitattributes(t *testing.T) {
	repo := newRepo(t)
	res, err := Setup(context.Background(), SetupOptions{RepoPath: repo, Executable: "/x/docsmaker", Patterns: []string{"*.java"}, Shared: true})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(res.AttributesFile) != ".gitattributes" {
		t.Fatalf("attributes file = %s", res.AttributesFile)
	}
}

func TestSetup_NotARepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	_, err := Setup(context.Background(), SetupOptions{RepoPath: t.TempDir(), Executable: "x", Patterns: []string{"*.java"}})
	if err == nil {
		t.Fatal("expected an error outside a repository")
	}
}

func TestEnsureWired_NonRepoPasses(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	f := filepath.Join(dir, "A.java")
	writeFile(t, f, "class A {}")
	if err := EnsureWired(context.Background(), dir, []string{f}); err != nil {
		t.Fatalf("a non-repo has nothing to leak into: %v", err)
	}
}
