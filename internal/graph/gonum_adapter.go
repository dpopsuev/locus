package graph

import (
	"gonum.org/v1/gonum/graph/simple"
)

// stringGraph adapts Locus's string-based node IDs to gonum's int64 model.
// It provides bidirectional mapping and builds a gonum DirectedGraph from Edge slices.
// Self-loops are tracked separately since gonum's simple.DirectedGraph doesn't support them.
type stringGraph struct {
	g         *simple.DirectedGraph
	nameToID  map[string]int64
	idToName  map[int64]string
	selfLoops []string // nodes with self-edges
	nextID    int64
}

// newStringGraph creates an empty adapter graph.
func newStringGraph() *stringGraph {
	return &stringGraph{
		g:        simple.NewDirectedGraph(),
		nameToID: make(map[string]int64),
		idToName: make(map[int64]string),
	}
}

// nodeID returns the gonum int64 ID for a string name, creating it if needed.
func (sg *stringGraph) nodeID(name string) int64 {
	if id, ok := sg.nameToID[name]; ok {
		return id
	}
	id := sg.nextID
	sg.nextID++
	sg.nameToID[name] = id
	sg.idToName[id] = name
	sg.g.AddNode(simple.Node(id))
	return id
}

// nodeName returns the string name for a gonum int64 ID.
func (sg *stringGraph) nodeName(id int64) string {
	return sg.idToName[id]
}

// fromEdges builds a stringGraph from a slice of Edge-satisfying values.
func fromEdges[E Edge](edges []E) *stringGraph {
	sg := newStringGraph()
	for _, e := range edges {
		src := sg.nodeID(e.Source())
		dst := sg.nodeID(e.Target())
		if src == dst {
			// gonum's simple.DirectedGraph panics on self-loops; track separately.
			sg.selfLoops = append(sg.selfLoops, e.Source())
			continue
		}
		if !sg.g.HasEdgeFromTo(src, dst) {
			sg.g.SetEdge(simple.Edge{F: simple.Node(src), T: simple.Node(dst)})
		}
	}
	return sg
}

// asUndirected builds an undirected copy of the directed graph for
// algorithms like ConnectedComponents that require graph.Undirected.
func asUndirected(sg *stringGraph) *simple.UndirectedGraph {
	ug := simple.NewUndirectedGraph()
	// Add all nodes.
	it := sg.g.Nodes()
	for it.Next() {
		ug.AddNode(it.Node())
	}
	// Add edges bidirectionally.
	edges := sg.g.Edges()
	for edges.Next() {
		e := edges.Edge()
		if !ug.HasEdgeBetween(e.From().ID(), e.To().ID()) {
			ug.SetEdge(simple.Edge{F: e.From(), T: e.To()})
		}
	}
	return ug
}
