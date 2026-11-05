package inserter

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/gitfilter"
)

// BlockCommentInserter writes `/** ... */` doc comments, the convention
// shared by Java, Kotlin, C#, TypeScript, etc.
//
// Every block it writes is wrapped in sentinel lines:
//
//	/**
//	 * docsmaker:ai-begin:v1
//	 * Summary text.
//	 * docsmaker:ai-end
//	 */
//
// A doc comment written by a human is never touched. A block carrying the
// sentinels (from an earlier run) is refreshed in place, so inserting twice
// is idempotent.
type BlockCommentInserter struct {
	// Verify is called with the original and the modified source before the
	// file is replaced; a non-nil error aborts the write. It should check
	// that the modified source still parses and declares the same things.
	Verify func(ctx context.Context, before, after []byte) error
}

// InsertDocs implements Inserter.
func (b BlockCommentInserter) InsertDocs(ctx context.Context, file string, insertions []Insertion) ([]Outcome, error) {
	src, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	if !utf8.Valid(src) {
		return skipAll(insertions, "file is not valid UTF-8"), nil
	}

	eol := "\n"
	if bytes.Contains(src, []byte("\r\n")) {
		eol = "\r\n"
	}

	// Work bottom-to-top so earlier byte offsets stay valid.
	order := make([]int, len(insertions))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(i, j int) bool {
		return insertions[order[i]].Target.StartByte > insertions[order[j]].Target.StartByte
	})

	outcomes := make([]Outcome, len(insertions))
	out := src
	changed := false
	for _, idx := range order {
		ins := insertions[idx]
		oc := Outcome{Insertion: ins}
		next, action, reason := applyOne(out, ins, eol)
		oc.Action, oc.Reason = action, reason
		if action == ActionInserted || action == ActionReplaced {
			out, changed = next, true
		}
		outcomes[idx] = oc
	}
	if !changed {
		return outcomes, nil
	}

	if b.Verify != nil {
		if err := b.Verify(ctx, src, out); err != nil {
			return nil, fmt.Errorf("inserter: %s would not be valid after insertion, left untouched: %w", file, err)
		}
	}
	if err := writeAtomic(file, out); err != nil {
		return nil, err
	}
	return outcomes, nil
}

func skipAll(insertions []Insertion, reason string) []Outcome {
	out := make([]Outcome, len(insertions))
	for i, ins := range insertions {
		out[i] = Outcome{Insertion: ins, Action: ActionSkipped, Reason: reason}
	}
	return out
}

// applyOne returns src with ins applied, or src unchanged plus the reason.
func applyOne(src []byte, ins Insertion, eol string) ([]byte, Action, string) {
	start := ins.Target.StartByte
	if start <= 0 || start > len(src) {
		return src, ActionSkipped, "declaration offset is outside the file (file changed since analysis?)"
	}

	lineStart := bytes.LastIndexByte(src[:start], '\n') + 1
	indent := string(src[lineStart:start])
	if strings.TrimLeft(indent, " \t") != "" {
		return src, ActionSkipped, "declaration does not start its own line"
	}

	block := renderBlock(ins.Content, indent, eol)

	// Is there a doc comment immediately above?
	head := bytes.TrimRight(src[:lineStart], " \t\r\n")
	if bytes.HasSuffix(head, []byte("*/")) {
		open := bytes.LastIndex(head, []byte("/*"))
		if open >= 0 && bytes.HasPrefix(head[open:], []byte("/**")) {
			docStart := bytes.LastIndexByte(head[:open], '\n') + 1
			if strings.TrimLeft(string(head[docStart:open]), " \t") != "" {
				return src, ActionSkipped, "existing doc comment shares its line with other code"
			}
			existing := string(src[docStart:lineStart])
			if !gitfilter.HasAIBlock(existing) {
				return src, ActionSkipped, "has a hand-written doc comment"
			}
			if existing == block {
				return src, ActionUnchanged, ""
			}
			return splice(src, docStart, lineStart, block), ActionReplaced, ""
		}
	}
	return splice(src, lineStart, lineStart, block), ActionInserted, ""
}

func splice(src []byte, from, to int, block string) []byte {
	out := make([]byte, 0, len(src)+len(block))
	out = append(out, src[:from]...)
	out = append(out, block...)
	return append(out, src[to:]...)
}

// renderBlock builds the sentinel-wrapped comment, indented to match the
// declaration and ending with a line terminator.
func renderBlock(content, indent, eol string) string {
	var sb strings.Builder
	line := func(s string) {
		sb.WriteString(indent)
		if s == "" {
			sb.WriteString(" *")
		} else {
			sb.WriteString(" * " + s)
		}
		sb.WriteString(eol)
	}

	sb.WriteString(indent + "/**" + eol)
	line(gitfilter.BeginMarker(gitfilter.CurrentVersion))
	for _, l := range strings.Split(sanitize(content), "\n") {
		line(strings.TrimRight(l, " \t"))
	}
	line(gitfilter.EndMarker())
	sb.WriteString(indent + " */" + eol)
	return sb.String()
}

// sanitize makes model output safe to embed in a block comment: it can't
// close the comment early, forge a sentinel, or start a Java unicode escape.
func sanitize(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.ReplaceAll(s, "*/", "* /")
	s = strings.ReplaceAll(s, "/*", "/ *")
	s = strings.ReplaceAll(s, `\u`, `\\u`)
	return gitfilter.Neutralize(strings.TrimSpace(s))
}

// writeAtomic replaces path via a temp file in the same directory.
func writeAtomic(path string, data []byte) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".docsmaker-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if err := errors.Join(werr, cerr); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Chmod(tmpName, info.Mode().Perm()); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}
