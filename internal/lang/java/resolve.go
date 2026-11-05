package java

import (
	"strings"

	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/langmodel"
)

// symbolTable maps a simple type name to every rawType across the project
// sharing that name. Usually a single entry; more than one means the simple
// name is ambiguous across packages (handled in resolveTypeRef).
type symbolTable map[string][]*rawType

func buildSymbolTable(files []fileResult) symbolTable {
	st := symbolTable{}
	for _, f := range files {
		for _, t := range f.types {
			st[t.name] = append(st[t.name], t)
		}
	}
	return st
}

// normalizeForLookup strips array/varargs suffixes and any package
// qualification from a raw type reference, leaving the simple name to key
// the symbol table with.
func normalizeForLookup(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.TrimSuffix(s, "...")
	s = strings.TrimSpace(s)
	for strings.HasSuffix(s, "[]") {
		s = strings.TrimSpace(strings.TrimSuffix(s, "[]"))
	}
	if idx := strings.LastIndex(s, "."); idx >= 0 {
		s = s[idx+1:]
	}
	return s
}

// resolveTypeRef looks up a raw type reference against the project's symbol
// table. No import resolution: a simple name is matched same-package first,
// then against any project type sharing that name — good enough for
// documentation, but a type name reused across two unrelated packages could
// resolve to the wrong one (documented limitation, not a compiler).
func resolveTypeRef(st symbolTable, raw, pkg string) langmodel.TypeRef {
	if raw == "" {
		return langmodel.TypeRef{}
	}

	base := normalizeForLookup(raw)
	candidates := st[base]
	if len(candidates) == 0 {
		return langmodel.TypeRef{Name: strings.TrimSpace(raw), External: true}
	}

	for _, c := range candidates {
		if c.pkg == pkg {
			return langmodel.TypeRef{Name: c.name, Package: c.pkg}
		}
	}
	c := candidates[0]
	return langmodel.TypeRef{Name: c.name, Package: c.pkg}
}

// Resolve turns the raw, per-file parse results for an entire project into
// a fully cross-referenced CodeModel.
func Resolve(files []fileResult) *langmodel.CodeModel {
	st := buildSymbolTable(files)

	packages := map[string]*langmodel.Package{}
	var pkgOrder []string
	var relationships []langmodel.Relationship

	for _, f := range files {
		for _, rt := range f.types {
			decl := resolveTypeDecl(rt, st)

			pkgEntry, ok := packages[rt.pkg]
			if !ok {
				pkgEntry = &langmodel.Package{Name: rt.pkg}
				packages[rt.pkg] = pkgEntry
				pkgOrder = append(pkgOrder, rt.pkg)
			}
			pkgEntry.Types = append(pkgEntry.Types, decl)

			relationships = append(relationships, relationshipsFor(rt, st)...)
		}
	}

	pkgList := make([]langmodel.Package, 0, len(pkgOrder))
	for _, name := range pkgOrder {
		pkgList = append(pkgList, *packages[name])
	}

	return &langmodel.CodeModel{
		Language:      "java",
		Packages:      pkgList,
		Relationships: relationships,
	}
}

func relationshipsFor(rt *rawType, st symbolTable) []langmodel.Relationship {
	self := langmodel.TypeRef{Name: rt.name, Package: rt.pkg}
	var rels []langmodel.Relationship

	if rt.superclassRaw != "" {
		rels = append(rels, langmodel.Relationship{
			From: self, To: resolveTypeRef(st, rt.superclassRaw, rt.pkg), Kind: langmodel.RelExtends,
		})
	}

	implKind := langmodel.RelImplements
	if rt.kind == langmodel.KindInterface {
		implKind = langmodel.RelExtends
	}
	for _, ifaceRaw := range rt.interfacesRaw {
		rels = append(rels, langmodel.Relationship{
			From: self, To: resolveTypeRef(st, ifaceRaw, rt.pkg), Kind: implKind,
		})
	}

	for _, field := range rt.fields {
		target := resolveTypeRef(st, field.typeRaw, rt.pkg)
		if !target.External {
			rels = append(rels, langmodel.Relationship{From: self, To: target, Kind: langmodel.RelHasField})
		}
	}

	return rels
}

func resolveTypeDecl(rt *rawType, st symbolTable) langmodel.TypeDecl {
	decl := langmodel.TypeDecl{
		Kind:        rt.kind,
		Name:        rt.name,
		Package:     rt.pkg,
		ExistingDoc: rt.existingDoc,
		SourceRange: rt.srcRange,
	}

	if rt.superclassRaw != "" {
		decl.Extends = append(decl.Extends, resolveTypeRef(st, rt.superclassRaw, rt.pkg))
	}
	if rt.kind == langmodel.KindInterface {
		for _, ifaceRaw := range rt.interfacesRaw {
			decl.Extends = append(decl.Extends, resolveTypeRef(st, ifaceRaw, rt.pkg))
		}
	} else {
		for _, ifaceRaw := range rt.interfacesRaw {
			decl.Implements = append(decl.Implements, resolveTypeRef(st, ifaceRaw, rt.pkg))
		}
	}

	for _, f := range rt.fields {
		decl.Fields = append(decl.Fields, langmodel.Field{
			Name:        f.name,
			Type:        resolveTypeRef(st, f.typeRaw, rt.pkg),
			Modifiers:   f.modifiers,
			SourceRange: f.srcRange,
		})
	}

	for _, m := range rt.methods {
		decl.Methods = append(decl.Methods, resolveMethod(m, rt, st))
	}

	return decl
}

func resolveMethod(m rawMethod, owner *rawType, st symbolTable) langmodel.Method {
	var params []langmodel.Param
	for _, p := range m.params {
		params = append(params, langmodel.Param{Name: p.name, Type: resolveTypeRef(st, p.typeRaw, owner.pkg)})
	}

	var returnType langmodel.TypeRef
	if m.returnRaw != "" {
		returnType = resolveTypeRef(st, m.returnRaw, owner.pkg)
	}

	var callsOut []langmodel.CallRef
	for _, c := range m.calls {
		callsOut = append(callsOut, resolveCall(c, owner, st))
	}

	return langmodel.Method{
		Name:        m.name,
		Params:      params,
		ReturnType:  returnType,
		Modifiers:   m.modifiers,
		CallsOut:    callsOut,
		ExistingDoc: m.existingDoc,
		SourceRange: m.srcRange,
	}
}

// resolveCall resolves a call's receiver only for the cases that matter for
// documentation-quality relationships: implicit/explicit `this`, and a bare
// identifier that names one of the owning type's own fields. Local
// variables, chained calls, static references, and casts are left
// unresolved rather than guessed — see the plan's documented scope limit on
// call-graph resolution.
func resolveCall(c rawCall, owner *rawType, st symbolTable) langmodel.CallRef {
	var receiver langmodel.TypeRef

	switch {
	case c.objectRaw == "" || c.objectRaw == "this":
		receiver = langmodel.TypeRef{Name: owner.name, Package: owner.pkg}
	default:
		if fieldType, ok := fieldTypeOf(owner, c.objectRaw); ok {
			receiver = resolveTypeRef(st, fieldType, owner.pkg)
		} else {
			return langmodel.CallRef{
				MethodName: c.methodName,
				Receiver:   langmodel.TypeRef{Name: c.objectRaw, External: true},
				Resolved:   false,
			}
		}
	}

	resolved := !receiver.External && receiverDeclaresMethod(receiver, c.methodName, st)
	return langmodel.CallRef{MethodName: c.methodName, Receiver: receiver, Resolved: resolved}
}

func fieldTypeOf(owner *rawType, name string) (string, bool) {
	for _, f := range owner.fields {
		if f.name == name {
			return f.typeRaw, true
		}
	}
	return "", false
}

func receiverDeclaresMethod(receiver langmodel.TypeRef, methodName string, st symbolTable) bool {
	for _, t := range st[receiver.Name] {
		if t.pkg != receiver.Package {
			continue
		}
		for _, m := range t.methods {
			if m.name == methodName {
				return true
			}
		}
	}
	return false
}
