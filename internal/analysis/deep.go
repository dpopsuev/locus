package analysis

// DeepAnalyzer extracts cross-function, cross-package structural
// information for Tier 3 diagrams (dataflow, call graph, state machine).
//
// Three implementations exist, mirroring TypeAnalyzer:
//   - TreeSitterDeepAnalyzer (syntactic, D&C by package)
//   - LSPDeepAnalyzer (semantic, single gopls connection)
//   - RegexDeepAnalyzer (best-effort fallback)
//
// DeepFallbackAnalyzer chains them LSP -> TreeSitter -> Regex.
type DeepAnalyzer interface {
	CallGraph(root string, opts CallGraphOpts) (*CallGraph, error)
	DataFlowTrace(root, entry string, depth int) (*DataFlow, error)
	DetectStateMachines(root string) ([]StateMachine, error)
}
