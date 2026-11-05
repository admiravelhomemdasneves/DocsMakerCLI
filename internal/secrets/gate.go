package secrets

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ErrAborted is returned when the developer chooses to abort the run at a
// confirmation prompt.
var ErrAborted = errors.New("run aborted at a sensitive-information prompt")

// AuditLog records every decision taken at a gate, so a reviewer can see
// afterwards what was redacted, skipped or explicitly allowed. Entries
// carry only a masked value, never the full secret.
type AuditLog interface {
	Record(decision Decision, f Finding)
}

// FileAuditLog appends one line per decision to Path (conventionally
// <project>/.docsmaker/audit.log, gitignored).
type FileAuditLog struct {
	Path string
}

func (l *FileAuditLog) Record(decision Decision, f Finding) {
	if err := os.MkdirAll(filepath.Dir(l.Path), 0o755); err != nil {
		return
	}
	file, err := os.OpenFile(l.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return // auditing must never take the run down
	}
	defer file.Close()
	fmt.Fprintf(file, "%s decision=%s kind=%s where=%q value=%q\n",
		time.Now().UTC().Format(time.RFC3339), decision, f.Kind, f.Where, Mask(f.Match))
}

// Screened is the outcome of screening one piece of text.
type Screened struct {
	// Text is the text to use going forward: the original, or a redacted copy.
	Text string
	// Skip means the developer asked to omit whatever this text belongs to.
	Skip bool
}

// Gate screens text with a Scanner and resolves every finding through an
// Approver. docgen calls it twice per unit of work: on the prompt before it
// reaches the model, and on the model's output before it is kept.
type Gate struct {
	Scanner  Scanner
	Approver Approver
	Audit    AuditLog // optional
	// Allow is consulted before the Approver: a value on it passes without a
	// prompt, in every policy including strict and --yes (it is a decision the
	// developer already took explicitly). Optional.
	Allow Allowlist
}

// Screen scans text and applies the developer's decision for each finding.
// where describes what is being screened (it is shown in prompts and logged).
func (g *Gate) Screen(ctx context.Context, where, text string) (Screened, error) {
	findings := g.Scanner.Scan(text)
	if len(findings) == 0 {
		return Screened{Text: text}, nil
	}

	var toRedact []Finding
	decided := map[string]Decision{} // same value in the same text: ask once

	for _, f := range findings {
		f.Where = where

		key := string(f.Kind) + "\x00" + f.Match
		d, seen := decided[key]
		if !seen {
			if g.Allow != nil && g.Allow.Allows(f.Match) {
				d = DecisionAllowlisted
			} else {
				var err error
				d, err = g.Approver.Resolve(ctx, f)
				if err != nil {
					var blocked *BlockedError
					if errors.As(err, &blocked) {
						g.record(DecisionAbort, f)
					}
					return Screened{}, err
				}
				if d == DecisionAllowAlways {
					if g.Allow == nil {
						d = DecisionAllowOnce // nowhere to remember it
					} else if err := g.Allow.Add(f.Kind, f.Match, f.Where); err != nil {
						return Screened{}, fmt.Errorf("secrets: saving allowlist entry: %w", err)
					}
				}
			}
			decided[key] = d
			g.record(d, f)
		}

		switch d {
		case DecisionRedact:
			toRedact = append(toRedact, f)
		case DecisionSkip:
			return Screened{Skip: true}, nil
		case DecisionAbort:
			return Screened{}, ErrAborted
		case DecisionAllowOnce, DecisionAllowAlways, DecisionAllowlisted:
			// included as-is, nothing to do
		}
	}

	return Screened{Text: Redact(text, toRedact)}, nil
}

func (g *Gate) record(d Decision, f Finding) {
	if g.Audit != nil {
		g.Audit.Record(d, f)
	}
}
