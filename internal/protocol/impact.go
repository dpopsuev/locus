package protocol

import (
	"github.com/dpopsuev/locus/internal/arch"
)

// ImpactResult holds the blast radius and risk for a component change.
type ImpactResult struct {
	Component   string   `json:"component"`
	DirectDeps  []string `json:"direct_dependents"`
	TransDeps   []string `json:"transitive_dependents"`
	BlastRadius int      `json:"blast_radius"` // percentage of total components affected
	RiskLevel   string   `json:"risk_level"`   // low, medium, high, critical
}

// ComputeImpact computes the transitive blast radius for a component change.
// Edges represent "From imports To". Direct dependents are components that import the given component.
func ComputeImpact(edges []arch.ArchEdge, services []arch.ArchService, component string) (*ImpactResult, error) {
	result := &ImpactResult{Component: component}

	// Build reverse adjacency: for each To, list all From (dependents)
	reverse := make(map[string]map[string]bool)
	componentSet := make(map[string]bool)
	for _, s := range services {
		componentSet[s.Name] = true
	}

	for _, e := range edges {
		if e.To == component {
			if reverse[component] == nil {
				reverse[component] = make(map[string]bool)
			}
			reverse[component][e.From] = true
		}
	}

	// Direct dependents: who imports component
	directSet := make(map[string]bool)
	for _, e := range edges {
		if e.To == component && componentSet[e.From] {
			directSet[e.From] = true
		}
	}
	for d := range directSet {
		result.DirectDeps = append(result.DirectDeps, d)
	}

	// Build full reverse graph for transitive walk
	for _, e := range edges {
		if reverse[e.To] == nil {
			reverse[e.To] = make(map[string]bool)
		}
		reverse[e.To][e.From] = true
	}

	// Transitive: BFS from direct dependents
	transSet := make(map[string]bool)
	for d := range directSet {
		transSet[d] = true
	}
	queue := make([]string, 0, len(directSet))
	for d := range directSet {
		queue = append(queue, d)
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for dep := range reverse[cur] {
			if componentSet[dep] && !transSet[dep] && dep != component {
				transSet[dep] = true
				queue = append(queue, dep)
			}
		}
	}
	for t := range transSet {
		result.TransDeps = append(result.TransDeps, t)
	}

	// Blast radius: affected / total * 100
	total := len(componentSet)
	affected := len(transSet)
	if total > 0 {
		result.BlastRadius = affected * 100 / total
	}

	// Risk level
	switch {
	case result.BlastRadius >= 50:
		result.RiskLevel = "critical"
	case result.BlastRadius >= 25:
		result.RiskLevel = "high"
	case result.BlastRadius >= 10:
		result.RiskLevel = "medium"
	default:
		result.RiskLevel = "low"
	}

	return result, nil
}
