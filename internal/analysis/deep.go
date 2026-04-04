package analysis

import "github.com/dpopsuev/locus/internal/oculus"

// Constants re-exported from oculus.
const (
	DefaultCallGraphDepth = oculus.DefaultCallGraphDepth
	DefaultDataFlowDepth  = oculus.DefaultDataFlowDepth
	DefaultLSPTimeout     = oculus.DefaultLSPTimeout
)

// Layer identifiers re-exported from oculus.
const (
	LayerLSP        = oculus.LayerLSP
	LayerGoAST      = oculus.LayerGoAST
	LayerTreeSitter = oculus.LayerTreeSitter
	LayerRegex      = oculus.LayerRegex
	LayerPython     = oculus.LayerPython
	LayerTypeScript = oculus.LayerTypeScript
)

// Deep types re-exported from oculus.
type CallGraphOpts = oculus.CallGraphOpts
type CallEdge = oculus.CallEdge
type FuncNode = oculus.FuncNode
type CallGraph = oculus.CallGraph
type DataFlowNode = oculus.DataFlowNode
type DataFlowEdge = oculus.DataFlowEdge
type TrustBoundary = oculus.TrustBoundary
type DataFlow = oculus.DataFlow
type StateTransition = oculus.StateTransition
type StateMachine = oculus.StateMachine

// Internal constants used by analyzers (not part of public Oculus API).
const (
	nodeFuncDecl      = "function_declaration"
	nodeMethodDecl    = "method_declaration"
	nodeTypeID        = "type_identifier"
	nodeStructType    = "struct_type"
	nodeInterfaceType = "interface_type"
	nodePointerType   = "pointer_type"
	nodeQualifiedType = "qualified_type"
)

const (
	kindStruct    = "struct"
	kindInterface = "interface"
)

const (
	dirVendor   = "vendor"
	dirTestdata = "testdata"
)

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

const pkgRoot = "(root)"
