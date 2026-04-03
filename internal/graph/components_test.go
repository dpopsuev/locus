package graph

import (
	"testing"
)

func TestConnectedComponents_TwoClusters(t *testing.T) {
	e := edges("a", "b", "b", "c", "d", "e")
	groups := ConnectedComponents(e)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d: %v", len(groups), groups)
	}
	// Largest first.
	if len(groups[0]) != 3 {
		t.Errorf("expected largest group size 3, got %d", len(groups[0]))
	}
	if len(groups[1]) != 2 {
		t.Errorf("expected second group size 2, got %d", len(groups[1]))
	}
}

func TestConnectedComponents_SingleComponent(t *testing.T) {
	e := edges("a", "b", "b", "c", "c", "a")
	groups := ConnectedComponents(e)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if len(groups[0]) != 3 {
		t.Errorf("expected 3 nodes, got %d", len(groups[0]))
	}
}

func TestConnectedComponents_Disconnected(t *testing.T) {
	e := edges("a", "b", "c", "d", "e", "f")
	groups := ConnectedComponents(e)
	if len(groups) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(groups))
	}
	for _, g := range groups {
		if len(g) != 2 {
			t.Errorf("expected group size 2, got %d: %v", len(g), g)
		}
	}
}

func TestConnectedComponents_Empty(t *testing.T) {
	groups := ConnectedComponents[testEdge](nil)
	if len(groups) != 0 {
		t.Errorf("expected empty, got %v", groups)
	}
}
