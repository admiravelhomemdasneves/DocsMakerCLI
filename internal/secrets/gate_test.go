package secrets

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const dirty = `url = "jdbc:mysql://admin:hunter2@localhost/db";`

// scriptedApprover answers every finding with a fixed decision and counts asks.
type scriptedApprover struct {
	decision Decision
	asked    []Finding
}

func (s *scriptedApprover) Resolve(_ context.Context, f Finding) (Decision, error) {
	s.asked = append(s.asked, f)
	return s.decision, nil
}

func newGate(a Approver) *Gate { return &Gate{Scanner: DefaultScanner{}, Approver: a} }

func TestGate_CleanTextNeverAsks(t *testing.T) {
	a := &scriptedApprover{}
	got, err := newGate(a).Screen(context.Background(), "x", "public int add(int a, int b) { return a + b; }")
	if err != nil || got.Skip || len(a.asked) != 0 {
		t.Fatalf("got %+v err=%v asked=%d, want untouched text and no prompts", got, err, len(a.asked))
	}
}

func TestGate_Redact(t *testing.T) {
	a := &scriptedApprover{decision: DecisionRedact}
	got, err := newGate(a).Screen(context.Background(), "prompt for X", dirty)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got.Text, "hunter2") || !strings.Contains(got.Text, "[REDACTED:connection-string]") {
		t.Fatalf("not redacted: %q", got.Text)
	}
	if a.asked[0].Where != "prompt for X" {
		t.Fatalf("Where = %q, want it set by the gate", a.asked[0].Where)
	}
}

func TestGate_Skip(t *testing.T) {
	got, err := newGate(&scriptedApprover{decision: DecisionSkip}).Screen(context.Background(), "x", dirty)
	if err != nil || !got.Skip {
		t.Fatalf("got %+v err=%v, want Skip", got, err)
	}
}

func TestGate_AllowOnceKeepsText(t *testing.T) {
	got, err := newGate(&scriptedApprover{decision: DecisionAllowOnce}).Screen(context.Background(), "x", dirty)
	if err != nil || got.Text != dirty {
		t.Fatalf("got %+v err=%v, want original text", got, err)
	}
}

func TestGate_AbortReturnsErrAborted(t *testing.T) {
	_, err := newGate(&scriptedApprover{decision: DecisionAbort}).Screen(context.Background(), "x", dirty)
	if !errors.Is(err, ErrAborted) {
		t.Fatalf("err = %v, want ErrAborted", err)
	}
}

func TestGate_BlockApproverHardBlocks(t *testing.T) {
	_, err := newGate(BlockApprover{}).Screen(context.Background(), "prompt for X", dirty)
	var blocked *BlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("err = %v, want *BlockedError", err)
	}
	if strings.Contains(err.Error(), "hunter2") {
		t.Fatalf("error message leaked the secret: %v", err)
	}
}

func TestGate_SameValueAskedOncePerText(t *testing.T) {
	a := &scriptedApprover{decision: DecisionRedact}
	text := dirty + "\n" + dirty
	got, err := newGate(a).Screen(context.Background(), "x", text)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.asked) != 1 {
		t.Fatalf("asked %d times, want 1 for an identical value", len(a.asked))
	}
	if strings.Contains(got.Text, "hunter2") {
		t.Fatalf("second occurrence not redacted: %q", got.Text)
	}
}

func TestGate_AuditLogNeverHoldsFullSecret(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	gate := newGate(&scriptedApprover{decision: DecisionAllowOnce})
	gate.Audit = &FileAuditLog{Path: path}

	if _, err := gate.Screen(context.Background(), "prompt for X", dirty); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	log := string(data)
	if !strings.Contains(log, "decision=allow-once") || !strings.Contains(log, "kind=connection-string") {
		t.Fatalf("audit line missing fields: %s", log)
	}
	if strings.Contains(log, "hunter2") {
		t.Fatalf("audit log leaked the secret: %s", log)
	}
}

func TestTerminalApprover_Choices(t *testing.T) {
	cases := []struct {
		input string
		want  Decision
	}{
		{"r\n", DecisionRedact},
		{"\n", DecisionRedact}, // default is the safe option
		{"s\n", DecisionSkip},
		{"A\n", DecisionAllowOnce},
		{"q\n", DecisionAbort},
		{"zzz\nr\n", DecisionRedact}, // invalid answer re-prompts
	}
	for _, tc := range cases {
		var out bytes.Buffer
		a := NewTerminalApprover(strings.NewReader(tc.input), &out)
		got, err := a.Resolve(context.Background(), Finding{Kind: KindConnString, Match: "jdbc:mysql://u:hunter2@h/db", Where: "w"})
		if err != nil || got != tc.want {
			t.Errorf("input %q: got %v err=%v, want %v", tc.input, got, err, tc.want)
		}
		if strings.Contains(out.String(), "hunter2") {
			t.Errorf("input %q: prompt printed the full secret: %s", tc.input, out.String())
		}
	}
}

func TestTerminalApprover_EOFAborts(t *testing.T) {
	a := NewTerminalApprover(strings.NewReader(""), &bytes.Buffer{})
	got, err := a.Resolve(context.Background(), Finding{Kind: KindGenericSecret, Match: "hunter2"})
	if err == nil || got != DecisionAbort {
		t.Fatalf("got %v err=%v, want abort with an error when stdin is closed", got, err)
	}
}
