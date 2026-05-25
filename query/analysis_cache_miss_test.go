package query_test

// analysis_cache_miss_test.go probes three failure modes that produce the
// symptom "analysis tools return null/empty after a successful scan_local":
//
//  A. Without cache_key: getOrScan cold path looks up plain sha → MISS
//     (scan stored under sha-full) → triggers a fresh ScanAndBuild whose
//     results may differ from the intent=full scan.
//
//  B. With a stale / wrong-format cache_key: getOrScan returns
//     ErrNoCachedReport immediately; the MCP SDK may surface this as an
//     error-content tool result that the agent renders as null.
//
//  C. With the correct cache_key: must succeed with the same data that
//     scan_local returned (195 components, 110 edges, 6 cycles).
//
// If mode A or mode B causes tools to return empty data while the operator
// believes they have the scan result in hand, this file pinpoints which
// failure mode is active.

import (
	"context"
	"errors"
	"testing"

	"github.com/dpopsuev/locus/internal/store"
	"github.com/dpopsuev/oculus/v3/cache"
	"github.com/dpopsuev/oculus/v3/engine"
)

// summaryNoComponents is the sentinel returned by ComputeRiskScores when the
// fetched report has 0 services. Seeing this means getOrScan returned an
// empty or wrong report.
const summaryNoComponents = "no components"

// scanResult bundles the engine, the filesystem path used for scanning, and
// the ScanResult. The filesystem path and the result's ModulePath are
// intentionally kept separate — ModulePath is the module name (e.g. the
// tsconfig/package name), NOT the filesystem directory.
type scanResult struct {
	eng    *engine.Engine
	dir    string // absolute filesystem path passed to ScanProject
	result *engine.ScanResult
}

// newScanAndEngine runs ScanProject(intent=full) on the TypeScript fixture and
// returns the engine, the filesystem dir, and the ScanResult.
func newScanAndEngine(t *testing.T) scanResult {
	t.Helper()
	dir := tsFixture(t)
	sc := cache.New(t.TempDir())
	db := store.NewFilesystem(sc, t.TempDir())
	eng := engine.New(db, nil)

	res, err := eng.ScanProject(context.Background(), dir, engine.ScanOpts{Intent: "full"})
	if err != nil {
		t.Fatalf("ScanProject: %v", err)
	}
	if len(res.Report.Architecture.Services) == 0 {
		t.Fatalf("ScanProject produced 0 services — fixture or scanner is broken")
	}
	return scanResult{eng: eng, dir: dir, result: res}
}

// --- Mode A: analysis called WITHOUT cache_key ---

// TestAnalysis_WithoutCacheKey_RiskScores_UsesData verifies that GetRiskScores
// called WITHOUT a cache_key (the agent does not pass cache_key forward)
// still returns populated risk scores, not "no components".
//
// Given a completed scan with intent=full
// When GetRiskScores is called without a cache_key
// Then it does NOT return {summary: "no components"}
//
// This test exposes Mode-A failure: the cold path triggers a fresh scan whose
// result may differ from the intent=full scan (different store key = sha vs sha-full).
func TestAnalysis_WithoutCacheKey_RiskScores_UsesData(t *testing.T) {
	sr := newScanAndEngine(t)
	eng, dir, result := sr.eng, sr.dir, sr.result

	// Deliberately omit cache_key — mimics agents that do not forward it.
	risk, err := eng.GetRiskScores(context.Background(), dir)
	if err != nil {
		// Mode-B: getOrScan cold path missed the plain-sha entry (only sha-full exists)
		// and the fresh ScanAndBuild failed. In production this surfaces as an
		// error-content MCP tool result that some agents render as null.
		t.Fatalf("GetRiskScores (no cache_key): %v\n"+
			"Mode-B failure: plain-sha cold path scan failed; agents calling without cache_key see null", err)
	}
	if risk.Summary == summaryNoComponents {
		t.Errorf("GetRiskScores returned 'no components' without cache_key\n"+
			"Mode-A failure: cold path scan produced 0 services; scan_local had services=%d edges=%d",
			len(result.Report.Architecture.Services),
			len(result.Report.Architecture.Edges))
	}
	t.Logf("GetRiskScores (no cache_key): summary=%q scores=%d", risk.Summary, len(risk.Scores))
}

// TestAnalysis_WithoutCacheKey_Cycles_MatchesScan verifies that GetCycles
// called without cache_key returns the same cycle count as the scan report.
//
// Given a completed scan with 6 cycles
// When GetCycles is called without a cache_key
// Then it returns 6 cycles, not []
func TestAnalysis_WithoutCacheKey_Cycles_MatchesScan(t *testing.T) {
	sr := newScanAndEngine(t)
	eng, dir, result := sr.eng, sr.dir, sr.result
	wantCycles := len(result.Report.Cycles)

	cycleReport, err := eng.GetCycles(context.Background(), dir, nil)
	if err != nil {
		t.Fatalf("GetCycles (no cache_key): %v\n"+
			"Mode-B failure: plain-sha cold path scan failed; cycles returns [] in production", err)
	}
	if len(cycleReport.Cycles) != wantCycles {
		t.Errorf("GetCycles returned %d cycle(s), scan had %d\n"+
			"Mode-A failure: cold path scan produced a different graph (plain-sha entry differs from sha-full entry)",
			len(cycleReport.Cycles), wantCycles)
	}
	t.Logf("GetCycles (no cache_key): cycles=%d (scan had %d)", len(cycleReport.Cycles), wantCycles)
}

// --- Mode B: analysis called WITH wrong / stale cache_key ---

// TestAnalysis_WithWrongCacheKey_ReturnsError verifies that a wrong/stale
// cache_key produces a clear ErrNoCachedReport — NOT silent empty data.
// The MCP SDK surfaces this as a tool-result with isError=true; if agents
// render that as null they need to check for error content in the response.
//
// Given a completed scan with a valid cache_key
// When GetRiskScores is called with a stale cache_key (wrong sha)
// Then it returns ErrNoCachedReport, not "no components"
func TestAnalysis_WithWrongCacheKey_ReturnsError(t *testing.T) {
	sr := newScanAndEngine(t)
	eng, dir, result := sr.eng, sr.dir, sr.result
	_ = result

	staleKey := dir + "@0000000000000000000000000000000000000000-full"
	_, err := eng.GetRiskScores(context.Background(), dir, staleKey)
	if err == nil {
		t.Errorf("GetRiskScores with stale cache_key returned nil error — " +
			"expected ErrNoCachedReport; silent success with empty data is misleading")
		return
	}
	if !errors.Is(err, engine.ErrNoCachedReport) {
		t.Errorf("GetRiskScores with stale cache_key: got %v, want ErrNoCachedReport", err)
	}
	t.Logf("GetRiskScores (stale cache_key): correctly returned %v", err)
}

// --- Mode D: analysis called with ModulePath instead of filesystem path ---

// TestAnalysis_ModulePathVsFilesystemPath documents that result.Report.ModulePath
// is NOT the filesystem path. Calling analysis tools with ModulePath (the module
// name, e.g. the TypeScript package name) causes the cold-path scan to target a
// non-existent directory and fail — producing the null/empty symptom.
//
// Given scan_local returned a ScanResult with ModulePath="somename"
// When an analysis tool is called with path=ModulePath (not the filesystem dir)
// Then the cold-path scan fails because "somename" is not a valid directory
//
// This is the most common operator error: confusing the module name in the
// scan summary with the filesystem path to pass to analysis tools.
func TestAnalysis_ModulePathVsFilesystemPath(t *testing.T) {
	sr := newScanAndEngine(t)
	eng, result := sr.eng, sr.result

	moduleName := result.Report.ModulePath
	if moduleName == sr.dir {
		t.Skip("ModulePath equals filesystem dir for this fixture — test not applicable")
	}

	// Calling with ModulePath (not the filesystem dir). Without a cache_key,
	// getOrScan runs ScanAndBuild on `moduleName` which is not a real directory.
	_, err := eng.GetRiskScores(context.Background(), moduleName)
	if err == nil {
		// Some environments may have a directory matching the module name.
		t.Logf("GetRiskScores(path=ModulePath=%q) returned no error — directory exists", moduleName)
		return
	}
	t.Logf("GetRiskScores(path=ModulePath=%q): %v\n"+
		"This is Mode-D: agent used module name as path; scan fails; tools return null.\n"+
		"Fix: always pass the filesystem path, not the module name, to analysis tools.", moduleName, err)
	// The test PASSES even with an error here — it's documenting the failure mode,
	// not asserting it must succeed. The scan_local response should clearly
	// distinguish 'module path' from 'filesystem path' to prevent this confusion.
}

// --- Mode E: plain-sha written after intent scan (no cold rescan on analysis) ---

// TestScanProject_PlainSHA_PopulatedAfterIntentScan verifies that after
// ScanProject(intent=full), the plain-sha entry is also written so that
// getOrScan called without a cache_key finds data immediately.
//
// Given ScanProject is called with intent=full on a fresh empty cache
// When GetRiskScores is called without a cache_key
// Then it returns data from the plain-sha entry (no cold rescan triggered)
func TestScanProject_PlainSHA_PopulatedAfterIntentScan(t *testing.T) {
	sr := newScanAndEngine(t)
	eng, dir, result := sr.eng, sr.dir, sr.result

	// After scan_local (intent=full), analysis tools without cache_key must
	// use the plain-sha entry — they must not trigger a fresh ScanAndBuild
	// which could time out on large workspaces.
	risk, err := eng.GetRiskScores(context.Background(), dir)
	if err != nil {
		t.Fatalf("GetRiskScores (no cache_key): %v\n"+
			"plain-sha entry was not written by ScanProject(intent=full); "+
			"cold rescan triggered and may time out on large workspaces", err)
	}
	if risk.Summary == summaryNoComponents {
		t.Errorf("GetRiskScores returned 'no components'; scan had services=%d edges=%d",
			len(result.Report.Architecture.Services),
			len(result.Report.Architecture.Edges))
	}
	t.Logf("GetRiskScores (no cache_key, post-intent-full): %q — plain-sha entry was found", risk.Summary)
}

// --- Mode C: analysis WITH correct cache_key (must always pass) ---

// TestAnalysis_WithCorrectCacheKey_AllToolsReturnData is the reference case:
// when the cache_key from scan_local is passed verbatim to every analysis
// tool, all tools must return populated data.
//
// Given scan_local completed and returned a cache_key
// When each analysis tool is called with that cache_key
// Then none returns an error, null, or "no components"
func TestAnalysis_WithCorrectCacheKey_AllToolsReturnData(t *testing.T) {
	sr := newScanAndEngine(t)
	eng, dir, result := sr.eng, sr.dir, sr.result
	ck := result.CacheKey

	ctx := context.Background()

	t.Run("risk_scores", func(t *testing.T) {
		r, err := eng.GetRiskScores(ctx, dir, ck)
		if err != nil {
			t.Fatalf("GetRiskScores: %v", err)
		}
		if r.Summary == summaryNoComponents {
			t.Errorf("risk_scores: 'no components' with correct cache_key — getOrScan returned wrong report")
		}
		t.Logf("risk_scores: %q", r.Summary)
	})

	t.Run("cycles", func(t *testing.T) {
		r, err := eng.GetCycles(ctx, dir, nil, ck)
		if err != nil {
			t.Fatalf("GetCycles: %v", err)
		}
		if len(r.Cycles) != len(result.Report.Cycles) {
			t.Errorf("cycles: got %d, scan had %d — cache_key returned wrong report",
				len(r.Cycles), len(result.Report.Cycles))
		}
	})

	t.Run("symbol_search", func(t *testing.T) {
		r, err := eng.SearchSymbols(ctx, dir, "handleRequest", ck)
		if err != nil {
			t.Fatalf("SearchSymbols: %v", err)
		}
		if len(r.Matches) == 0 {
			t.Errorf("symbol_search: 0 matches for 'handleRequest' with correct cache_key")
		}
	})

	t.Run("hot_spots", func(t *testing.T) {
		_, err := eng.GetHotSpots(ctx, dir, 30, 10, ck)
		if err != nil {
			t.Fatalf("GetHotSpots: %v", err)
		}
	})

	t.Run("dependencies", func(t *testing.T) {
		if len(result.Report.Architecture.Services) == 0 {
			t.Skip("no services in scan result")
		}
		component := result.Report.Architecture.Services[0].Name
		r, err := eng.GetDependencies(ctx, dir, component, ck)
		if err != nil {
			t.Fatalf("GetDependencies(%q): %v", component, err)
		}
		t.Logf("dependencies(%q): fan_in=%d fan_out=%d",
			component, len(r.FanIn), len(r.FanOut))
	})
}
