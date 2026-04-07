package core

import (
	"github.com/dpopsuev/locus/internal/arch"
	"github.com/dpopsuev/locus/internal/history"
	"github.com/dpopsuev/locus/internal/oculus"
)

// Options controls which diagram is rendered and how it is scoped.
type Options struct {
	Type         string // dependency, c4, coupling, churn, layers, tree, classes, sequence, er, dataflow, callgraph, state
	Scope        string // restrict to a single component (empty = all)
	Depth        int    // grouping depth override (0 = use report's SuggestedDepth)
	TopN         int    // limit items shown (0 = all)
	Entry        string // entry point function name (sequence, dataflow, callgraph)
	ExportedOnly bool   // only exported functions (callgraph)
	Theme        string // "light" | "dark" | "natural" (default)
	Enrich       string // comma-separated metrics to show on node labels: loc, fan_in, churn
}

// Input bundles everything the renderers may need. Not every renderer
// uses every field — e.g. churn needs History while dependency does not.
type Input struct {
	Report        *arch.ContextReport
	History       []history.EntrySummary
	Analyzer      oculus.TypeAnalyzer
	DeepAnalyzer  oculus.DeepAnalyzer
	Root          string // repository root path (needed by Tier 2/3 renderers)
	ResolvedTheme *ResolvedTheme
	HexaRoles     map[string]string // component name → hexa role (domain, port, adapter, infra, app, entrypoint)
}
