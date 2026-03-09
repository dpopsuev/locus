package analysis

// TypeAnalyzer extracts type-level structural metadata from source code.
// Three implementations exist: LSPAnalyzer (semantic), TreeSitterAnalyzer
// (syntactic), and RegexAnalyzer (best-effort). FallbackAnalyzer chains
// them in order of fidelity.
type TypeAnalyzer interface {
	Classes(root string) ([]ClassInfo, error)
	Implements(root string) ([]ImplEdge, error)
	FieldRefs(root string) ([]FieldRef, error)
	CallChain(root, entry string, depth int) ([]Call, error)
	EntryPoints(root string) ([]EntryPoint, error)
	NestingDepth(root string) ([]NestingResult, error)
}
