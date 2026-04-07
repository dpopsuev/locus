package history

import (
	"testing"

	"github.com/dpopsuev/locus/internal/arch"
)

func TestDiffReportsNoChanges(t *testing.T) {
	r := &arch.ContextReport{ScanCore: arch.ScanCore{Architecture: arch.ArchModel{
		Services: []arch.ArchService{{Name: "a"}, {Name: "b"}},
		Edges:    []arch.ArchEdge{{From: "a", To: "b"}},
	}}}
	d := DiffReports(r, r)
	if d.Summary != summaryNoChanges {
		t.Errorf("expected %q, got %q", summaryNoChanges, d.Summary)
	}
}

func TestDiffReportsComponentChanges(t *testing.T) {
	old := &arch.ContextReport{ScanCore: arch.ScanCore{Architecture: arch.ArchModel{
		Services: []arch.ArchService{{Name: "a"}, {Name: "b"}, {Name: "c"}},
	}}}
	updated := &arch.ContextReport{ScanCore: arch.ScanCore{Architecture: arch.ArchModel{
		Services: []arch.ArchService{{Name: "a"}, {Name: "d"}},
	}}}
	d := DiffReports(old, updated)
	if len(d.AddedComponents) != 1 || d.AddedComponents[0] != "d" {
		t.Errorf("expected added=[d], got %v", d.AddedComponents)
	}
	if len(d.RemovedComponents) != 2 {
		t.Errorf("expected 2 removed, got %d", len(d.RemovedComponents))
	}
}

func TestDiffReportsEdgeChanges(t *testing.T) {
	old := &arch.ContextReport{ScanCore: arch.ScanCore{Architecture: arch.ArchModel{
		Edges: []arch.ArchEdge{{From: "a", To: "b"}, {From: "b", To: "c"}},
	}}}
	updated := &arch.ContextReport{ScanCore: arch.ScanCore{Architecture: arch.ArchModel{
		Edges: []arch.ArchEdge{{From: "a", To: "b"}, {From: "a", To: "c"}},
	}}}
	d := DiffReports(old, updated)
	if len(d.AddedEdges) != 1 || d.AddedEdges[0] != "a->c" {
		t.Errorf("expected added=[a->c], got %v", d.AddedEdges)
	}
	if len(d.RemovedEdges) != 1 || d.RemovedEdges[0] != "b->c" {
		t.Errorf("expected removed=[b->c], got %v", d.RemovedEdges)
	}
}

func TestDiffReportsChurnDeltas(t *testing.T) {
	old := &arch.ContextReport{GraphMetrics: arch.GraphMetrics{HotSpots: []arch.HotSpot{{Component: "x", Churn: 10}, {Component: "y", Churn: 5}}}}
	updated := &arch.ContextReport{GraphMetrics: arch.GraphMetrics{HotSpots: []arch.HotSpot{{Component: "x", Churn: 15}, {Component: "z", Churn: 3}}}}
	d := DiffReports(old, updated)
	if len(d.ChurnDeltas) != 3 {
		t.Fatalf("expected 3 churn deltas, got %d", len(d.ChurnDeltas))
	}
	found := map[string]ChurnDelta{}
	for _, cd := range d.ChurnDeltas {
		found[cd.Component] = cd
	}
	if found["x"].Delta != 5 {
		t.Errorf("x delta: want 5, got %d", found["x"].Delta)
	}
	if found["y"].Delta != -5 {
		t.Errorf("y delta: want -5, got %d", found["y"].Delta)
	}
	if found["z"].Delta != 3 {
		t.Errorf("z delta: want 3, got %d", found["z"].Delta)
	}
}

func TestDiffReportsSummary(t *testing.T) {
	old := &arch.ContextReport{ScanCore: arch.ScanCore{Architecture: arch.ArchModel{
		Services: []arch.ArchService{{Name: "a"}},
		Edges:    []arch.ArchEdge{{From: "a", To: "b"}},
	}}}
	updated := &arch.ContextReport{ScanCore: arch.ScanCore{Architecture: arch.ArchModel{
		Services: []arch.ArchService{{Name: "a"}, {Name: "b"}},
		Edges:    []arch.ArchEdge{{From: "a", To: "b"}, {From: "b", To: "c"}},
	}}}
	d := DiffReports(old, updated)
	if d.Summary == summaryNoChanges {
		t.Error("expected changes in summary")
	}
}
