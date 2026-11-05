// Package gitfilter implements the git clean/smudge filter that keeps
// AI-inserted documentation lines out of anything staged/committed, even
// when the rest of the same file is tracked and pushed normally.
//
// AI-generated Javadoc blocks are wrapped between sentinel marker lines:
//
//	/**
//	 * // docsmaker:ai-begin:v1
//	 * Valida o payload do pedido recebido.
//	 * @param req o pedido em bruto
//	 * // docsmaker:ai-end
//	 */
//
// The version suffix on ai-begin lets future filter versions change how a
// block is parsed without breaking files tagged by an older version.
package gitfilter

import "regexp"

var (
	beginSentinelRe = regexp.MustCompile(`docsmaker:ai-begin(:v\d+)?`)
	endSentinelRe   = regexp.MustCompile(`docsmaker:ai-end`)
)

// BeginMarker renders the begin sentinel for the given version, e.g. "v1".
func BeginMarker(version string) string {
	return "docsmaker:ai-begin:" + version
}

// EndMarker renders the end sentinel.
func EndMarker() string {
	return "docsmaker:ai-end"
}

// CurrentVersion is the sentinel version new blocks are written with.
const CurrentVersion = "v1"

// HasAIBlock reports whether s contains a docsmaker AI-generated block.
func HasAIBlock(s string) bool {
	return beginSentinelRe.MatchString(s)
}

// Neutralize defuses anything in text that the clean filter would mistake
// for a sentinel, so model output can never forge or prematurely close a
// block.
func Neutralize(text string) string {
	text = beginSentinelRe.ReplaceAllString(text, "docsmaker: ai-begin")
	return endSentinelRe.ReplaceAllString(text, "docsmaker: ai-end")
}
