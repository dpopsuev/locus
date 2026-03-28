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

	// Build component set and full reverse graph.
	componentSet := make(map[string]bool, len(services))
	for i := range services {
		componentSet[services[i].Name] = true
	}

	reverse := buildReverseGraph(edges)

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
	transSet := bfsTransitive(directSet, reverse, componentSet, component)
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

// buildReverseGraph builds a reverse adjacency map: for each To, list all From.
func buildReverseGraph(edges []arch.ArchEdge) map[string]map[string]bool {
	reverse := make(map[string]map[string]bool)
	for _, e := range edges {
		if reverse[e.To] == nil {
			reverse[e.To] = make(map[string]bool)
		}
		reverse[e.To][e.From] = true
	}
	return reverse
}

// bfsTransitive performs BFS from seed through the reverse graph, skipping the origin component.
func bfsTransitive(seed map[string]bool, reverse map[string]map[string]bool, componentSet map[string]bool, origin string) map[string]bool {
	visited := make(map[string]bool, len(seed))
	queue := make([]string, 0, len(seed))
	for d := range seed {
		visited[d] = true
		queue = append(queue, d)
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for dep := range reverse[cur] {
			if componentSet[dep] && !visited[dep] && dep != origin {
				visited[dep] = true
				queue = append(queue, dep)
			}
		}
	}
	return visited
}

// classifyRisk maps a blast radius percentage to a risk level string.
func classifyRisk(blastRadius int) string {
	switch {
	case blastRadius >= 50:
		return RiskCritical
	case blastRadius >= 25:
		return RiskHigh
	case blastRadius >= 10:
		return RiskMedium
	default:
		return RiskLow
	}
}
