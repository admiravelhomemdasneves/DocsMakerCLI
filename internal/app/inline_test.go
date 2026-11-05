package app

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"

	_ "github.com/admiravelhomemdasneves/DocsMakerCLI/internal/lang/java"
	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/langmodel"
)

func javaAnalyzer(t *testing.T) langmodel.LanguageAnalyzer {
	t.Helper()
	factory, ok := langmodel.Lookup("java")
	if !ok {
		t.Fatal("java analyzer not registered")
	}
	return factory()
}

func TestParseVerifier(t *testing.T) {
	verify := parseVerifier(javaAnalyzer(t))
	const orig = "package p;\npublic class A {\n    void f() {}\n}\n"
	ctx := context.Background()

	withDoc := "package p;\n/**\n * docs\n */\npublic class A {\n    void f() {}\n}\n"
	if err := verify(ctx, []byte(orig), []byte(withDoc)); err != nil {
		t.Fatalf("a doc comment must verify: %v", err)
	}
	if err := verify(ctx, []byte(orig), []byte("package p;\npublic class A {\n    void f() {}\n")); err == nil {
		t.Fatal("a file with a missing brace must be rejected")
	}
	if err := verify(ctx, []byte(orig), []byte("package p;\npublic class A {\n}\n")); err == nil {
		t.Fatal("a file that lost a method must be rejected")
	}
}

func TestInit_WiresARepoAndIsIdempotent(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	repo := t.TempDir()
	if out, err := exec.Command("git", "-C", repo, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}

	res, err := Init(context.Background(), InitOptions{RepoPath: repo, Executable: "/opt/docsmaker"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.AttributesChanged || !res.ConfigChanged || len(res.Patterns) == 0 || res.Patterns[0] != "*.java" {
		t.Fatalf("unexpected first result: %+v", res)
	}
	attrs, _ := os.ReadFile(res.AttributesFile)
	if !strings.Contains(string(attrs), "*.java filter=docsmaker-ai-doc") {
		t.Fatalf("attributes = %q", attrs)
	}

	res, err = Init(context.Background(), InitOptions{RepoPath: repo, Executable: "/opt/docsmaker"})
	if err != nil || res.AttributesChanged || res.ConfigChanged || res.ExcludeChanged {
		t.Fatalf("second run should be a no-op: %+v, %v", res, err)
	}
}
