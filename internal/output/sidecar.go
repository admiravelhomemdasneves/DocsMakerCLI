package output

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SidecarWriter writes generated artifacts under a project-local directory
// (by convention <project>/.docsmaker/doc, gitignored by `docsmaker init`)
// instead of touching any tracked source file — the safe default output
// mode described in the plan.
type SidecarWriter struct {
	Dir string
}

var _ Writer = (*SidecarWriter)(nil)

// NewSidecarWriter returns a SidecarWriter rooted at dir.
func NewSidecarWriter(dir string) *SidecarWriter {
	return &SidecarWriter{Dir: dir}
}

func (w *SidecarWriter) Write(ctx context.Context, artifact Artifact) error {
	dir := filepath.Join(w.Dir, artifact.Level)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("output: creating sidecar dir %s: %w", dir, err)
	}

	name := sanitizeFilename(artifact.Target)
	if name == "" {
		name = "project"
	}
	path := filepath.Join(dir, name+".md")

	if err := os.WriteFile(path, []byte(artifact.Content), 0o644); err != nil {
		return fmt.Errorf("output: writing %s: %w", path, err)
	}
	return nil
}

// sanitizeFilename turns a target like "com.example.Dog#greet" into a safe
// filename component.
func sanitizeFilename(target string) string {
	replacer := strings.NewReplacer("#", "__", string(filepath.Separator), "_", "/", "_")
	return replacer.Replace(target)
}
