// Package inserter splices AI-generated documentation directly into source
// files (--mode inline), wrapping each inserted block in the sentinel
// markers that internal/gitfilter later strips before a commit.
package inserter

import (
	"context"

	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/langmodel"
)

// Insertion asks for Content to be written as a doc comment directly above
// the declaration at Target.
type Insertion struct {
	Target  langmodel.Range
	Name    string // for reporting only, e.g. "com.foo.Bar#baz"
	Content string
}

// Action is what happened to one Insertion.
type Action string

const (
	ActionInserted  Action = "inserted"  // a new block was written
	ActionReplaced  Action = "replaced"  // an earlier AI block was refreshed
	ActionUnchanged Action = "unchanged" // the existing AI block already matched
	ActionSkipped   Action = "skipped"   // left alone; see Outcome.Reason
)

// Outcome reports the result for one Insertion.
type Outcome struct {
	Insertion
	Action Action
	Reason string
}

// Inserter is the port for writing generated doc blocks back into a source
// file at the right location for each declaration.
type Inserter interface {
	// InsertDocs applies all insertions for one file atomically: either the
	// file is rewritten with every applicable block, or it is left untouched
	// and an error is returned. Implementations must verify the result still
	// parses (and declares the same things) before replacing the file.
	InsertDocs(ctx context.Context, file string, insertions []Insertion) ([]Outcome, error)
}
