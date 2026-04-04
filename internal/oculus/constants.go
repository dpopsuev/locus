package oculus

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
