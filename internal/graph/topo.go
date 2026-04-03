package graph

import (
	"errors"
	"sort"
)

// ErrCycleDetected is returned when TopologicalSort encounters a cycle.
var ErrCycleDetected = errors.New("cycle detected: topological sort impossible")

// TopologicalSort returns nodes in dependency order (sources first, sinks last).
// Returns ErrCycleDetected if the graph contains cycles.
func TopologicalSort[E Edge](edges []E) ([]string, error) {
	adj := buildAdj(edges)
	nodes := collectNodes(edges)

	inDeg := make(map[string]int, len(nodes))
	for n := range nodes {
		inDeg[n] = 0
	}
	for _, e := range edges {
		inDeg[e.Target()]++
	}

	// Seed with zero-indegree nodes, sorted for determinism.
	queue := make([]string, 0)
	for n := range nodes {
		if inDeg[n] == 0 {
			queue = append(queue, n)
		}
	}
	sort.Strings(queue)

	var result []string
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		result = append(result, node)
		for _, next := range adj[node] {
			inDeg[next]--
			if inDeg[next] == 0 {
				queue = append(queue, next)
			}
		}
	}

	if len(result) != len(nodes) {
		return nil, ErrCycleDetected
	}
	return result, nil
}
