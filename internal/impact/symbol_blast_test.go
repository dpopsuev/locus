package impact

import (
	"fmt"
	"testing"

	"github.com/dpopsuev/oculus"
)

func TestComputeSymbolBlastRadius_SimpleChain(t *testing.T) {
	// A calls B, B calls C. Blast of C should show B as direct, A as transitive.
	edges := []oculus.CallEdge{
		{Caller: "A", Callee: "B", CallerPkg: "pkg/a", CalleePkg: "pkg/b"},
		{Caller: "B", Callee: "C", CallerPkg: "pkg/b", CalleePkg: "pkg/c"},
	}

	report := ComputeSymbolBlastRadius(edges, "C", 10)

	if len(report.DirectCallers) != 1 {
		t.Fatalf("expected 1 direct caller, got %d", len(report.DirectCallers))
	}
	if report.DirectCallers[0].Caller != "B" {
		t.Errorf("expected direct caller B, got %s", report.DirectCallers[0].Caller)
	}
	if len(report.TransCallers) != 1 {
		t.Fatalf("expected 1 transitive caller, got %d", len(report.TransCallers))
	}
	if report.TransCallers[0].Caller != "A" {
		t.Errorf("expected transitive caller A, got %s", report.TransCallers[0].Caller)
	}
	if len(report.AffectedPkgs) != 2 {
		t.Errorf("expected 2 affected packages, got %d", len(report.AffectedPkgs))
	}
}

func TestComputeSymbolBlastRadius_FanIn(t *testing.T) {
	// A and B both call C. Blast of C should show both as direct callers.
	edges := []oculus.CallEdge{
		{Caller: "A", Callee: "C", CallerPkg: "pkg/a", CalleePkg: "pkg/c"},
		{Caller: "B", Callee: "C", CallerPkg: "pkg/b", CalleePkg: "pkg/c"},
	}

	report := ComputeSymbolBlastRadius(edges, "C", 10)

	if len(report.DirectCallers) != 2 {
		t.Fatalf("expected 2 direct callers, got %d", len(report.DirectCallers))
	}
	callerNames := make(map[string]bool)
	for _, c := range report.DirectCallers {
		callerNames[c.Caller] = true
	}
	if !callerNames["A"] || !callerNames["B"] {
		t.Errorf("expected direct callers A and B, got %v", callerNames)
	}
	if len(report.TransCallers) != 0 {
		t.Errorf("expected 0 transitive callers, got %d", len(report.TransCallers))
	}
}

func TestComputeSymbolBlastRadius_NoCallers(t *testing.T) {
	// Orphan symbol with no callers.
	edges := []oculus.CallEdge{
		{Caller: "X", Callee: "Y", CallerPkg: "pkg/x", CalleePkg: "pkg/y"},
	}

	report := ComputeSymbolBlastRadius(edges, "Orphan", 5)

	if len(report.DirectCallers) != 0 {
		t.Errorf("expected 0 direct callers, got %d", len(report.DirectCallers))
	}
	if len(report.TransCallers) != 0 {
		t.Errorf("expected 0 transitive callers, got %d", len(report.TransCallers))
	}
	if len(report.AffectedPkgs) != 0 {
		t.Errorf("expected 0 affected packages, got %d", len(report.AffectedPkgs))
	}
	if report.BlastRadius != 0 {
		t.Errorf("expected blast radius 0, got %d", report.BlastRadius)
	}
	if report.RiskLevel != "low" {
		t.Errorf("expected risk level low, got %s", report.RiskLevel)
	}
}

func TestComputeSymbolBlastRadius_CrossPackage(t *testing.T) {
	// Callers in different packages: verify all packages collected.
	edges := []oculus.CallEdge{
		{Caller: "A", Callee: "Target", CallerPkg: "alpha", CalleePkg: "core"},
		{Caller: "B", Callee: "Target", CallerPkg: "beta", CalleePkg: "core"},
		{Caller: "C", Callee: "Target", CallerPkg: "gamma", CalleePkg: "core"},
		{Caller: "D", Callee: "A", CallerPkg: "delta", CalleePkg: "alpha"},
	}

	report := ComputeSymbolBlastRadius(edges, "Target", 5)

	if len(report.DirectCallers) != 3 {
		t.Fatalf("expected 3 direct callers, got %d", len(report.DirectCallers))
	}
	if len(report.TransCallers) != 1 {
		t.Fatalf("expected 1 transitive caller, got %d", len(report.TransCallers))
	}
	// 4 packages: alpha, beta, gamma, delta
	if len(report.AffectedPkgs) != 4 {
		t.Errorf("expected 4 affected packages, got %d: %v", len(report.AffectedPkgs), report.AffectedPkgs)
	}
	// 4/5 = 80%
	if report.BlastRadius != 80 {
		t.Errorf("expected blast radius 80, got %d", report.BlastRadius)
	}
	if report.RiskLevel != "critical" {
		t.Errorf("expected risk level critical, got %s", report.RiskLevel)
	}
}

func TestComputeSymbolBlastRadius_Cycle(t *testing.T) {
	// A calls B, B calls A. Blast of A should terminate without infinite loop.
	edges := []oculus.CallEdge{
		{Caller: "A", Callee: "B", CallerPkg: "pkg/a", CalleePkg: "pkg/b"},
		{Caller: "B", Callee: "A", CallerPkg: "pkg/b", CalleePkg: "pkg/a"},
	}

	report := ComputeSymbolBlastRadius(edges, "A", 4)

	// B is a direct caller of A.
	if len(report.DirectCallers) != 1 {
		t.Fatalf("expected 1 direct caller, got %d", len(report.DirectCallers))
	}
	if report.DirectCallers[0].Caller != "B" {
		t.Errorf("expected direct caller B, got %s", report.DirectCallers[0].Caller)
	}
	// BFS from B finds A calling B, but A is already visited (the symbol itself),
	// so no transitive callers.
	if len(report.TransCallers) != 0 {
		t.Errorf("expected 0 transitive callers (cycle terminated), got %d", len(report.TransCallers))
	}
	if report.Symbol != "A" {
		t.Errorf("expected symbol A, got %s", report.Symbol)
	}
}

func TestComputeSymbolBlastRadius_RiskLevels(t *testing.T) {
	tests := []struct {
		name     string
		affected int
		total    int
		wantRisk string
		wantPct  int
	}{
		{"low", 1, 20, "low", 5},
		{"medium", 3, 20, "medium", 15},
		{"high", 6, 20, "high", 30},
		{"critical", 11, 20, "critical", 55},
		{"zero_total", 0, 0, "low", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build edges: tt.affected different callers from different packages.
			var edges []oculus.CallEdge
			for i := 0; i < tt.affected; i++ {
				edges = append(edges, oculus.CallEdge{
					Caller:    fmt.Sprintf("Caller%d", i),
					Callee:    "Target",
					CallerPkg: fmt.Sprintf("pkg%d", i),
					CalleePkg: "core",
				})
			}

			report := ComputeSymbolBlastRadius(edges, "Target", tt.total)

			if report.BlastRadius != tt.wantPct {
				t.Errorf("blast radius: got %d, want %d", report.BlastRadius, tt.wantPct)
			}
			if string(report.RiskLevel) != tt.wantRisk {
				t.Errorf("risk level: got %s, want %s", report.RiskLevel, tt.wantRisk)
			}
		})
	}
}
