// Package secrets implements the two-gate sensitive-info detection described
// in the plan: every snippet is scanned before it enters an LLM prompt, and
// every generated output is scanned again before it is written anywhere.
package secrets

// Kind identifies the category of a Finding, used both for messaging and for
// redaction placeholders.
type Kind string

const (
	KindAWSKey        Kind = "aws-key"
	KindConnString    Kind = "connection-string"
	KindPrivateKey    Kind = "private-key"
	KindJWT           Kind = "jwt"
	KindAPIToken      Kind = "api-token"
	KindGenericSecret Kind = "generic-secret"
	KindHighEntropy   Kind = "high-entropy"
)

// Finding is a single potential secret located in a piece of text.
type Finding struct {
	Kind      Kind
	Match     string // the exact sensitive substring, for masking/redaction
	StartByte int
	EndByte   int
	// Where describes what was being screened when this was found (e.g.
	// "prompt for method com.foo.Bar#baz (Bar.java:12)"). Set by Gate, not
	// by the Scanner, so scanners stay context-free.
	Where string
}

// Scanner is the port for sensitive-info detection. Implementations combine
// regex detectors (known credential shapes) with entropy-based detection
// (opaque secrets that don't match a known pattern).
type Scanner interface {
	// Scan finds every potential secret in text, sorted by StartByte and
	// non-overlapping. An empty result means clean.
	Scan(text string) []Finding
}
