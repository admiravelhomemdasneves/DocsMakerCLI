// Package langmodel defines the language-agnostic intermediate representation
// (CodeModel) that every analyzer produces and every downstream consumer
// (docgen, secrets, inserter) reads. Nothing outside internal/lang/* knows
// about tree-sitter, javadoc syntax, or any other source-language detail.
package langmodel

import "encoding/json"

// TypeKind classifies a TypeDecl.
type TypeKind int

const (
	KindClass TypeKind = iota
	KindInterface
	KindEnum
	KindRecord
	KindAnnotation
)

func (k TypeKind) String() string {
	switch k {
	case KindClass:
		return "class"
	case KindInterface:
		return "interface"
	case KindEnum:
		return "enum"
	case KindRecord:
		return "record"
	case KindAnnotation:
		return "annotation"
	default:
		return "unknown"
	}
}

// MarshalJSON renders a TypeKind as its string name so --format json is a
// readable, stable contract rather than leaking the internal int encoding —
// this is the same JSON a future `docsmaker serve` API would hand a UI.
func (k TypeKind) MarshalJSON() ([]byte, error) {
	return json.Marshal(k.String())
}

// RelKind classifies a Relationship between two types.
type RelKind int

const (
	RelExtends RelKind = iota
	RelImplements
	RelUses
	RelHasField
	RelCalls
)

func (k RelKind) String() string {
	switch k {
	case RelExtends:
		return "extends"
	case RelImplements:
		return "implements"
	case RelUses:
		return "uses"
	case RelHasField:
		return "hasField"
	case RelCalls:
		return "calls"
	default:
		return "unknown"
	}
}

// MarshalJSON renders a RelKind as its string name; see TypeKind.MarshalJSON.
func (k RelKind) MarshalJSON() ([]byte, error) {
	return json.Marshal(k.String())
}

// Range locates a span of source inside a file, in byte offsets, so that
// downstream consumers (e.g. inserter) can splice generated docs back into
// the original file without re-parsing.
type Range struct {
	File      string `json:"file"`
	StartByte int    `json:"startByte"`
	EndByte   int    `json:"endByte"`
	StartLine int    `json:"startLine"`
	EndLine   int    `json:"endLine"`
}

// TypeRef is a reference to a type, possibly unresolved (e.g. JDK/third-party
// types that live outside the analyzed project). External is true whenever
// the analyzer could not map the name to a TypeDecl within the CodeModel.
type TypeRef struct {
	Name     string `json:"name"`               // simple or qualified name as it appears in source
	Package  string `json:"package,omitempty"`  // best-effort qualifying package, empty if unknown
	External bool   `json:"external,omitempty"` // true if this type is outside the analyzed project (JDK, third-party, unresolved)
}

// Param is a single method/constructor parameter.
type Param struct {
	Name string  `json:"name"`
	Type TypeRef `json:"type"`
}

// CallRef is a reference to a call made from within a method body. Resolved
// only when the receiver's declared type and the called method could both be
// matched against the project's symbol table; otherwise Resolved is false and
// docgen should treat the call as informational-only.
type CallRef struct {
	MethodName string  `json:"methodName"`
	Receiver   TypeRef `json:"receiver"`
	Resolved   bool    `json:"resolved"`
}

// Method describes a single method or constructor.
type Method struct {
	Name        string    `json:"name"`
	Params      []Param   `json:"params,omitempty"`
	ReturnType  TypeRef   `json:"returnType"`
	Modifiers   []string  `json:"modifiers,omitempty"`
	CallsOut    []CallRef `json:"callsOut,omitempty"`
	ExistingDoc string    `json:"existingDoc,omitempty"`
	SourceRange Range     `json:"sourceRange"`
}

// Field describes a single field/attribute of a type.
type Field struct {
	Name        string   `json:"name"`
	Type        TypeRef  `json:"type"`
	Modifiers   []string `json:"modifiers,omitempty"`
	SourceRange Range    `json:"sourceRange"`
}

// Relationship is a directed edge between two types in the project, used to
// build the "relações entre entidades" the doc pipeline surfaces at class and
// package level.
type Relationship struct {
	From TypeRef `json:"from"`
	To   TypeRef `json:"to"`
	Kind RelKind `json:"kind"`
}

// TypeDecl describes a single class/interface/enum/record/annotation.
type TypeDecl struct {
	Kind        TypeKind  `json:"kind"`
	Name        string    `json:"name"`
	Package     string    `json:"package"`
	Extends     []TypeRef `json:"extends,omitempty"`
	Implements  []TypeRef `json:"implements,omitempty"`
	Fields      []Field   `json:"fields,omitempty"`
	Methods     []Method  `json:"methods,omitempty"`
	ExistingDoc string    `json:"existingDoc,omitempty"`
	SourceRange Range     `json:"sourceRange"`
}

// Package groups the types declared under a single package/namespace.
type Package struct {
	Name  string     `json:"name"`
	Types []TypeDecl `json:"types"`
}

// CodeModel is the complete language-agnostic result of analyzing a project.
// It is the only artifact that docgen, secrets, and inserter operate on —
// none of them import anything from internal/lang/*.
type CodeModel struct {
	Language      string         `json:"language"`
	Packages      []Package      `json:"packages"`
	Relationships []Relationship `json:"relationships,omitempty"`
}

// Diagnostic is a non-fatal issue surfaced during parsing/analysis (e.g. a
// file that failed to parse, or a reference that could not be resolved).
type Diagnostic struct {
	File    string `json:"file"`
	Message string `json:"message"`
	Warning bool   `json:"warning,omitempty"`
}
