package docgen

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/andresantos/docsmakercli/internal/cache"
	"github.com/andresantos/docsmakercli/internal/langmodel"
	"github.com/andresantos/docsmakercli/internal/llm"
)

// fakeClient is a deterministic, in-memory llm.Client stand-in: no network,
// no Ollama required, so pipeline routing/caching logic can be tested fast
// and without depending on a local model being installed.
type fakeClient struct {
	calls int
}

func (f *fakeClient) Generate(ctx context.Context, req llm.Request) (llm.Response, error) {
	f.calls++
	return llm.Response{Text: fmt.Sprintf("summary #%d for: %.30s", f.calls, req.Prompt)}, nil
}

func (f *fakeClient) Available(ctx context.Context) bool { return true }

var _ llm.Client = (*fakeClient)(nil)

// buildTestModel writes one small Java file to disk (so readSpan can find
// real source text) and returns the corresponding hand-built CodeModel —
// avoids depending on internal/lang/java (and therefore CGo) just to test
// docgen's own orchestration logic.
func buildTestModel(t *testing.T) *langmodel.CodeModel {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "Greeter.java")
	src := "package com.example;\n\npublic class Greeter {\n    public String greet(String name) {\n        return \"hi \" + name;\n    }\n}\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	greetStart := len("package com.example;\n\npublic class Greeter {\n    ")
	greetEnd := greetStart + len("public String greet(String name) {\n        return \"hi \" + name;\n    }")

	method := langmodel.Method{
		Name:       "greet",
		Params:     []langmodel.Param{{Name: "name", Type: langmodel.TypeRef{Name: "String", External: true}}},
		ReturnType: langmodel.TypeRef{Name: "String", External: true},
		SourceRange: langmodel.Range{
			File:      path,
			StartByte: greetStart,
			EndByte:   greetEnd,
		},
	}

	decl := langmodel.TypeDecl{
		Kind:    langmodel.KindClass,
		Name:    "Greeter",
		Package: "com.example",
		Methods: []langmodel.Method{method},
	}

	return &langmodel.CodeModel{
		Language: "java",
		Packages: []langmodel.Package{{Name: "com.example", Types: []langmodel.TypeDecl{decl}}},
	}
}

func TestRun_ProjectLevel_ProducesMethodClassPackageProjectArtifacts(t *testing.T) {
	model := buildTestModel(t)
	client := &fakeClient{}
	p := New(client, nil, "test-model")

	artifacts, err := p.Run(context.Background(), model, Options{Level: LevelProject, ProjectName: "sample"})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	var levels []Level
	for _, a := range artifacts {
		levels = append(levels, a.Level)
	}
	want := []Level{LevelMethod, LevelClass, LevelPackage, LevelProject}
	if len(levels) != len(want) {
		t.Fatalf("levels = %v, want %v", levels, want)
	}
	for i := range want {
		if levels[i] != want[i] {
			t.Fatalf("levels = %v, want %v", levels, want)
		}
	}

	// method -> class -> package -> project means 4 LLM calls for this
	// single-method, single-class, single-package model.
	if client.calls != 4 {
		t.Fatalf("client.calls = %d, want 4", client.calls)
	}

	readme := artifacts[len(artifacts)-1]
	if readme.Level != LevelProject || readme.Content == "" {
		t.Fatalf("expected a non-empty project artifact, got %+v", readme)
	}
}

func TestRun_ClassLevel_TargetsSpecificClass(t *testing.T) {
	model := buildTestModel(t)
	client := &fakeClient{}
	p := New(client, nil, "test-model")

	artifacts, err := p.Run(context.Background(), model, Options{Level: LevelClass, Target: "com.example.Greeter"})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(artifacts) != 2 { // method + class, no package/project
		t.Fatalf("got %d artifacts, want 2 (method + class)", len(artifacts))
	}
	if artifacts[1].Level != LevelClass || artifacts[1].Target != "com.example.Greeter" {
		t.Fatalf("artifacts[1] = %+v, want class-level com.example.Greeter", artifacts[1])
	}
}

func TestRun_MethodLevel_UnknownTargetErrors(t *testing.T) {
	model := buildTestModel(t)
	p := New(&fakeClient{}, nil, "test-model")

	_, err := p.Run(context.Background(), model, Options{Level: LevelMethod, Target: "com.example.Greeter#nope"})
	if err == nil {
		t.Fatal("expected an error for an unknown method target")
	}
}

func TestRun_CachingAvoidsRepeatedLLMCalls(t *testing.T) {
	model := buildTestModel(t)
	client := &fakeClient{}
	store := cache.NewFileStore(filepath.Join(t.TempDir(), "cache"))
	p := New(client, store, "test-model")

	if _, err := p.Run(context.Background(), model, Options{Level: LevelProject, ProjectName: "sample"}); err != nil {
		t.Fatalf("first Run returned error: %v", err)
	}
	firstCalls := client.calls

	if _, err := p.Run(context.Background(), model, Options{Level: LevelProject, ProjectName: "sample"}); err != nil {
		t.Fatalf("second Run returned error: %v", err)
	}

	if client.calls != firstCalls {
		t.Fatalf("second Run made %d additional LLM calls, want 0 (everything should hit cache)", client.calls-firstCalls)
	}
}
