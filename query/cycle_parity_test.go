package query_test

// RED test for LCS-BUG-70
//
// locus_codograph scan_local (ScanProject with Depth>0) runs cycle detection on
// the component-level graph produced by buildGroupEdges.  When two packages share
// the same grouping prefix their mutual import edge is silently dropped
// (fromGroup == toGroup → continue) before DetectCycles is called, so the scan
// summary shows "0 cycles".
//
// locus_analysis cycles (GetCycles without a cache_key) falls back to a fresh
// ScanAndBuild call with no Depth/Grouped opts, producing the fine-grained
// package-level graph where the mutual import is a real edge.  DetectCycles on
// that graph finds the cycle.
//
// The test proves both conditions hold against the cyclic monorepo fixture:
//   1. ScanProject (depth=2) → 0 cycles in the returned report.
//   2. GetCycles (no key)    → ≥1 cycle in the returned report.
//   3. The RenderScanSummary string says "0 cycles" with no granularity label.
//
// All three assertions must hold for the test to be genuinely RED.  The test
// should go GREEN once the fix is applied (option 1: add granularity labels,
// or option 2: align granularity between the two paths).

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dpopsuev/locus/internal/store"
	engine "github.com/dpopsuev/oculus/v3/engine"
	"github.com/dpopsuev/oculus/v3/cache"
)

// newEngine mirrors newClient from query_test.go but returns a raw *engine.Engine
// so we can call ScanProject and GetCycles directly.
func newEngine(t *testing.T) *engine.Engine {
	t.Helper()
	dir := t.TempDir()
	sc := cache.New(dir)
	fs := store.NewFilesystem(sc, filepath.Join(dir, "history"))
	return engine.New(fs, []string{locusRoot(t)})
}

// cyclicMonorepoFixture returns the path to the testdata/bug70 synthetic TypeScript
// monorepo. It contains two mutually-importing packages:
//   packages/api/src      → imports packages/api/src/core
//   packages/api/src/core → imports packages/api/src
// Both packages collapse to the same component ("packages/api") at depth=2,
// so the cycle is invisible in the grouped component graph.
func cyclicMonorepoFixture(t *testing.T) string {
	t.Helper()
	return filepath.Join(locusRoot(t), "testdata", "bug70")
}

// TestScanProject_DepthGroupingHidesIntraComponentCycles is a focused sanity check:
// ScanProject with Depth=2 must report 0 cycles for the cyclic monorepo fixture.
// If this fails the fixture itself is wrong.
func TestScanProject_DepthGroupingHidesIntraComponentCycles(t *testing.T) {
	eng := newEngine(t)
	ctx := context.Background()

	result, err := eng.ScanProject(ctx, cyclicMonorepoFixture(t), engine.ScanOpts{
		Depth:  2,
		Intent: "coupling",
	})
	if err != nil {
		t.Fatalf("ScanProject: %v", err)
	}

	if got := len(result.Report.Cycles); got != 0 {
		t.Fatalf("fixture pre-condition failed: expected 0 component-level cycles (depth=2), got %d — fix the fixture", got)
	}
}

// TestGetCycles_UngroupedFallbackFindsModuleLevelCycles is a focused sanity check:
// GetCycles without a cache_key must find ≥1 cycle in the cyclic monorepo fixture.
// If this fails the fixture itself is wrong.
func TestGetCycles_UngroupedFallbackFindsModuleLevelCycles(t *testing.T) {
	eng := newEngine(t)
	ctx := context.Background()

	report, err := eng.GetCycles(ctx, cyclicMonorepoFixture(t), nil)
	if err != nil {
		t.Fatalf("GetCycles: %v", err)
	}

	if len(report.Cycles) == 0 {
		t.Fatal("fixture pre-condition failed: expected GetCycles to find ≥1 module-level cycle — fix the fixture")
	}
}

// TestCycleCountParity_ComponentAndModuleLevelMustAgreeOrBeLabelled is the main regression test.
//
// It will FAIL (RED) until the fix is applied because:
//   - codographCycles == 0  (component-level, grouped)
//   - analysisCycles  >= 1  (module-level, ungrouped)
//   - the summary string contains no granularity qualifier
//
// After the fix, either:
//   (a) summary contains "component_level" and "module_level" labels, OR
//   (b) codographCycles == analysisCycles (granularity aligned).
func TestCycleCountParity_ComponentAndModuleLevelMustAgreeOrBeLabelled(t *testing.T) {
	eng := newEngine(t)
	ctx := context.Background()
	fixture := cyclicMonorepoFixture(t)

	// --- Step 1: simulate locus_codograph scan_local ---
	// Uses ScanProject with Depth=2 (grouped, component-level).
	scanResult, err := eng.ScanProject(ctx, fixture, engine.ScanOpts{
		Depth:  2,
		Intent: "coupling",
	})
	if err != nil {
		t.Fatalf("ScanProject (codograph path): %v", err)
	}
	codographCycles := len(scanResult.Report.Cycles)
	summary := engine.RenderScanSummary(scanResult, "")

	// --- Step 2: simulate locus_analysis cycles (no cache_key) ---
	// GetCycles falls back to getOrScan → fresh ScanAndBuild with no grouping.
	// The cache from Step 1 is stored under "sha-coupling" (intent suffix), so
	// the bare-sha lookup in getOrScan misses and triggers a new ungrouped scan.
	cycleReport, err := eng.GetCycles(ctx, fixture, nil)
	if err != nil {
		t.Fatalf("GetCycles (analysis path): %v", err)
	}
	analysisCycles := len(cycleReport.Cycles)

	// Guard: fixture must actually demonstrate the disagreement.
	if analysisCycles == 0 {
		t.Fatal("fixture sanity: GetCycles found 0 module-level cycles; the fixture or scanner is broken")
	}

	// Core assertion: the two tools must agree, or the summary must be labelled.
	//
	// The summary must carry granularity labels when the two counts differ.
	if codographCycles != analysisCycles {
		hasLabel := strings.Contains(summary, "component-level") ||
			strings.Contains(summary, "module-level")
		if !hasLabel {
			t.Errorf(
				"LCS-BUG-70: locus_codograph reports %d cycle(s) (component-level, depth=2) "+
					"but locus_analysis reports %d cycle(s) (module-level, ungrouped) "+
					"for the same repo with no granularity label in the summary.\n\n"+
					"codograph summary:\n  %s",
				codographCycles, analysisCycles, summary,
			)
		}
	}
}
