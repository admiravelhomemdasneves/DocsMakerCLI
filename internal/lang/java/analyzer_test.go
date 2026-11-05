package java

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/langmodel"
)

func writeJava(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func findType(model *langmodel.CodeModel, name string) *langmodel.TypeDecl {
	for _, pkg := range model.Packages {
		for i := range pkg.Types {
			if pkg.Types[i].Name == name {
				return &pkg.Types[i]
			}
		}
	}
	return nil
}

func findMethod(decl *langmodel.TypeDecl, name string) *langmodel.Method {
	for i := range decl.Methods {
		if decl.Methods[i].Name == name {
			return &decl.Methods[i]
		}
	}
	return nil
}

func hasRelationship(model *langmodel.CodeModel, fromName, toName string, kind langmodel.RelKind) bool {
	for _, r := range model.Relationships {
		if r.From.Name == fromName && r.To.Name == toName && r.Kind == kind {
			return true
		}
	}
	return false
}

const greeterSrc = `package com.example;

public interface Greeter {
    String greet(String name);
}
`

const namedSrc = `package com.example;

public interface Named extends Greeter {
    String getName();
}
`

const animalSrc = `package com.example;

/**
 * Base type for anything that can be greeted.
 */
public abstract class Animal {
    protected String name;

    public Animal(String name) {
        this.name = name;
    }

    public String getName() {
        return name;
    }
}
`

const dogSrc = `package com.example;

public class Dog extends Animal implements Greeter {
    private final Greeter greeter;

    public Dog(String name, Greeter greeter) {
        super(name);
        this.greeter = greeter;
    }

    /**
     * Greets using the configured Greeter, falling back to the dog's own name.
     */
    public String greet(String name) {
        String own = getName();
        return greeter.greet(own);
    }
}
`

func TestParseProject_ExtendsImplementsFieldsAndCalls(t *testing.T) {
	dir := t.TempDir()
	files := []string{
		writeJava(t, dir, "Greeter.java", greeterSrc),
		writeJava(t, dir, "Named.java", namedSrc),
		writeJava(t, dir, "Animal.java", animalSrc),
		writeJava(t, dir, "Dog.java", dogSrc),
	}

	a := &Analyzer{}
	model, diags, err := a.ParseProject(context.Background(), files)
	if err != nil {
		t.Fatalf("ParseProject returned error: %v", err)
	}
	for _, d := range diags {
		t.Logf("diagnostic: %+v", d)
	}

	if model.Language != "java" {
		t.Fatalf("Language = %q, want java", model.Language)
	}

	// Package grouping
	if len(model.Packages) != 1 || model.Packages[0].Name != "com.example" {
		t.Fatalf("Packages = %+v, want a single com.example package", model.Packages)
	}
	if len(model.Packages[0].Types) != 4 {
		t.Fatalf("got %d types, want 4", len(model.Packages[0].Types))
	}

	// interface extends interface
	named := findType(model, "Named")
	if named == nil {
		t.Fatal("Named type not found")
	}
	if named.Kind != langmodel.KindInterface {
		t.Fatalf("Named.Kind = %v, want interface", named.Kind)
	}
	if len(named.Extends) != 1 || named.Extends[0].Name != "Greeter" || named.Extends[0].External {
		t.Fatalf("Named.Extends = %+v, want resolved [Greeter]", named.Extends)
	}

	// class extends class, implements interface
	dog := findType(model, "Dog")
	if dog == nil {
		t.Fatal("Dog type not found")
	}
	if len(dog.Extends) != 1 || dog.Extends[0].Name != "Animal" || dog.Extends[0].External {
		t.Fatalf("Dog.Extends = %+v, want resolved [Animal]", dog.Extends)
	}
	if len(dog.Implements) != 1 || dog.Implements[0].Name != "Greeter" || dog.Implements[0].External {
		t.Fatalf("Dog.Implements = %+v, want resolved [Greeter]", dog.Implements)
	}

	// field of a project type
	if len(dog.Fields) != 1 || dog.Fields[0].Name != "greeter" || dog.Fields[0].Type.Name != "Greeter" || dog.Fields[0].Type.External {
		t.Fatalf("Dog.Fields = %+v, want resolved greeter:Greeter", dog.Fields)
	}
	if !hasRelationship(model, "Dog", "Greeter", langmodel.RelHasField) {
		t.Fatalf("expected Dog -HasField-> Greeter relationship, got %+v", model.Relationships)
	}
	if !hasRelationship(model, "Dog", "Animal", langmodel.RelExtends) {
		t.Fatalf("expected Dog -Extends-> Animal relationship, got %+v", model.Relationships)
	}
	if !hasRelationship(model, "Dog", "Greeter", langmodel.RelImplements) {
		t.Fatalf("expected Dog -Implements-> Greeter relationship, got %+v", model.Relationships)
	}

	// existing javadoc preserved
	animal := findType(model, "Animal")
	if animal == nil || animal.ExistingDoc == "" {
		t.Fatalf("Animal.ExistingDoc should be non-empty, got %+v", animal)
	}

	// method-level: params, return type, existing doc
	greet := findMethod(dog, "greet")
	if greet == nil {
		t.Fatal("Dog.greet method not found")
	}
	if greet.ReturnType.Name != "String" {
		t.Fatalf("greet.ReturnType = %+v, want String", greet.ReturnType)
	}
	if len(greet.Params) != 1 || greet.Params[0].Name != "name" {
		t.Fatalf("greet.Params = %+v, want [name]", greet.Params)
	}
	if greet.ExistingDoc == "" {
		t.Fatal("greet.ExistingDoc should be non-empty")
	}

	// call resolution: getName() unqualified -> resolved to Dog itself only if
	// Dog declares it; Dog doesn't (it's on Animal), so this call should be
	// unresolved per the plan's documented scope (no inherited-method lookup).
	var getNameCall, greeterGreetCall *langmodel.CallRef
	for i := range greet.CallsOut {
		switch {
		case greet.CallsOut[i].MethodName == "getName":
			getNameCall = &greet.CallsOut[i]
		case greet.CallsOut[i].MethodName == "greet" && greet.CallsOut[i].Receiver.Name == "Greeter":
			greeterGreetCall = &greet.CallsOut[i]
		}
	}
	if getNameCall == nil {
		t.Fatal("expected a call to getName in CallsOut")
	}
	if greeterGreetCall == nil {
		t.Fatal("expected a resolved call to greeter.greet in CallsOut")
	}
	if !greeterGreetCall.Resolved {
		t.Fatalf("greeter.greet() call should resolve (Greeter field, Greeter declares greet): %+v", greeterGreetCall)
	}
}

func TestParseProject_UnknownFileSkippedAsDiagnostic(t *testing.T) {
	a := &Analyzer{}
	model, diags, err := a.ParseProject(context.Background(), []string{filepath.Join(t.TempDir(), "missing.java")})
	if err != nil {
		t.Fatalf("ParseProject returned error: %v", err)
	}
	if len(diags) == 0 {
		t.Fatal("expected a diagnostic for the missing file")
	}
	if len(model.Packages) != 0 {
		t.Fatalf("expected no packages, got %+v", model.Packages)
	}
}

const signalsSrc = `package com.example;

public abstract class Rules {
    private int n;

    public int getN() {
        return n;
    }

    public abstract boolean accepts(int v);

    public double fee(int qty, String kind) {
        if (qty < 0) {
            throw new IllegalArgumentException("negative");
        }
        if ("VIP".equals(kind)) {
            return 0.0;
        } else if (qty >= 100) {
            return 0.15;
        }
        return qty > 10 ? 2.5 : 1;
    }

    public String label(int code) {
        switch (code) {
            case 200:
                return "ok";
            case 404:
                return "missing";
            default:
                return "";
        }
    }

    public int sum(int[] xs) {
        int total = 0;
        for (int i = 0; i < xs.length; i++) {
            total += xs[i] + 1;
        }
        return total;
    }
}
`

func TestParseProject_MethodSignals(t *testing.T) {
	dir := t.TempDir()
	files := []string{writeJava(t, dir, "Rules.java", signalsSrc)}

	model, _, err := (&Analyzer{}).ParseProject(context.Background(), files)
	if err != nil {
		t.Fatal(err)
	}
	rules := findType(model, "Rules")
	if rules == nil {
		t.Fatal("type Rules not found")
	}

	if m := findMethod(rules, "accepts"); m == nil || m.Signals != nil {
		t.Errorf("abstract method should have nil Signals, got %+v", m)
	}

	cases := map[string]langmodel.Signals{
		// one return statement, nothing else
		"getN": {Statements: 1},
		// 1 throw; decisions and operators counted as follows:
		// branches = if(qty<0) + if(VIP) + else-if(qty>=100) + ternary = 4
		// comparisons = qty<0, equals, qty>=100, qty>10 = 4
		// literals = "negative", "VIP", 100, 0.15, 10, 2.5 = 6 (0.0 and 1 are trivial)
		"fee": {Statements: 7, Branches: 4, Throws: 1, Comparisons: 4, Literals: 6},
		// two `case` labels count, `default` does not; the grammar models a
		// switch as an expression, so only the three returns are statements
		"label": {Statements: 3, Branches: 2, Literals: 4},
		// the loop's 0 and 1 are not thresholds; i < xs.length is a comparison
		"sum": {Statements: 5, Comparisons: 1},
	}
	for name, want := range cases {
		m := findMethod(rules, name)
		if m == nil || m.Signals == nil {
			t.Errorf("%s: missing method or Signals (%+v)", name, m)
			continue
		}
		if *m.Signals != want {
			t.Errorf("%s: Signals = %+v, want %+v", name, *m.Signals, want)
		}
	}
}
