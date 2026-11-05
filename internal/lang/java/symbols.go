package java

import (
	"context"
	"os"
	"strings"

	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/langmodel"
	sitter "github.com/smacker/go-tree-sitter"
)

// rawParam is a method/constructor parameter before its type has been
// resolved against the project's symbol table.
type rawParam struct {
	name    string
	typeRaw string
}

// rawCall is a method_invocation found in a method body, before its
// receiver has been resolved.
type rawCall struct {
	methodName string
	objectRaw  string // "" for an unqualified call or an implicit/explicit `this` call
}

// rawMethod is a method or constructor before type resolution.
type rawMethod struct {
	name        string
	params      []rawParam
	returnRaw   string // "" for constructors
	modifiers   []string
	existingDoc string
	calls       []rawCall
	signals     *langmodel.Signals // nil when the method has no body
	srcRange    langmodel.Range
}

// rawField is a field before its type has been resolved.
type rawField struct {
	name      string
	typeRaw   string
	modifiers []string
	srcRange  langmodel.Range
}

// rawType is a top-level class/interface/enum/record/annotation before
// extends/implements references have been resolved. Nested/inner type
// declarations are intentionally not walked — out of scope for MVP, same
// spirit as skipping generics inference and overload resolution.
type rawType struct {
	kind        langmodel.TypeKind
	name        string
	pkg         string
	existingDoc string
	srcRange    langmodel.Range

	// superclassRaw is the class this type extends (KindClass only).
	superclassRaw string
	// interfacesRaw is: for a class, its `implements` list; for an
	// interface, its `extends` list. Which Relationship kind it becomes is
	// decided in resolve.go based on `kind`.
	interfacesRaw []string

	fields  []rawField
	methods []rawMethod
}

// parseFile parses a single Java source file into its package name and the
// top-level types it declares.
func parseFile(ctx context.Context, path string) (pkg string, types []*rawType, diags []langmodel.Diagnostic, err error) {
	source, readErr := os.ReadFile(path)
	if readErr != nil {
		return "", nil, nil, readErr
	}

	tree, parseErr := parseSource(ctx, source)
	if parseErr != nil {
		return "", nil, []langmodel.Diagnostic{{File: path, Message: parseErr.Error()}}, nil
	}
	defer tree.Close()

	root := tree.RootNode()
	if root.HasError() {
		diags = append(diags, langmodel.Diagnostic{
			File:    path,
			Message: "syntax errors in file; extraction may be incomplete",
			Warning: true,
		})
	}

	for i := 0; i < int(root.NamedChildCount()); i++ {
		c := root.NamedChild(i)
		if c.Type() == "package_declaration" && c.NamedChildCount() > 0 {
			pkg = text(c.NamedChild(0), source)
		}
	}

	for i := 0; i < int(root.NamedChildCount()); i++ {
		c := root.NamedChild(i)
		kind, ok := typeKindOf(c.Type())
		if !ok {
			continue
		}
		types = append(types, extractType(c, source, kind, pkg, path))
	}

	return pkg, types, diags, nil
}

func typeKindOf(nodeType string) (langmodel.TypeKind, bool) {
	switch nodeType {
	case "class_declaration":
		return langmodel.KindClass, true
	case "interface_declaration":
		return langmodel.KindInterface, true
	case "enum_declaration":
		return langmodel.KindEnum, true
	case "record_declaration":
		return langmodel.KindRecord, true
	case "annotation_type_declaration":
		return langmodel.KindAnnotation, true
	default:
		return 0, false
	}
}

func extractType(n *sitter.Node, source []byte, kind langmodel.TypeKind, pkg, file string) *rawType {
	rt := &rawType{
		kind:        kind,
		name:        text(n.ChildByFieldName("name"), source),
		pkg:         pkg,
		existingDoc: existingDocFor(n, source),
		srcRange:    rangeOf(n, file),
	}

	switch kind {
	case langmodel.KindClass:
		if sc := childByFieldOrType(n, "superclass", "superclass"); sc != nil && sc.NamedChildCount() > 0 {
			rt.superclassRaw = stripGenerics(text(sc.NamedChild(0), source))
		}
		if ifaces := childByFieldOrType(n, "interfaces", "super_interfaces"); ifaces != nil {
			rt.interfacesRaw = extractTypeList(ifaces, source)
		}
	case langmodel.KindInterface:
		if ext := childByFieldOrType(n, "extends", "extends_interfaces"); ext != nil {
			rt.interfacesRaw = extractTypeList(ext, source)
		}
	}

	if body := n.ChildByFieldName("body"); body != nil {
		for i := 0; i < int(body.NamedChildCount()); i++ {
			member := body.NamedChild(i)
			switch member.Type() {
			case "field_declaration":
				rt.fields = append(rt.fields, extractFields(member, source, file)...)
			case "method_declaration":
				rt.methods = append(rt.methods, extractMethod(member, source, file, false))
			case "constructor_declaration":
				rt.methods = append(rt.methods, extractMethod(member, source, file, true))
			}
		}
	}

	return rt
}

func extractTypeList(n *sitter.Node, source []byte) []string {
	listNode := childByFieldOrType(n, "", "type_list")
	if listNode == nil {
		listNode = n
	}
	var out []string
	for i := 0; i < int(listNode.NamedChildCount()); i++ {
		out = append(out, stripGenerics(text(listNode.NamedChild(i), source)))
	}
	return out
}

func extractFields(fieldDecl *sitter.Node, source []byte, file string) []rawField {
	typeRaw := stripGenerics(text(fieldDecl.ChildByFieldName("type"), source))
	mods := modifiers(childByFieldOrType(fieldDecl, "", "modifiers"))

	var fields []rawField
	for i := 0; i < int(fieldDecl.NamedChildCount()); i++ {
		c := fieldDecl.NamedChild(i)
		if c.Type() != "variable_declarator" {
			continue
		}
		fields = append(fields, rawField{
			name:      text(c.ChildByFieldName("name"), source),
			typeRaw:   typeRaw,
			modifiers: mods,
			srcRange:  rangeOf(fieldDecl, file),
		})
	}
	return fields
}

func extractMethod(n *sitter.Node, source []byte, file string, isConstructor bool) rawMethod {
	returnRaw := ""
	if !isConstructor {
		returnRaw = stripGenerics(text(n.ChildByFieldName("type"), source))
	}

	var params []rawParam
	if paramsNode := n.ChildByFieldName("parameters"); paramsNode != nil {
		for i := 0; i < int(paramsNode.NamedChildCount()); i++ {
			p := paramsNode.NamedChild(i)
			if p.Type() != "formal_parameter" && p.Type() != "spread_parameter" {
				continue
			}
			params = append(params, rawParam{
				name:    text(p.ChildByFieldName("name"), source),
				typeRaw: stripGenerics(text(p.ChildByFieldName("type"), source)),
			})
		}
	}

	var calls []rawCall
	var signals *langmodel.Signals
	if body := n.ChildByFieldName("body"); body != nil {
		collectCalls(body, source, &calls)
		signals = computeSignals(body, source)
	}

	return rawMethod{
		name:        text(n.ChildByFieldName("name"), source),
		params:      params,
		returnRaw:   returnRaw,
		modifiers:   modifiers(childByFieldOrType(n, "", "modifiers")),
		existingDoc: existingDocFor(n, source),
		calls:       calls,
		signals:     signals,
		srcRange:    rangeOf(n, file),
	}
}

// computeSignals measures a method body; see langmodel.Signals.
func computeSignals(body *sitter.Node, source []byte) *langmodel.Signals {
	s := &langmodel.Signals{}
	countSignals(body, source, s)
	return s
}

func countSignals(n *sitter.Node, source []byte, s *langmodel.Signals) {
	switch t := n.Type(); {
	case t == "local_variable_declaration" || strings.HasSuffix(t, "_statement"):
		s.Statements++
		switch t {
		case "if_statement":
			s.Branches++
		case "throw_statement":
			s.Throws++
		}
	case t == "ternary_expression":
		s.Branches++
	case t == "switch_label":
		// `default` is the fallthrough, not a decision of its own.
		if strings.HasPrefix(strings.TrimSpace(text(n, source)), "case") {
			s.Branches++
		}
	case t == "binary_expression":
		switch text(n.ChildByFieldName("operator"), source) {
		case "<", ">", "<=", ">=", "==", "!=":
			s.Comparisons++
		}
	case t == "method_invocation":
		switch text(n.ChildByFieldName("name"), source) {
		case "equals", "equalsIgnoreCase", "compareTo", "matches":
			s.Comparisons++
		}
	case t == "string_literal":
		if len(strings.Trim(text(n, source), `"`)) > 0 {
			s.Literals++
		}
	case strings.HasSuffix(t, "_integer_literal") || t == "decimal_floating_point_literal" || t == "hex_floating_point_literal":
		if !trivialNumber(text(n, source)) {
			s.Literals++
		}
	}
	for i := 0; i < int(n.NamedChildCount()); i++ {
		countSignals(n.NamedChild(i), source, s)
	}
}

// trivialNumber reports whether a numeric literal is 0 or 1 in any spelling
// (0, 1L, 0.0, 1.0f, ...): loop starts, increments and identity values are
// not business thresholds.
func trivialNumber(lit string) bool {
	lit = strings.ToLower(strings.TrimRight(strings.ReplaceAll(lit, "_", ""), "lfd"))
	switch lit {
	case "0", "1", "0.0", "1.0", "0.", "1.", ".0":
		return true
	}
	return false
}

func collectCalls(n *sitter.Node, source []byte, out *[]rawCall) {
	if n.Type() == "method_invocation" {
		*out = append(*out, rawCall{
			methodName: text(n.ChildByFieldName("name"), source),
			objectRaw:  text(n.ChildByFieldName("object"), source),
		})
	}
	for i := 0; i < int(n.NamedChildCount()); i++ {
		collectCalls(n.NamedChild(i), source, out)
	}
}

// existingDocFor returns n's preceding /** ... */ comment, if any, so the
// pipeline can later decide whether to preserve, augment, or replace
// human-written documentation rather than blindly overwriting it.
func existingDocFor(n *sitter.Node, source []byte) string {
	prev := n.PrevSibling()
	if prev == nil {
		return ""
	}
	t := text(prev, source)
	if strings.HasPrefix(strings.TrimSpace(t), "/**") {
		return t
	}
	return ""
}

// stripGenerics drops <...> type-argument sections (including nested ones)
// from a raw type reference, e.g. "Map<String, List<Integer>>" -> "Map".
// Documentation-level type resolution doesn't need generic arguments.
func stripGenerics(s string) string {
	var b strings.Builder
	depth := 0
	for _, r := range s {
		switch r {
		case '<':
			depth++
			continue
		case '>':
			if depth > 0 {
				depth--
			}
			continue
		}
		if depth == 0 {
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}
