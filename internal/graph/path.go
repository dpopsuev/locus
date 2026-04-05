package graph

import (
	"gonum.org/v1/gonum/graph/path"
)

// ShortestPath finds the shortest directed path from src to dst using BFS.
// Returns the path as an ordered list of node names and true if found,
// or nil and false if no path exists.
func ShortestPath[E Edge](edges []E, src, dst string) ([]string, bool) {
	if src == dst {
		return []string{src}, true
	}
	if len(edges) == 0 {
		return nil, false
	}

	sg := fromEdges(edges)
	srcID, srcOK := sg.nameToID[src]
	dstID, dstOK := sg.nameToID[dst]
	if !srcOK || !dstOK {
		return nil, false
	}

	// Use gonum's BFS shortest path.
	shortest := path.DijkstraFrom(sg.g.Node(srcID), sg.g)
	nodes, _ := shortest.To(dstID)
	if len(nodes) == 0 {
		return nil, false
	}

	result := make([]string, len(nodes))
	for i, n := range nodes {
		result[i] = sg.nodeName(n.ID())
	}
	return result, true
}
