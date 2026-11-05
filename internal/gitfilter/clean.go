package gitfilter

import (
	"fmt"
	"io"
	"regexp"
)

// emptyJavadocRe matches a Javadoc block that, once its content lines have
// been stripped, contains nothing but the opening/closing markers and bare
// "*" continuation lines — i.e. the block insertion's insertion left behind
// an empty stub that should disappear rather than linger in the commit.
var emptyJavadocRe = regexp.MustCompile(`(?m)^[ \t]*/\*\*[ \t]*\r?\n(?:[ \t]*\*[ \t]*\r?\n)*[ \t]*\*/[ \t]*\r?\n?`)

// Clean implements the stdin -> stdout contract git expects from a clean
// filter: read all of r, strip every docsmaker:ai-begin/docsmaker:ai-end
// block, and write the result to w. A begin with no matching end (or a
// stray end with no begin) before EOF is treated as malformed input: the
// original content is passed through unmodified and a warning is written to
// warn, rather than risk silently corrupting what gets committed. Git
// surfaces filter stderr to the user, so warn should be the filter
// subcommand's os.Stderr.
func Clean(r io.Reader, w io.Writer, warn io.Writer) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}

	lines := splitLinesKeepEnding(string(data))

	depth := 0
	for _, line := range lines {
		switch {
		case beginSentinelRe.MatchString(line):
			depth++
		case endSentinelRe.MatchString(line):
			depth--
			if depth < 0 {
				fmt.Fprintln(warn, "docsmaker: unmatched ai-end sentinel, passing file through unmodified")
				_, err := w.Write(data)
				return err
			}
		}
	}
	if depth != 0 {
		fmt.Fprintln(warn, "docsmaker: unmatched ai-begin sentinel, passing file through unmodified")
		_, err := w.Write(data)
		return err
	}

	var stripped []byte
	inBlock := false
	for _, line := range lines {
		switch {
		case beginSentinelRe.MatchString(line):
			inBlock = true
			continue
		case endSentinelRe.MatchString(line):
			inBlock = false
			continue
		case inBlock:
			continue
		}
		stripped = append(stripped, line...)
	}

	result := emptyJavadocRe.ReplaceAll(stripped, nil)
	_, err = w.Write(result)
	return err
}

// Smudge is the identity transform. The working tree already holds the full
// inline docs from insertion (inserter package), and Clean already stripped
// the AI lines before commit, so there is nothing to restore on checkout.
func Smudge(r io.Reader, w io.Writer) error {
	_, err := io.Copy(w, r)
	return err
}

// splitLinesKeepEnding splits s into lines, keeping each line's terminator
// attached, so re-joining the returned slice reproduces s exactly.
func splitLinesKeepEnding(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i+1])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
