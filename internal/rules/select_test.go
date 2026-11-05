package rules

import (
	"reflect"
	"testing"

	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/langmodel"
)

func method(name string, line int, s *langmodel.Signals) langmodel.Method {
	return langmodel.Method{Name: name, Signals: s, SourceRange: langmodel.Range{StartLine: line}}
}

func testModel() *langmodel.CodeModel {
	return &langmodel.CodeModel{Packages: []langmodel.Package{
		{Name: "com.x", Types: []langmodel.TypeDecl{
			{Name: "Pricing", Package: "com.x", Methods: []langmodel.Method{
				method("getRate", 5, &langmodel.Signals{Statements: 1}),                                            // 0
				method("discount", 9, &langmodel.Signals{Statements: 6, Branches: 3, Comparisons: 3, Literals: 5}), // 14
				method("abstractRule", 20, nil),
				method("check", 24, &langmodel.Signals{Statements: 2, Branches: 1, Throws: 1, Comparisons: 1}), // 5
				method("tiny", 30, &langmodel.Signals{Statements: 1, Branches: 1}),                             // 2
			}},
		}},
		{Name: "com.y", Types: []langmodel.TypeDecl{
			{Name: "Tax", Package: "com.y", Methods: []langmodel.Method{
				method("vat", 7, &langmodel.Signals{Statements: 3, Branches: 2, Comparisons: 3, Literals: 3}), // 10
				method("check", 40, &langmodel.Signals{Statements: 3, Branches: 2, Throws: 1}),                // 6
			}},
		}},
	}}
}

func targets(cs []Candidate) []string {
	var out []string
	for _, c := range cs {
		out = append(out, c.Target())
	}
	return out
}

func TestScore(t *testing.T) {
	if got := Score(langmodel.Signals{Statements: 9}); got != 0 {
		t.Errorf("a body with no decisions scores %d, want 0", got)
	}
	if got := Score(langmodel.Signals{Branches: 1, Throws: 1, Comparisons: 1, Literals: 1}); got != 6 {
		t.Errorf("Score = %d, want 2+2+1+1 = 6", got)
	}
}

func TestSelectCandidatesFiltersByScoreAndBody(t *testing.T) {
	got := targets(SelectCandidates(testModel(), SelectOptions{MinScore: DefaultMinScore}))
	want := []string{"com.x.Pricing#discount", "com.x.Pricing#check", "com.y.Tax#vat", "com.y.Tax#check"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("candidates = %v, want %v (no getter, no abstract, no score-2 method; source order)", got, want)
	}
}

func TestSelectCandidatesMinScoreZeroKeepsEveryBody(t *testing.T) {
	got := SelectCandidates(testModel(), SelectOptions{})
	if len(got) != 6 {
		t.Errorf("got %d candidates, want the 6 methods that have a body", len(got))
	}
}

func TestSelectCandidatesMaxMethodsKeepsTopScoresInSourceOrder(t *testing.T) {
	got := targets(SelectCandidates(testModel(), SelectOptions{MinScore: DefaultMinScore, MaxMethods: 2}))
	want := []string{"com.x.Pricing#discount", "com.y.Tax#vat"} // scores 14 and 10, in source order
	if !reflect.DeepEqual(got, want) {
		t.Errorf("top-2 = %v, want %v", got, want)
	}
}

func TestSelectCandidatesMaxMethodsTieFavoursSourceOrder(t *testing.T) {
	model := &langmodel.CodeModel{Packages: []langmodel.Package{{Name: "p", Types: []langmodel.TypeDecl{
		{Name: "A", Package: "p", Methods: []langmodel.Method{
			method("a", 1, &langmodel.Signals{Branches: 2}),
			method("b", 2, &langmodel.Signals{Branches: 2}),
			method("c", 3, &langmodel.Signals{Branches: 2}),
		}},
	}}}}
	got := targets(SelectCandidates(model, SelectOptions{MinScore: 1, MaxMethods: 2}))
	if want := []string{"p.A#a", "p.A#b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("tie-break = %v, want %v", got, want)
	}
}

func TestSelectCandidatesScope(t *testing.T) {
	cases := map[string][]string{
		"com.y":                  {"com.y.Tax#vat", "com.y.Tax#check"},
		"com.x.Pricing":          {"com.x.Pricing#discount", "com.x.Pricing#check"},
		"com.x.Pricing#discount": {"com.x.Pricing#discount"},
		"com.y.Tax#check":        {"com.y.Tax#check"},
		"com.nope":               nil,
		"com.x.Pricing#nope":     nil,
	}
	for scope, want := range cases {
		got := targets(SelectCandidates(testModel(), SelectOptions{Scope: scope, MinScore: DefaultMinScore}))
		if !reflect.DeepEqual(got, want) {
			t.Errorf("scope %q: %v, want %v", scope, got, want)
		}
	}
}

func TestCandidateID(t *testing.T) {
	cs := SelectCandidates(testModel(), SelectOptions{Scope: "com.y.Tax#check", MinScore: 1})
	if len(cs) != 1 || cs[0].ID() != "com.y.Tax#check@L40" {
		t.Fatalf("candidates = %+v, want one with ID com.y.Tax#check@L40", cs)
	}
}

func TestScopeExists(t *testing.T) {
	for scope, want := range map[string]bool{
		"": true, "com.x": true, "com.x.Pricing": true, "com.x.Pricing#getRate": true, // even a method that is no candidate
		"com.nope": false, "com.x.Pricing#nope": false, "Pricing": false,
	} {
		if got := ScopeExists(testModel(), scope); got != want {
			t.Errorf("ScopeExists(%q) = %v, want %v", scope, got, want)
		}
	}
}
