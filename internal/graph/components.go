package graph

import (
	"sort"

	"gonum.org/v1/gonum/graph/topo"
)

// ConnectedComponents finds connected components in an undirected view of the
// directed graph (edges treated as bidirectional). Returns groups of nodes,
// each sorted alphabetically. Groups are sorted by size descending.
func ConnectedComponents[E Edge](edges []E) [][]string {
	if len(edges) == 0 {
		return nil
	}
	sg := fromEdges(edges)
	components := topo.ConnectedComponents(asUndirected(sg))

	groups := make([][]string, 0, len(components))
	for _, comp := range components {
		group := make([]string, len(comp))
		for i, n := range comp {
			group[i] = sg.nodeName(n.ID())
		}
		sort.Strings(group)
		groups = append(groups, group)
	}

	sort.Slice(groups, func(i, j int) bool {
		return len(groups[i]) > len(groups[j])
	})
	return groups
}
