package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// captureOllama serves one canned answer and records the decoded request body.
func captureOllama(t *testing.T, answer string) (*OllamaClient, *map[string]any) {
	t.Helper()
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/generate" {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("bad request body: %v", err)
		}
		json.NewEncoder(w).Encode(map[string]any{"response": answer, "done": true})
	}))
	t.Cleanup(srv.Close)
	return NewOllamaClient(srv.URL), &got
}

func TestGenerateSendsFormatOnlyWhenSet(t *testing.T) {
	c, got := captureOllama(t, "hello")

	if _, err := c.Generate(context.Background(), Request{Model: "m", System: "s", Prompt: "p"}); err != nil {
		t.Fatal(err)
	}
	if _, present := (*got)["format"]; present {
		t.Errorf("free-text request must not carry a format field, got %v", *got)
	}

	resp, err := c.Generate(context.Background(), Request{Model: "m", Prompt: "p", Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	if (*got)["format"] != "json" {
		t.Errorf("format = %v, want json", (*got)["format"])
	}
	if (*got)["stream"] != false || (*got)["model"] != "m" || (*got)["prompt"] != "p" {
		t.Errorf("unexpected request body: %v", *got)
	}
	if resp.Text != "hello" {
		t.Errorf("Text = %q, want hello", resp.Text)
	}
}
