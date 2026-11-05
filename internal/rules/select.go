package rules

import (
	"sort"
	"strings"

	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/langmodel"
)

// DefaultMinScore is the score a method needs to be worth sending to the
// model. Two decisions, or one decision and a couple of thresholds, clear it;
// getters, setters and plain assignments score 0.
const DefaultMinScore = 3

// Score rates how likely a method is to embed a business rule, from the
// structural Signals the analyzer measured. Decisions and explicit throws
// weigh double: they are where rules live. Comparisons and magic literals
// (thresholds, codes) add evidence.
func Score(s langmodel.Signals) int {
	return 2*s.Branches + 2*s.Throws + s.Comparisons + s.Literals
}

// Candidate is a method selected for rule extraction.
type Candidate struct {
	Package *langmodel.Package
	Type    *langmodel.TypeDecl
	Method  *langmodel.Method
	Score   int
}

// ID is the stable rule identifier this candidate would produce.
func (c Candidate) ID() string {
	return RuleID(c.Type.Package, c.Type.Name, c.Method.Name, c.Method.SourceRange.StartLine)
}

// Target is the "pkg.Type#method" name of the candidate.
func (c Candidate) Target() string {
	return Target(c.Type.Package, c.Type.Name, c.Method.Name)
}

// SelectOptions narrows which methods become candidates.
type SelectOptions struct {
	// Scope limits the search: "" for the whole project, a package name, a
	// "pkg.Type", or a "pkg.Type#method".
	Scope string
	// MinScore is the lowest Score accepted. Use DefaultMinScore unless a
	// caller wants more (or less) recall.
	MinScore int
	// MaxMethods keeps only the N highest-scoring candidates (0 = no limit).
	// Ties keep source order, so the same project always yields the same set.
	MaxMethods int
}

// SelectCandidates picks the methods of model worth analysing for business
// rules. Methods without a body (Signals == nil) and methods scoring below
// opts.MinScore are skipped. The result is in model (source) order.
func SelectCandidates(model *langmodel.CodeModel, opts SelectOptions) []Candidate {
	var all []Candidate
	for pi := range model.Packages {
		pkg := &model.Packages[pi]
		for ti := range pkg.Types {
			decl := &pkg.Types[ti]
			for mi := range decl.Methods {
				m := &decl.Methods[mi]
				if m.Signals == nil || !inScope(opts.Scope, decl, m) {
					continue
				}
				if score := Score(*m.Signals); score >= opts.MinScore {
					all = append(all, Candidate{Package: pkg, Type: decl, Method: m, Score: score})
				}
			}
		}
	}

	if opts.MaxMethods <= 0 || len(all) <= opts.MaxMethods {
		return all
	}

	// Keep the top N by score (stable, so ties favour earlier source order),
	// then put the survivors back in source order.
	ranked := make([]int, len(all))
	for i := range ranked {
		ranked[i] = i
	}
	sort.SliceStable(ranked, func(a, b int) bool { return all[ranked[a]].Score > all[ranked[b]].Score })
	keep := ranked[:opts.MaxMethods]
	sort.Ints(keep)
	out := make([]Candidate, len(keep))
	for i, idx := range keep {
		out[i] = all[idx]
	}
	return out
}

func inScope(scope string, decl *langmodel.TypeDecl, m *langmodel.Method) bool {
	if scope == "" {
		return true
	}
	if owner, method, ok := strings.Cut(scope, "#"); ok {
		return owner == fqcn(decl) && method == m.Name
	}
	return scope == decl.Package || scope == fqcn(decl)
}

func fqcn(decl *langmodel.TypeDecl) string {
	if decl.Package == "" {
		return decl.Name
	}
	return decl.Package + "." + decl.Name
}

// ScopeExists reports whether scope (see SelectOptions.Scope) names at least
// one method of model, regardless of score. It lets a caller tell a mistyped
// target from a correct one whose methods are simply not candidates.
func ScopeExists(model *langmodel.CodeModel, scope string) bool {
	for pi := range model.Packages {
		for ti := range model.Packages[pi].Types {
			decl := &model.Packages[pi].Types[ti]
			for mi := range decl.Methods {
				if inScope(scope, decl, &decl.Methods[mi]) {
					return true
				}
			}
		}
	}
	return false
}
