package query_test

// TestCycleSCC_* validates the SCC-based cycle reporting introduced to fix the
// Johnson's-algorithm inflation problem observed on Alef (518 simple cycles from
// 6 coupling knots).
//
// Fixture: testdata/cycles/scc_inflation — 5 TypeScript packages in a single
// tight SCC with 10 cross-edges. Johnson's algorithm finds 9 simple cycles;
// Tarjan's SCC algorithm correctly identifies 1 coupling knot.
//
// The test mirrors what happened on Alef at SHA eef5e9eb:
//   locus_analysis cycles → 518  (all simple cycles, Johnson)
//   actual SCCs             → 6   (actionable knots, Tarjan)

import (
	"context"
	"strings"
	"testing"

	engine "github.com/dpopsuev/oculus/v3/engine"
	"github.com/dpopsuev/oculus/v3/graph"
)

const sccInflationFixture = "scc_inflation"

// TestCycleSCC_InflationRatio proves that simple-cycle count inflates relative
// to SCC count for the same graph.  The fixture has 9 simple cycles but 1 SCC.
func TestCycleSCC_InflationRatio(t *testing.T) {
	eng := newEngine(t)
	ctx := context.Background()
	root := fixturePath(t, sccInflationFixture)

	report, err := eng.GetCycles(ctx, root, nil)
	if err != nil {
		t.Fatalf("GetCycles: %v", err)
	}

	simpleCycles := len(report.Cycles)
	if simpleCycles < 2 {
		t.Fatalf("fixture sanity: expected ≥2 simple cycles, got %d", simpleCycles)
	}

	// Compute SCCs directly on the same edges stored in the report.
	// GetCycles returns the cached ContextReport; we need the arch edges.
	// Re-scan to get the full report.
	result, err := eng.ScanProject(ctx, root, engine.ScanOpts{Intent: "coupling"})
	if err != nil {
		t.Fatalf("ScanProject: %v", err)
	}

	sccCount := len(result.Report.CycleGroups)
	if sccCount == 0 {
		t.Fatal("expected ≥1 SCC in CycleGroups")
	}

	// The key invariant: SCCs ≤ simple cycles, often << simple cycles.
	if sccCount > simpleCycles {
		t.Errorf("SCC count (%d) > simple cycle count (%d) — something is wrong", sccCount, simpleCycles)
	}

	t.Logf("simple cycles (Johnson): %d", simpleCycles)
	t.Logf("SCCs (Tarjan):           %d", sccCount)
	t.Logf("inflation factor:         %.1fx", float64(simpleCycles)/float64(sccCount))
}

// TestCycleSCC_SingleKnot asserts that the fixture's 5 mutually-coupled packages
// collapse to exactly 1 SCC containing all 5 packages.
func TestCycleSCC_SingleKnot(t *testing.T) {
	eng := newEngine(t)
	ctx := context.Background()
	root := fixturePath(t, sccInflationFixture)

	result, err := eng.ScanProject(ctx, root, engine.ScanOpts{Intent: "coupling"})
	if err != nil {
		t.Fatalf("ScanProject: %v", err)
	}

	groups := result.Report.CycleGroups
	if len(groups) != 1 {
		t.Fatalf("expected 1 SCC, got %d: %v", len(groups), groups)
	}
	if len(groups[0]) != 5 {
		t.Errorf("expected SCC to contain 5 packages, got %d: %v", len(groups[0]), groups[0])
	}

	// All 5 packages must be present.
	want := map[string]bool{
		"packages/core": true, "packages/events": true,
		"packages/handlers": true, "packages/state": true, "packages/router": true,
	}
	for _, pkg := range groups[0] {
		if !want[pkg] {
			t.Errorf("unexpected package in SCC: %q", pkg)
		}
		delete(want, pkg)
	}
	for pkg := range want {
		t.Errorf("missing package from SCC: %q", pkg)
	}
}

// TestCycleSCC_SummaryUsesGroups asserts that RenderScanSummary surfaces the SCC
// count ("N cyclic group(s)") rather than the raw Johnson cycle count.
func TestCycleSCC_SummaryUsesGroups(t *testing.T) {
	eng := newEngine(t)
	ctx := context.Background()
	root := fixturePath(t, sccInflationFixture)

	result, err := eng.ScanProject(ctx, root, engine.ScanOpts{Intent: "coupling"})
	if err != nil {
		t.Fatalf("ScanProject: %v", err)
	}

	summary := engine.RenderScanSummary(result, "")
	t.Logf("summary: %s", summary)

	if !strings.Contains(summary, "cyclic group") {
		t.Errorf("expected 'cyclic group' in summary, got: %s", summary)
	}
	// Must NOT expose the raw Johnson count as the headline figure.
	if strings.Contains(summary, "9 cycles") {
		t.Errorf("summary should not expose raw simple-cycle count (9), got: %s", summary)
	}
}

// TestCycleSCC_AlefAnalogy documents the ratio that prompted this fix:
// Alef had 518 simple cycles from 6 SCCs (≈86x inflation).
// The fixture produces a measurable ratio; assert it is > 1.
func TestCycleSCC_AlefAnalogy(t *testing.T) {
	eng := newEngine(t)
	ctx := context.Background()
	root := fixturePath(t, sccInflationFixture)

	result, err := eng.ScanProject(ctx, root, engine.ScanOpts{Intent: "coupling"})
	if err != nil {
		t.Fatalf("ScanProject: %v", err)
	}

	edges := result.Report.Architecture.Edges
	simpleCycles := graph.DetectCycles(edges)
	sccs := graph.StronglyConnectedComponents(edges)

	if len(sccs) == 0 {
		t.Fatal("no SCCs found")
	}
	ratio := float64(len(simpleCycles)) / float64(len(sccs))
	if ratio <= 1.0 {
		t.Errorf("expected inflation ratio > 1, got %.2f (simple=%d, sccs=%d)",
			ratio, len(simpleCycles), len(sccs))
	}
	t.Logf("Alef analogy: %d simple cycles / %d SCCs = %.1fx inflation (Alef was 518/6 = 86x)",
		len(simpleCycles), len(sccs), ratio)
}
