package impact

import (
	"testing"

	"github.com/dpopsuev/locus/internal/arch"
)

func testServices() []arch.ArchService {
	return []arch.ArchService{
		{Name: "A", LOC: 100, Symbols: []string{"Fa1", "Fa2"}},
		{Name: "B", LOC: 200, Symbols: []string{"Fb1"}},
		{Name: "C", LOC: 150, Symbols: []string{"Fc1", "Fc2", "Fc3"}},
		{Name: "D", LOC: 80, Symbols: []string{"Fd1"}},
	}
}

func testEdges() []arch.ArchEdge {
	return []arch.ArchEdge{
		{From: "A", To: "B", CallSites: 3},
		{From: "A", To: "C", CallSites: 2},
		{From: "B", To: "C", CallSites: 5},
		{From: "C", To: "D", CallSites: 1},
	}
}

func TestWhatIf_EmptyMoves(t *testing.T) {
	delta, err := ComputeWhatIf(testServices(), testEdges(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if delta.ComponentsBefore != 4 || delta.ComponentsAfter != 4 {
		t.Errorf("expected 4→4, got %d→%d", delta.ComponentsBefore, delta.ComponentsAfter)
	}
	if delta.EdgesBefore != 4 || delta.EdgesAfter != 4 {
		t.Errorf("expected 4→4 edges, got %d→%d", delta.EdgesBefore, delta.EdgesAfter)
	}
	if len(delta.RemovedEdges) != 0 || len(delta.AddedEdges) != 0 {
		t.Error("expected no edge changes for empty moves")
	}
}

func TestWhatIf_Delete(t *testing.T) {
	moves := []FileMove{{From: "B"}} // delete B
	delta, err := ComputeWhatIf(testServices(), testEdges(), nil, moves)
	if err != nil {
		t.Fatal(err)
	}

	if delta.ComponentsAfter != 3 {
		t.Errorf("expected 3 components after deletion, got %d", delta.ComponentsAfter)
	}

	// Edges A→B and B→C should be removed.
	if len(delta.RemovedEdges) != 2 {
		t.Errorf("expected 2 removed edges, got %d", len(delta.RemovedEdges))
	}

	// Fan-in for B should go from 1→0 (absent), C from 2→1.
	foundC := false
	for _, d := range delta.FanInDelta {
		if d.Component == "C" {
			foundC = true
			if d.Before != 2 || d.After != 1 {
				t.Errorf("expected C fan-in 2→1, got %d→%d", d.Before, d.After)
			}
		}
	}
	if !foundC {
		t.Error("expected fan-in delta for C")
	}
}

func TestWhatIf_Rename(t *testing.T) {
	moves := []FileMove{{From: "B", To: "X"}} // rename B→X
	delta, err := ComputeWhatIf(testServices(), testEdges(), nil, moves)
	if err != nil {
		t.Fatal(err)
	}

	if delta.ComponentsAfter != 4 {
		t.Errorf("expected 4 components after rename, got %d", delta.ComponentsAfter)
	}

	// Old edges A→B and B→C should be removed, new A→X and X→C added.
	if len(delta.RemovedEdges) != 2 {
		t.Errorf("expected 2 removed edges (A→B, B→C), got %d", len(delta.RemovedEdges))
	}
	if len(delta.AddedEdges) != 2 {
		t.Errorf("expected 2 added edges (A→X, X→C), got %d", len(delta.AddedEdges))
	}
}

func TestWhatIf_Merge(t *testing.T) {
	// Merge B into A — two services become one.
	moves := []FileMove{{From: "B", To: "A"}}
	delta, err := ComputeWhatIf(testServices(), testEdges(), nil, moves)
	if err != nil {
		t.Fatal(err)
	}

	if delta.ComponentsAfter != 3 {
		t.Errorf("expected 3 components after merge, got %d", delta.ComponentsAfter)
	}

	// A→B edge becomes self-loop (removed), B→C becomes A→C (merges with existing A→C).
	// We should have fewer edges.
	if delta.EdgesAfter > delta.EdgesBefore {
		t.Error("expected fewer edges after merge")
	}
}

func TestWhatIf_CycleBreak(t *testing.T) {
	// Create a cycle: A→B→A.
	services := []arch.ArchService{
		{Name: "A"}, {Name: "B"},
	}
	edges := []arch.ArchEdge{
		{From: "A", To: "B"},
		{From: "B", To: "A"},
	}
	cycles := arch.DetectCycles(edges)
	if len(cycles) == 0 {
		t.Fatal("expected a cycle in setup")
	}

	// Delete B to break the cycle.
	moves := []FileMove{{From: "B"}}
	delta, err := ComputeWhatIf(services, edges, cycles, moves)
	if err != nil {
		t.Fatal(err)
	}

	if len(delta.RemovedCycles) == 0 {
		t.Error("expected at least one removed cycle")
	}
	if len(delta.NewCycles) != 0 {
		t.Error("expected no new cycles")
	}
}

func TestWhatIf_CycleIntroduce(t *testing.T) {
	// A→B→C, no cycle. Rename C→A creates A→B→A cycle.
	services := []arch.ArchService{
		{Name: "A"}, {Name: "B"}, {Name: "C"},
	}
	edges := []arch.ArchEdge{
		{From: "A", To: "B"},
		{From: "B", To: "C"},
	}

	moves := []FileMove{{From: "C", To: "A"}}
	delta, err := ComputeWhatIf(services, edges, nil, moves)
	if err != nil {
		t.Fatal(err)
	}

	if len(delta.NewCycles) == 0 {
		t.Error("expected new cycle after merging C into A")
	}
}

func TestWhatIf_FanDelta(t *testing.T) {
	delta, err := ComputeWhatIf(testServices(), testEdges(), nil, []FileMove{{From: "D"}})
	if err != nil {
		t.Fatal(err)
	}

	// D had fan-in=1 (from C). After deletion, D is gone so fan-in delta should show D: 1→0.
	foundD := false
	for _, d := range delta.FanInDelta {
		if d.Component == "D" {
			foundD = true
			if d.Before != 1 || d.After != 0 {
				t.Errorf("expected D fan-in 1→0, got %d→%d", d.Before, d.After)
			}
		}
	}
	if !foundD {
		t.Error("expected fan-in delta for D")
	}
}
