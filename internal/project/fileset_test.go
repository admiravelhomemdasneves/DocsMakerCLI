package project

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverFiles_RespectsGitignoreAndAlwaysIgnoredDirs(t *testing.T) {
	root := t.TempDir()

	writeFile(t, filepath.Join(root, ".gitignore"), "generated/\n*.tmp.java\n")
	writeFile(t, filepath.Join(root, "src", "Foo.java"), "class Foo {}")
	writeFile(t, filepath.Join(root, "src", "Bar.tmp.java"), "class Bar {}") // matched by *.tmp.java
	writeFile(t, filepath.Join(root, "generated", "Gen.java"), "class Gen {}")
	writeFile(t, filepath.Join(root, "target", "classes", "Baked.java"), "class Baked {}")
	writeFile(t, filepath.Join(root, "README.md"), "not java")

	files, err := DiscoverFiles(root, []string{".java"})
	if err != nil {
		t.Fatalf("DiscoverFiles returned error: %v", err)
	}

	var rel []string
	for _, f := range files {
		r, _ := filepath.Rel(root, f)
		rel = append(rel, filepath.ToSlash(r))
	}
	sort.Strings(rel)

	want := []string{"src/Foo.java"}
	if len(rel) != len(want) || rel[0] != want[0] {
		t.Fatalf("DiscoverFiles = %v, want %v", rel, want)
	}
}

func TestDetectBuildSystem_Maven(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "pom.xml"), `<project><artifactId>my-app</artifactId></project>`)

	info := DetectBuildSystem(root)
	if info.System != BuildSystemMaven {
		t.Fatalf("System = %v, want maven", info.System)
	}
	if info.Name != "my-app" {
		t.Fatalf("Name = %q, want my-app", info.Name)
	}
}

func TestDetectBuildSystem_NoneFallsBackToDirName(t *testing.T) {
	root := t.TempDir()
	info := DetectBuildSystem(root)
	if info.System != BuildSystemNone {
		t.Fatalf("System = %v, want none", info.System)
	}
	if info.Name != filepath.Base(root) {
		t.Fatalf("Name = %q, want %q", info.Name, filepath.Base(root))
	}
}
