package graph

import "sort"

// ConnectedComponents finds connected components in an undirected view of the
// directed graph (edges treated as bidirectional). Returns groups of nodes,
// each sorted alphabetically. Groups are sorted by size descending.
func ConnectedComponents[E Edge](edges []E) [][]string {
	// Build undirected adjacency.
	adj := make(map[string]map[string]bool)
	for _, e := range edges {
		s, t := e.Source(), e.Target()
		if adj[s] == nil {
			adj[s] = make(map[string]bool)
		}
		if adj[t] == nil {
			adj[t] = make(map[string]bool)
		}
		adj[s][t] = true
		adj[t][s] = true
	}

	visited := make(map[string]bool)

	// Sorted iteration for determinism.
	nodes := make([]string, 0, len(adj))
	for n := range adj {
		nodes = append(nodes, n)
	}
	sort.Strings(nodes)

	groups := make([][]string, 0, len(nodes)/2)
	for _, n := range nodes {
		if visited[n] {
			continue
		}
		group := bfsCollect(n, adj, visited)
		sort.Strings(group)
		groups = append(groups, group)
	}

	// Largest groups first.
	sort.Slice(groups, func(i, j int) bool {
		return len(groups[i]) > len(groups[j])
	})
	return groups
}

func bfsCollect(start string, adj map[string]map[string]bool, visited map[string]bool) []string {
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
