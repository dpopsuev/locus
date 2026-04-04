// Package oculus provides language-agnostic symbol resolution, call graph
// analysis, and type system inspection. It is the algorithm core that will
// eventually be extracted as a standalone library (github.com/dpopsuev/oculus).
//
// Zero dependency on Locus domain packages (arch, clinic, constraint, diagram,
// impact, protocol). Depends only on model (for Symbol/SymbolKind/Language).
package oculus

// ClassInfo describes a type declaration (struct, interface, class, trait).
type ClassInfo struct {
	Name     string       `json:"name"`
	Package  string       `json:"package"`
	Kind     string       `json:"kind"` // "struct", "interface", "class", "trait"
	Fields   []FieldInfo  `json:"fields,omitempty"`
	Methods  []MethodInfo `json:"methods,omitempty"`
	Exported bool         `json:"exported"`
}

// FieldInfo describes a single field within a type.
type FieldInfo struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Exported bool   `json:"exported"`
	Tag      string `json:"tag,omitempty"`
}

// MethodInfo describes a method on a type.
type MethodInfo struct {
	Name      string `json:"name"`
	Signature string `json:"signature"`
	Exported  bool   `json:"exported"`
}

// ImplEdge captures a type relationship (implements, extends, embeds).
type ImplEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"` // "implements", "extends", "embeds"
}

// FieldRef captures a struct field that references another declared type.
type FieldRef struct {
	Owner   string `json:"owner"`
	Field   string `json:"field"`
	RefType string `json:"ref_type"`
}

// Call represents a single call in a call chain.
type Call struct {
	Caller  string `json:"caller"`
	Callee  string `json:"callee"`
	Package string `json:"package"`
	Line    int    `json:"line,omitempty"`
}

// EntryPoint represents a structurally significant entry function.
type EntryPoint struct {
	Name    string `json:"name"`
	Kind    string `json:"kind"` // "main", "http_handler", "cli_command", "test"
	Package string `json:"package"`
	File    string `json:"file"`
	Line    int    `json:"line,omitempty"`
}

// NestingResult holds the maximum nesting depth for a single function.
type NestingResult struct {
	Function string `json:"function"`
	Package  string `json:"package"`
	MaxDepth int    `json:"max_depth"`
}

// CallGraphOpts configures call graph construction.
type CallGraphOpts struct {
	Entry        string // entry function name; empty = all exported
	Depth        int    // max recursion depth; 0 = default (10)
	ExportedOnly bool   // only include exported functions as roots
	Scope        string // limit to this package prefix
}

// CallEdge represents a single caller->callee edge in the call graph.
type CallEdge struct {
	Caller       string `json:"caller"`
	Callee       string `json:"callee"`
	CallerPkg    string `json:"caller_pkg"`
	CalleePkg    string `json:"callee_pkg"`
	Line         int    `json:"line,omitempty"`
	File         string `json:"file,omitempty"`
	ReceiverType string `json:"receiver_type,omitempty"`
	CrossPkg     bool   `json:"cross_pkg,omitempty"`
}

// FuncNode represents a function in the call graph.
type FuncNode struct {
	Name    string `json:"name"`
	Package string `json:"package"`
	Line    int    `json:"line,omitempty"`
}

// CallGraph is the result of call graph analysis.
type CallGraph struct {
	Nodes []FuncNode `json:"nodes"`
	Edges []CallEdge `json:"edges"`
	Layer string     `json:"layer,omitempty"`
}

// DataFlowNode represents a participant in a data flow.
type DataFlowNode struct {
	Name string `json:"name"`
	Kind string `json:"kind"` // "process", "data_store", "external", "entry"
	Pkg  string `json:"package,omitempty"`
}

// DataFlowEdge represents data moving between nodes.
type DataFlowEdge struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Label string `json:"label,omitempty"`
}

// TrustBoundary defines a security boundary containing nodes.
type TrustBoundary struct {
	Name  string   `json:"name"`
	Nodes []string `json:"nodes"`
}

// DataFlow is the result of data flow analysis.
type DataFlow struct {
	Nodes      []DataFlowNode  `json:"nodes"`
	Edges      []DataFlowEdge  `json:"edges"`
	Boundaries []TrustBoundary `json:"boundaries,omitempty"`
	Layer      string          `json:"layer,omitempty"`
}

// StateTransition represents a transition between states.
type StateTransition struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Trigger string `json:"trigger,omitempty"`
}

// StateMachine represents a detected state machine pattern.
type StateMachine struct {
	Name        string            `json:"name"`
	Package     string            `json:"package"`
	States      []string          `json:"states"`
	Transitions []StateTransition `json:"transitions"`
	Initial     string            `json:"initial,omitempty"`
}
