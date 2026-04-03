package graph

// Cohesion computes the edge density within a group of nodes.
// Returns (actual edges) / (possible edges), where possible = n*(n-1)/2
// for an undirected view. Returns 1.0 for groups with fewer than 2 nodes.
func Cohesion(group []string, adj AdjMap) float64 {
	n := len(group)
	if n < 2 {
		return 1.0
	}
	possibleEdges := n * (n - 1) / 2
	groupSet := make(NodeSet, n)
	for _, s := range group {
		groupSet[s] = true
	}
	counted := make(NodeSet)
	actualEdges := 0
	for _, s := range group {
		for neighbor := range adj[s] {
			if !groupSet[neighbor] {
				continue
			}
			fwd := s + "|" + neighbor
			rev := neighbor + "|" + s
			if !counted[fwd] && !counted[rev] {
				counted[fwd] = true
				actualEdges++
			}
		}
	}
	return float64(actualEdges) / float64(possibleEdges)
}

// BFSGroup performs BFS from start through undirected adjacency, returning
// all reachable nodes as a group. Marks visited nodes in the provided set
// to avoid re-processing across multiple calls.
func BFSGroup(start string, adj AdjMap, visited NodeSet) []string {
	queue := []string{start}
	visited[start] = true
	var group []string
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		group = append(group, cur)
		for neighbor := range adj[cur] {
			if !visited[neighbor] {
				visited[neighbor] = true
				queue = append(queue, neighbor)
			}
		}
	}
	return group
}
