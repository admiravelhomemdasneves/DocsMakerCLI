package prompts

import (
	"fmt"
	"strings"

	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/langmodel"
)

// rulesSystemTemplate is the system prompt for business-rule extraction. The
// model must answer with one JSON object; internal/rules.Parse reads it. The
// worked example uses an unrelated domain (shipping) so it cannot leak into
// answers about billing code.
const rulesSystemTemplate = `You are a senior engineer recovering the business rules hidden in legacy code. ` +
	`You are given one Java method (signature, source, related field declarations and any existing documentation). ` +
	`Decide whether it enforces a business rule: a policy, threshold, eligibility test, tariff, validation or ` +
	`calculation that a domain expert would recognise. Technical plumbing (getters, mapping, logging, null checks ` +
	`with no business meaning) is not a rule.

Answer with ONE JSON object and nothing else:
{"hasRule": true, "statement": "...", "conditions": ["..."], "rationale": "...", "evidence": ["..."], "confidence": "high|medium|low", "openQuestions": ["..."]}

Rules for the answer:
- If the method contains no business rule, answer exactly {"hasRule": false}.
- statement: one or two sentences saying what the code enforces, with the exact values it uses (thresholds, rates, limits, codes). Never state a value that is not in the code. If the method holds several related rules, cover them all here.
- conditions: each distinct condition or threshold, one short item each.
- rationale: your best hypothesis of why the rule exists. It is a guess, so word it as one (for example "Probably ..."), and say so plainly when the code gives no hint.
- evidence: one to four lines copied verbatim from the source that the statement rests on.
- confidence: "high" only when the code is explicit and its intent obvious; "low" when you are guessing the intent.
- openQuestions: what only someone who knows the business could confirm. May be empty.
- Write statement, conditions, rationale and openQuestions in {{LANG}}. Keep evidence exactly as it appears in the code.

Example of a good answer, for a different method:
{"hasRule": true, "statement": "Shipping is free when the order total is at least 50, otherwise a flat fee of 4.90 applies.", "conditions": ["total >= 50 means free shipping", "otherwise the fee is 4.90"], "rationale": "Probably an incentive to raise the average basket size.", "evidence": ["if (total >= 50) return 0;", "return 4.90;"], "confidence": "high", "openQuestions": ["Does the threshold include taxes?"]}`

// languageName turns a CLI language code into the name used in the prompt.
// Unknown values are passed through, so any language the model knows works.
func languageName(code string) string {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "", "pt", "pt-pt":
		return "European Portuguese (pt-PT)"
	case "en", "en-us", "en-gb":
		return "English"
	default:
		return code
	}
}

func rulesSystem(lang string) string {
	return strings.ReplaceAll(rulesSystemTemplate, "{{LANG}}", languageName(lang))
}

// BusinessRules builds the prompt that asks for the business rule a single
// method implements. fieldDecls are the owner's field declarations as written
// in source (constants often hold the thresholds); pass them in declaration
// order, the prompt is a cache key and must be deterministic. lang is the
// language of the free-text parts of the answer ("pt", "en", ...).
func BusinessRules(pkg, ownerType string, m *langmodel.Method, source string, fieldDecls []string, lang string) (system, user string) {
	var b strings.Builder
	fmt.Fprintf(&b, "Class: %s.%s\n", pkg, ownerType)
	fmt.Fprintf(&b, "Signature: %s\n", methodSignature(m))

	if len(fieldDecls) > 0 {
		b.WriteString("\nFields of the class:\n")
		for _, f := range fieldDecls {
			fmt.Fprintf(&b, "  - %s\n", f)
		}
	}
	if doc := strings.TrimSpace(m.ExistingDoc); doc != "" {
		fmt.Fprintf(&b, "\nExisting documentation:\n%s\n", doc)
	}
	fmt.Fprintf(&b, "\nSource:\n%s\n", source)
	return rulesSystem(lang), b.String()
}

// maxRetryEcho caps how much of a bad answer is echoed back in a retry, so a
// runaway generation cannot blow up the next prompt.
const maxRetryEcho = 1500

// BusinessRulesRetry rebuilds the user prompt after the model's answer could
// not be used, quoting the answer and the problem so it can correct itself.
// The system prompt is unchanged, so only the user text differs (and with it
// the cache key).
func BusinessRulesRetry(user, badAnswer, problem string) string {
	badAnswer = strings.TrimSpace(badAnswer)
	if len(badAnswer) > maxRetryEcho {
		badAnswer = badAnswer[:maxRetryEcho] + "..."
	}
	var b strings.Builder
	b.WriteString(user)
	fmt.Fprintf(&b, "\nYour previous answer could not be used: %s\n", problem)
	fmt.Fprintf(&b, "Previous answer:\n%s\n", badAnswer)
	b.WriteString("Answer again with only the JSON object described in the instructions.\n")
	return b.String()
}
