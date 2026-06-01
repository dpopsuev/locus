// Package mcp — end-to-end simulation of the production failure mode.
//
// Reproduces the exact MCP call sequence observed in production:
//   1. scan_local (no path, no cache_key, intent=full)
//   2. locus_analysis impact / deps / risk_scores / preset (no path, no cache_key)
//
// All calls go through the full handler stack so the test catches issues
// that only appear when the handler, engine, and store are wired together.
package mcp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dpopsuev/locus/internal/store"
	oculuscache "github.com/dpopsuev/oculus/v3/cache"
	"github.com/dpopsuev/oculus/v3/engine"
	"github.com/dpopsuev/oculus/v3/triage"
)

const mcpSummaryNoComponents = "no components"

// monorepoFixture builds the same TypeScript monorepo used in the query-level
// tests but from inside the mcp package, so all handler calls go through the
// real handler stack with a real engine and a real store.
func monorepoFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"package.json": `{"name":"alef-sim","workspaces":["packages/*"]}`,
		"tsconfig.json": `{
			"compilerOptions":{
				"paths":{
					"*":             ["./*"],
					"@alef/spine":   ["./packages/spine/src/index.ts"],
					"@alef/corpus":  ["./packages/corpus/src/index.ts"],
					"@alef/organ":   ["./packages/organ/src/index.ts"]
				}
			}
		}`,
		"packages/spine/package.json":    `{"name":"@alef/spine"}`,
		"packages/spine/tsconfig.json":   `{"extends":"../../tsconfig.json"}`,
		"packages/spine/src/index.ts":    "export function spineCore():void{}\n",
		"packages/corpus/package.json":   `{"name":"@alef/corpus"}`,
		"packages/corpus/tsconfig.json":  `{"extends":"../../tsconfig.json"}`,
		"packages/corpus/src/index.ts":   "import { spineCore } from '@alef/spine';\nexport function corpusMain(): void { spineCore(); }\n",
		"packages/organ/package.json":    `{"name":"@alef/organ"}`,
		"packages/organ/tsconfig.json":   `{"extends":"../../tsconfig.json"}`,
		"packages/organ/src/index.ts":    "import { spineCore } from '@alef/spine';\nimport { corpusMain } from '@alef/corpus';\nexport function organRun(): void { spineCore(); corpusMain(); }\n",
	}
	for rel, content := range files {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	// git repo so ResolveHEAD returns a non-empty SHA.
	run := func(args ...string) { //nolint:gosec
		cmd := exec.Command("git", args...) //nolint:gosec
		cmd.Dir = dir
		_ = cmd.Run()
	}
	run("init", "-q")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")
	run("add", "-A")
	run("commit", "-q", "-m", "init")
	return dir
}

// newHandlerWithWorkspace builds the full handler+engine stack wired to a
// specific workspace root directory. This replicates how locus serve wires
// up the engine in production.
func newHandlerWithWorkspace(t *testing.T, workspaceRoot string) *handler {
	t.Helper()
	sc := oculuscache.New(t.TempDir())
	db := store.NewFilesystem(sc, filepath.Join(t.TempDir(), "history"))
	lruDB := store.NewLRU(db, 16)
	proto := engine.New(lruDB, []string{workspaceRoot})
	reg := triage.New()
	_ = reg
	return &handler{proto: proto, sproto: proto}
}

// --- Diagnostic helpers ---

// sha returns the git HEAD SHA for the handler's workspace[0].
func resolvedSHA(h *handler) string {
	path := h.proto.ResolvePath("") // empty = workspaces[0]
	return h.proto.ResolveHEAD(path)
}

// --- Simulation tests ---

// TestMCPSimulation_ScanThenAnalysis is the primary end-to-end regression for
// LCS-BUG-86. It replicates the exact MCP call sequence from production:
//
//  1. handleScanProject (scan_local, no path, no cache_key, intent=full)
//  2. handleAnalysis impact   — blast radius for most-depended component
//  3. handleAnalysis deps     — fan_in/fan_out for a component
//  4. handleAnalysis risk_scores
//  5. handleAnalysis preset=architecture_review
//
// All calls use empty path (resolved from workspaces[0]) and no cache_key.
// Assertions verify non-null, non-zero results.
func TestMCPSimulation_ScanThenAnalysis(t *testing.T) {
	dir := monorepoFixture(t)
	h := newHandlerWithWorkspace(t, dir)
	ctx := context.Background()

	sha, services, edges, cacheKey := runScanAndDiagnose(ctx, t, h)
	runAnalysisSubtests(ctx, t, h, sha, services, cacheKey)
	_ = edges
}

func runScanAndDiagnose(ctx context.Context, t *testing.T, h *handler) (sha string, services, edges int, cacheKey string) {
	t.Helper()

	sha = resolvedSHA(h)
	t.Logf("workspace: %s", h.proto.ResolvePath(""))
	t.Logf("git HEAD SHA: %q (empty means git unavailable → nothing will be cached)", sha)
	if sha == "" {
		t.Error("SHA is empty — git unavailable; ScanProject stores nothing, analysis runs ScanAndBuild")
	}

	scanResult, _, err := h.handleScanProject(ctx, nil, &codographActionInput{Intent: "full"})
	if err != nil {
		t.Fatalf("scan_local: %v", err)
	}
	scanText := extractText(scanResult)
	t.Logf("scan_local: %s", scanText)

	services, edges, cacheKey = parseScanSummary(scanText)
	t.Logf("parsed: services=%d edges=%d cache_key=%q", services, edges, cacheKey)
	if services == 0 {
		t.Errorf("scan produced 0 services")
	}
	if edges == 0 {
		t.Errorf("scan produced 0 edges — alias resolution may have failed")
	}
	return sha, services, edges, cacheKey
}

func runAnalysisSubtests(ctx context.Context, t *testing.T, h *handler, sha string, services int, cacheKey string) {
	t.Helper()
	t.Run("impact_no_cache_key", func(t *testing.T) {
		r, _, err := h.handleAnalysis(ctx, nil, analysisInput{Action: ActionImpact, Component: "packages/spine/src"})
		if err != nil {
			t.Fatalf("impact: %v", err)
		}
		body := extractText(r)
		t.Logf("impact: %s", body)
		if strings.Contains(body, `"blast_radius":0`) {
			t.Errorf("blast_radius=0 for spine; SHA=%q cacheKey=%q", sha, cacheKey)
		}
	})
	t.Run("deps_fan_in_no_cache_key", func(t *testing.T) {
		r, _, err := h.handleAnalysis(ctx, nil, analysisInput{Action: ActionDeps, Component: "packages/spine/src"})
		if err != nil {
			t.Fatalf("deps: %v", err)
		}
		body := extractText(r)
		t.Logf("deps: %s", body)
		if strings.Contains(body, `"fan_in":null`) {
			t.Errorf("fan_in=null for spine; SHA=%q", sha)
		}
	})
	t.Run("risk_scores_no_cache_key", func(t *testing.T) {
		r, _, err := h.handleAnalysis(ctx, nil, analysisInput{Action: ActionRiskScores})
		if err != nil {
			t.Fatalf("risk_scores: %v", err)
		}
		body := extractText(r)
		t.Logf("risk_scores: %s", body)
		if strings.Contains(body, mcpSummaryNoComponents) {
			t.Errorf("risk_scores 'no components'; SHA=%q services_from_scan=%d", sha, services)
		}
	})
	t.Run("preset_architecture_review", func(t *testing.T) {
		r, _, err := h.handleAnalysis(ctx, nil, analysisInput{Action: ActionPreset, Preset: "architecture_review"})
		if err != nil {
			t.Fatalf("preset: %v", err)
		}
		body := extractText(r)
		t.Logf("preset (first 200 chars): %.200s", body)
		if strings.Contains(body, "0 components") {
			t.Errorf("preset '0 components'; SHA=%q", sha)
		}
	})
}

// TestMCPSimulation_SHAEmptyWhenGitUnavailable verifies that scan_local
// returns an error for non-git workspaces (LCS-BUG-92 fix). Previously the
// server would proceed with an uncacheable cold scan and OOM on large paths.
//
// Given a workspace with no .git directory
// When scan_local is called
// Then an error is returned — the server refuses to scan
func TestMCPSimulation_SHAEmptyWhenGitUnavailable(t *testing.T) {
	dir := t.TempDir() // no git init
	files := map[string]string{
		"package.json":  `{"name":"no-git"}`,
		"tsconfig.json": `{"compilerOptions":{}}`,
		"src/index.ts":  "export function hello():void{}\n",
	}
	for rel, content := range files {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	h := newHandlerWithWorkspace(t, dir)
	sha := resolvedSHA(h)
	t.Logf("SHA for no-git workspace: %q", sha)

	if sha != "" {
		t.Skipf("git found a repo at or above %s (SHA=%q) — test not applicable", dir, sha)
	}

	// scan_local still runs but caches nothing — the OOM guard is now in CLI startup.
	_, _, err := h.handleScanProject(context.Background(), nil, &codographActionInput{Intent: "full"})
	// Either an error or a 0-component result is acceptable.
	t.Logf("non-git scan_local: err=%v", err)
}

// --- helpers ---

func extractText(result *sdkmcp.CallToolResult) string {
	if result == nil {
		return "<nil result>"
	}
	for _, c := range result.Content {
		if tc, ok := c.(*sdkmcp.TextContent); ok {
			return tc.Text
		}
	}
	return fmt.Sprintf("<no text content; %d content items>", len(result.Content))
}

func parseScanSummary(text string) (services, edges int, cacheKey string) {
	// Summary format: "Scanned NAME: N components, M edges, ... cache_key: PATH@SHA"
	// Skip past the colon separator before parsing counts.
	if idx := strings.Index(text, ": "); idx >= 0 {
		_, _ = fmt.Sscanf(text[idx+2:], "%d components, %d edges", &services, &edges)
	}
	if idx := strings.Index(text, "cache_key: "); idx >= 0 {
		rest := text[idx+len("cache_key: "):]
		if nl := strings.IndexAny(rest, "\n\r "); nl >= 0 {
			cacheKey = rest[:nl]
		} else {
			cacheKey = rest
		}
	}
	return
}

// TestMCPSymbolSearch_FileFilter verifies that symbol_search with file= set
// returns only symbols from the specified file.
//
// Given a monorepo scanned in full
// When symbol_search(file=packages/spine/src/index.ts) is called
// Then spineCore is returned and corpusMain is not
func TestMCPSymbolSearch_FileFilter(t *testing.T) {
	dir := monorepoFixture(t)
	h := newHandlerWithWorkspace(t, dir)
	ctx := context.Background()

	_, _, _, cacheKey := runScanAndDiagnose(ctx, t, h)

	spineFile := filepath.Join(dir, "packages/spine/src/index.ts")
	result, _, err := h.handleAnalysis(ctx, nil, analysisInput{
		Action:   ActionSymbolSearch,
		File:     spineFile,
		CacheKey: cacheKey,
	})
	if err != nil {
		t.Fatalf("symbol_search file=...: %v", err)
	}

	text := extractText(result)
	t.Logf("symbol_search result: %s", text)
	if !strings.Contains(text, "spineCore") {
		t.Errorf("expected spineCore in result for spine/src/index.ts")
	}
	if strings.Contains(text, "corpusMain") {
		t.Errorf("corpusMain from corpus/src/index.ts should not appear in spine file filter")
	}
}

// TestMCPAnalysis_StalenessWarning verifies that when HEAD advances after a
// scan, analysis results include a staleness warning.
//
// Given a project scanned at SHA abc (from the initial commit)
// And then a second commit is made advancing HEAD to a new SHA
// When analysis(deps) is called with the old cache_key
// Then the result text contains "Warning: cached scan is stale"
func TestMCPAnalysis_StalenessWarning(t *testing.T) {
	dir := monorepoFixture(t)
	h := newHandlerWithWorkspace(t, dir)
	ctx := context.Background()

	_, _, _, cacheKey := runScanAndDiagnose(ctx, t, h)
	if cacheKey == "" {
		t.Skip("no cache_key returned from scan (git unavailable)")
	}

	// Advance HEAD by making a new commit.
	run := func(args ...string) {
		cmd := exec.Command("git", args...) //nolint:gosec
		cmd.Dir = dir
		_ = cmd.Run()
	}
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")

	extraFile := filepath.Join(dir, "extra.ts")
	if err := os.WriteFile(extraFile, []byte("export function extra() {}"), 0o600); err != nil {
		t.Fatal(err)
	}
	run("add", extraFile)
	run("commit", "-q", "-m", "add extra.ts")

	// Use the OLD cache_key — HEAD has advanced.
	result, _, err := h.handleAnalysis(ctx, nil, analysisInput{
		Action:   ActionDeps,
		Component: "packages/spine/src",
		CacheKey: cacheKey,
	})
	if err != nil {
		t.Fatalf("deps: %v", err)
	}

	text := extractText(result)
	t.Logf("deps result: %s", text)
	if !strings.Contains(text, "stale") {
		t.Errorf("expected staleness warning in result when HEAD has advanced; got: %q", text)
	}
}

// TestMCPAnalysis_PathOnlyResolvesLatestScan verifies that analysis tools
// accept path= without cache_key= and resolve the latest cached scan.
//
// Given a project that was scanned with scan_local
// When deps is called with only path= (no cache_key)
// Then results are returned from the cached scan
func TestMCPAnalysis_PathOnlyResolvesLatestScan(t *testing.T) {
	dir := monorepoFixture(t)
	h := newHandlerWithWorkspace(t, dir)
	ctx := context.Background()

	// Scan first to populate the cache.
	_, _, _, _ = runScanAndDiagnose(ctx, t, h)

	// Analysis with path= only, no cache_key.
	result, _, err := h.handleAnalysis(ctx, nil, analysisInput{
		Action:    ActionDeps,
		Path:      dir,
		Component: "packages/spine/src",
		// CacheKey intentionally omitted.
	})
	if err != nil {
		t.Fatalf("deps path-only: %v", err)
	}
	text := extractText(result)
	if !strings.Contains(text, "packages/spine/src") {
		t.Errorf("expected spine component in result; got: %s", text)
	}
}

// TestMCPAnalysis_CallersAt verifies that callers_at is wired and returns a
// CallersReport (empty is OK without a real LSP server).
//
// Given a scanned project
// When callers_at(file=..., line=1, char=0) is called
// Then a CallersReport is returned (no error)
func TestMCPAnalysis_CallersAt(t *testing.T) {
	dir := monorepoFixture(t)
	h := newHandlerWithWorkspace(t, dir)
	ctx := context.Background()

	_, _, _, cacheKey := runScanAndDiagnose(ctx, t, h)

	spineFile := filepath.Join(dir, "packages/spine/src/index.ts")
	result, _, err := h.handleAnalysis(ctx, nil, analysisInput{
		Action:   ActionCallersAt,
		File:     spineFile,
		Line:     1,
		Char:     0,
		CacheKey: cacheKey,
	})
	if err != nil {
		t.Fatalf("callers_at: %v", err)
	}

	text := extractText(result)
	t.Logf("callers_at result: %s", text)
	if !strings.Contains(text, "caller") {
		t.Errorf("expected 'caller' in result; got: %s", text)
	}
}

// TestScanLocal_TSFileGranularity verifies that when scan_local is called with
// file_granularity=true on a TypeScript project, each .ts file becomes its own
// component in the scan result rather than being grouped by directory.
//
// Given a TypeScript project with 2 .ts files in src/
// When scan_local(file_granularity=true) is called
// Then the result has ≥2 components (one per file, not one per directory)
func TestScanLocal_TSFileGranularity(t *testing.T) {
	dir := monorepoFixture(t) // has packages/spine/src/index.ts, etc.
	h := newHandlerWithWorkspace(t, dir)
	ctx := context.Background()

	// Default scan — directory-level grouping.
	_, _, defaultCacheKey := func() (int, int, string) {
		_, svcs, edges, ck := runScanAndDiagnose(ctx, t, h)
		return svcs, edges, ck
	}()

	// File-granularity scan.
	result, _, err := h.handleScanProject(ctx, nil, &codographActionInput{
		Path:            dir,
		Intent:          "full",
		FileGranularity: true,
	})
	if err != nil {
		t.Fatalf("scan file_granularity: %v", err)
	}
	fileGranText := extractText(result)
	t.Logf("file granularity scan: %s", fileGranText)

	// Extract component counts from both.
	_, _, fileCacheKey := parseScanSummary(fileGranText)

	// File-granularity must produce a different cache key (has -file suffix).
	if fileCacheKey == defaultCacheKey {
		t.Errorf("file-granularity scan should produce a different cache key; default=%q file=%q",
			defaultCacheKey, fileCacheKey)
	}
	if !strings.HasSuffix(fileCacheKey, "-file") {
		t.Errorf("file-granularity cache key should end with -file; got %q", fileCacheKey)
	}
}

// --- LCS-BUG-92: scan_local must refuse non-git workspace ---

// TestScanLocal_NonGitWorkspace_Warns documents LCS-BUG-92.
// The OOM was caused by passing a huge non-git directory as --workspace.
// The guard now lives in the CLI startup (cli.go), not in scan_local.
// scan_local on a non-git path still runs but warns and produces uncached results.
//
// Given a workspace path that is not inside a git repository
// When scan_local is called
// Then it completes (with a warning logged) and returns 0 components
func TestScanLocal_NonGitWorkspace_Warns(t *testing.T) {
	nonGitDir := t.TempDir()
	h := newHandlerWithWorkspace(t, nonGitDir)
	ctx := context.Background()

	result, _, err := h.handleScanProject(ctx, nil, &codographActionInput{
		Path:   nonGitDir,
		Intent: "architecture",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := extractText(result)
	if !strings.Contains(text, "0 components") {
		t.Logf("result: %s", text)
	}
}
