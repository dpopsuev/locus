package analysis

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
