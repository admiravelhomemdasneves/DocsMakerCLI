// Package llm defines the port for talking to a local language model, plus
// the Ollama adapter that implements it. Nothing in internal/docgen imports
// Ollama directly — it only depends on the Client interface below, so a
// different local runtime could be swapped in without touching the pipeline.
package llm

import "context"

// Request is a single generation request. Prompt is fully assembled by the
// caller (internal/docgen/promptbuilder.go) — this package has no opinion on
// prompt structure, only on how to execute one against the local model.
type Request struct {
	Model  string
	System string
	Prompt string
	// Format constrains the model's output. "" is free text; "json" makes the
	// runtime emit syntactically valid JSON, which small local models need to
	// answer structured prompts reliably. It says nothing about the shape.
	Format string
}

// Response is the model's generated text for a Request.
type Response struct {
	Text string
}

// Client is the port every LLM runtime adapter implements. It is
// intentionally local-only by construction: no implementation in this
// codebase is allowed to call out to a remote/cloud endpoint. Adding such an
// implementation would defeat the "no code leaves the machine" guarantee the
// whole tool exists to provide.
type Client interface {
	// Generate runs req against the local model and returns its output.
	Generate(ctx context.Context, req Request) (Response, error)
	// Available reports whether the local runtime is installed and reachable
	// without attempting to install or start anything (see bootstrap.go for that).
	Available(ctx context.Context) bool
}
