package prompts

import (
	"strings"
	"testing"

	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/langmodel"
)

// The project prompt is a cache key input: it must not depend on Go's
// randomised map iteration order.
func TestProject_IsDeterministic(t *testing.T) {
	summaries := map[string]string{"c.pkg": "C", "a.pkg": "A", "b.pkg": "B", "d.pkg": "D", "e.pkg": "E"}
	_, first := Project("demo", summaries)
	for i := 0; i < 200; i++ {
		if _, again := Project("demo", summaries); again != first {
			t.Fatalf("prompt changed between calls:\n%s\n---\n%s", first, again)
		}
	}
}

func rulesMethod() *langmodel.Method {
	return &langmodel.Method{
		Name:        "discountFor",
		Params:      []langmodel.Param{{Name: "quantity", Type: langmodel.TypeRef{Name: "int"}}},
		ReturnType:  langmodel.TypeRef{Name: "double"},
		ExistingDoc: "/** Legacy: do not touch. */",
	}
}

func TestBusinessRules_IncludesEverythingTheModelNeeds(t *testing.T) {
	system, user := BusinessRules("com.x", "DiscountPolicy", rulesMethod(), "return 0.15;",
		[]string{"private static final int TIER = 100;"}, "pt")

	for _, want := range []string{
		"Class: com.x.DiscountPolicy",
		"Signature: double discountFor(int quantity)",
		"private static final int TIER = 100;",
		"Legacy: do not touch.",
		"Source:\nreturn 0.15;",
	} {
		if !strings.Contains(user, want) {
			t.Errorf("user prompt lacks %q:\n%s", want, user)
		}
	}
	for _, want := range []string{`"hasRule"`, "verbatim", "European Portuguese (pt-PT)"} {
		if !strings.Contains(system, want) {
			t.Errorf("system prompt lacks %q", want)
		}
	}
	if strings.Contains(system, "{{LANG}}") {
		t.Error("language placeholder was not substituted")
	}
}

func TestBusinessRules_OmitsEmptySections(t *testing.T) {
	m := rulesMethod()
	m.ExistingDoc = ""
	_, user := BusinessRules("p", "T", m, "x", nil, "en")
	for _, absent := range []string{"Fields of the class", "Existing documentation"} {
		if strings.Contains(user, absent) {
			t.Errorf("user prompt should not contain %q when there is nothing to show", absent)
		}
	}
}

func TestBusinessRules_Language(t *testing.T) {
	cases := map[string]string{
		"pt": "European Portuguese (pt-PT)", "": "European Portuguese (pt-PT)", "PT-PT": "European Portuguese (pt-PT)",
		"en": "English", "fr": "in fr.",
	}
	for code, want := range cases {
		system, _ := BusinessRules("p", "T", rulesMethod(), "x", nil, code)
		if !strings.Contains(system, want) {
			t.Errorf("lang %q: system prompt lacks %q", code, want)
		}
	}
}

// Like every prompt, this one is a cache key: identical inputs must give
// byte-identical output, and the language must be part of the key.
func TestBusinessRules_IsDeterministicAndLanguageSensitive(t *testing.T) {
	fields := []string{"a", "b", "c"}
	s1, u1 := BusinessRules("p", "T", rulesMethod(), "src", fields, "pt")
	for i := 0; i < 50; i++ {
		s2, u2 := BusinessRules("p", "T", rulesMethod(), "src", fields, "pt")
		if s1 != s2 || u1 != u2 {
			t.Fatal("prompt changed between identical calls")
		}
	}
	if s3, _ := BusinessRules("p", "T", rulesMethod(), "src", fields, "en"); s3 == s1 {
		t.Error("changing the language must change the system prompt")
	}
}

func TestBusinessRulesRetry(t *testing.T) {
	_, user := BusinessRules("p", "T", rulesMethod(), "src", nil, "en")
	retry := BusinessRulesRetry(user, "  not json  ", "no JSON object found")
	for _, want := range []string{user, "no JSON object found", "not json", "only the JSON object"} {
		if !strings.Contains(retry, want) {
			t.Errorf("retry prompt lacks %q", want)
		}
	}
	if retry == user {
		t.Error("retry must differ from the original prompt (it is a different cache key)")
	}

	long := BusinessRulesRetry(user, strings.Repeat("x", 10000), "p")
	if len(long) > len(user)+maxRetryEcho+400 {
		t.Errorf("retry echoes too much of a long answer: %d bytes", len(long))
	}
}
