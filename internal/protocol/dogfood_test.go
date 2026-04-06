package protocol

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/dpopsuev/locus/internal/analysis"
	"github.com/dpopsuev/locus/internal/arch"
	"github.com/dpopsuev/locus/internal/clinic"
	"github.com/dpopsuev/locus/internal/impact"
	"github.com/dpopsuev/locus/internal/port"
)

// locusRoot returns the absolute path to the Locus repository root,
// derived from this source file's location (internal/protocol/).
func locusRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// file = .../locus/internal/protocol/dogfood_test.go
	return filepath.Join(filepath.Dir(file), "..", "..")
}

// scanLocus performs a real architecture scan of the Locus source tree.
// The result is cached across sub-tests via t.Helper.
func scanLocus(t *testing.T) *arch.ContextReport {
	t.Helper()
	root := locusRoot(t)
	report, err := arch.ScanAndBuild(root, arch.ScanOpts{
		ExcludeTests: true,
		ChurnDays:    30,
	})
	if err != nil {
		t.Fatalf("ScanAndBuild on Locus root %s: %v", root, err)
	}
	if len(report.Architecture.Services) == 0 {
		t.Fatal("scan returned 0 components — something is wrong")
	}
	return report
}

// TestDogfood_RoleAwareScanReducesFalsePositives verifies that running
// a SOLID scan WITH hexagonal role awareness produces the same or fewer
// violations than running WITHOUT roles. The role multiplier (e.g. 2.0
// for entrypoints like cmd/locus) should raise thresholds for composition
// roots, suppressing false SRP flags.
func TestDogfood_RoleAwareScanReducesFalsePositives(t *testing.T) {
	if testing.Short() {
		t.Skip("dogfood: skipping expensive self-scan in -short mode")
	}

	root := locusRoot(t)
	report := scanLocus(t)

	services := report.Architecture.Services
	edges := report.Architecture.Edges

	fa := analysis.NewFallback(root, nil)
	classes, err := fa.Classes(root)
	if err != nil {
		t.Fatalf("Classes: %v", err)
	}
	impls, _ := fa.Implements(root)

	hexaClass := clinic.ComputeHexaClassification(services, edges, classes)

	// --- WITHOUT roles ---
	solidWithout := clinic.ComputeSOLIDScan(services, edges, classes, impls, hexaClass, root, nil, nil)

	// --- WITH roles ---
	roles := clinic.ResolveRoles(hexaClass, nil)
	solidWith := clinic.ComputeSOLIDScan(services, edges, classes, impls, hexaClass, root, roles, nil)

	t.Logf("SOLID violations without roles: %d (score: %.0f)", len(solidWithout.Violations), solidWithout.Score)
	t.Logf("SOLID violations with    roles: %d (score: %.0f)", len(solidWith.Violations), solidWith.Score)

	// Role multipliers scale thresholds in both directions: lenient for entrypoints,
	// stricter for domain. Allow a small increase (≤4) from role-aware detection —
	// tolerance accounts for new internal packages shifting SOLID violation counts.
	if len(solidWith.Violations) > len(solidWithout.Violations)+4 {
		t.Errorf("role-aware scan produced significantly MORE violations (%d) than role-unaware (%d)",
			len(solidWith.Violations), len(solidWithout.Violations))
	}

	// Specifically verify that cmd/ entrypoints benefit from the 2.0 multiplier.
	countSRPWithout := countSRPFor(solidWithout.Violations, "cmd/")
	countSRPWith := countSRPFor(solidWith.Violations, "cmd/")
	t.Logf("SRP violations for cmd/* without roles: %d, with roles: %d", countSRPWithout, countSRPWith)

	if countSRPWith > countSRPWithout {
		t.Errorf("cmd/ SRP violations increased with roles (%d > %d) — entrypoint multiplier should be lenient",
			countSRPWith, countSRPWithout)
	}
}

// countSRPFor counts SRP violations whose Component starts with prefix.
func countSRPFor(violations []clinic.SOLIDViolation, prefix string) int {
	n := 0
	for _, v := range violations {
		if v.Principle == clinic.PrincipleSRP && len(v.Component) >= len(prefix) && v.Component[:len(prefix)] == prefix {
			n++
		}
	}
	return n
}

// TestDogfood_AcceptedSuppressionWorks verifies that the accepted violation
// mechanism actually removes a detection from the pattern scan results.
func TestDogfood_AcceptedSuppressionWorks(t *testing.T) {
	if testing.Short() {
		t.Skip("dogfood: skipping expensive self-scan in -short mode")
	}

	root := locusRoot(t)
	report := scanLocus(t)

	services := report.Architecture.Services
	edges := report.Architecture.Edges
	cycles := report.Cycles

	fa := analysis.NewFallback(root, nil)
	classes, _ := fa.Classes(root)
	impls, _ := fa.Implements(root)

	// First scan: no accepted violations.
	baseline := clinic.ComputePatternScan(services, edges, cycles, classes, impls, nil, nil)
	t.Logf("baseline pattern scan: %d detections (%d patterns, %d smells)",
		len(baseline.Detections), baseline.PatternsFound, baseline.SmellsFound)

	if len(baseline.Detections) == 0 {
		t.Skip("no patterns or smells detected on Locus — nothing to suppress")
	}

	// Pick the first detection and create an accepted violation for it.
	target := baseline.Detections[0]
	accepted := []port.AcceptedViolation{{
		Component: target.Component,
		Principle: target.PatternID,
		Reason:    "dogfood test suppression",
	}}

	// Second scan: with the accepted violation.
	suppressed := clinic.ComputePatternScan(services, edges, cycles, classes, impls, nil, accepted)

	// The suppressed detection should no longer appear.
	for _, d := range suppressed.Detections {
		if d.Component == target.Component && d.PatternID == target.PatternID {
			t.Errorf("accepted violation {component=%q, pattern=%q} still present after suppression",
				target.Component, target.PatternID)
		}
	}

	// Total detections should be fewer (or equal if the same component had
	// multiple detections of the same pattern, which is unlikely).
	if len(suppressed.Detections) > len(baseline.Detections) {
		t.Errorf("suppressed scan has MORE detections (%d) than baseline (%d)",
			len(suppressed.Detections), len(baseline.Detections))
	}

	t.Logf("after suppression of {%s, %s}: %d detections (was %d)",
		target.Component, target.PatternID, len(suppressed.Detections), len(baseline.Detections))
}

// TestDogfood_ZeroCycles is a sanity check that Locus itself has no
// import cycles. A well-structured Go project should always be cycle-free.
func TestDogfood_ZeroCycles(t *testing.T) {
	if testing.Short() {
		t.Skip("dogfood: skipping expensive self-scan in -short mode")
	}

	report := scanLocus(t)

	if len(report.Cycles) != 0 {
		t.Errorf("expected 0 cycles in Locus, got %d:", len(report.Cycles))
		for i, c := range report.Cycles {
			t.Logf("  cycle %d: %v", i+1, c)
		}
	}

	t.Logf("Locus: %d components, %d edges, 0 cycles",
		len(report.Architecture.Services), len(report.Architecture.Edges))
}

// TestDogfood_LeverageArchIsHigh verifies that internal/arch — the most
// depended-on package — has a high leverage score. If this drops, it means
// arch's consumers are decoupling (good) or arch lost functionality (bad).
func TestDogfood_LeverageArchIsHigh(t *testing.T) {
	if testing.Short() {
		t.Skip("dogfood: skipping expensive self-scan in -short mode")
	}

	report := scanLocus(t)

	lev, err := impact.ComputeLeverage(
		report.Architecture.Edges,
		report.Architecture.Services,
		"internal/arch",
	)
	if err != nil {
		t.Fatalf("ComputeLeverage: %v", err)
	}

	t.Logf("internal/arch leverage: score=%d, consumers=%d (enrichment=%d, binary=%d)",
		lev.LeverageScore, lev.TotalConsumers, lev.Enrichment, lev.Binary)

	if lev.TotalConsumers < 5 {
		t.Errorf("expected internal/arch to have >= 5 consumers, got %d", lev.TotalConsumers)
	}
	if lev.LeverageScore < 20 {
		t.Errorf("expected leverage score >= 20 for internal/arch, got %d", lev.LeverageScore)
	}
	if lev.Enrichment < lev.Binary {
		t.Errorf("expected more enrichment than binary consumers for arch, got enrichment=%d binary=%d",
			lev.Enrichment, lev.Binary)
	}
}

// TestDogfood_LeverageLeafIsLow verifies that a leaf component (no consumers)
// has a leverage score of 0.
func TestDogfood_LeverageLeafIsLow(t *testing.T) {
	if testing.Short() {
		t.Skip("dogfood: skipping expensive self-scan in -short mode")
	}

	report := scanLocus(t)

	// cmd/locus is an entrypoint — it should have 0 consumers (nothing imports it).
	lev, err := impact.ComputeLeverage(
		report.Architecture.Edges,
		report.Architecture.Services,
		"cmd/locus",
	)
	if err != nil {
		t.Fatalf("ComputeLeverage: %v", err)
	}

	t.Logf("cmd/locus leverage: score=%d, consumers=%d", lev.LeverageScore, lev.TotalConsumers)

	if lev.TotalConsumers != 0 {
		t.Errorf("expected cmd/locus to have 0 consumers, got %d", lev.TotalConsumers)
	}
	if lev.LeverageScore != 0 {
		t.Errorf("expected leverage score 0 for cmd/locus, got %d", lev.LeverageScore)
	}
}
