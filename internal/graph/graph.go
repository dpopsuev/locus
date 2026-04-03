// Package graph provides generic graph algorithms that operate on any edge type
// satisfying the Edge interface. This is the algorithm kernel for Locus —
// BFS, cycle detection, fan-in/out, import depth, and layer purity checking.
package graph

import (
	"sort"
	"strings"
)

// Edge is the minimal interface that any directed edge must satisfy.
type Edge interface {
	Source() string
	Target() string
}

// Cycle is an ordered list of node names forming a circular dependency.
type Cycle []string

// DepthMap maps node names to their import depth (longest path from a root).
// Nodes participating in cycles get depth -1.
type DepthMap map[string]int

// FanIn returns the number of incoming edges per node.
func FanIn[E Edge](edges []E) map[string]int {
	fi := make(map[string]int, len(edges))
	for _, e := range edges {
		fi[e.Target()]++
	}
	return fi
}

// FanOut returns the number of outgoing edges per node.
func FanOut[E Edge](edges []E) map[string]int {
	fo := make(map[string]int, len(edges))
	for _, e := range edges {
		fo[e.Source()]++
	}
	return fo
}

// ReverseAdj builds a reverse adjacency map: for each target, list all sources.
func ReverseAdj[E Edge](edges []E) map[string]map[string]bool {
	reverse := make(map[string]map[string]bool)
	for _, e := range edges {
		t := e.Target()
		if reverse[t] == nil {
			reverse[t] = make(map[string]bool)
		}
		reverse[t][e.Source()] = true
	}
	return reverse
}

// BFS performs breadth-first search from seed nodes through adjacency,
// returning all visited nodes. Only nodes in validSet are traversed;
// nodes in skip are excluded.
func BFS(seed map[string]bool, adj map[string]map[string]bool, validSet, skip map[string]bool) map[string]bool {
	visited := make(map[string]bool, len(seed))
	queue := make([]string, 0, len(seed))
	for d := range seed {
		visited[d] = true
		queue = append(queue, d)
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for neighbor := range adj[cur] {
			if validSet[neighbor] && !visited[neighbor] && !skip[neighbor] {
				visited[neighbor] = true
				queue = append(queue, neighbor)
			}
		}
	}
	return visited
}

// DetectCycles finds all distinct cycles in a directed graph using DFS with
// color marking (white/gray/black). Returns cycles sorted for determinism.
func DetectCycles[E Edge](edges []E) []Cycle {
	adj := buildAdj(edges)
	nodes := nodeSet(edges)

	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := make(map[string]int, len(nodes))
	var cycles []Cycle

	var dfs func(node string, stack []string)
	dfs = func(node string, stack []string) {
		color[node] = gray
		stack = append(stack, node)
		for _, next := range adj[node] {
			switch color[next] {
			case gray:
				start := -1
				for i, s := range stack {
					if s == next {
						start = i
						break
					}
				}
				if start >= 0 {
					c := make(Cycle, len(stack)-start)
					copy(c, stack[start:])
					cycles = append(cycles, normalizeCycle(c))
				}
			case white:
				dfs(next, stack)
			}
		}
		color[node] = black
	}

	sorted := make([]string, 0, len(nodes))
	for n := range nodes {
		sorted = append(sorted, n)
	}
	sort.Strings(sorted)

	for _, n := range sorted {
		if color[n] == white {
			dfs(n, nil)
		}
	}

	return deduplicateCycles(cycles)
}

// ImportDepth computes the longest path from any root (node with zero in-degree)
// to each node. Nodes participating in cycles get depth -1.
func ImportDepth[E Edge](edges []E) DepthMap {
	adj := buildAdj(edges)
	nodes := nodeSet(edges)
	inDeg := make(map[string]int, len(nodes))
	for n := range nodes {
		inDeg[n] = 0
	}
	for _, e := range edges {
		inDeg[e.Target()]++
	}

	cycleNodes := make(map[string]bool)
	for _, c := range DetectCycles(edges) {
		for _, n := range c {
			cycleNodes[n] = true
		}
	}

	depth := make(DepthMap, len(nodes))
	for n := range nodes {
		if cycleNodes[n] {
			depth[n] = -1
			continue
		}
		depth[n] = 0
	}

	queue := make([]string, 0)
	for n := range nodes {
		if inDeg[n] == 0 && !cycleNodes[n] {
			queue = append(queue, n)
		}
	}
	sort.Strings(queue)

	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		for _, next := range adj[node] {
			if cycleNodes[next] {
				continue
			}
			if d := depth[node] + 1; d > depth[next] {
				depth[next] = d
			}
			inDeg[next]--
			if inDeg[next] == 0 {
				queue = append(queue, next)
			}
		}
	}

	return depth
}

// --- helpers ---

func buildAdj[E Edge](edges []E) map[string][]string {
	adj := make(map[string][]string)
	for _, e := range edges {
		adj[e.Source()] = append(adj[e.Source()], e.Target())
	}
	return adj
}

func nodeSet[E Edge](edges []E) map[string]bool {
	nodes := make(map[string]bool)
	for _, e := range edges {
		nodes[e.Source()] = true
		nodes[e.Target()] = true
	}
	return nodes
}

func normalizeCycle(c Cycle) Cycle {
	if len(c) == 0 {
		return c
	}
	minIdx := 0
	for i, n := range c {
		if n < c[minIdx] {
			minIdx = i
		}
	}
	out := make(Cycle, len(c))
	for i := range c {
		out[i] = c[(minIdx+i)%len(c)]
	}
	return out
}

func deduplicateCycles(cycles []Cycle) []Cycle {
	seen := make(map[string]bool, len(cycles))
	var result []Cycle
	for _, c := range cycles {
		key := cycleKey(c)
		if !seen[key] {
			seen[key] = true
			result = append(result, c)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return cycleKey(result[i]) < cycleKey(result[j])
	})
	return result
}

func cycleKey(c Cycle) string {
	return strings.Join(c, "->")
}
