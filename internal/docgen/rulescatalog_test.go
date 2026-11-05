package docgen

import (
	"strings"
	"testing"

	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/langmodel"
	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/rules"
)

func catalogResults() []RuleResult {
	at := func(line int) langmodel.Range { return langmodel.Range{StartLine: line} }
	return []RuleResult{
		{Target: "p.A#one", Range: at(3), Status: RuleFound, Rule: &rules.Rule{
			Statement: "Limit is 500.", Conditions: []string{"x > 500"}, Rationale: "Probably risk control.",
			Evidence: []string{"if (x > 500)"}, Confidence: rules.ConfidenceHigh, OpenQuestions: []string{"Per month?"}}},
		{Target: "p.A#plumbing", Range: at(9), Status: RuleNone},
		{Target: "p.B#two", Range: at(5), Status: RuleFound, EvidenceDropped: 2, Rule: &rules.Rule{
			Statement: "Fee is 2%.", Confidence: rules.ConfidenceLow}},
		{Target: "p.A#three", Range: at(20), Status: RuleFound, Rule: &rules.Rule{
			Statement: "Minimum age 18.", Confidence: rules.ConfidenceMedium}},
		{Target: "p.C#secret", Range: at(7), Status: RuleOmitted, Reason: "skipped"},
		{Target: "p.C#broken", Range: at(11), Status: RuleFailed, Reason: "unusable answer"},
	}
}

func TestBuildRulesCatalog_Portuguese(t *testing.T) {
	got := BuildRulesCatalog("demo", catalogResults(), "pt")
	for _, want := range []string{
		"# Regras de negócio — demo",
		"hipótese inferida, não um facto",
		"3 regra(s) em 2 classe(s); 1 método(s) sem regra, 1 omitido(s), 1 com falha.",
		"## p.A", "### one (linha 3) — Confiança: alta",
		"**Regra:** Limit is 500.",
		"**Condições:**", "- x > 500",
		"**Porquê provável (inferido):** Probably risk control.",
		"**Evidência:**\n\n```\nif (x > 500)\n```",
		"**Questões em aberto:**", "- Per month?",
		"### two (linha 5) — Confiança: baixa",
		"2 linha(s) de evidência citada(s) pelo modelo não existem no código e foram removidas",
		"## Métodos sem resultado",
		"- `p.C#secret` (linha 7): omitido (conteúdo sensível)",
		"- `p.C#broken` (linha 11): falhou — unusable answer",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("catalog lacks %q\n%s", want, got)
		}
	}
}

func TestBuildRulesCatalog_GroupsByClassInFirstSeenOrder(t *testing.T) {
	got := BuildRulesCatalog("demo", catalogResults(), "en")
	a, b := strings.Index(got, "## p.A"), strings.Index(got, "## p.B")
	if a < 0 || b < 0 || a > b {
		t.Fatalf("p.A must come before p.B:\n%s", got)
	}
	// p.A#three is listed after p.B in the results but belongs to p.A's section.
	if three := strings.Index(got, "### three"); three < a || three > b {
		t.Errorf("a class's rules must stay together:\n%s", got)
	}
	if strings.Contains(got, "p.C\n") && strings.Contains(got, "## p.C") {
		t.Error("a class with no rule must not get a section")
	}
	if strings.Contains(got, "plumbing") {
		t.Error("methods without a rule are counted, not listed")
	}
}

func TestBuildRulesCatalog_EnglishAndOtherLanguagesUseEnglishLabels(t *testing.T) {
	for _, lang := range []string{"en", "fr", "de"} {
		got := BuildRulesCatalog("demo", catalogResults(), lang)
		if !strings.Contains(got, "# Business rules — demo") || !strings.Contains(got, "Confidence: high") {
			t.Errorf("lang %q should use English report labels:\n%s", lang, got)
		}
	}
	for _, lang := range []string{"", "pt", "PT-PT", "pt-BR"} {
		if !strings.Contains(BuildRulesCatalog("demo", nil, lang), "Regras de negócio") {
			t.Errorf("lang %q should use Portuguese report labels", lang)
		}
	}
}

func TestBuildRulesCatalog_EmptyAndDeterministic(t *testing.T) {
	if got := BuildRulesCatalog("demo", nil, "en"); !strings.Contains(got, "No business rules found.") ||
		!strings.Contains(got, "0 rule(s) in 0 class(es)") {
		t.Errorf("empty report: %s", got)
	}
	first := BuildRulesCatalog("demo", catalogResults(), "pt")
	for i := 0; i < 20; i++ {
		if BuildRulesCatalog("demo", catalogResults(), "pt") != first {
			t.Fatal("catalog changed between identical calls")
		}
	}
}

func TestBuildClassRulesAndOwners(t *testing.T) {
	res := catalogResults()
	if got := RuleOwners(res); strings.Join(got, ",") != "p.A,p.B" {
		t.Errorf("RuleOwners = %v, want [p.A p.B]", got)
	}
	got := BuildClassRules("p.A", res, "en")
	if !strings.HasPrefix(got, "# p.A\n") || !strings.Contains(got, "Limit is 500.") || !strings.Contains(got, "Minimum age 18.") {
		t.Errorf("class report: %s", got)
	}
	if strings.Contains(got, "Fee is 2%") {
		t.Error("class report must only hold that class's rules")
	}
}

func TestSummarizeRules(t *testing.T) {
	s := SummarizeRules(catalogResults())
	want := RulesSummary{Found: 3, None: 1, Omitted: 1, Failed: 1, Classes: 2}
	if s != want {
		t.Errorf("summary = %+v, want %+v", s, want)
	}
}
