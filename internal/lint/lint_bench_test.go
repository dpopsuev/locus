package lint_test

import (
	"runtime"
	"testing"

	"github.com/dpopsuev/locus/internal/arch"
	"github.com/dpopsuev/locus/internal/lint"
)

// locusRoot is the path to the Locus repo root relative to this file.
const locusRoot = "../.."

func scanLocus(tb testing.TB) *arch.ContextReport {
	tb.Helper()
	report, err := arch.ScanAndBuild(locusRoot, arch.ScanOpts{
		Intent:       arch.IntentHealth,
		ExcludeTests: true,
	})
	if err != nil {
		tb.Fatalf("ScanAndBuild: %v", err)
	}
	return report
}

// BenchmarkLintRun benchmarks lint.Run on the Locus codebase itself.
func BenchmarkLintRun(b *testing.B) {
	report := scanLocus(b)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		lint.Run(report, lint.RunOpts{Root: locusRoot})
	}
}

// TestLintMemoryBudget verifies that lint.Run stays under a reasonable
// memory budget when processing the Locus codebase.
func TestLintMemoryBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping memory budget test in -short mode")
	}

	report := scanLocus(t)

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	result := lint.Run(report, lint.RunOpts{Root: locusRoot})

	runtime.ReadMemStats(&after)

	if result == nil {
		t.Fatal("Run returned nil")
	}

	// Budget: lint.Run should allocate less than 64 MB for the Locus codebase.
	// The scan itself does class/impl analysis which dominates allocation.
	const budgetBytes = 64 * 1024 * 1024
	allocated := after.TotalAlloc - before.TotalAlloc
	t.Logf("lint.Run allocated %d bytes (%.2f MB), violations: %d, score: %.1f",
		allocated, float64(allocated)/(1024*1024), len(result.Violations), result.Score)

	if allocated > budgetBytes {
		t.Errorf("lint.Run allocated %d bytes (%.2f MB), budget is %d bytes (%.0f MB)",
			allocated, float64(allocated)/(1024*1024), budgetBytes, float64(budgetBytes)/(1024*1024))
	}
}
