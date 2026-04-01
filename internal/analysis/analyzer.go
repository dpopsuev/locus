package analysis

// --- ISP: Role-specific interfaces (TSK-240) ---

// ClassAnalyzer extracts type declarations and implementation relationships.
type ClassAnalyzer interface {
	Classes(root string) ([]ClassInfo, error)
	Implements(root string) ([]ImplEdge, error)
}

// CallAnalyzer extracts call chains and entry points.
type CallAnalyzer interface {
	CallChain(root, entry string, depth int) ([]Call, error)
	EntryPoints(root string) ([]EntryPoint, error)
}

// MetricAnalyzer extracts structural metrics (nesting, field references).
type MetricAnalyzer interface {
	FieldRefs(root string) ([]FieldRef, error)
	NestingDepth(root string) ([]NestingResult, error)
}

// --- Composite interface (backward compatible) ---

// TypeAnalyzer extracts type-level structural metadata from source code.
// Composed of ClassAnalyzer + CallAnalyzer + MetricAnalyzer.
// Three implementations exist: LSPAnalyzer (semantic), TreeSitterAnalyzer
// (syntactic), and RegexAnalyzer (best-effort). FallbackAnalyzer chains
// them in order of fidelity.
type TypeAnalyzer interface {
	ClassAnalyzer
	CallAnalyzer
	MetricAnalyzer
}
