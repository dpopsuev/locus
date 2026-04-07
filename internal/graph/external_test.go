package graph_test

import (
	"testing"

	"github.com/dpopsuev/locus/internal/graph"
)

type edge struct{ from, to string }

func (e edge) Source() string { return e.from }
func (e edge) Target() string { return e.to }

func TestFanIn_External(t *testing.T) {
	fi := graph.FanIn([]edge{{"a", "b"}, {"c", "b"}, {"d", "b"}})
	if fi["b"] != 3 {
		t.Errorf("FanIn[b] = %d, want 3", fi["b"])
	}
}

func TestDetectCycles_External(t *testing.T) {
	cycles := graph.DetectCycles([]edge{{"a", "b"}, {"b", "c"}, {"c", "a"}})
	if len(cycles) == 0 {
		t.Error("expected cycle")
	}
}

func TestShortestPath_External(t *testing.T) {
	path, ok := graph.ShortestPath([]edge{{"a", "b"}, {"b", "c"}, {"a", "c"}}, "a", "c")
	if !ok || len(path) != 2 {
		t.Errorf("path = %v, ok = %v, want 2-hop direct", path, ok)
	}
}

func TestTopologicalSort_External(t *testing.T) {
	sorted, err := graph.TopologicalSort([]edge{{"a", "b"}, {"b", "c"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(sorted) != 3 {
		t.Errorf("len = %d, want 3", len(sorted))
	}
}

func TestCheckLayerPurity_External(t *testing.T) {
	// layers: bottom(0)=low, top(1)=high. Edge low→high = rank 0→1 = lower imports higher = violation.
	violations := graph.CheckLayerPurity([]edge{{"low", "high"}}, []string{"low", "high"})
	if len(violations) == 0 {
		t.Error("expected violation")
	}
}
