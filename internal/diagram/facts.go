package diagram

import (
	"fmt"
	"strings"

	"github.com/dpopsuev/locus/internal/arch"
	"github.com/dpopsuev/locus/internal/graph"
)

const (
	labelSick  = "sick"
	labelFatal = "fatal"
)

// RenderFacts returns plain-text machine-readable assertions from the same
// data used by Mermaid renderers. Agents reason about these without parsing
// diagram syntax.
func RenderFacts(report *arch.ContextReport) string {
	var b strings.Builder

	b.WriteString("# Architecture Facts\n\n")

	// Dependency facts.
	for _, e := range report.Architecture.Edges {
		fmt.Fprintf(&b, "%s depends on %s (weight: %d)\n", e.From, e.To, e.Weight)
	}

	// Health facts.
	fanIn := graph.FanIn(report.Architecture.Edges)
	for i := range report.Architecture.Services {
		s := &report.Architecture.Services[i]
		h := ClassifyHealth(fanIn[s.Name], s.Churn)
		if h != Healthy {
			label := labelSick
			if h == Fatal {
				label = labelFatal
			}
			fmt.Fprintf(&b, "%s is %s (fan-in: %d, churn: %d)\n", s.Name, label, fanIn[s.Name], s.Churn)
		}
	}

	// Cycle facts.
	for _, c := range report.Cycles {
		fmt.Fprintf(&b, "cycle: %s\n", strings.Join(c, " → "))
	}

	// Layer violation facts.
	for _, v := range report.LayerViolations {
		fmt.Fprintf(&b, "violation: %s → %s (upward import)\n", v.From, v.To)
	}

	// Boundary crossing facts.
	for _, bc := range report.BoundaryCrossings {
		fmt.Fprintf(&b, "boundary_crossing: %s (%s) → %s (%s)\n", bc.From, bc.FromZone, bc.To, bc.ToZone)
	}

	// Summary.
	fmt.Fprintf(&b, "\n%d components, %d edges, %d cycles, %d violations\n",
		len(report.Architecture.Services), len(report.Architecture.Edges),
		len(report.Cycles), len(report.LayerViolations))

	return b.String()
}
