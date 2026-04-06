package lang

// Rules defines language-specific naming conventions for symbol quality checks.
// Each language implements this interface with its own idioms — Go allows noun
// types, Python expects snake_case verbs, etc. GenericRules is the safe default.
type Rules interface {
	// IsVerblessViolation returns true if the exported symbol name violates
	// the language's naming convention. kind is the symbol's structural role:
	// "function", "method", "struct", "interface", "type", "constant", "variable".
	IsVerblessViolation(name string, kind string) bool

	// StdlibIdioms returns abbreviations considered idiomatic for the language's
	// standard library (e.g. Go: DB, HTTP, URL; Python: os, io, re).
	StdlibIdioms() map[string]bool

	// KnownVerbPrefixes returns verb prefixes that indicate an action name
	// (e.g. Get, Set, Create for Go; get_, set_, create_ for Python).
	KnownVerbPrefixes() []string
}

// isNonFunctionKind returns true for symbol kinds that are types/values
// (not functions/methods). These are typically exempt from verb-prefix rules.
func isNonFunctionKind(kind string) bool {
	switch kind {
	case "struct", "interface", "class", "enum", "type-parameter",
		"constant", "variable", "field", "property", "enum-member":
		return true
	}
	return false
}
