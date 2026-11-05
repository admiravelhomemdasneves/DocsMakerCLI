package gitfilter

import (
	"bytes"
	"strings"
	"testing"
)

func TestClean_StripsSentinelBlock(t *testing.T) {
	input := `public class Foo {
/**
 * // docsmaker:ai-begin:v1
 * Valida o payload do pedido recebido.
 * @param req o pedido em bruto
 * // docsmaker:ai-end
 */
    void bar(String req) {}
}
`
	want := `public class Foo {
    void bar(String req) {}
}
`

	var out, warn bytes.Buffer
	if err := Clean(strings.NewReader(input), &out, &warn); err != nil {
		t.Fatalf("Clean returned error: %v", err)
	}
	if warn.Len() != 0 {
		t.Fatalf("unexpected warning: %s", warn.String())
	}
	if out.String() != want {
		t.Fatalf("Clean output mismatch:\ngot:\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestClean_LeavesUntaggedContentUntouched(t *testing.T) {
	input := `public class Foo {
    /** A perfectly normal, human-written comment. */
    void bar() {}
}
`
	var out, warn bytes.Buffer
	if err := Clean(strings.NewReader(input), &out, &warn); err != nil {
		t.Fatalf("Clean returned error: %v", err)
	}
	if out.String() != input {
		t.Fatalf("Clean should not touch untagged content:\ngot:\n%s\nwant:\n%s", out.String(), input)
	}
}

func TestClean_UnmatchedBeginPassesThroughWithWarning(t *testing.T) {
	input := `/**
 * // docsmaker:ai-begin:v1
 * dangling, no end sentinel
 */
`
	var out, warn bytes.Buffer
	if err := Clean(strings.NewReader(input), &out, &warn); err != nil {
		t.Fatalf("Clean returned error: %v", err)
	}
	if out.String() != input {
		t.Fatalf("malformed input should pass through unmodified:\ngot:\n%s\nwant:\n%s", out.String(), input)
	}
	if warn.Len() == 0 {
		t.Fatal("expected a warning for unmatched begin sentinel")
	}
}

func TestSmudge_IsIdentity(t *testing.T) {
	input := "anything at all\nwith multiple lines\n"
	var out bytes.Buffer
	if err := Smudge(strings.NewReader(input), &out); err != nil {
		t.Fatalf("Smudge returned error: %v", err)
	}
	if out.String() != input {
		t.Fatalf("Smudge should be identity:\ngot:\n%s\nwant:\n%s", out.String(), input)
	}
}
