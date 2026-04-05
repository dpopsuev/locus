package graph

import (
	"errors"

	"gonum.org/v1/gonum/graph/topo"
)

// ErrCycleDetected is returned when TopologicalSort encounters a cycle.
var ErrCycleDetected = errors.New("cycle detected: topological sort impossible")

// TopologicalSort returns nodes in dependency order (sources first, sinks last).
// Returns ErrCycleDetected if the graph contains cycles.
func TopologicalSort[E Edge](edges []E) ([]string, error) {
	if len(edges) == 0 {
		return nil, nil
	}
	sg := fromEdges(edges)
	sorted, err := topo.Sort(sg.g)
	if err != nil {
		return nil, ErrCycleDetected
	}
	result := make([]string, len(sorted))
	for i, n := range sorted {
		result[i] = sg.nodeName(n.ID())
	}
	return result, nil
}
