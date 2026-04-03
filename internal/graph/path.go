package graph

// ShortestPath finds the shortest directed path from src to dst using BFS.
// Returns the path as an ordered list of node names and true if found,
// or nil and false if no path exists.
func ShortestPath[E Edge](edges []E, src, dst string) ([]string, bool) {
	if src == dst {
		return []string{src}, true
	}

	adj := buildAdj(edges)
	visited := map[string]bool{src: true}
	parent := map[string]string{src: ""}
	queue := []string{src}

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, next := range adj[cur] {
			if visited[next] {
				continue
			}
			visited[next] = true
			parent[next] = cur
			if next == dst {
				return reconstructPath(parent, src, dst), true
			}
			queue = append(queue, next)
		}
	}
	return nil, false
}

func reconstructPath(parent map[string]string, src, dst string) []string {
	var path []string
	for cur := dst; cur != src; cur = parent[cur] {
		path = append(path, cur)
	}
	path = append(path, src)
	// Reverse.
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return path
}
