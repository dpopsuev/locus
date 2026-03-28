package diagram

import (
	"fmt"
	"strings"

	"github.com/dpopsuev/locus/internal/analysis"
	"github.com/dpopsuev/locus/internal/arch"
	"github.com/dpopsuev/locus/internal/history"
)

const themeNatural = "natural"

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
	Analyzer      analysis.TypeAnalyzer
	DeepAnalyzer  analysis.DeepAnalyzer
	Root          string // repository root path (needed by Tier 2/3 renderers)
	ResolvedTheme *ResolvedTheme
}

// Render dispatches to the appropriate renderer by type name.
func Render(in Input, opts Options) (string, error) {
	if in.ResolvedTheme == nil {
		theme := DefaultTheme()
		mode := opts.Theme
		if mode == "" {
			mode = themeNatural
		}
		in.ResolvedTheme = theme.Resolve(mode)
	}
	switch opts.Type {
	case "dependency":
		return renderDependency(in, opts), nil
	case "c4":
		return renderC4(in, opts), nil
	case "coupling":
		return renderCoupling(in, opts), nil
	case "churn":
		return renderChurn(in, opts), nil
	case "layers":
		return renderLayers(in, opts), nil
	case "tree":
		return renderTree(in, opts), nil
	case "classes":
		return renderClasses(in, opts)
	case "interfaces":
		return renderInterfaces(in, opts)
	case "sequence":
		return renderSequence(in, opts)
	case "er":
		return renderER(in, opts)
	case "dataflow":
		return renderDataflow(in, opts)
	case "callgraph":
		return renderCallGraph(in, opts)
	case "state":
		return renderState(in, opts)
	case "zones":
		return renderZones(in, opts), nil
	default:
		return "", fmt.Errorf("%w %q (use: %s)", ErrUnknownDiagramType, opts.Type, strings.Join(Types(), ", "))
	}
}

// Types returns the list of supported diagram type names.
func Types() []string {
	return []string{
		"dependency", "c4", "coupling", "churn", "layers", "tree",
		"classes", "interfaces", "sequence", "er",
		"dataflow", "callgraph", "state", "zones",
	}
}
