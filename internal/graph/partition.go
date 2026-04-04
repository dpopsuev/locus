package graph

import "sort"

// Partition reorders nodes to group tightly-coupled components together.
// Uses reverse Cuthill-McKee bandwidth reduction on the undirected
// adjacency view. Returns the reordered node list.
func Partition[E Edge](edges []E) []string {
	if len(edges) == 0 {
		return nil
	}

	// Build undirected adjacency.
	adj := make(AdjMap)
	nodes := make(NodeSet)
	for _, e := range edges {
		s, t := e.Source(), e.Target()
		nodes[s] = true
		nodes[t] = true
		if adj[s] == nil {
			adj[s] = make(map[string]bool)
		}
		if adj[t] == nil {
			adj[t] = make(map[string]bool)
		}
		adj[s][t] = true
		adj[t][s] = true
	}

	// Find the node with minimum degree (peripheral start for Cuthill-McKee).
	sorted := make([]string, 0, len(nodes))
	for n := range nodes {
		sorted = append(sorted, n)
	}
	sort.Strings(sorted)

	minDeg := len(nodes)
	start := sorted[0]
	for _, n := range sorted {
		deg := len(adj[n])
		if deg < minDeg {
			minDeg = deg
			start = n
		}
	}

	// BFS from minimum-degree node (Cuthill-McKee ordering).
	visited := make(NodeSet)
	order := make([]string, 0, len(nodes))
	queue := []string{start}
	visited[start] = true

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		order = append(order, cur)

		// Sort neighbors by degree ascending.
		neighbors := make([]string, 0, len(adj[cur]))
		for n := range adj[cur] {
			if !visited[n] {
				neighbors = append(neighbors, n)
			}
		}
		sort.Slice(neighbors, func(i, j int) bool {
			return len(adj[neighbors[i]]) < len(adj[neighbors[j]])
		})
		for _, n := range neighbors {
			if !visited[n] {
				visited[n] = true
				queue = append(queue, n)
			}
		}
	}

	// Add disconnected nodes.
	for _, n := range sorted {
		if !visited[n] {
			order = append(order, n)
		}
	}

	// Reverse for RCM (better bandwidth reduction).
	for i, j := 0, len(order)-1; i < j; i, j = i+1, j-1 {
		order[i], order[j] = order[j], order[i]
	}

	return order
}
