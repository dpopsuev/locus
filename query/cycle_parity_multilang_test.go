package query_test

// Multi-language cycle detection parity tests.
//
// Each fixture in testdata/cycles/<lang>/ contains two packages with a mutual
// import — packages/api/src ↔ packages/api/src/core — arranged so that at
// grouping depth=2 both packages collapse into the single component
// "packages/api", making the cycle invisible at component level while remaining
// detectable at module (package) level.
//
// Per-language notes:
//
//   typescript  ts_scanner, relative imports, cycles valid       → grouping suppresses ✓
//   python      python_scanner, dot-notation namespaces          → dot-paths don't split
//               on "/" so InferDefaultGroups can't collapse them; only module-level is tested
//   java        ctags_scanner, package imports                   → grouping suppresses ✓
//   kotlin      ctags_scanner, package imports                   → grouping suppresses ✓
//   csharp      ctags_scanner, using directives (needs global.json marker for
//               survey.DetectFromMarkers to pick C#)             → grouping suppresses ✓
//   c           ctags_scanner, #include edge extraction          → grouping suppresses ✓

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	engine "github.com/dpopsuev/oculus/v3/engine"
)

// cycleLangFixture describes one language fixture and what the scanner can assert.
type cycleLangFixture struct {
	// lang is the human-readable name and the subdirectory under testdata/cycles/.
	lang string
	// groupingSupported: if true, ScanProject(depth=2) is expected to suppress the
	// cycle so that component-level count == 0 while module-level count >= 1.
	// If false only module-level detection is asserted.
	groupingSupported bool
	// requiresTool: if non-empty, the test is skipped when that binary is absent.
	requiresTool string
}

var cycleLangFixtures = []cycleLangFixture{
	{lang: "typescript", groupingSupported: true},
	{lang: "python", groupingSupported: false}, // dot-namespaces don't collapse under depth grouping
	{lang: "java", groupingSupported: true, requiresTool: "ctags"},
	{lang: "kotlin", groupingSupported: true, requiresTool: "ctags"},
	{lang: "csharp", groupingSupported: true, requiresTool: "ctags"},
	{lang: "c", groupingSupported: true, requiresTool: "ctags"},
}

func fixturePath(t *testing.T, lang string) string {
	t.Helper()
	return filepath.Join(locusRoot(t), "testdata", "cycles", lang)
}

// TestCycleDetection_ModuleLevel asserts that every language fixture produces
// at least one cycle when scanned without grouping (module-level graph).
func TestCycleDetection_ModuleLevel(t *testing.T) {
	for _, fx := range cycleLangFixtures {
		fx := fx
		t.Run(fx.lang, func(t *testing.T) {
			if fx.requiresTool != "" {
				if _, err := exec.LookPath(fx.requiresTool); err != nil {
					t.Skipf("%s not on PATH — skipping %s fixture", fx.requiresTool, fx.lang)
				}
			}

			eng := newEngine(t)
			ctx := context.Background()

			report, err := eng.GetCycles(ctx, fixturePath(t, fx.lang), nil)
			if err != nil {
				t.Fatalf("GetCycles: %v", err)
			}
			if len(report.Cycles) == 0 {
				t.Errorf("%s: expected ≥1 module-level cycle, got 0 — fixture or scanner is broken", fx.lang)
			}
		})
	}
}

// TestCycleDetection_GroupingSuppress asserts that for languages where depth
// grouping collapses the two packages into one component, ScanProject(depth=2)
// reports 0 component-level cycles while ModuleLevelCycles surfaces the real count.
func TestCycleDetection_GroupingSuppress(t *testing.T) {
	for _, fx := range cycleLangFixtures {
		fx := fx
		if !fx.groupingSupported {
			continue
		}
		t.Run(fx.lang, func(t *testing.T) {
			if fx.requiresTool != "" {
				if _, err := exec.LookPath(fx.requiresTool); err != nil {
					t.Skipf("%s not on PATH — skipping %s fixture", fx.requiresTool, fx.lang)
				}
			}

			eng := newEngine(t)
			ctx := context.Background()
			root := fixturePath(t, fx.lang)

			result, err := eng.ScanProject(ctx, root, engine.ScanOpts{
				Depth:  2,
				Intent: "coupling",
			})
			if err != nil {
				t.Fatalf("ScanProject: %v", err)
			}

			componentCycles := len(result.Report.Cycles)
			moduleCycles := len(result.Report.ModuleLevelCycles)

			if componentCycles != 0 {
				t.Errorf("%s: expected 0 component-level cycles at depth=2, got %d", fx.lang, componentCycles)
			}
			if moduleCycles == 0 {
				t.Errorf("%s: expected ≥1 module-level cycles in ModuleLevelCycles, got 0", fx.lang)
			}
		})
	}
}

// TestCycleDetection_SummaryLabel asserts that RenderScanSummary labels both
// granularities when grouping suppresses cycles that exist at module level.
func TestCycleDetection_SummaryLabel(t *testing.T) {
	for _, fx := range cycleLangFixtures {
		fx := fx
		if !fx.groupingSupported {
			continue
		}
		t.Run(fx.lang, func(t *testing.T) {
			if fx.requiresTool != "" {
				if _, err := exec.LookPath(fx.requiresTool); err != nil {
					t.Skipf("%s not on PATH — skipping %s fixture", fx.requiresTool, fx.lang)
				}
			}

			eng := newEngine(t)
			ctx := context.Background()
			root := fixturePath(t, fx.lang)

			result, err := eng.ScanProject(ctx, root, engine.ScanOpts{
				Depth:  2,
				Intent: "coupling",
			})
			if err != nil {
				t.Fatalf("ScanProject: %v", err)
			}

			summary := engine.RenderScanSummary(result, "")

			if !strings.Contains(summary, "component-level") || !strings.Contains(summary, "module-level") {
				t.Errorf("%s: summary missing granularity labels\ngot: %s", fx.lang, summary)
			}

			// Verify the format: "0 component-level (N module-level) cycles"
			if !strings.Contains(summary, "0 component-level") {
				t.Errorf("%s: expected '0 component-level' in summary\ngot: %s", fx.lang, summary)
			}
			t.Logf("summary: %s", fmt.Sprintf("%q", summary))
		})
	}
}
