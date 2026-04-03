package impact

import (
	"github.com/dpopsuev/locus/internal/arch"
	"github.com/dpopsuev/locus/internal/graph"
	"github.com/dpopsuev/locus/internal/port"
)

// ImpactResult holds the blast radius and risk for a component change.
type ImpactResult struct {
	Component   string         `json:"component"`
	DirectDeps  []string       `json:"direct_dependents"`
	TransDeps   []string       `json:"transitive_dependents"`
	BlastRadius int            `json:"blast_radius"` // percentage of total components affected
	RiskLevel   port.RiskLevel `json:"risk_level"`   // low, medium, high, critical
}

// ComputeImpact computes the transitive blast radius for a component change.
// Edges represent "From imports To". Direct dependents are components that import the given component.
func ComputeImpact(edges []arch.ArchEdge, services []arch.ArchService, component string) (*ImpactResult, error) {
	result := &ImpactResult{Component: component}

	// Build component set and full reverse graph.
	componentSet := make(map[string]bool, len(services))
	for i := range services {
		componentSet[services[i].Name] = true
	}

	reverse := graph.ReverseAdj(edges)

	// Direct dependents: who imports component.
	directSet := make(map[string]bool)
	for dep := range reverse[component] {
		if componentSet[dep] {
			directSet[dep] = true
		}
	}
	for d := range directSet {
		result.DirectDeps = append(result.DirectDeps, d)
	}

	// Transitive: BFS from direct dependents.
	skip := map[string]bool{component: true}
	transSet := graph.BFS(directSet, reverse, componentSet, skip)
	for t := range transSet {
		result.TransDeps = append(result.TransDeps, t)
	}

	// Blast radius: affected / total * 100.
	total := len(componentSet)
	affected := len(transSet)
	if total > 0 {
		result.BlastRadius = affected * 100 / total
	}

	result.RiskLevel = classifyRisk(result.BlastRadius)
	return result, nil
}

// classifyRisk maps a blast radius percentage to a risk level string.
func classifyRisk(blastRadius int) port.RiskLevel {
	switch {
	case blastRadius >= 50:
		return port.RiskCritical
	case blastRadius >= 25:
		return port.RiskHigh
	case blastRadius >= 10:
		return port.RiskMedium
	default:
		return port.RiskLow
	}
}
