package graph

import (
	"testing"
)

// testEdge is a minimal Edge implementation for testing.
type testEdge struct {
	from, to string
}

func (e testEdge) Source() string { return e.from }
func (e testEdge) Target() string { return e.to }

func edges(pairs ...string) []testEdge {
	var out []testEdge
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, testEdge{pairs[i], pairs[i+1]})
	}
	return out
}

func TestFanIn(t *testing.T) {
	fi := FanIn(edges("a", "b", "a", "c", "b", "c"))
	if fi["b"] != 1 {
		t.Errorf("b fan-in: got %d, want 1", fi["b"])
	}
	if fi["c"] != 2 {
		t.Errorf("c fan-in: got %d, want 2", fi["c"])
	}
	if fi["a"] != 0 {
		t.Errorf("a fan-in: got %d, want 0", fi["a"])
	}
}

func TestFanOut(t *testing.T) {
	fo := FanOut(edges("a", "b", "a", "c", "b", "c"))
	if fo["a"] != 2 {
		t.Errorf("a fan-out: got %d, want 2", fo["a"])
	}
	if fo["b"] != 1 {
		t.Errorf("b fan-out: got %d, want 1", fo["b"])
	}
}

func TestReverseAdj(t *testing.T) {
	rev := ReverseAdj(edges("a", "b", "c", "b"))
	if !rev["b"]["a"] || !rev["b"]["c"] {
		t.Errorf("expected b to have reverse edges from a and c, got %v", rev["b"])
	}
}

func TestBFS(t *testing.T) {
	adj := map[string]map[string]bool{
		"a": {"b": true, "c": true},
		"b": {"d": true},
	}
	valid := map[string]bool{"a": true, "b": true, "c": true, "d": true}
	seed := map[string]bool{"a": true}
	visited := BFS(seed, adj, valid, nil)
	for _, n := range []string{"a", "b", "c", "d"} {
		if !visited[n] {
			t.Errorf("expected %s to be visited", n)
		}
	}
}

func TestBFS_WithSkip(t *testing.T) {
	adj := map[string]map[string]bool{
		"a": {"b": true, "c": true},
		"b": {"d": true},
	}
	valid := map[string]bool{"a": true, "b": true, "c": true, "d": true}
	seed := map[string]bool{"a": true}
	skip := map[string]bool{"b": true}
	visited := BFS(seed, adj, valid, skip)
	if visited["b"] {
		t.Error("b should be skipped")
	}
	if !visited["c"] {
		t.Error("c should be visited")
	}
	if visited["d"] {
		t.Error("d should not be reached (b is skipped)")
	}
}

func TestDetectCycles_NoCycles(t *testing.T) {
	cycles := DetectCycles(edges("a", "b", "b", "c", "a", "c"))
	if len(cycles) != 0 {
		t.Fatalf("expected no cycles, got %v", cycles)
	}
}

func TestDetectCycles_SimpleCycle(t *testing.T) {
	cycles := DetectCycles(edges("a", "b", "b", "c", "c", "a"))
	if len(cycles) != 1 {
		t.Fatalf("expected 1 cycle, got %d: %v", len(cycles), cycles)
	}
	if cycles[0][0] != "a" {
		t.Errorf("expected cycle to start with 'a', got %v", cycles[0])
	}
}

func TestDetectCycles_SelfLoop(t *testing.T) {
	cycles := DetectCycles(edges("a", "a"))
	if len(cycles) != 1 {
		t.Fatalf("expected 1 cycle, got %d: %v", len(cycles), cycles)
	}
}

func TestDetectCycles_MultipleCycles(t *testing.T) {
	cycles := DetectCycles(edges("a", "b", "b", "a", "c", "d", "d", "c"))
	if len(cycles) != 2 {
		t.Fatalf("expected 2 cycles, got %d: %v", len(cycles), cycles)
	}
}

func TestImportDepth_DAG(t *testing.T) {
	depth := ImportDepth(edges("root", "mid", "mid", "leaf", "root", "leaf"))
	if depth["root"] != 0 {
		t.Errorf("root: got %d, want 0", depth["root"])
	}
	if depth["mid"] != 1 {
		t.Errorf("mid: got %d, want 1", depth["mid"])
	}
	if depth["leaf"] != 2 {
		t.Errorf("leaf: got %d, want 2", depth["leaf"])
	}
}

func TestImportDepth_CycleNodes(t *testing.T) {
	depth := ImportDepth(edges("a", "b", "b", "a", "root", "a"))
	if depth["a"] != -1 {
		t.Errorf("a: got %d, want -1", depth["a"])
	}
	if depth["b"] != -1 {
		t.Errorf("b: got %d, want -1", depth["b"])
	}
	if depth["root"] != 0 {
		t.Errorf("root: got %d, want 0", depth["root"])
	}
}

func TestCheckLayerPurity_NoViolation(t *testing.T) {
	e := edges("cmd", "protocol", "protocol", "store")
	layers := []string{"store", "model", "protocol", "cmd"}
	violations := CheckLayerPurity(e, layers)
	if len(violations) != 0 {
		t.Fatalf("expected no violations, got %v", violations)
	}
}

func TestCheckLayerPurity_Violation(t *testing.T) {
	layers := []string{"store", "model", "protocol", "cmd"}
	e := edges("store", "cmd")
	violations := CheckLayerPurity(e, layers)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}
	if violations[0].From != "store" || violations[0].To != "cmd" {
		t.Errorf("unexpected violation: %+v", violations[0])
	}
}

func TestCheckLayerPurity_Empty(t *testing.T) {
	violations := CheckLayerPurity[testEdge](nil, nil)
	if violations != nil {
		t.Fatalf("expected nil, got %v", violations)
	}
}
