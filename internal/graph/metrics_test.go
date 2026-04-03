package graph

import (
	"testing"
)

func TestCohesion_FullyConnected(t *testing.T) {
	adj := map[string]map[string]bool{
		"a": {"b": true, "c": true},
		"b": {"a": true, "c": true},
		"c": {"a": true, "b": true},
	}
	c := Cohesion([]string{"a", "b", "c"}, adj)
	if c != 1.0 {
		t.Errorf("expected 1.0 for fully connected, got %f", c)
	}
}

func TestCohesion_Sparse(t *testing.T) {
	adj := map[string]map[string]bool{
		"a": {"b": true},
		"b": {"a": true},
		"c": {},
	}
	// 3 nodes, 1 edge out of 3 possible = 0.333
	c := Cohesion([]string{"a", "b", "c"}, adj)
	if c < 0.3 || c > 0.4 {
		t.Errorf("expected ~0.33, got %f", c)
	}
}

func TestCohesion_SingleNode(t *testing.T) {
	c := Cohesion([]string{"a"}, nil)
	if c != 1.0 {
		t.Errorf("expected 1.0 for single node, got %f", c)
	}
}

func TestBFSGroup_Basic(t *testing.T) {
	adj := map[string]map[string]bool{
		"a": {"b": true},
		"b": {"a": true, "c": true},
		"c": {"b": true},
		"d": {},
	}
	visited := make(map[string]bool)
	group := BFSGroup("a", adj, visited)
	if len(group) != 3 {
		t.Errorf("expected 3 nodes, got %d: %v", len(group), group)
	}
	if visited["d"] {
		t.Error("d should not be visited")
	}
}
