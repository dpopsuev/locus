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

// Common tree-sitter node type names used across analyzers.
const (
	nodeFuncDecl      = "function_declaration"
	nodeMethodDecl    = "method_declaration"
	nodeTypeID        = "type_identifier"
	nodeStructType    = "struct_type"
	nodeInterfaceType = "interface_type"
	nodePointerType   = "pointer_type"
	nodeQualifiedType = "qualified_type"
)

// Common kind strings for ClassInfo.
const (
	kindStruct    = "struct"
	kindInterface = "interface"
)

// Common directory/file names used for skip logic.
const (
	dirVendor   = "vendor"
	dirTestdata = "testdata"
)

// Common file extensions.
const (
	extGo   = ".go"
	extRust = ".rs"
	extPy   = ".py"
	extTS   = ".ts"
	extJS   = ".js"
	extJava = ".java"
	extTSX  = ".tsx"
	extJSX  = ".jsx"
)

// pkgRoot is the package name used for files at the repository root.
const pkgRoot = "(root)"

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
