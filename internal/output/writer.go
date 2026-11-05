// Package output implements where generated documentation ends up: the
// console (default for --mode console), a gitignored sidecar directory
// (default safe mode), or handed off to internal/inserter for inline
// insertion (--mode inline).
package output

import "context"

// Artifact is a single piece of generated documentation ready to be written.
// Level distinguishes method/class/package/project docs so a Writer can
// render or route each differently if it wants to.
type Artifact struct {
	Level   string // "method", "class", "package", "project"
	Target  string // fully-qualified name the artifact documents, e.g. "com.foo.Bar#baz" or "com.foo.Bar"
	Content string
}

// Writer is the port for emitting a generated Artifact. internal/app calls
// exactly one Writer per run, selected by --mode.
type Writer interface {
	Write(ctx context.Context, artifact Artifact) error
}
