package docgen

import (
	"context"
	"fmt"
	"strings"

	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/langmodel"
	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/llm/prompts"
	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/rules"
)

// RuleStatus is what became of one candidate method.
type RuleStatus string

const (
	RuleFound   RuleStatus = "rule"    // a business rule was extracted
	RuleNone    RuleStatus = "none"    // the model found no business rule
	RuleOmitted RuleStatus = "omitted" // skipped at a sensitive-information prompt
	RuleFailed  RuleStatus = "failed"  // no usable answer; see Reason
)

// RuleResult is the outcome for one candidate method. Methods the selector
// did not pick do not appear at all.
type RuleResult struct {
	ID     string          `json:"id"`
	Target string          `json:"target"`
	Range  langmodel.Range `json:"range"`
	Score  int             `json:"score"`
	Status RuleStatus      `json:"status"`
	// Rule is set only when Status == RuleFound.
	Rule *rules.Rule `json:"rule,omitempty"`
	// Reason explains an omitted or failed result.
	Reason string `json:"reason,omitempty"`
	// EvidenceDropped counts evidence lines the model quoted that are not in
	// the source; a quality signal, not an error.
	EvidenceDropped int `json:"evidenceDropped,omitempty"`
}

// RulesOptions configures ExtractRules.
type RulesOptions struct {
	// Scope, MinScore and MaxMethods are passed to rules.SelectCandidates.
	Scope      string
	MinScore   int
	MaxMethods int
	// Lang is the language of the free-text parts of each rule ("pt", "en").
	Lang string
	// Progress, if set, is called before each candidate with its 1-based
	// position, the total, and its target (a run is slow: one model call each).
	Progress func(n, total int, target string)
}

// maxFieldDecls bounds how many field declarations accompany a method in its
// prompt, so a class with hundreds of fields cannot swamp a small model.
const maxFieldDecls = 30

// ExtractRules asks the model for the business rule inside each candidate
// method of model, one call per method, in source order.
//
// Every call goes through generateWith, so the secrets gates, the cache and
// the audit log apply exactly as they do to documentation. A method whose
// answer cannot be used degrades to RuleFailed and the run carries on; only
// errors that make continuing pointless (the model is unreachable, the
// developer aborted at a sensitive-information prompt, the context ended)
// stop it, returning the results gathered so far alongside the error.
func (p *Pipeline) ExtractRules(ctx context.Context, model *langmodel.CodeModel, opts RulesOptions) ([]RuleResult, error) {
	cands := rules.SelectCandidates(model, rules.SelectOptions{
		Scope: opts.Scope, MinScore: opts.MinScore, MaxMethods: opts.MaxMethods,
	})

	results := make([]RuleResult, 0, len(cands))
	for i, c := range cands {
		if err := ctx.Err(); err != nil {
			return results, err
		}
		if opts.Progress != nil {
			opts.Progress(i+1, len(cands), c.Target())
		}
		res, err := p.extractRule(ctx, c, opts.Lang)
		if err != nil {
			return results, fmt.Errorf("docgen: extracting rules from %s: %w", c.Target(), err)
		}
		results = append(results, res)
	}
	return results, nil
}

func (p *Pipeline) extractRule(ctx context.Context, c rules.Candidate, lang string) (RuleResult, error) {
	res := RuleResult{ID: c.ID(), Target: c.Target(), Range: c.Method.SourceRange, Score: c.Score}

	source, err := readSpan(c.Method.SourceRange)
	if err != nil {
		res.Status, res.Reason = RuleFailed, "source unavailable: "+err.Error()
		return res, nil
	}

	system, user := prompts.BusinessRules(c.Type.Package, c.Type.Name, c.Method, source, fieldDeclarations(c.Type), lang)
	where := fmt.Sprintf("business rules of %s (%s)", c.Target(), location(c.Method.SourceRange))
	opt := genOptions{
		format: "json",
		accept: func(text string) error { _, err := rules.Parse(text); return err },
	}

	text, err := p.generateWith(ctx, where, system, user, opt)
	if err != nil {
		return res, err
	}
	if text == OmittedSummary {
		return omitted(res), nil
	}

	ext, parseErr := rules.Parse(text)
	if parseErr != nil {
		// One second chance, showing the model what was wrong with its answer.
		retry := prompts.BusinessRulesRetry(user, text, parseErr.Error())
		if text, err = p.generateWith(ctx, where+" [retry]", system, retry, opt); err != nil {
			return res, err
		}
		if text == OmittedSummary {
			return omitted(res), nil
		}
		if ext, parseErr = rules.Parse(text); parseErr != nil {
			res.Status, res.Reason = RuleFailed, "unusable answer after retry: "+parseErr.Error()
			return res, nil
		}
	}

	if !ext.HasRule {
		res.Status = RuleNone
		return res, nil
	}

	rule := ext.Rule(res.ID, res.Target, res.Range)
	res.EvidenceDropped = rule.CheckEvidence(source)
	if err := rule.Validate(); err != nil {
		res.Status, res.Reason = RuleFailed, err.Error()
		return res, nil
	}
	res.Status, res.Rule = RuleFound, rule
	return res, nil
}

func omitted(res RuleResult) RuleResult {
	res.Status, res.Reason = RuleOmitted, "skipped at a sensitive-information prompt"
	return res
}

// fieldDeclarations returns the owner's field declarations as written in
// source, in declaration order (the prompt is a cache key), whitespace
// collapsed. Fields whose source cannot be read are left out.
func fieldDeclarations(decl *langmodel.TypeDecl) []string {
	var out []string
	for _, f := range decl.Fields {
		if len(out) == maxFieldDecls {
			break
		}
		if src, err := readSpan(f.SourceRange); err == nil {
			out = append(out, strings.Join(strings.Fields(src), " "))
		}
	}
	return out
}
