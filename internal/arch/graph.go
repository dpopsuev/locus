package arch

import "sort"

// Cycle is an ordered list of component names forming a circular dependency.
type Cycle []string

// DepthMap maps component names to their import depth (longest path from a root).
// Components in cycles get depth -1.
type DepthMap map[string]int

// LayerViolation flags an edge where a lower-layer package imports from a higher layer.
type LayerViolation struct {
	From      string `json:"from"`
	To        string `json:"to"`
	FromLayer string `json:"from_layer"`
	ToLayer   string `json:"to_layer"`
}

// DetectCycles finds all distinct cycles in the dependency graph using DFS with
// color marking (white/gray/black). Returns cycles sorted for determinism.
func DetectCycles(edges []ArchEdge) []Cycle {
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

// ComputeImportDepth computes the longest path from any root (node with zero
// in-degree) to each node. Nodes participating in cycles get depth -1.
func ComputeImportDepth(edges []ArchEdge) DepthMap {
	adj := buildAdj(edges)
	nodes := nodeSet(edges)
	inDeg := make(map[string]int, len(nodes))
	for n := range nodes {
		inDeg[n] = 0
	}
	for _, e := range edges {
		inDeg[e.To]++
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

	// Kahn's algorithm-style BFS for longest path
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

// CheckLayerPurity detects edges where a package in a lower layer imports from a
// higher layer. layers is ordered from bottom (index 0) to top (index N-1).
// A violation occurs when From is in a higher layer than To (importing downward
// is fine; importing upward is a violation).
func CheckLayerPurity(edges []ArchEdge, layers []string) []LayerViolation {
	if len(layers) == 0 {
		return nil
	}
	rank := make(map[string]int, len(layers))
	for i, l := range layers {
		rank[l] = i
	}

	var violations []LayerViolation
	for _, e := range edges {
		fromRank, fromOK := rank[e.From]
		toRank, toOK := rank[e.To]
		if !fromOK || !toOK {
			continue
		}
		if fromRank < toRank {
			violations = append(violations, LayerViolation{
				From:      e.From,
				To:        e.To,
				FromLayer: e.From,
				ToLayer:   e.To,
			})
		}
	}
	return violations
}

func buildAdj(edges []ArchEdge) map[string][]string {
	adj := make(map[string][]string)
	for _, e := range edges {
		adj[e.From] = append(adj[e.From], e.To)
	}
	return adj
}

func nodeSet(edges []ArchEdge) map[string]bool {
	nodes := make(map[string]bool)
	for _, e := range edges {
		nodes[e.From] = true
		nodes[e.To] = true
	}
	return nodes
}

// normalizeCycle rotates a cycle so the lexicographically smallest element is first.
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
	if len(c) == 0 {
		return ""
	}
	s := make([]string, len(c))
	copy(s, c)
	return join(s, "->")
}

func join(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	n := len(sep) * (len(parts) - 1)
	for _, p := range parts {
		n += len(p)
	}
	b := make([]byte, 0, n)
	for i, p := range parts {
		if i > 0 {
			b = append(b, sep...)
		}
		b = append(b, p...)
	}
	return string(b)
}
