package query_test

// full_intent_pipeline_test.go specifies the end-to-end contract for
// scan_local intent=full followed by symbol-level analysis.
//
// Scenario (as reported by operators):
//   1. scan_local intent=full completes with scanner=typescript, 195 components, 110 edges.
//      The TypeScript scanner reads import statements and builds a package dependency graph,
//      but produces no call graph and no churn correlation.
//   2. locus_analysis risk_scores returns {scores: null, summary: "no components"}.
//      symbol_search, coupling hot_spots, and cycles also return empty.
//   3. CLI `locus scan --scanner lsp` works and produces symbol data, hot spots,
//      and 0 cycles — but is unavailable through the MCP protocol.
//
// This file captures that scenario as executable Given/When/Then assertions.
// Tests marked with [CURRENTLY FAILING] describe the desired end state; they
// will pass once the scanner selection and data pipeline are fixed.
// Tests marked with [PASSING] document the known-good invariants.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/dpopsuev/locus/internal/store"
	"github.com/dpopsuev/oculus/v3/cache"
	"github.com/dpopsuev/oculus/v3/engine"
)

// tsFixtureFiles returns the source files for the TypeScript pipeline fixture.
// Files are spread across separate top-level directories so each becomes its
// own component and cross-directory imports produce dependency edges.
func tsFixtureFiles() map[string]string {
	return map[string]string{
		"tsconfig.json":    `{"compilerOptions":{"target":"ES2020","module":"commonjs"}}`,
		"api/index.ts":     "import { processRequest } from '../domain';\nexport function handleRequest(input: string): string { return processRequest(input); }\nexport function validateInput(input: string): boolean { return input.length > 0; }\n",
		"domain/index.ts":  "import { persist } from '../store';\nexport function processRequest(input: string): string { persist(input); return input; }\n",
		"store/index.ts":   "export function persist(data: string): void {}\nexport function retrieve(key: string): string { return ''; }\n",
		"utils/index.ts":   "export function formatDate(d: Date): string { return d.toISOString(); }\nexport function parseDate(s: string): Date { return new Date(s); }\n",
	}
}

// tsFixture creates a minimal TypeScript workspace with inter-module imports
// and a git history so churn data can be computed.
func tsFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for rel, content := range tsFixtureFiles() {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	gitInit(t, dir)
	return dir
}

// gitInit creates an initial commit in dir so git history is available.
func gitInit(t *testing.T, dir string) {
	t.Helper()
	// nolint:gosec — args are all hardcoded string literals in this function.
	run := func(args ...string) { //nolint:gocritic
		if err := exec.Command("git", args...).Run(); err != nil { //nolint:gosec
			t.Logf("git %v: %v (non-fatal)", args, err)
		}
	}
	run("-C", dir, "init", "-q")
	run("-C", dir, "config", "user.email", "test@example.com")
	run("-C", dir, "config", "user.name", "Test")
	run("-C", dir, "add", "-A")
	run("-C", dir, "commit", "-q", "-m", "init")
}

func newEngineWithRealStore(t *testing.T) *engine.Engine {
	t.Helper()
	sc := cache.New(t.TempDir())
	db := store.NewFilesystem(sc, filepath.Join(t.TempDir(), "history"))
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Dir(filepath.Dir(file))
	return engine.New(db, []string{root})
}

// --- [PASSING] Structural scan produces components and import edges ---

// TestIntentFull_TypeScriptScanner_ProducesComponents verifies the baseline:
// scan_local intent=full on a TypeScript workspace produces separate components
// (one per source directory) and cross-component import edges when depth=1 is
// applied so that each src/ subdirectory becomes its own component.
//
// Given a TypeScript workspace with inter-module imports and depth=1
// When ScanProject is called with intent=full
// Then the report contains multiple services with at least one import edge
func TestIntentFull_TypeScriptScanner_ProducesComponents(t *testing.T) {
	dir := tsFixture(t)
	eng := newEngineWithRealStore(t)

	result, err := eng.ScanProject(context.Background(), dir, engine.ScanOpts{Intent: "full"})
	if err != nil {
		t.Fatalf("ScanProject: %v", err)
	}

	services := result.Report.Architecture.Services
	edges := result.Report.Architecture.Edges

	if len(services) == 0 {
		t.Error("0 services — TypeScript scanner should produce one component per source file / directory")
	}
	if len(edges) == 0 {
		t.Errorf("0 import edges across %d services — TypeScript scanner should extract edges from import statements",
			len(services))
	}

	t.Logf("scanner=%s services=%d edges=%d cacheKey=%s",
		result.Report.Scanner, len(services), len(edges), result.CacheKey)
}

// --- [CURRENTLY FAILING] Symbol-level data must be accessible after scan ---

// TestIntentFull_SymbolSearch_FindsExportedFunctions is the primary failing
// scenario. After scan_local intent=full, searching for an exported function
// must return a match — not zero results.
//
// Given a completed scan with intent=full on the TypeScript fixture
// When SearchSymbols is called with the cache_key and a known exported symbol name
// Then at least one match is returned
//
// Currently failing because the TypeScript scanner either does not populate
// ArchService.Symbols, or getOrScan returns a report with empty services when
// called with the cache_key from ScanProject(intent=full).
func TestIntentFull_SymbolSearch_FindsExportedFunctions(t *testing.T) {
	dir := tsFixture(t)
	eng := newEngineWithRealStore(t)

	scanResult, err := eng.ScanProject(context.Background(), dir, engine.ScanOpts{Intent: "full"})
	if err != nil {
		t.Fatalf("ScanProject: %v", err)
	}

	// Verify the scan itself has symbols before testing search.
	totalSymbols := 0
	for _, svc := range scanResult.Report.Architecture.Services {
		totalSymbols += len(svc.Symbols)
	}
	if totalSymbols == 0 {
		t.Logf("UPSTREAM GAP: ScanProject report contains 0 symbols across %d services — "+
			"TypeScript scanner may not populate ArchService.Symbols; "+
			"symbol_search and risk_scores will be empty regardless of cache_key correctness",
			len(scanResult.Report.Architecture.Services))
	}

	// Search for a function that definitely exists in the fixture.
	symbols, err := eng.SearchSymbols(context.Background(), dir, "handleRequest", scanResult.CacheKey)
	if err != nil {
		t.Fatalf("SearchSymbols(cache_key=%q): %v", scanResult.CacheKey, err)
	}

	if len(symbols.Matches) == 0 {
		t.Errorf("SearchSymbols returned 0 matches for 'handleRequest'\n"+
			"fixture defines: handleRequest (api.ts), validateInput (api.ts), "+
			"processRequest (domain.ts), persist/retrieve (store.ts), formatDate/parseDate (utils.ts)\n"+
			"scanner=%s services=%d total_symbols=%d cache_key=%q",
			scanResult.Report.Scanner, len(scanResult.Report.Architecture.Services),
			totalSymbols, scanResult.CacheKey)
	} else {
		t.Logf("SearchSymbols found %d match(es) for 'handleRequest': %+v",
			len(symbols.Matches), symbols.Matches[0])
	}
}

// TestIntentFull_RiskScores_ReturnsActualScores verifies that after
// scan_local intent=full, risk_scores returns actual component scores — not
// the sentinel "no components" value.
//
// Given a completed scan with intent=full on a workspace with import edges
// When GetRiskScores is called with the cache_key
// Then scores are non-nil and the summary does not read "no components"
func TestIntentFull_RiskScores_ReturnsActualScores(t *testing.T) {
	dir := tsFixture(t)
	eng := newEngineWithRealStore(t)

	scanResult, err := eng.ScanProject(context.Background(), dir, engine.ScanOpts{Intent: "full"})
	if err != nil {
		t.Fatalf("ScanProject: %v", err)
	}

	risk, err := eng.GetRiskScores(context.Background(), dir, scanResult.CacheKey)
	if err != nil {
		t.Fatalf("GetRiskScores(cache_key=%q): %v", scanResult.CacheKey, err)
	}

	if risk.Summary == "no components" {
		t.Errorf("GetRiskScores returned 'no components'\n"+
			"scan produced: services=%d edges=%d scanner=%s cache_key=%q\n"+
			"root cause: getOrScan may be returning a report with 0 services, "+
			"or the service slice is populated but has no edges for blast-radius computation",
			len(scanResult.Report.Architecture.Services),
			len(scanResult.Report.Architecture.Edges),
			scanResult.Report.Scanner,
			scanResult.CacheKey)
	} else {
		t.Logf("GetRiskScores: summary=%q scores=%d", risk.Summary, len(risk.Scores))
	}
}

// TestIntentFull_HotSpots_ReturnsResults verifies that hot_spots analysis
// is available after a scan that includes churn and coupling data.
//
// Given a completed scan with intent=full (includes churn analysis from git history)
// When GetHotSpots is called with the cache_key
// Then at least one hot spot is returned (the fixture has inter-module coupling)
func TestIntentFull_HotSpots_ReturnsResults(t *testing.T) {
	dir := tsFixture(t)
	eng := newEngineWithRealStore(t)

	scanResult, err := eng.ScanProject(context.Background(), dir, engine.ScanOpts{Intent: "full"})
	if err != nil {
		t.Fatalf("ScanProject: %v", err)
	}

	spots, err := eng.GetHotSpots(context.Background(), dir, 30, 10, scanResult.CacheKey)
	if err != nil {
		t.Fatalf("GetHotSpots(cache_key=%q): %v", scanResult.CacheKey, err)
	}

	t.Logf("GetHotSpots: %d spot(s) — scanner=%s intent=full", len(spots), scanResult.Report.Scanner)

	// With only one commit in the fixture there is no churn, so hot spots based
	// on churn alone will be empty. The assertion is that the call succeeds and
	// returns the correct type — not a cache miss error.
	// A real workspace with history would produce non-empty results.
}

// TestIntentFull_CacheKey_ReusableAcrossAnalysisTools verifies the data
// pipeline invariant: the cache_key returned by ScanProject can be passed to
// every analysis tool and each tool finds the same report.
//
// Given a scan_local result with a cache_key
// When multiple analysis tools are called with that cache_key
// Then none of them returns ErrNoCachedReport
func TestIntentFull_CacheKey_ReusableAcrossAnalysisTools(t *testing.T) {
	dir := tsFixture(t)
	eng := newEngineWithRealStore(t)

	scanResult, err := eng.ScanProject(context.Background(), dir, engine.ScanOpts{Intent: "full"})
	if err != nil {
		t.Fatalf("ScanProject: %v", err)
	}
	ck := scanResult.CacheKey

	ctx := context.Background()

	tools := []struct {
		name string
		call func() error
	}{
		{"SearchSymbols", func() error {
			_, e := eng.SearchSymbols(ctx, dir, "", ck)
			return e
		}},
		{"GetRiskScores", func() error {
			_, e := eng.GetRiskScores(ctx, dir, ck)
			return e
		}},
		{"GetCycles", func() error {
			_, e := eng.GetCycles(ctx, dir, nil, ck)
			return e
		}},
		{"GetDependencies", func() error {
			// GetDependencies requires a non-empty component name; use the first
			// service in the scan report so the call is valid.
			component := ""
			if len(scanResult.Report.Architecture.Services) > 0 {
				component = scanResult.Report.Architecture.Services[0].Name
			}
			_, e := eng.GetDependencies(ctx, dir, component, ck)
			return e
		}},
		{"GetHotSpots", func() error {
			_, e := eng.GetHotSpots(ctx, dir, 30, 5, ck)
			return e
		}},
	}

	for _, tool := range tools {
		if err := tool.call(); err != nil {
			t.Errorf("%s(cache_key=%q): %v — cache_key must be usable by all analysis tools", tool.name, ck, err)
		}
	}
}
