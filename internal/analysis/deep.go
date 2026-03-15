package analysis

const (
	// DefaultCallGraphDepth is the max traversal depth for call graph analysis.
	DefaultCallGraphDepth = 10
	// DefaultDataFlowDepth is the max traversal depth for data flow tracing.
	DefaultDataFlowDepth = 8
	// DefaultLSPTimeout is the default timeout for LSP server operations.
	DefaultLSPTimeout = 30 // seconds
)

// Analyzer layer identifiers for CallGraph/DataFlow results.
const (
	LayerLSP        = "lsp"
	LayerGoAST      = "goast"
	LayerTreeSitter = "treesitter"
	LayerRegex      = "regex"
	LayerPython     = "python"
	LayerTypeScript = "typescript"
)

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
