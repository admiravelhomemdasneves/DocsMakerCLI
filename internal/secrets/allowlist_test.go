package secrets

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type failingApprover struct{}

func (failingApprover) Resolve(context.Context, Finding) (Decision, error) {
	return DecisionAbort, errors.New("the approver must not be asked")
}

func newAllowlist(t *testing.T) *FileAllowlist {
	t.Helper()
	a, err := LoadAllowlist(filepath.Join(t.TempDir(), ".docsmaker", "allowlist.json"))
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestAllowlist_StoresOnlyAHash(t *testing.T) {
	a := newAllowlist(t)
	if err := a.Add(KindHighEntropy, "s3cr3t-looking-VALUE-1234567", "Foo.java:3"); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(a.Path)
	if strings.Contains(string(raw), "s3cr3t-looking") {
		t.Fatalf("the value itself was written to disk:\n%s", raw)
	}

	reloaded, err := LoadAllowlist(a.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !reloaded.Allows("s3cr3t-looking-VALUE-1234567") || reloaded.Allows("something else") {
		t.Fatal("persisted entry should match exactly its own value")
	}
	if err := a.Add(KindHighEntropy, "s3cr3t-looking-VALUE-1234567", "Foo.java:9"); err != nil || len(a.Entries()) != 1 {
		t.Fatalf("adding the same value twice should not duplicate: %v %d", err, len(a.Entries()))
	}
}

func TestAllowlist_PatternsAreAnchoredAndNotTooBroad(t *testing.T) {
	a := newAllowlist(t)
	if err := a.AddPattern(`example-[a-z]{4,}`); err != nil {
		t.Fatal(err)
	}
	if !a.Allows("example-token") || a.Allows("my-example-token") || a.Allows("example-token-with-suffix1") {
		t.Fatal("pattern must match the whole value only")
	}
	for _, bad := range []string{`.*`, `.+`, `[a-z0-9]+`, `(`} {
		if err := a.AddPattern(bad); err == nil {
			t.Errorf("pattern %q should be rejected", bad)
		}
	}
}

func TestAllowlist_RemoveByPrefixOrPattern(t *testing.T) {
	a := newAllowlist(t)
	_ = a.Add(KindAPIToken, "value-one-aaaaaaaaaaaa", "A:1")
	_ = a.AddPattern(`ok-[0-9]{3}`)
	id := a.Entries()[0].SHA256

	if _, err := a.Remove(id[:3]); err == nil {
		t.Fatal("a too-short prefix must be rejected")
	}
	if _, err := a.Remove(id[:8]); err != nil || a.Allows("value-one-aaaaaaaaaaaa") {
		t.Fatalf("remove by prefix failed: %v", err)
	}
	if _, err := a.Remove(`ok-[0-9]{3}`); err != nil || a.Allows("ok-123") {
		t.Fatalf("remove pattern failed: %v", err)
	}
	if _, err := a.Remove("ffffffffff"); err == nil {
		t.Fatal("removing something absent should fail")
	}
}

func TestAllowlist_CorruptFileIsAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "allowlist.json")
	for _, content := range []string{"{not json", `{"version":1,"typo":[]}`, `{"version":9}`} {
		_ = os.WriteFile(path, []byte(content), 0o644)
		if _, err := LoadAllowlist(path); err == nil {
			t.Errorf("%q should not load silently", content)
		}
	}
}

func TestGate_AllowlistedValueIsNeverAsked_EvenWhenStrict(t *testing.T) {
	a := newAllowlist(t)
	_ = a.Add(KindConnString, `jdbc:mysql://admin:hunter2@localhost/db`, "x")

	g := &Gate{Scanner: DefaultScanner{}, Approver: BlockApprover{}, Allow: a}
	got, err := g.Screen(context.Background(), "w", dirty)
	if err != nil || got.Skip || got.Text != dirty {
		t.Fatalf("an allowlisted value should pass untouched: %+v %v", got, err)
	}

	// A different secret is still blocked.
	other := `url = "jdbc:mysql://root:another-pass@db.internal/prod";`
	if _, err := g.Screen(context.Background(), "w", other); err == nil {
		t.Fatal("a value that is not allowlisted must still be blocked")
	}
}

func TestGate_ChoosingAllowAlwaysPersistsAndStopsTheQuestions(t *testing.T) {
	al := newAllowlist(t)
	asks := &scriptedApprover{decision: DecisionAllowAlways}
	g := &Gate{Scanner: DefaultScanner{}, Approver: asks, Allow: al}

	got, err := g.Screen(context.Background(), "prompt for Foo (Foo.java:3)", dirty)
	if err != nil || got.Text != dirty || len(asks.asked) != 1 {
		t.Fatalf("first run: %+v %v asked=%d", got, err, len(asks.asked))
	}

	// Next run, new process: reload from disk; nobody can be asked any more.
	reloaded, _ := LoadAllowlist(al.Path)
	g2 := &Gate{Scanner: DefaultScanner{}, Approver: failingApprover{}, Allow: reloaded}
	if got, err := g2.Screen(context.Background(), "again", dirty); err != nil || got.Text != dirty {
		t.Fatalf("second run should be silent: %+v %v", got, err)
	}
	if e := reloaded.Entries(); len(e) != 1 || !strings.Contains(e[0].Where, "Foo.java:3") {
		t.Fatalf("entry should record where it came from: %+v", e)
	}
}

func TestGate_AllowAlwaysWithoutAnAllowlistDegradesToOnce(t *testing.T) {
	g := &Gate{Scanner: DefaultScanner{}, Approver: &scriptedApprover{decision: DecisionAllowAlways}}
	if got, err := g.Screen(context.Background(), "w", dirty); err != nil || got.Text != dirty {
		t.Fatalf("%+v %v", got, err)
	}
}
