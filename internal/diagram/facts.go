package diagram

import (
	"fmt"
	"strings"

	"github.com/dpopsuev/locus/internal/arch"
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
	fanIn := make(map[string]int)
	for _, e := range report.Architecture.Edges {
		fanIn[e.To]++
	}
	for _, s := range report.Architecture.Services {
		h := ClassifyHealth(fanIn[s.Name], s.Churn)
		if h != Healthy {
			label := "sick"
			if h == Fatal {
				label = "fatal"
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

	// Summary.
	fmt.Fprintf(&b, "\n%d components, %d edges, %d cycles, %d violations\n",
		len(report.Architecture.Services), len(report.Architecture.Edges),
		len(report.Cycles), len(report.LayerViolations))

	return b.String()
}
