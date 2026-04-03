package graph

import (
	"testing"
)

func TestShortestPath_Direct(t *testing.T) {
	e := edges("a", "b")
	path, ok := ShortestPath(e, "a", "b")
	if !ok {
		t.Fatal("expected path")
	}
	if len(path) != 2 || path[0] != "a" || path[1] != "b" {
		t.Errorf("expected [a b], got %v", path)
	}
}

func TestShortestPath_MultiHop(t *testing.T) {
	e := edges("a", "b", "b", "c", "c", "d")
	path, ok := ShortestPath(e, "a", "d")
	if !ok {
		t.Fatal("expected path")
	}
	if len(path) != 4 {
		t.Errorf("expected 4 hops, got %d: %v", len(path), path)
	}
}

func TestShortestPath_ShortcutPreferred(t *testing.T) {
	// a→b→c→d and a→d (shortcut). BFS should find a→d.
	e := edges("a", "b", "b", "c", "c", "d", "a", "d")
	path, ok := ShortestPath(e, "a", "d")
	if !ok {
		t.Fatal("expected path")
	}
	if len(path) != 2 {
		t.Errorf("expected shortcut [a d], got %v", path)
	}
}

func TestShortestPath_NoPath(t *testing.T) {
	e := edges("a", "b", "c", "d")
	_, ok := ShortestPath(e, "a", "d")
	if ok {
		t.Error("expected no path")
	}
}

func TestShortestPath_SameNode(t *testing.T) {
	e := edges("a", "b")
	path, ok := ShortestPath(e, "a", "a")
	if !ok {
		t.Fatal("expected path")
	}
	if len(path) != 1 || path[0] != "a" {
		t.Errorf("expected [a], got %v", path)
	}
}

func TestShortestPath_Empty(t *testing.T) {
	_, ok := ShortestPath[testEdge](nil, "a", "b")
	if ok {
		t.Error("expected no path in empty graph")
	}
}
