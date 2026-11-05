package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DefaultBaseURL is where Ollama listens by default; overridable via
// config/OLLAMA_HOST for non-default setups.
const DefaultBaseURL = "http://127.0.0.1:11434"

// DefaultModel is the out-of-the-box model docsmaker pulls and uses when no
// override is given — small enough for a reasonable first-run download and
// per-request latency, per the plan's bootstrap design.
const DefaultModel = "qwen2.5-coder:7b"

var _ Client = (*OllamaClient)(nil)

// OllamaClient implements llm.Client against a local Ollama instance. It is
// the only Client implementation in this codebase, deliberately: adding a
// remote/cloud implementation would defeat the "no code leaves the machine"
// guarantee, so that choice is enforced by what simply doesn't exist here,
// not by a runtime flag.
type OllamaClient struct {
	BaseURL string
	HTTP    *http.Client
}

// NewOllamaClient builds a client against baseURL (DefaultBaseURL if empty).
func NewOllamaClient(baseURL string) *OllamaClient {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &OllamaClient{
		BaseURL: baseURL,
		HTTP:    &http.Client{Timeout: 5 * time.Minute},
	}
}

type generateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	System string `json:"system,omitempty"`
	Stream bool   `json:"stream"`
	Format string `json:"format,omitempty"`
}

type generateResponse struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

// Generate sends req to Ollama's /api/generate with streaming disabled (a
// single JSON response is simplest for the request/response shape this
// pipeline needs; streaming is revisited only if a use case needs
// token-by-token output, e.g. a live console render).
func (c *OllamaClient) Generate(ctx context.Context, req Request) (Response, error) {
	body, err := json.Marshal(generateRequest{
		Model:  req.Model,
		Prompt: req.Prompt,
		System: req.System,
		Stream: false,
		Format: req.Format,
	})
	if err != nil {
		return Response{}, fmt.Errorf("llm: encode request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return Response{}, fmt.Errorf("llm: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return Response{}, fmt.Errorf("llm: request to local Ollama failed (is it running?): %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return Response{}, fmt.Errorf("llm: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return Response{}, fmt.Errorf("llm: ollama returned %s: %s", resp.Status, string(data))
	}

	var out generateResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return Response{}, fmt.Errorf("llm: decode response: %w", err)
	}

	return Response{Text: out.Response}, nil
}

// Available reports whether the local Ollama API is reachable, without
// attempting to install or start anything — see bootstrap.go for that.
func (c *OllamaClient) Available(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/version", nil)
	if err != nil {
		return false
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
