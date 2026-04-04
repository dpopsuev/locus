package oculus

// --- ISP: Role-specific interfaces ---

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

// TypeAnalyzer extracts type-level structural metadata from source code.
// Composed of ClassAnalyzer + CallAnalyzer + MetricAnalyzer.
type TypeAnalyzer interface {
	ClassAnalyzer
	CallAnalyzer
	MetricAnalyzer
}

// DeepAnalyzer extracts cross-function, cross-package structural
// information for call graphs, data flow, and state machines.
type DeepAnalyzer interface {
	CallGraph(root string, opts CallGraphOpts) (*CallGraph, error)
	DataFlowTrace(root, entry string, depth int) (*DataFlow, error)
	DetectStateMachines(root string) ([]StateMachine, error)
}
