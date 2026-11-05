package docgen

import (
	"fmt"
	"strings"

	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/rules"
)

// ruleLabels are the fixed words around the model's text in a rules report.
// Only the report's own wording is localised here; the rules themselves come
// from the model in whatever language was asked for.
type ruleLabels struct {
	title, disclaimer                             string
	summary                                       string // Sprintf: rules, classes, none, omitted, failed
	rule, conditions, rationale, evidence, openQ  string
	confidence, line                              string
	high, medium, low                             string
	noResult, omittedWhy, failedWhy, nothingFound string
	droppedEvidence                               string // Sprintf: count
}

var (
	labelsPT = ruleLabels{
		title: "Regras de negócio",
		disclaimer: "Extraídas automaticamente por um modelo de IA local, só a partir do código. " +
			"O “porquê” de cada regra é uma hipótese inferida, não um facto: confirme-o com quem conhece o negócio.",
		summary: "%d regra(s) em %d classe(s); %d método(s) sem regra, %d omitido(s), %d com falha.",
		rule:    "Regra", conditions: "Condições", rationale: "Porquê provável (inferido)",
		evidence: "Evidência", openQ: "Questões em aberto",
		confidence: "Confiança", line: "linha",
		high: "alta", medium: "média", low: "baixa",
		noResult:   "Métodos sem resultado",
		omittedWhy: "omitido (conteúdo sensível)", failedWhy: "falhou",
		nothingFound:    "Nenhuma regra de negócio encontrada.",
		droppedEvidence: "%d linha(s) de evidência citada(s) pelo modelo não existem no código e foram removidas",
	}
	labelsEN = ruleLabels{
		title: "Business rules",
		disclaimer: "Extracted automatically by a local AI model, from the code alone. " +
			"The “why” of each rule is an inferred hypothesis, not a fact: confirm it with someone who knows the business.",
		summary: "%d rule(s) in %d class(es); %d method(s) without a rule, %d omitted, %d failed.",
		rule:    "Rule", conditions: "Conditions", rationale: "Probable reason (inferred)",
		evidence: "Evidence", openQ: "Open questions",
		confidence: "Confidence", line: "line",
		high: "high", medium: "medium", low: "low",
		noResult:   "Methods without a result",
		omittedWhy: "omitted (sensitive content)", failedWhy: "failed",
		nothingFound:    "No business rules found.",
		droppedEvidence: "%d evidence line(s) quoted by the model are not in the code and were removed",
	}
)

// labelsFor picks the report wording: Portuguese for "", "pt" and "pt-PT",
// English for everything else (a language the model can write in, but whose
// report labels we do not ship).
func labelsFor(lang string) ruleLabels {
	l := strings.ToLower(strings.TrimSpace(lang))
	if l == "" || l == "pt" || strings.HasPrefix(l, "pt-") {
		return labelsPT
	}
	return labelsEN
}

// RulesSummary counts results by status.
type RulesSummary struct {
	Found   int `json:"found"`
	None    int `json:"none"`
	Omitted int `json:"omitted"`
	Failed  int `json:"failed"`
	Classes int `json:"classes"` // classes that have at least one rule
}

// SummarizeRules counts results by status.
func SummarizeRules(results []RuleResult) RulesSummary {
	var s RulesSummary
	classes := map[string]bool{}
	for _, r := range results {
		switch r.Status {
		case RuleFound:
			s.Found++
			classes[ownerOf(r.Target)] = true
		case RuleNone:
			s.None++
		case RuleOmitted:
			s.Omitted++
		case RuleFailed:
			s.Failed++
		}
	}
	s.Classes = len(classes)
	return s
}

// RuleOwners returns the classes that have at least one found rule, in the
// order they first appear in results.
func RuleOwners(results []RuleResult) []string {
	var owners []string
	seen := map[string]bool{}
	for _, r := range results {
		if o := ownerOf(r.Target); r.Status == RuleFound && !seen[o] {
			seen[o] = true
			owners = append(owners, o)
		}
	}
	return owners
}

// BuildRulesCatalog renders the whole report: a header, a summary line, one
// section per class that has rules, and a list of methods that produced no
// result. Everything but the model's own sentences comes from results, and
// the output is a pure function of its inputs.
func BuildRulesCatalog(projectName string, results []RuleResult, lang string) string {
	l := labelsFor(lang)
	s := SummarizeRules(results)

	var b strings.Builder
	fmt.Fprintf(&b, "# %s — %s\n\n> %s\n\n", l.title, projectName, l.disclaimer)
	fmt.Fprintf(&b, l.summary+"\n\n", s.Found, s.Classes, s.None, s.Omitted, s.Failed)

	owners := RuleOwners(results)
	if len(owners) == 0 {
		fmt.Fprintf(&b, "%s\n\n", l.nothingFound)
	}
	for _, owner := range owners {
		writeOwnerSection(&b, owner, results, l, 2)
	}

	var skipped []RuleResult
	for _, r := range results {
		if r.Status == RuleOmitted || r.Status == RuleFailed {
			skipped = append(skipped, r)
		}
	}
	if len(skipped) > 0 {
		fmt.Fprintf(&b, "## %s\n\n", l.noResult)
		for _, r := range skipped {
			why := l.failedWhy
			if r.Status == RuleOmitted {
				why = l.omittedWhy
			}
			fmt.Fprintf(&b, "- `%s` (%s %d): %s", r.Target, l.line, r.Range.StartLine, why)
			if r.Status == RuleFailed && r.Reason != "" {
				fmt.Fprintf(&b, " — %s", r.Reason)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	return b.String()
}

// BuildClassRules renders the report for a single class, for the per-class
// sidecar files.
func BuildClassRules(owner string, results []RuleResult, lang string) string {
	l := labelsFor(lang)
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n> %s\n\n", owner, l.disclaimer)
	writeOwnerRules(&b, owner, results, l, 2)
	return b.String()
}

func writeOwnerSection(b *strings.Builder, owner string, results []RuleResult, l ruleLabels, level int) {
	fmt.Fprintf(b, "%s %s\n\n", strings.Repeat("#", level), owner)
	writeOwnerRules(b, owner, results, l, level+1)
}

func writeOwnerRules(b *strings.Builder, owner string, results []RuleResult, l ruleLabels, level int) {
	for _, r := range results {
		if r.Status != RuleFound || r.Rule == nil || ownerOf(r.Target) != owner {
			continue
		}
		rule := r.Rule
		fmt.Fprintf(b, "%s %s (%s %d) — %s: %s\n\n", strings.Repeat("#", level),
			methodOf(r.Target), l.line, r.Range.StartLine, l.confidence, confidenceWord(rule.Confidence, l))
		fmt.Fprintf(b, "**%s:** %s\n\n", l.rule, rule.Statement)
		writeList(b, l.conditions, rule.Conditions)
		if rule.Rationale != "" {
			fmt.Fprintf(b, "**%s:** %s\n\n", l.rationale, rule.Rationale)
		}
		if len(rule.Evidence) > 0 {
			fmt.Fprintf(b, "**%s:**\n\n```\n%s\n```\n\n", l.evidence, strings.Join(rule.Evidence, "\n"))
		}
		writeList(b, l.openQ, rule.OpenQuestions)
		if r.EvidenceDropped > 0 {
			fmt.Fprintf(b, "_%s._\n\n", fmt.Sprintf(l.droppedEvidence, r.EvidenceDropped))
		}
	}
}

func writeList(b *strings.Builder, title string, items []string) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(b, "**%s:**\n\n", title)
	for _, it := range items {
		fmt.Fprintf(b, "- %s\n", it)
	}
	b.WriteString("\n")
}

func confidenceWord(c rules.Confidence, l ruleLabels) string {
	switch c.Rank() {
	case 3:
		return l.high
	case 2:
		return l.medium
	default:
		return l.low
	}
}

// ownerOf and methodOf split a "pkg.Type#method" target.
func ownerOf(target string) string {
	owner, _, _ := strings.Cut(target, "#")
	return owner
}

func methodOf(target string) string {
	_, method, _ := strings.Cut(target, "#")
	return method
}
