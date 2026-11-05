package secrets

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Decision is the developer's response to a single Finding.
type Decision int

const (
	// DecisionRedact masks the matched span before it reaches the LLM prompt
	// or any output, but still lets generation proceed for that snippet.
	DecisionRedact Decision = iota
	// DecisionSkip omits the enclosing method/field from doc generation entirely.
	DecisionSkip
	// DecisionAllowOnce includes the snippet as-is for this occurrence only.
	// Never persisted as a standing "always allow" — every allowance is a
	// fresh decision, logged to the audit trail.
	DecisionAllowOnce
	// DecisionAbort stops the whole run.
	DecisionAbort
	// DecisionAllowAlways is chosen by the developer at a prompt: the value is
	// not a secret, include it and remember that (by hash) so it is never asked
	// about again.
	DecisionAllowAlways
	// DecisionAllowlisted is recorded when a value already on the allowlist
	// passes without a prompt.
	DecisionAllowlisted
)

func (d Decision) String() string {
	switch d {
	case DecisionRedact:
		return "redact"
	case DecisionSkip:
		return "skip"
	case DecisionAllowOnce:
		return "allow-once"
	case DecisionAbort:
		return "abort"
	case DecisionAllowAlways:
		return "allow-always"
	case DecisionAllowlisted:
		return "allowlisted"
	default:
		return "unknown"
	}
}

// Approver is the port that resolves a Finding into a Decision. The CLI
// adapter blocks on a terminal prompt; a future UI adapter (see plan's
// "Extensibilidade para UI") would resolve the same Finding via a callback
// or modal instead — internal/docgen never knows which one it's talking to.
type Approver interface {
	Resolve(ctx context.Context, f Finding) (Decision, error)
}

// BlockedError is returned when a finding is hard-blocked (strict mode, or
// non-interactive runs where nobody can be asked).
type BlockedError struct {
	Finding Finding
}

func (e *BlockedError) Error() string {
	return fmt.Sprintf(
		"sensitive information blocked (%s) in %s: %s — remove it from the source, "+
			"or re-run interactively with --secrets prompt (and without --yes) to redact, skip or explicitly allow it",
		e.Finding.Kind, e.Finding.Where, Mask(e.Finding.Match))
}

// BlockApprover never lets a finding through: every finding aborts the run.
// Used for --secrets strict and for non-interactive (--yes) runs, where
// "no answer" must never silently mean "allow".
type BlockApprover struct{}

func (BlockApprover) Resolve(_ context.Context, f Finding) (Decision, error) {
	return DecisionAbort, &BlockedError{Finding: f}
}

// TerminalApprover asks the developer at a terminal. Prompts go to Out
// (the CLI passes stderr, so stdout stays clean for results / JSON).
type TerminalApprover struct {
	in  *bufio.Reader
	out io.Writer
}

// NewTerminalApprover reads answers from in and writes prompts to out.
func NewTerminalApprover(in io.Reader, out io.Writer) *TerminalApprover {
	return &TerminalApprover{in: bufio.NewReader(in), out: out}
}

func (a *TerminalApprover) Resolve(ctx context.Context, f Finding) (Decision, error) {
	fmt.Fprintf(a.out, "\n[!] Potential sensitive information detected (%s)\n", f.Kind)
	fmt.Fprintf(a.out, "    where: %s\n", f.Where)
	fmt.Fprintf(a.out, "    value: %s\n", Mask(f.Match))

	for {
		if err := ctx.Err(); err != nil {
			return DecisionAbort, err
		}
		fmt.Fprint(a.out, "    [r] redact and continue  [s] skip this snippet  [a] allow once (include as-is)\n    [w] not a secret: include it and never ask again for this value  [q] abort run\n    choice [r]: ")

		line, err := a.in.ReadString('\n')
		if err != nil && strings.TrimSpace(line) == "" {
			if errors.Is(err, io.EOF) {
				err = errors.New("no input available (stdin closed); re-run with --secrets strict or fix the source")
			}
			return DecisionAbort, fmt.Errorf("secrets: confirmation prompt could not be answered: %w", err)
		}

		switch strings.ToLower(strings.TrimSpace(line)) {
		case "", "r":
			return DecisionRedact, nil
		case "s":
			return DecisionSkip, nil
		case "a":
			return DecisionAllowOnce, nil
		case "w":
			return DecisionAllowAlways, nil
		case "q":
			return DecisionAbort, nil
		default:
			fmt.Fprintln(a.out, "    unrecognized choice, please answer r, s, a, w or q")
		}
	}
}
