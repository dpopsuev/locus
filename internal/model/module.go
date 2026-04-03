package model

import "encoding/json"

// Language identifies the programming language of a project.
type Language int

const (
	LangUnknown Language = iota
	LangGo
	LangRust
	LangPython
	LangTypeScript
	LangC
	LangCpp
	LangJava
	LangJavaScript
	LangZig
	LangKotlin
	LangSwift
	LangCSharp
	LangLua
	LangProto
	LangShell
)

var langNames = [...]string{
	LangUnknown:    "unknown",
	LangGo:         "go",
	LangRust:       "rust",
	LangPython:     "python",
	LangTypeScript: "typescript",
	LangC:          "c",
	LangCpp:        "cpp",
	LangJava:       "java",
	LangJavaScript: "javascript",
	LangZig:        "zig",
	LangKotlin:     "kotlin",
	LangSwift:      "swift",
	LangCSharp:     "csharp",
	LangLua:        "lua",
	LangProto:      "proto",
	LangShell:      "shell",
}

func (l Language) String() string {
	if int(l) < len(langNames) {
		return langNames[l]
	}
	return "unknown"
}

func (l Language) MarshalJSON() ([]byte, error) {
	return json.Marshal(l.String())
}

func (l *Language) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	for i, name := range langNames {
		if name == s {
			*l = Language(i)
			return nil
		}
	}
	return nil
}

// Project represents a source code project (module, crate, package, etc.).
type Project struct {
	Path            string           `json:"path"`
	Language        Language         `json:"language,omitempty"`
	Namespaces      []*Namespace     `json:"namespaces"`
	DependencyGraph *DependencyGraph `json:"dependency_graph,omitempty"`
}

// NewProject creates a project with the given root path.
func NewProject(path string) *Project {
	return &Project{Path: path}
}

// AddNamespace appends a namespace to the project.
func (p *Project) AddNamespace(ns *Namespace) {
	p.Namespaces = append(p.Namespaces, ns)
}

// Namespace represents an organizational unit of symbols (Go package,
// Rust module, Python package, TypeScript module, etc.).
type Namespace struct {
	Name       string    `json:"name"`
	ImportPath string    `json:"import_path"`
	Files      []*File   `json:"files,omitempty"`
	Symbols    []*Symbol `json:"symbols,omitempty"`
}

// NewNamespace creates a namespace with the given name and import path.
func NewNamespace(name, importPath string) *Namespace {
	return &Namespace{Name: name, ImportPath: importPath}
}

// AddFile appends a file to the namespace.
func (ns *Namespace) AddFile(f *File) {
	ns.Files = append(ns.Files, f)
}

// AddSymbol appends a symbol to the namespace.
func (ns *Namespace) AddSymbol(s *Symbol) {
	ns.Symbols = append(ns.Symbols, s)
}

// File represents a single source file.
type File struct {
	Path    string `json:"path"`
	Package string `json:"package"`
	Lines   int    `json:"lines,omitempty"`
}

// NewFile creates a file record.
func NewFile(path, pkg string) *File {
	return &File{Path: path, Package: pkg}
}

// Symbol represents a declared name in a namespace.
type Symbol struct {
	Name         string     `json:"name"`
	Kind         SymbolKind `json:"kind"`
	Exported     bool       `json:"exported"`
	File         string     `json:"file,omitempty"`
	Line         int        `json:"line,omitempty"`
	Dependencies []string   `json:"dependencies,omitempty"`
}

// SymbolsFromNames creates exported Symbol values from a list of names.
// Convenience for tests and backward-compatible callers.
func SymbolsFromNames(names ...string) []Symbol {
	syms := make([]Symbol, len(names))
	for i, n := range names {
		syms[i] = Symbol{Name: n, Exported: true}
	}
	return syms
}

// SymbolKind classifies a declared symbol. Values match the LSP specification
// (https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/#symbolKind)
// for zero-cost mapping from LSP responses.
type SymbolKind int

const (
	SymbolFile          SymbolKind = 1
	SymbolModule        SymbolKind = 2
	SymbolNamespace     SymbolKind = 3
	SymbolPackage       SymbolKind = 4
	SymbolClass         SymbolKind = 5
	SymbolMethod        SymbolKind = 6
	SymbolProperty      SymbolKind = 7
	SymbolField         SymbolKind = 8
	SymbolConstructor   SymbolKind = 9
	SymbolEnum          SymbolKind = 10
	SymbolInterface     SymbolKind = 11
	SymbolFunction      SymbolKind = 12
	SymbolVariable      SymbolKind = 13
	SymbolConstant      SymbolKind = 14
	SymbolString        SymbolKind = 15
	SymbolNumber        SymbolKind = 16
	SymbolBoolean       SymbolKind = 17
	SymbolArray         SymbolKind = 18
	SymbolObject        SymbolKind = 19
	SymbolKey           SymbolKind = 20
	SymbolNull          SymbolKind = 21
	SymbolEnumMember    SymbolKind = 22
	SymbolStruct        SymbolKind = 23
	SymbolEvent         SymbolKind = 24
	SymbolOperator      SymbolKind = 25
	SymbolTypeParameter SymbolKind = 26
)

var symbolKindNames = map[SymbolKind]string{
	SymbolFile:          "file",
	SymbolModule:        "module",
	SymbolNamespace:     "namespace",
	SymbolPackage:       "package",
	SymbolClass:         "class",
	SymbolMethod:        "method",
	SymbolProperty:      "property",
	SymbolField:         "field",
	SymbolConstructor:   "constructor",
	SymbolEnum:          "enum",
	SymbolInterface:     "interface",
	SymbolFunction:      "function",
	SymbolVariable:      "variable",
	SymbolConstant:      "constant",
	SymbolString:        "string",
	SymbolNumber:        "number",
	SymbolBoolean:       "boolean",
	SymbolArray:         "array",
	SymbolObject:        "object",
	SymbolKey:           "key",
	SymbolNull:          "null",
	SymbolEnumMember:    "enum-member",
	SymbolStruct:        "struct",
	SymbolEvent:         "event",
	SymbolOperator:      "operator",
	SymbolTypeParameter: "type-parameter",
}

var symbolKindByName map[string]SymbolKind

func init() {
	symbolKindByName = make(map[string]SymbolKind, len(symbolKindNames))
	for k, v := range symbolKindNames {
		symbolKindByName[v] = k
	}
}

func (k SymbolKind) String() string {
	if name, ok := symbolKindNames[k]; ok {
		return name
	}
	return "unknown"
}

func (k SymbolKind) MarshalJSON() ([]byte, error) {
	return json.Marshal(k.String())
}

func (k *SymbolKind) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	if kind, ok := symbolKindByName[s]; ok {
		*k = kind
	}
	return nil
}

// DependencyGraph is a directed graph of namespace-to-namespace dependencies.
type DependencyGraph struct {
	Edges []DependencyEdge `json:"edges"`
}

// NewDependencyGraph creates an empty dependency graph.
func NewDependencyGraph() *DependencyGraph {
	return &DependencyGraph{}
}

// AddEdge records a dependency from one namespace to another.
// Duplicate edges increment weight instead of creating a new entry.
func (g *DependencyGraph) AddEdge(from, to string, external bool) {
	for i := range g.Edges {
		if g.Edges[i].From == from && g.Edges[i].To == to {
			g.Edges[i].Weight++
			return
		}
	}
	g.Edges = append(g.Edges, DependencyEdge{From: from, To: to, External: external, Weight: 1})
}

// SetEdgeCoupling updates CallSites and LOCSurface for an existing edge.
func (g *DependencyGraph) SetEdgeCoupling(from, to string, callSites, locSurface int) {
	for i := range g.Edges {
		if g.Edges[i].From == from && g.Edges[i].To == to {
			g.Edges[i].CallSites = callSites
			g.Edges[i].LOCSurface = locSurface
			return
		}
	}
}

// EdgesFrom returns all edges originating from the given namespace.
func (g *DependencyGraph) EdgesFrom(ns string) []DependencyEdge {
	var out []DependencyEdge
	for _, e := range g.Edges {
		if e.From == ns {
			out = append(out, e)
		}
	}
	return out
}

// DependencyEdge represents a dependency from one namespace to another.
// Weight counts the number of distinct imported symbols (0 = unknown/not computed).
// CallSites counts total invocations of symbols from the dependency.
// LOCSurface counts distinct source lines that reference the dependency.
type DependencyEdge struct {
	From       string `json:"from"`
	To         string `json:"to"`
	External   bool   `json:"external"`
	Weight     int    `json:"weight,omitempty"`
	CallSites  int    `json:"call_sites,omitempty"`
	LOCSurface int    `json:"loc_surface,omitempty"`
}
