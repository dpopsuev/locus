package diagram

import (
	"fmt"
	"strings"

	"github.com/dpopsuev/locus/internal/arch"
	"github.com/dpopsuev/locus/internal/diagram/behavioral"
	"github.com/dpopsuev/locus/internal/diagram/core"
	"github.com/dpopsuev/locus/internal/diagram/metrics"
	"github.com/dpopsuev/locus/internal/diagram/structural"
	"github.com/dpopsuev/locus/internal/diagram/typediag"
)

// Render dispatches to the appropriate renderer by type name.
func Render(in core.Input, opts core.Options) (string, error) {
	if in.ResolvedTheme == nil {
		theme := core.DefaultTheme()
		mode := opts.Theme
		if mode == "" {
			mode = core.ThemeNatural
		}
		in.ResolvedTheme = theme.Resolve(mode)
	}
	switch opts.Type {
	case "dependency":
		return structural.Dependency(in, opts), nil
	case "c4":
		return structural.C4(in, opts), nil
	case "tree":
		return structural.Tree(in, opts), nil
	case "layers":
		return structural.Layers(in, opts), nil
	case "zones":
		return structural.Zones(in, opts), nil
	case "dsm":
		return structural.DSM(in, opts), nil
	case "sequence":
		return behavioral.Sequence(in, opts)
	case "callgraph":
		return behavioral.CallGraph(in, opts)
	case "dataflow":
		return behavioral.Dataflow(in, opts)
	case "state":
		return behavioral.State(in, opts)
	case "classes":
		return typediag.Classes(in, opts)
	case "interfaces":
		return typediag.Interfaces(in, opts)
	case "er":
		return typediag.ER(in, opts)
	case "coupling":
		return metrics.Coupling(in, opts), nil
	case "churn":
		return metrics.Churn(in, opts), nil
	case "hexa":
		return metrics.Hexa(in, opts)
	default:
		return "", fmt.Errorf("%w %q (use: %s)", core.ErrUnknownDiagramType, opts.Type, strings.Join(Types(), ", "))
	}
}

// RenderFacts returns plain-text machine-readable assertions.
func RenderFacts(report *arch.ContextReport) string {
	return metrics.Facts(report)
}

// Types returns the list of supported diagram type names.
func Types() []string {
	return []string{
		"dependency", "c4", "coupling", "churn", "layers", "tree",
		"classes", "interfaces", "sequence", "er",
		"dataflow", "callgraph", "state", "zones", "hexa", "dsm",
	}
}
