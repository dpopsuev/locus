package impact

import (
	"testing"

	"github.com/dpopsuev/locus/internal/arch"
	"github.com/dpopsuev/locus/internal/graph"
	"github.com/dpopsuev/locus/internal/model"
)

func TestComputeIndependenceScores(t *testing.T) {
	services := []arch.ArchService{
		{Name: "core", LOC: 500, Symbols: model.SymbolsFromNames("A", "B", "C", "D", "E")},
		{Name: "util", LOC: 50, Symbols: model.SymbolsFromNames("X")},
		{Name: "big", LOC: 2000, Symbols: model.SymbolsFromNames("Y")},
	}
	fanIn := graph.CountMap{"core": 10, "util": 2, "big": 1}

	scores := ComputeIndependenceScores(services, fanIn)
	if len(scores) != 3 {
		t.Fatalf("expected 3 scores, got %d", len(scores))
	}

	// core: (10 × 5) / 500 = 0.1
	// util: (2 × 1) / 50 = 0.04
	// big: (1 × 1) / 2000 = 0.0005
	// Order: core > util > big
	if scores[0].Component != "core" {
		t.Errorf("expected core first (highest independence), got %s", scores[0].Component)
	}
	t.Logf("independence: core=%.4f, util=%.4f, big=%.4f",
		scores[0].Independence, scores[1].Independence, scores[2].Independence)
}

func TestDetectCohesionClusters_CoImported(t *testing.T) {
	// consumer1 imports both A and B. consumer2 imports both A and B.
	// A and B should form a cohesion cluster.
	edges := []arch.ArchEdge{
		{From: "consumer1", To: "A"},
		{From: "consumer1", To: "B"},
		{From: "consumer2", To: "A"},
		{From: "consumer2", To: "B"},
		{From: "consumer3", To: "C"}, // C is independent
	}

	clusters := DetectCohesionClusters(edges, 0.7)
	if len(clusters) == 0 {
		t.Fatal("expected at least 1 cohesion cluster")
	}

	found := false
	for _, c := range clusters {
		if len(c.Members) == 2 {
			found = true
			t.Logf("cluster: %v, co-import=%.2f, consumers=%d", c.Members, c.CoImportPct, c.Consumers)
		}
	}
	if !found {
		t.Error("expected a 2-member cluster (A, B)")
	}
}

func TestDetectCohesionClusters_NoCluster(t *testing.T) {
	// Each component has different consumers — no co-import.
	edges := []arch.ArchEdge{
		{From: "c1", To: "A"},
		{From: "c2", To: "B"},
		{From: "c3", To: "C"},
	}

	clusters := DetectCohesionClusters(edges, 0.7)
	if len(clusters) != 0 {
		t.Errorf("expected 0 clusters, got %d", len(clusters))
	}
}

func TestComputeConsolidation(t *testing.T) {
	services := []arch.ArchService{
		{Name: "A", LOC: 100, Symbols: model.SymbolsFromNames("X")},
		{Name: "B", LOC: 100, Symbols: model.SymbolsFromNames("Y")},
		{Name: "C", LOC: 200, Symbols: model.SymbolsFromNames("Z")},
	}
	edges := []arch.ArchEdge{
		{From: "C", To: "A"},
		{From: "C", To: "B"},
	}

	report := ComputeConsolidation(services, edges)
	if len(report.IndependenceScores) != 3 {
		t.Errorf("expected 3 scores, got %d", len(report.IndependenceScores))
	}
	if report.Summary == "" {
		t.Error("expected non-empty summary")
	}
	t.Logf("consolidation: %s", report.Summary)
}
