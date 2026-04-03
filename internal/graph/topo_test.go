package graph

import (
	"testing"
)

func TestTopologicalSort_DAG(t *testing.T) {
	e := edges("a", "b", "b", "c", "a", "c")
	result, err := TopologicalSort(e)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(result))
	}
	// a must come before b, b before c.
	idx := make(map[string]int)
	for i, n := range result {
		idx[n] = i
	}
	if idx["a"] > idx["b"] || idx["b"] > idx["c"] {
		t.Errorf("invalid order: %v", result)
	}
}

func TestTopologicalSort_SingleNode(t *testing.T) {
	e := edges("a", "b")
	result, err := TopologicalSort(e)
	if err != nil {
		t.Fatal(err)
	}
	if result[0] != "a" || result[1] != "b" {
		t.Errorf("expected [a b], got %v", result)
	}
}

func TestTopologicalSort_Cycle(t *testing.T) {
	e := edges("a", "b", "b", "a")
	_, err := TopologicalSort(e)
	if err == nil {
		t.Error("expected error for cycle")
	}
}

func TestTopologicalSort_Empty(t *testing.T) {
	result, err := TopologicalSort[testEdge](nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty, got %v", result)
	}
}
