package arch

import (
	"testing"
)

func TestDetectCycles_NoCycles(t *testing.T) {
	edges := []ArchEdge{
		{From: "a", To: "b"},
		{From: "b", To: "c"},
		{From: "a", To: "c"},
	}
	cycles := DetectCycles(edges)
	if len(cycles) != 0 {
		t.Fatalf("expected no cycles, got %v", cycles)
	}
}

func TestDetectCycles_SimpleCycle(t *testing.T) {
	edges := []ArchEdge{
		{From: "a", To: "b"},
		{From: "b", To: "c"},
		{From: "c", To: "a"},
	}
	cycles := DetectCycles(edges)
	if len(cycles) != 1 {
		t.Fatalf("expected 1 cycle, got %d: %v", len(cycles), cycles)
	}
	if cycles[0][0] != "a" {
		t.Errorf("expected cycle to start with 'a', got %v", cycles[0])
	}
	if len(cycles[0]) != 3 {
		t.Errorf("expected cycle length 3, got %d", len(cycles[0]))
	}
}

func TestDetectCycles_SelfLoop(t *testing.T) {
	edges := []ArchEdge{
		{From: "a", To: "a"},
	}
	cycles := DetectCycles(edges)
	if len(cycles) != 1 {
		t.Fatalf("expected 1 cycle, got %d: %v", len(cycles), cycles)
	}
	if len(cycles[0]) != 1 || cycles[0][0] != "a" {
		t.Errorf("expected [a], got %v", cycles[0])
	}
}

func TestDetectCycles_MultipleCycles(t *testing.T) {
	edges := []ArchEdge{
		{From: "a", To: "b"},
		{From: "b", To: "a"},
		{From: "c", To: "d"},
		{From: "d", To: "c"},
	}
	cycles := DetectCycles(edges)
	if len(cycles) != 2 {
		t.Fatalf("expected 2 cycles, got %d: %v", len(cycles), cycles)
	}
}

func TestComputeImportDepth_DAG(t *testing.T) {
	edges := []ArchEdge{
		{From: "root", To: "mid"},
		{From: "mid", To: "leaf"},
		{From: "root", To: "leaf"},
	}
	depth := ComputeImportDepth(edges)
	if depth["root"] != 0 {
		t.Errorf("root depth: got %d, want 0", depth["root"])
	}
	if depth["mid"] != 1 {
		t.Errorf("mid depth: got %d, want 1", depth["mid"])
	}
	if depth["leaf"] != 2 {
		t.Errorf("leaf depth: got %d, want 2", depth["leaf"])
	}
}

func TestComputeImportDepth_CycleNodes(t *testing.T) {
	edges := []ArchEdge{
		{From: "a", To: "b"},
		{From: "b", To: "a"},
		{From: "root", To: "a"},
	}
	depth := ComputeImportDepth(edges)
	if depth["a"] != -1 {
		t.Errorf("a depth: got %d, want -1", depth["a"])
	}
	if depth["b"] != -1 {
		t.Errorf("b depth: got %d, want -1", depth["b"])
	}
	if depth["root"] != 0 {
		t.Errorf("root depth: got %d, want 0", depth["root"])
	}
}

func TestCheckLayerPurity_NoViolation(t *testing.T) {
	edges := []ArchEdge{
		{From: "cmd", To: "protocol"},
		{From: "protocol", To: "store"},
	}
	layers := []string{"store", "model", "protocol", "cmd"}
	violations := CheckLayerPurity(edges, layers)
	if len(violations) != 0 {
		t.Fatalf("expected no violations, got %v", violations)
	}
}

func TestCheckLayerPurity_Violation(t *testing.T) {
	layers := []string{"store", "model", "protocol", "cmd"}

	// store(0) -> cmd(3) is an upward import — violation
	edges := []ArchEdge{
		{From: "store", To: "cmd"},
	}
	violations := CheckLayerPurity(edges, layers)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation for upward import, got %d: %v", len(violations), violations)
	}
	if violations[0].From != "store" || violations[0].To != "cmd" {
		t.Errorf("unexpected violation: %+v", violations[0])
	}

	// protocol(2) -> cmd(3) is also upward — violation
	edges = []ArchEdge{
		{From: "cmd", To: "store"},
		{From: "protocol", To: "cmd"},
	}
	violations = CheckLayerPurity(edges, layers)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %v", len(violations), violations)
	}
	if violations[0].From != "protocol" || violations[0].To != "cmd" {
		t.Errorf("unexpected violation: %+v", violations[0])
	}
}

func TestCheckLayerPurity_Empty(t *testing.T) {
	violations := CheckLayerPurity(nil, nil)
	if violations != nil {
		t.Fatalf("expected nil, got %v", violations)
	}
}
