package diagram

import (
	"fmt"
	"strings"

	"github.com/dpopsuev/locus/internal/arch"
)

// renderDependency produces a Mermaid flowchart from the ArchModel.
// This is a refactored version of arch.RenderMermaid routed through
// the diagram subsystem so all diagrams share one interface.
func renderDependency(report *arch.ContextReport, opts Options) string {
	m := report.Architecture
	var b strings.Builder
	b.WriteString("graph TD\n")

	for _, s := range m.Services {
		if opts.Scope != "" && s.Name != opts.Scope && !isEdgeNeighbor(m.Edges, opts.Scope, s.Name) {
			continue
		}
		id := mermaidID(s.Name)
		label := s.Name
		if s.Churn > 0 {
			label += fmt.Sprintf(" [churn:%d]", s.Churn)
		}
		fmt.Fprintf(&b, "    %s[\"%s\"]\n", id, label)
	}

	for _, e := range m.Edges {
		if opts.Scope != "" && e.From != opts.Scope && e.To != opts.Scope {
			continue
		}
		fromID := mermaidID(e.From)
		toID := mermaidID(e.To)
		if e.Weight > 0 {
			fmt.Fprintf(&b, "    %s -->|\"%d\"| %s\n", fromID, e.Weight, toID)
		} else {
			fmt.Fprintf(&b, "    %s --> %s\n", fromID, toID)
		}
	}

	return b.String()
}

func isEdgeNeighbor(edges []arch.ArchEdge, scope, name string) bool {
	for _, e := range edges {
		if (e.From == scope && e.To == name) || (e.To == scope && e.From == name) {
			return true
		}
	}
	return false
}
