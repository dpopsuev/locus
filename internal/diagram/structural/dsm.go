package structural

import (
	"fmt"
	"strings"

	"github.com/dpopsuev/locus/internal/diagram/core"
	"github.com/dpopsuev/locus/internal/graph"
)

// DSM renders a Dependency Structure Matrix as a text table.
// Components are reordered by RCM partitioning to reveal clusters.
// Mermaid doesn't support matrix diagrams natively, so we use a
// pre-formatted text table with x markers for dependencies.
func DSM(in core.Input, _ core.Options) string {
	m := in.Report.Architecture

	// Partition nodes for cluster grouping.
	order := graph.Partition(m.Edges)
	if len(order) == 0 {
		return "# DSM\n\nNo components to display."
	}

	// Build edge lookup.
	hasEdge := make(map[string]map[string]bool)
	for _, e := range m.Edges {
		if hasEdge[e.From] == nil {
			hasEdge[e.From] = make(map[string]bool)
		}
		hasEdge[e.From][e.To] = true
	}

	// Abbreviate component names for readability.
	abbrev := make([]string, len(order))
	for i, name := range order {
		parts := strings.Split(name, "/")
		if len(parts) > 1 {
			abbrev[i] = parts[len(parts)-1]
		} else {
			abbrev[i] = name
		}
	}

	// Render matrix.
	var b strings.Builder
	b.WriteString("# Dependency Structure Matrix\n\n")
	b.WriteString(fmt.Sprintf("%d components, RCM-partitioned\n\n", len(order)))

	// Header row.
	maxName := 0
	for _, a := range abbrev {
		if len(a) > maxName {
			maxName = len(a)
		}
	}
	if maxName > 20 {
		maxName = 20
	}

	// Column header indices.
	b.WriteString(strings.Repeat(" ", maxName+2))
	for i := range order {
		b.WriteString(fmt.Sprintf("%2d ", i+1))
	}
	b.WriteByte('\n')

	// Separator.
	b.WriteString(strings.Repeat(" ", maxName+2))
	b.WriteString(strings.Repeat("---", len(order)))
	b.WriteByte('\n')

	// Data rows.
	for i, from := range order {
		name := abbrev[i]
		if len(name) > maxName {
			name = name[:maxName]
		}
		fmt.Fprintf(&b, "%*s: ", maxName, name)
		for j, to := range order {
			switch {
			case i == j:
				b.WriteString(" \u25a0 ")
			case hasEdge[from][to]:
				b.WriteString(" \u00d7 ")
			default:
				b.WriteString(" \u00b7 ")
			}
		}
		fmt.Fprintf(&b, " [%d]\n", i+1)
	}

	// Legend.
	b.WriteString("\n\u25a0 = self  \u00d7 = depends on  \u00b7 = no dependency\n")

	return b.String()
}
