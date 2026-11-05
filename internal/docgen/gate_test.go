package docgen

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/langmodel"
	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/llm"
	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/secrets"
)

const secretURL = "jdbc:mysql://admin:hunter2@localhost/db"

// recordingClient records every prompt it receives and answers via reply.
type recordingClient struct {
	prompts []string
	reply   func(llm.Request) string
}

func (c *recordingClient) Generate(_ context.Context, req llm.Request) (llm.Response, error) {
	c.prompts = append(c.prompts, req.Prompt)
	if c.reply != nil {
		return llm.Response{Text: c.reply(req)}, nil
	}
	return llm.Response{Text: "a harmless summary"}, nil
}

func (c *recordingClient) Available(context.Context) bool { return true }

// buildSecretModel writes a tiny class holding a (synthetic) credential in
// both a field initializer and a method body, and returns its CodeModel.
func buildSecretModel(t *testing.T) *langmodel.CodeModel {
	t.Helper()
	src := "package com.example;\n\npublic class Db {\n" +
		"    private final String url = \"" + secretURL + "\";\n\n" +
		"    public String url() {\n        return \"" + secretURL + "\";\n    }\n}\n"
	path := filepath.Join(t.TempDir(), "Db.java")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	fStart := strings.Index(src, "private final String url")
	fEnd := fStart + strings.Index(src[fStart:], ";") + 1
	mStart := strings.Index(src, "public String url()")
	mEnd := mStart + strings.Index(src[mStart:], "\n    }") + len("\n    }")

	decl := langmodel.TypeDecl{
		Kind: langmodel.KindClass, Name: "Db", Package: "com.example",
		Fields: []langmodel.Field{{
			Name: "url", Type: langmodel.TypeRef{Name: "String", External: true},
			SourceRange: langmodel.Range{File: path, StartByte: fStart, EndByte: fEnd, StartLine: 4},
		}},
		Methods: []langmodel.Method{{
			Name: "url", ReturnType: langmodel.TypeRef{Name: "String", External: true},
			SourceRange: langmodel.Range{File: path, StartByte: mStart, EndByte: mEnd, StartLine: 6},
		}},
		SourceRange: langmodel.Range{File: path, StartLine: 3},
	}
	return &langmodel.CodeModel{
		Language: "java",
		Packages: []langmodel.Package{{Name: "com.example", Types: []langmodel.TypeDecl{decl}}},
	}
}

type fixedApprover struct{ d secrets.Decision }

func (f fixedApprover) Resolve(context.Context, secrets.Finding) (secrets.Decision, error) {
	return f.d, nil
}

func gateWith(a secrets.Approver) *secrets.Gate {
	return &secrets.Gate{Scanner: secrets.DefaultScanner{}, Approver: a}
}

func TestGate_RedactKeepsSecretOutOfEveryPrompt(t *testing.T) {
	client := &recordingClient{}
	p := New(client, nil, "m")
	p.Gate = gateWith(fixedApprover{secrets.DecisionRedact})

	artifacts, err := p.Run(context.Background(), buildSecretModel(t), Options{Level: LevelClass, Target: "com.example.Db"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(client.prompts) == 0 {
		t.Fatal("expected the model to be called")
	}
	for i, prompt := range client.prompts {
		if strings.Contains(prompt, "hunter2") {
			t.Fatalf("prompt %d reached the model with the secret:\n%s", i, prompt)
		}
	}
	if !strings.Contains(client.prompts[len(client.prompts)-1], "[REDACTED:connection-string]") {
		t.Fatalf("class prompt should show the redaction placeholder:\n%s", client.prompts[len(client.prompts)-1])
	}
	if artifacts[len(artifacts)-1].Level != LevelClass {
		t.Fatalf("expected a class artifact last, got %+v", artifacts)
	}
}

func TestGate_SkipOmitsTheUnitWithoutCallingTheModel(t *testing.T) {
	client := &recordingClient{}
	p := New(client, nil, "m")
	p.Gate = gateWith(fixedApprover{secrets.DecisionSkip})

	artifacts, err := p.Run(context.Background(), buildSecretModel(t), Options{Level: LevelMethod, Target: "com.example.Db#url"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(client.prompts) != 0 {
		t.Fatalf("model was called %d times, want 0 for a skipped unit", len(client.prompts))
	}
	if artifacts[0].Content != OmittedSummary {
		t.Fatalf("content = %q, want the omitted marker", artifacts[0].Content)
	}
}

func TestGate_BlockApproverStopsBeforeTheModel(t *testing.T) {
	client := &recordingClient{}
	p := New(client, nil, "m")
	p.Gate = gateWith(secrets.BlockApprover{})

	_, err := p.Run(context.Background(), buildSecretModel(t), Options{Level: LevelClass, Target: "com.example.Db"})
	var blocked *secrets.BlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("err = %v, want *secrets.BlockedError", err)
	}
	if len(client.prompts) != 0 {
		t.Fatalf("model was called %d times, want 0 when blocked", len(client.prompts))
	}
}

func TestGate_OutputGateCatchesASecretTheModelEchoes(t *testing.T) {
	// The prompt is clean, but the "model" invents a credential in its answer.
	client := &recordingClient{reply: func(llm.Request) string {
		return "It connects using " + secretURL + " for everything."
	}}
	model := &langmodel.CodeModel{Packages: []langmodel.Package{{Name: "p", Types: []langmodel.TypeDecl{{
		Kind: langmodel.KindClass, Name: "C", Package: "p",
	}}}}}

	p := New(client, nil, "m")
	p.Gate = gateWith(fixedApprover{secrets.DecisionRedact})
	artifacts, err := p.Run(context.Background(), model, Options{Level: LevelClass, Target: "p.C"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := artifacts[len(artifacts)-1].Content; strings.Contains(got, "hunter2") {
		t.Fatalf("generated output kept the secret: %s", got)
	}

	p.Gate = gateWith(secrets.BlockApprover{})
	if _, err := p.Run(context.Background(), model, Options{Level: LevelClass, Target: "p.C"}); err == nil {
		t.Fatal("blocking mode should fail when the model output contains a secret")
	}
}
