package inserter

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andresantos/docsmakercli/internal/gitfilter"
	"github.com/andresantos/docsmakercli/internal/langmodel"
)

const sample = "package p;\n\n" +
	"public class Greeter {\n" +
	"    private String name;\n\n" +
	"    /** Hand-written, must survive. */\n" +
	"    public String name() {\n        return name;\n    }\n\n" +
	"    @Override\n" +
	"    public String toString() {\n        return \"x\";\n    }\n}\n"

func writeSample(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "Greeter.java")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func at(src, needle string) langmodel.Range {
	i := strings.Index(src, needle)
	return langmodel.Range{StartByte: i, EndByte: i + len(needle)}
}

func ins(src, needle, text string) Insertion {
	return Insertion{Target: at(src, needle), Name: needle, Content: text}
}

func cleaned(t *testing.T, data []byte) string {
	t.Helper()
	var out bytes.Buffer
	if err := gitfilter.Clean(bytes.NewReader(data), &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

func TestInsert_WrapsInSentinelsAndCleanRestoresTheOriginalExactly(t *testing.T) {
	path := writeSample(t, sample)
	outcomes, err := BlockCommentInserter{}.InsertDocs(context.Background(), path, []Insertion{
		ins(sample, "public class Greeter", "A greeter.\n\nSecond paragraph."),
		ins(sample, "@Override", "Renders the greeter."),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range outcomes {
		if o.Action != ActionInserted {
			t.Fatalf("%s: %s (%s)", o.Name, o.Action, o.Reason)
		}
	}

	got, _ := os.ReadFile(path)
	text := string(got)
	for _, want := range []string{
		"/**\n * docsmaker:ai-begin:v1\n * A greeter.\n *\n * Second paragraph.\n * docsmaker:ai-end\n */\npublic class Greeter",
		"    /**\n     * docsmaker:ai-begin:v1\n     * Renders the greeter.\n     * docsmaker:ai-end\n     */\n    @Override",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing block:\n%s\n--- in ---\n%s", want, text)
		}
	}
	if !strings.Contains(text, "/** Hand-written, must survive. */") {
		t.Fatal("the hand-written doc must be untouched")
	}
	if c := cleaned(t, got); c != sample {
		t.Fatalf("clean filter did not restore the original:\n%s", c)
	}
}

func TestInsert_HandWrittenDocIsSkipped(t *testing.T) {
	path := writeSample(t, sample)
	outcomes, err := BlockCommentInserter{}.InsertDocs(context.Background(), path,
		[]Insertion{ins(sample, "public String name()", "Should not appear.")})
	if err != nil {
		t.Fatal(err)
	}
	if outcomes[0].Action != ActionSkipped {
		t.Fatalf("action = %s", outcomes[0].Action)
	}
	if got, _ := os.ReadFile(path); string(got) != sample {
		t.Fatal("file must be untouched")
	}
}

func TestInsert_IsIdempotentAndRefreshesAnEarlierAIBlock(t *testing.T) {
	path := writeSample(t, sample)
	ctx := context.Background()
	target := func(src string) Insertion { return ins(src, "public class Greeter", "First.") }

	if _, err := (BlockCommentInserter{}).InsertDocs(ctx, path, []Insertion{target(sample)}); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(path)

	// Offsets shifted: re-locate the declaration, as a fresh parse would.
	again := func(content string) Outcome {
		oc, err := BlockCommentInserter{}.InsertDocs(ctx, path,
			[]Insertion{{Target: at(string(first), "public class Greeter"), Content: content}})
		if err != nil {
			t.Fatal(err)
		}
		return oc[0]
	}

	if oc := again("First."); oc.Action != ActionUnchanged {
		t.Fatalf("same content: %s", oc.Action)
	}
	if now, _ := os.ReadFile(path); !bytes.Equal(now, first) {
		t.Fatal("unchanged insert must not alter the file")
	}

	if oc := again("Second."); oc.Action != ActionReplaced {
		t.Fatalf("new content: %s", oc.Action)
	}
	now, _ := os.ReadFile(path)
	if strings.Contains(string(now), "First.") || strings.Count(string(now), "ai-begin") != 1 {
		t.Fatalf("expected a single refreshed block:\n%s", now)
	}
	if c := cleaned(t, now); c != sample {
		t.Fatalf("clean after refresh:\n%s", c)
	}
}

func TestInsert_ModelOutputCannotBreakOutOfTheComment(t *testing.T) {
	path := writeSample(t, sample)
	hostile := "Ends */ early, opens /* nested, path C:\\users, fakes // docsmaker:ai-end here"
	if _, err := (BlockCommentInserter{}).InsertDocs(context.Background(), path,
		[]Insertion{ins(sample, "public class Greeter", hostile)}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	text := string(got)

	if n := strings.Count(text, "docsmaker:ai-end"); n != 1 {
		t.Fatalf("sentinel forged: %d ai-end markers\n%s", n, text)
	}
	if strings.Count(text, "*/") != strings.Count(sample, "*/")+1 {
		t.Fatalf("comment terminated early:\n%s", text)
	}
	if strings.Contains(text, `\users`) && !strings.Contains(text, `\\users`) {
		t.Fatalf("a bare \\u would be an illegal Java unicode escape:\n%s", text)
	}
	if c := cleaned(t, got); c != sample {
		t.Fatalf("clean after hostile content:\n%s", c)
	}
}

func TestInsert_PreservesCRLF(t *testing.T) {
	crlf := strings.ReplaceAll(sample, "\n", "\r\n")
	path := writeSample(t, crlf)
	if _, err := (BlockCommentInserter{}).InsertDocs(context.Background(), path,
		[]Insertion{ins(crlf, "public class Greeter", "Doc.")}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if bytes.Contains(bytes.ReplaceAll(got, []byte("\r\n"), nil), []byte("\n")) {
		t.Fatal("found a bare LF in a CRLF file")
	}
	if c := cleaned(t, got); c != crlf {
		t.Fatalf("clean after CRLF insert:\n%q", c)
	}
}

func TestInsert_FailedVerificationLeavesTheFileUntouched(t *testing.T) {
	path := writeSample(t, sample)
	_, err := BlockCommentInserter{
		Verify: func(context.Context, []byte, []byte) error { return errors.New("does not parse") },
	}.InsertDocs(context.Background(), path, []Insertion{ins(sample, "public class Greeter", "Doc.")})
	if err == nil {
		t.Fatal("expected the verification error")
	}
	if got, _ := os.ReadFile(path); string(got) != sample {
		t.Fatal("file changed despite failed verification")
	}
	if entries, _ := os.ReadDir(filepath.Dir(path)); len(entries) != 1 {
		t.Fatalf("temp file left behind: %v", entries)
	}
}

func TestInsert_NonUTF8FileIsSkipped(t *testing.T) {
	path := writeSample(t, "class A {\n    // caf\xe9\n    void f() {}\n}\n")
	src, _ := os.ReadFile(path)
	oc, err := BlockCommentInserter{}.InsertDocs(context.Background(), path,
		[]Insertion{ins(string(src), "void f()", "Doc.")})
	if err != nil || oc[0].Action != ActionSkipped {
		t.Fatalf("outcome = %+v, err = %v", oc, err)
	}
}
