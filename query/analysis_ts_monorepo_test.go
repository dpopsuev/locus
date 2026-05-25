package query_test

// analysis_ts_monorepo_test.go reproduces the root cause of the null-analysis
// bug on TypeScript monorepos with path aliases.
//
// Root cause: scan_local uses ScannerOverride="typescript" (explicit), so
// AutoScanner uses TypeScriptScanner at monorepo root and produces component
// names like "packages/spine/src". The getOrScan cold path uses no override,
// so AutoScanner finds multiple sub-projects via discoverSubProjects and
// switches to CompositeScanner, which names components "src" (sub-project-
// relative). The component names mismatch: GetDependencies("packages/spine/src")
// finds nothing → fan_in=null. ComputeRiskScores sees a collapsed service set
// from the composite scan → "no components".
//
// Fix: ScanProject warms the plain-sha cache slot from the hit path so that
// getOrScan never reaches the cold path after scan_local has run.

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/dpopsuev/locus/internal/store"
	oculuscache "github.com/dpopsuev/oculus/v3/cache"
	"github.com/dpopsuev/oculus/v3/engine"
)

// writeFixture writes a map of relative-path→content files to a temp dir
// and returns the dir path.
func writeFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, content := range files {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	return dir
}

// tsMonorepoFixture creates a TypeScript npm-workspaces monorepo with three
// packages that import each other via tsconfig path aliases. Each package has
// its own package.json — this is what triggers discoverSubProjects > 1 and
// causes AutoScanner to switch to CompositeScanner on the cold path.
func tsMonorepoFixture(t *testing.T) string {
	t.Helper()
	dir := writeFixture(t, map[string]string{
		// Root workspace markers
		"package.json": `{"name": "alef-monorepo", "workspaces": ["packages/*"]}`,
		"tsconfig.json": `{
			"compilerOptions": {
				"paths": {
					"*":                  ["./*"],
					"@alef/spine":        ["./packages/spine/src/index.ts"],
					"@alef/corpus":       ["./packages/corpus/src/index.ts"],
					"@alef/organ-llm":    ["./packages/organ-llm/src/index.ts"]
				}
			}
		}`,

		// spine — the core shared library, imported by everyone
		"packages/spine/package.json":      `{"name": "@alef/spine"}`,
		"packages/spine/tsconfig.json":     `{"extends": "../../tsconfig.json"}`,
		"packages/spine/src/index.ts":      "export function spineCore(): void {}\nexport function spineFan(): void {}\n",

		// corpus — imports spine
		"packages/corpus/package.json":     `{"name": "@alef/corpus"}`,
		"packages/corpus/tsconfig.json":    `{"extends": "../../tsconfig.json"}`,
		"packages/corpus/src/index.ts": `
import { spineCore } from '@alef/spine';
export function corpusMain(): void { spineCore(); }
`,

		// organ-llm — imports both spine and corpus
		"packages/organ-llm/package.json":  `{"name": "@alef/organ-llm"}`,
		"packages/organ-llm/tsconfig.json": `{"extends": "../../tsconfig.json"}`,
		"packages/organ-llm/src/index.ts": `
import { spineCore, spineFan } from '@alef/spine';
import { corpusMain } from '@alef/corpus';
export function llmRun(): void { spineCore(); spineFan(); corpusMain(); }
`,
	})
	gitInit(t, dir)
	return dir
}

// newMonorepoEngine wires a fresh engine to a temp store and scans the
// monorepo fixture with intent=full, returning the engine and scan result.
func newMonorepoEngine(t *testing.T) (*engine.Engine, string, *engine.ScanResult) {
	t.Helper()
	dir := tsMonorepoFixture(t)
	sc := oculuscache.New(t.TempDir())
	db := store.NewFilesystem(sc, t.TempDir())
	eng := engine.New(db, nil)

	result, err := eng.ScanProject(context.Background(), dir, engine.ScanOpts{Intent: "full"})
	if err != nil {
		t.Fatalf("ScanProject: %v", err)
	}
	if len(result.Report.Architecture.Services) == 0 {
		t.Fatalf("ScanProject produced 0 services — fixture or scanner is broken")
	}
	t.Logf("scan: %d services, %d edges, scanner=%s",
		len(result.Report.Architecture.Services),
		len(result.Report.Architecture.Edges),
		result.Report.Scanner)
	return eng, dir, result
}

// TestTSMonorepo_CrossPackageEdgesInScan verifies that after alias resolution
// the scan contains cross-package edges (spine imported by corpus and organ-llm).
//
// Given a TypeScript monorepo with tsconfig path aliases
// When ScanProject is called with intent=full
// Then the report contains internal edges crossing package boundaries
func TestTSMonorepo_CrossPackageEdgesInScan(t *testing.T) {
	_, _, result := newMonorepoEngine(t)

	crossPkg := 0
	for _, e := range result.Report.Architecture.Edges {
		if e.From != e.To {
			crossPkg++
		}
	}
	if crossPkg == 0 {
		t.Errorf("0 cross-package edges; aliases may not be resolving to internal namespaces\n"+
			"all edges: %v", result.Report.Architecture.Edges)
	}
	t.Logf("%d cross-package edges", crossPkg)
}

// TestTSMonorepo_AnalysisWithCacheKey verifies that analysis tools called
// WITH the cache_key from scan_local return correct data.
//
// Given a completed scan with cache_key
// When GetDependencies / GetImpact / GetRiskScores are called with that key
// Then fan_in/fan_out are non-nil, blast_radius > 0 for spine, scores populated
func TestTSMonorepo_AnalysisWithCacheKey(t *testing.T) {
	eng, dir, result := newMonorepoEngine(t)
	ck := result.CacheKey
	ctx := context.Background()

	// spine is imported by corpus AND organ-llm → should have fan_in entries.
	t.Run("spine_fan_in", func(t *testing.T) {
		r, err := eng.GetDependencies(ctx, dir, "packages/spine/src", ck)
		if err != nil {
			t.Fatalf("GetDependencies: %v", err)
		}
		if len(r.FanIn) == 0 {
			t.Errorf("spine fan_in=null; expected corpus and organ-llm as dependents\n"+
				"all services: %v", serviceNames(result))
		}
		t.Logf("spine fan_in=%d fan_out=%d", len(r.FanIn), len(r.FanOut))
	})

	// corpus imports spine → fan_out should include spine.
	t.Run("corpus_fan_out", func(t *testing.T) {
		r, err := eng.GetDependencies(ctx, dir, "packages/corpus/src", ck)
		if err != nil {
			t.Fatalf("GetDependencies: %v", err)
		}
		if len(r.FanOut) == 0 {
			t.Errorf("corpus fan_out=null; expected spine as dependency")
		}
		t.Logf("corpus fan_in=%d fan_out=%d", len(r.FanIn), len(r.FanOut))
	})

	// spine is imported by 2+ packages → blast_radius should be > 0.
	t.Run("spine_blast_radius", func(t *testing.T) {
		r, err := eng.GetImpact(ctx, dir, "packages/spine/src", ck)
		if err != nil {
			t.Fatalf("GetImpact: %v", err)
		}
		if r.BlastRadius == 0 {
			t.Errorf("spine blast_radius=0; expected >0 (imported by corpus and organ-llm)")
		}
		t.Logf("spine blast_radius=%d%% direct=%v", r.BlastRadius, r.DirectDeps)
	})

	// risk_scores must not return "no components".
	t.Run("risk_scores", func(t *testing.T) {
		r, err := eng.GetRiskScores(ctx, dir, ck)
		if err != nil {
			t.Fatalf("GetRiskScores: %v", err)
		}
		if r.Summary == summaryNoComponents {
			t.Errorf("risk_scores: 'no components' with cache_key %q — "+
				"getOrScan returned report with 0 services", ck)
		}
		t.Logf("risk_scores: %q", r.Summary)
	})
}

// TestTSMonorepo_AnalysisWithoutCacheKey is the primary regression test for
// the CompositeScanner mismatch bug. Analysis tools called WITHOUT a cache_key
// must return the same data as those called with it — not null from a
// CompositeScanner cold rescan that produces sub-project-relative names.
//
// Given a completed intent=full scan (plain-sha slot warmed from hit path)
// When analysis tools are called without a cache_key
// Then they return the same data as with the cache_key — no CompositeScanner mismatch
func TestTSMonorepo_AnalysisWithoutCacheKey(t *testing.T) {
	eng, dir, result := newMonorepoEngine(t)
	ctx := context.Background()

	t.Run("spine_fan_in_no_cache_key", func(t *testing.T) {
		r, err := eng.GetDependencies(ctx, dir, "packages/spine/src")
		if err != nil {
			t.Fatalf("GetDependencies (no cache_key): %v\n"+
				"cold rescan produced error — plain-sha slot not warmed", err)
		}
		if len(r.FanIn) == 0 {
			t.Errorf("spine fan_in=null without cache_key\n"+
				"root cause: getOrScan cold path used CompositeScanner which names\n"+
				"components 'src' (sub-project-relative) instead of 'packages/spine/src';\n"+
				"component name mismatch → no edges found → null\n"+
				"all services from scan: %v", serviceNames(result))
		}
		t.Logf("spine fan_in=%d (no cache_key)", len(r.FanIn))
	})

	t.Run("risk_scores_no_cache_key", func(t *testing.T) {
		r, err := eng.GetRiskScores(ctx, dir)
		if err != nil {
			t.Fatalf("GetRiskScores (no cache_key): %v", err)
		}
		if r.Summary == summaryNoComponents {
			t.Errorf("risk_scores 'no components' without cache_key\n"+
				"root cause: CompositeScanner cold rescan collapsed or misnamed services;\n"+
				"fix: warm plain-sha slot from cache-hit path in ScanProject")
		}
		t.Logf("risk_scores (no cache_key): %q", r.Summary)
	})
}

func serviceNames(result *engine.ScanResult) []string {
	names := make([]string, len(result.Report.Architecture.Services))
	for i, s := range result.Report.Architecture.Services {
		names[i] = s.Name
	}
	return names
}
