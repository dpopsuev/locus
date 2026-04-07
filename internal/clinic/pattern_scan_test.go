package clinic

import (
	"fmt"
	"testing"

	"github.com/dpopsuev/locus/internal/arch"
	"github.com/dpopsuev/locus/internal/graph"
	"github.com/dpopsuev/locus/internal/model"
	"github.com/dpopsuev/locus/internal/port"
	"github.com/dpopsuev/oculus"
)

func TestComputePatternScan_GodComponent(t *testing.T) {
	services := []arch.ArchService{
		{
			Name:    "pkg/monolith",
			Package: "example.com/pkg/monolith",
			LOC:     1500,
			Symbols: makeSymbols(35),
		},
	}

	// 10 fan-in edges, 8 fan-out edges.
	edges := make([]arch.ArchEdge, 0, 18)
	for i := range 10 {
		edges = append(edges, arch.ArchEdge{From: fmtPkg(i), To: "pkg/monolith"})
	}
	for i := range 8 {
		edges = append(edges, arch.ArchEdge{From: "pkg/monolith", To: fmtPkg(100 + i)})
	}

	report := ComputePatternScan(services, edges, nil, nil, nil, nil, nil)

	found := false
	for _, d := range report.Detections {
		if d.PatternID == "god_component" && d.Component == "pkg/monolith" {
			found = true
			if d.Kind != PatternKindSmell {
				t.Errorf("expected smell kind, got %s", d.Kind)
			}
			if d.Confidence < 0.5 {
				t.Errorf("expected confidence >= 0.5, got %f", d.Confidence)
			}
			if len(d.Evidence) == 0 {
				t.Error("expected non-empty evidence")
			}
		}
	}
	if !found {
		t.Fatal("god_component not detected for pkg/monolith")
	}
	if report.SmellsFound == 0 {
		t.Error("expected SmellsFound > 0")
	}
}

func TestComputePatternScan_CircularDependency(t *testing.T) {
	services := []arch.ArchService{
		{Name: "pkg/a", Package: "example.com/pkg/a"},
		{Name: "pkg/b", Package: "example.com/pkg/b"},
	}
	edges := []arch.ArchEdge{
		{From: "pkg/a", To: "pkg/b"},
		{From: "pkg/b", To: "pkg/a"},
	}
	cycles := []graph.Cycle{
		{"pkg/a", "pkg/b"},
	}

	report := ComputePatternScan(services, edges, cycles, nil, nil, nil, nil)

	foundA := false
	foundB := false
	for _, d := range report.Detections {
		if d.PatternID == "circular_dependency" {
			if d.Component == "pkg/a" {
				foundA = true
			}
			if d.Component == "pkg/b" {
				foundB = true
			}
			if d.Kind != PatternKindSmell {
				t.Errorf("expected smell kind, got %s", d.Kind)
			}
		}
	}
	if !foundA {
		t.Error("circular_dependency not detected for pkg/a")
	}
	if !foundB {
		t.Error("circular_dependency not detected for pkg/b")
	}
}

func TestComputePatternScan_LazyComponent(t *testing.T) {
	services := []arch.ArchService{
		{Name: "pkg/tiny", Package: "example.com/pkg/tiny", LOC: 10},
	}
	// No edges pointing to pkg/tiny → fan-in = 0.
	edges := []arch.ArchEdge{
		{From: "pkg/tiny", To: "pkg/other"},
	}

	report := ComputePatternScan(services, edges, nil, nil, nil, nil, nil)

	found := false
	for _, d := range report.Detections {
		if d.PatternID == "lazy_component" && d.Component == "pkg/tiny" {
			found = true
			if d.Kind != PatternKindSmell {
				t.Errorf("expected smell kind, got %s", d.Kind)
			}
		}
	}
	if !found {
		t.Fatal("lazy_component not detected for pkg/tiny")
	}
}

func TestComputePatternScan_Clean(t *testing.T) {
	services := []arch.ArchService{
		{Name: "pkg/core", Package: "example.com/pkg/core", LOC: 200, Symbols: makeSymbols(10), Churn: 3},
		{Name: "pkg/util", Package: "example.com/pkg/util", LOC: 150, Symbols: makeSymbols(5), Churn: 2},
	}
	edges := []arch.ArchEdge{
		{From: "pkg/util", To: "pkg/core", CallSites: 3},
		{From: "pkg/core", To: "pkg/util", CallSites: 2},
	}

	report := ComputePatternScan(services, edges, nil, nil, nil, nil, nil)

	for _, d := range report.Detections {
		if d.Kind == PatternKindSmell {
			// Allow low-confidence smells from bidirectional edges, but
			// god_component, shotgun_surgery, lazy_component should NOT fire.
			switch d.PatternID {
			case "god_component", "shotgun_surgery", "lazy_component":
				t.Errorf("unexpected smell %s detected for %s with confidence %f",
					d.PatternID, d.Component, d.Confidence)
			}
		}
	}
}

func TestComputePatternScan_SortOrder(t *testing.T) {
	// Create a service that triggers both a smell and a pattern.
	services := []arch.ArchService{
		{
			Name:    "pkg/big",
			Package: "example.com/pkg/big",
			LOC:     1500,
			Symbols: makeSymbols(35),
		},
	}
	edges := make([]arch.ArchEdge, 0, 18)
	for i := range 10 {
		edges = append(edges, arch.ArchEdge{From: fmtPkg(i), To: "pkg/big"})
	}
	for i := range 8 {
		edges = append(edges, arch.ArchEdge{From: "pkg/big", To: fmtPkg(100 + i)})
	}

	report := ComputePatternScan(services, edges, nil, nil, nil, nil, nil)

	if len(report.Detections) == 0 {
		t.Fatal("expected at least one detection")
	}

	// Verify smells come before patterns.
	seenPattern := false
	for _, d := range report.Detections {
		if d.Kind == PatternKindPattern {
			seenPattern = true
		}
		if d.Kind == PatternKindSmell && seenPattern {
			t.Error("smell appears after pattern in sorted detections")
		}
	}
}

func TestComputePatternScan_FeatureEnvy(t *testing.T) {
	services := []arch.ArchService{
		{Name: "pkg/envious", Package: "example.com/pkg/envious", LOC: 100},
	}
	edges := []arch.ArchEdge{
		{From: "pkg/envious", To: "pkg/target", CallSites: 20},
		{From: "pkg/envious", To: "pkg/other", CallSites: 2},
	}

	report := ComputePatternScan(services, edges, nil, nil, nil, nil, nil)

	found := false
	for _, d := range report.Detections {
		if d.PatternID == "feature_envy" && d.Component == "pkg/envious" {
			found = true
			if d.Confidence < thresholdFeatureEnvyPct {
				t.Errorf("expected confidence > %f, got %f", thresholdFeatureEnvyPct, d.Confidence)
			}
		}
	}
	if !found {
		t.Fatal("feature_envy not detected for pkg/envious")
	}
}

func TestComputePatternScan_Strategy(t *testing.T) {
	services := []arch.ArchService{
		{Name: "pkg/sorter", Package: "example.com/pkg/sorter", LOC: 80},
	}
	classes := []oculus.ClassInfo{
		{
			Name: "Sorter", Package: "example.com/pkg/sorter", Kind: "interface",
			Methods:  []oculus.MethodInfo{{Name: "Sort", Signature: "Sort([]int)", Exported: true}},
			Exported: true,
		},
		{Name: "QuickSort", Package: "example.com/pkg/sorter", Kind: "struct", Exported: true},
		{Name: "MergeSort", Package: "example.com/pkg/sorter", Kind: "struct", Exported: true},
	}
	impls := []oculus.ImplEdge{
		{From: "QuickSort", To: "Sorter", Kind: "implements"},
		{From: "MergeSort", To: "Sorter", Kind: "implements"},
	}

	report := ComputePatternScan(services, nil, nil, classes, impls, nil, nil)

	found := false
	for _, d := range report.Detections {
		if d.PatternID == "strategy" && d.Component == "pkg/sorter" {
			found = true
			if d.Kind != PatternKindPattern {
				t.Errorf("expected pattern kind, got %s", d.Kind)
			}
			if d.Severity != port.SeverityInfo {
				t.Errorf("expected info severity for pattern, got %s", d.Severity)
			}
		}
	}
	if !found {
		t.Fatal("strategy pattern not detected for pkg/sorter")
	}
}

func TestComputePatternScan_SeverityEscalation(t *testing.T) {
	// A god_component with very high metrics should get error severity.
	services := []arch.ArchService{
		{
			Name:    "pkg/mega",
			Package: "example.com/pkg/mega",
			LOC:     5000,
			Symbols: makeSymbols(100),
		},
	}
	edges := make([]arch.ArchEdge, 0, 40)
	for i := range 20 {
		edges = append(edges, arch.ArchEdge{From: fmtPkg(i), To: "pkg/mega"})
	}
	for i := range 20 {
		edges = append(edges, arch.ArchEdge{From: "pkg/mega", To: fmtPkg(200 + i)})
	}

	report := ComputePatternScan(services, edges, nil, nil, nil, nil, nil)

	for _, d := range report.Detections {
		if d.PatternID == "god_component" && d.Component == "pkg/mega" {
			if d.Severity != port.SeverityError {
				t.Errorf("expected error severity for high-confidence god_component, got %s (confidence=%f)",
					d.Severity, d.Confidence)
			}
			return
		}
	}
	t.Fatal("god_component not detected for pkg/mega")
}

func TestComputePatternScan_EmptyInput(t *testing.T) {
	report := ComputePatternScan(nil, nil, nil, nil, nil, nil, nil)

	if report.PatternsFound != 0 {
		t.Errorf("expected 0 patterns, got %d", report.PatternsFound)
	}
	if report.SmellsFound != 0 {
		t.Errorf("expected 0 smells, got %d", report.SmellsFound)
	}
	if report.Summary != "No patterns or smells detected" {
		t.Errorf("unexpected summary: %s", report.Summary)
	}
}

func TestGetPatternCatalog_All(t *testing.T) {
	report := GetPatternCatalog("")

	if len(report.Entries) != 25 {
		t.Fatalf("expected 25 entries, got %d", len(report.Entries))
	}
	if report.Summary != "25 catalog entries" {
		t.Errorf("unexpected summary: %s", report.Summary)
	}
}

func TestGetPatternCatalog_FilterSmells(t *testing.T) {
	report := GetPatternCatalog("smell")

	if len(report.Entries) != 13 {
		t.Fatalf("expected 13 smell entries, got %d", len(report.Entries))
	}
	for _, e := range report.Entries {
		if e.Kind != PatternKindSmell {
			t.Errorf("expected smell kind, got %s for %s", e.Kind, e.ID)
		}
	}
}

func TestGetPatternCatalog_FilterPatterns(t *testing.T) {
	report := GetPatternCatalog("pattern")

	if len(report.Entries) != 12 {
		t.Fatalf("expected 12 pattern entries, got %d", len(report.Entries))
	}
	for _, e := range report.Entries {
		if e.Kind != PatternKindPattern {
			t.Errorf("expected pattern kind, got %s for %s", e.Kind, e.ID)
		}
	}
}

func TestGetPatternCatalog_FilterByName(t *testing.T) {
	report := GetPatternCatalog("factory")

	if len(report.Entries) != 1 {
		t.Fatalf("expected 1 entry matching 'factory', got %d", len(report.Entries))
	}
	if report.Entries[0].ID != "factory" {
		t.Errorf("expected factory, got %s", report.Entries[0].ID)
	}
	if report.Summary != "1 entries matching 'factory'" {
		t.Errorf("unexpected summary: %s", report.Summary)
	}
}

func TestGetPatternCatalog_FilterByCategory(t *testing.T) {
	report := GetPatternCatalog("creational")

	// factory, builder, singleton are creational
	if len(report.Entries) != 3 {
		t.Fatalf("expected 3 creational entries, got %d", len(report.Entries))
	}
	for _, e := range report.Entries {
		if e.Category != "creational" {
			t.Errorf("expected creational category, got %s for %s", e.Category, e.ID)
		}
	}
}

func TestGetPatternCatalog_FilterCaseInsensitive(t *testing.T) {
	report := GetPatternCatalog("GOD")

	if len(report.Entries) != 1 {
		t.Fatalf("expected 1 entry matching 'GOD', got %d", len(report.Entries))
	}
	if report.Entries[0].ID != "god_component" {
		t.Errorf("expected god_component, got %s", report.Entries[0].ID)
	}
}

func TestGetPatternCatalog_SingleEntryHasSteps(t *testing.T) {
	report := GetPatternCatalog("god_component")

	if len(report.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(report.Entries))
	}
	entry := report.Entries[0]
	if len(entry.Steps) == 0 {
		t.Error("expected Steps to be populated for single exact-match entry")
	}
	if len(entry.Examples) == 0 {
		t.Error("expected Examples to be populated for single exact-match entry")
	}
}

func TestGetPatternCatalog_MultiEntryStripsVerbose(t *testing.T) {
	report := GetPatternCatalog("smell")

	for _, e := range report.Entries {
		if len(e.Steps) != 0 {
			t.Errorf("expected Steps to be nil for multi-entry result, got %d steps on %s", len(e.Steps), e.ID)
		}
		if len(e.Examples) != 0 {
			t.Errorf("expected Examples to be nil for multi-entry result, got %d examples on %s", len(e.Examples), e.ID)
		}
	}
}

func TestGetPatternCatalog_AllSmellsHaveSteps(t *testing.T) {
	// Verify every smell in the catalog has remediation steps defined.
	for _, e := range patternCatalog {
		if e.Kind != PatternKindSmell {
			continue
		}
		if len(e.Steps) == 0 {
			t.Errorf("smell %q missing Steps", e.ID)
		}
	}
}

func TestCoverageGap(t *testing.T) {
	// Component with fan-in > 3 but no edges from test packages → coverage_gap.
	services := []arch.ArchService{
		{Name: "pkg/core", Package: "example.com/pkg/core", LOC: 200, Symbols: makeSymbols(10)},
	}
	edges := make([]arch.ArchEdge, 0, 5)
	for i := range 5 {
		edges = append(edges, arch.ArchEdge{From: fmtPkg(i), To: "pkg/core"})
	}

	report := ComputePatternScan(services, edges, nil, nil, nil, nil, nil)

	found := false
	for _, d := range report.Detections {
		if d.PatternID == "coverage_gap" && d.Component == "pkg/core" {
			found = true
			if d.Kind != PatternKindSmell {
				t.Errorf("expected smell kind, got %s", d.Kind)
			}
			if d.Confidence < 0.6 {
				t.Errorf("expected confidence >= 0.6, got %f", d.Confidence)
			}
			if len(d.Evidence) == 0 {
				t.Error("expected non-empty evidence")
			}
		}
	}
	if !found {
		t.Fatal("coverage_gap not detected for pkg/core")
	}
}

func TestCoverageGap_WithIntegration(t *testing.T) {
	// Component imported from a test package → no coverage gap.
	services := []arch.ArchService{
		{Name: "pkg/core", Package: "example.com/pkg/core", LOC: 200, Symbols: makeSymbols(10)},
	}
	edges := []arch.ArchEdge{
		{From: "pkg/dep1", To: "pkg/core"},
		{From: "pkg/dep2", To: "pkg/core"},
		{From: "pkg/dep3", To: "pkg/core"},
		{From: "pkg/dep4", To: "pkg/core"},
		{From: "acceptance/smoke_test", To: "pkg/core"},
	}

	report := ComputePatternScan(services, edges, nil, nil, nil, nil, nil)

	for _, d := range report.Detections {
		if d.PatternID == "coverage_gap" && d.Component == "pkg/core" {
			t.Error("coverage_gap should NOT be detected when acceptance test imports the component")
		}
	}
}

func TestFragileContract(t *testing.T) {
	// Component with fan-in > 5, stateful type, but no New* constructor → fragile_contract.
	services := []arch.ArchService{
		{Name: "pkg/config", Package: "example.com/pkg/config", LOC: 300, Symbols: model.SymbolsFromNames("Load", "Save", "Validate", "Parse")},
	}
	edges := make([]arch.ArchEdge, 0, 8)
	for i := range 8 {
		edges = append(edges, arch.ArchEdge{From: fmtPkg(i), To: "pkg/config"})
	}
	classes := []oculus.ClassInfo{
		{Package: "example.com/pkg/config", Name: "Config", Kind: "struct", Fields: []oculus.FieldInfo{{Name: "path", Type: "string"}}},
	}

	report := ComputePatternScan(services, edges, nil, classes, nil, nil, nil)

	found := false
	for _, d := range report.Detections {
		if d.PatternID == "fragile_contract" && d.Component == "pkg/config" {
			found = true
			if d.Kind != PatternKindSmell {
				t.Errorf("expected smell kind, got %s", d.Kind)
			}
			if d.Confidence < 0.6 {
				t.Errorf("expected confidence >= 0.6, got %f", d.Confidence)
			}
			if len(d.Evidence) == 0 {
				t.Error("expected non-empty evidence")
			}
		}
	}
	if !found {
		t.Fatal("fragile_contract not detected for pkg/config")
	}
}

func TestFragileContract_WithConstructor(t *testing.T) {
	// Component with fan-in > 5 AND a New* constructor → not fragile.
	services := []arch.ArchService{
		{Name: "pkg/config", Package: "example.com/pkg/config", LOC: 300, Symbols: model.SymbolsFromNames("NewConfig", "Load", "Save", "Validate")},
	}
	edges := make([]arch.ArchEdge, 0, 8)
	for i := range 8 {
		edges = append(edges, arch.ArchEdge{From: fmtPkg(i), To: "pkg/config"})
	}

	report := ComputePatternScan(services, edges, nil, nil, nil, nil, nil)

	for _, d := range report.Detections {
		if d.PatternID == "fragile_contract" && d.Component == "pkg/config" {
			t.Error("fragile_contract should NOT be detected when component has New* constructor")
		}
	}
}

func TestStateMachineCandidate(t *testing.T) {
	// Component with a struct containing a state-like field + enough methods → detected.
	services := []arch.ArchService{
		{
			Name:    "pkg/workflow",
			Package: "example.com/pkg/workflow",
			LOC:     200,
			Symbols: makeSymbols(8), // 8 symbols > threshold 5
		},
	}
	classes := []oculus.ClassInfo{
		{
			Name:    "Workflow",
			Package: "example.com/pkg/workflow",
			Kind:    "struct",
			Fields: []oculus.FieldInfo{
				{Name: "State", Type: "WorkflowState", Exported: true},
				{Name: "Name", Type: "string", Exported: true},
			},
			Exported: true,
		},
	}

	report := ComputePatternScan(services, nil, nil, classes, nil, nil, nil)

	found := false
	for _, d := range report.Detections {
		if d.PatternID == "state_machine_candidate" && d.Component == "pkg/workflow" {
			found = true
			if d.Kind != PatternKindPattern {
				t.Errorf("expected pattern kind, got %s", d.Kind)
			}
			if d.Severity != port.SeverityInfo {
				t.Errorf("expected info severity for pattern, got %s", d.Severity)
			}
			if len(d.Evidence) == 0 {
				t.Error("expected non-empty evidence")
			}
		}
	}
	if !found {
		t.Fatal("state_machine_candidate not detected for pkg/workflow")
	}
}

func TestStateMachineCandidate_NoStateField(t *testing.T) {
	// Component with methods but no state-like field → NOT detected.
	services := []arch.ArchService{
		{
			Name:    "pkg/util",
			Package: "example.com/pkg/util",
			LOC:     200,
			Symbols: makeSymbols(10),
		},
	}
	classes := []oculus.ClassInfo{
		{
			Name:    "Helper",
			Package: "example.com/pkg/util",
			Kind:    "struct",
			Fields: []oculus.FieldInfo{
				{Name: "Name", Type: "string", Exported: true},
				{Name: "Value", Type: "int", Exported: true},
			},
			Exported: true,
		},
	}

	report := ComputePatternScan(services, nil, nil, classes, nil, nil, nil)

	for _, d := range report.Detections {
		if d.PatternID == "state_machine_candidate" && d.Component == "pkg/util" {
			t.Error("state_machine_candidate should NOT be detected without state-like field")
		}
	}
}

func TestMissingPattern(t *testing.T) {
	// High churn component with no patterns detected → missing_pattern smell emitted.
	services := []arch.ArchService{
		{
			Name:    "pkg/churn",
			Package: "example.com/pkg/churn",
			LOC:     300,
			Churn:   15,
			Symbols: makeSymbols(10),
		},
	}

	report := ComputePatternScan(services, nil, nil, nil, nil, nil, nil)

	found := false
	for _, d := range report.Detections {
		if d.PatternID == "missing_pattern" && d.Component == "pkg/churn" {
			found = true
			if d.Kind != PatternKindSmell {
				t.Errorf("expected smell kind, got %s", d.Kind)
			}
			if len(d.Evidence) < 2 {
				t.Errorf("expected at least 2 evidence items, got %d", len(d.Evidence))
			}
		}
	}
	if !found {
		t.Fatal("missing_pattern not detected for high-churn component with no patterns")
	}
}

func TestMissingPattern_WithPattern(t *testing.T) {
	// High churn but Strategy pattern detected → no missing_pattern.
	services := []arch.ArchService{
		{
			Name:    "pkg/sorter",
			Package: "example.com/pkg/sorter",
			LOC:     200,
			Churn:   15,
		},
	}
	classes := []oculus.ClassInfo{
		{
			Name: "Sorter", Package: "example.com/pkg/sorter", Kind: "interface",
			Methods:  []oculus.MethodInfo{{Name: "Sort", Signature: "Sort([]int)", Exported: true}},
			Exported: true,
		},
		{Name: "QuickSort", Package: "example.com/pkg/sorter", Kind: "struct", Exported: true},
		{Name: "MergeSort", Package: "example.com/pkg/sorter", Kind: "struct", Exported: true},
	}
	impls := []oculus.ImplEdge{
		{From: "QuickSort", To: "Sorter", Kind: "implements"},
		{From: "MergeSort", To: "Sorter", Kind: "implements"},
	}

	report := ComputePatternScan(services, nil, nil, classes, impls, nil, nil)

	// Strategy should be detected.
	strategyFound := false
	for _, d := range report.Detections {
		if d.PatternID == "strategy" && d.Component == "pkg/sorter" {
			strategyFound = true
		}
		if d.PatternID == "missing_pattern" && d.Component == "pkg/sorter" {
			t.Error("missing_pattern should NOT be detected when Strategy pattern is present")
		}
	}
	if !strategyFound {
		t.Fatal("strategy pattern should be detected for pkg/sorter")
	}
}

// ── Helpers ──

func makeSymbols(n int) []model.Symbol {
	syms := make([]model.Symbol, n)
	for i := range n {
		syms[i] = model.Symbol{Name: fmt.Sprintf("Symbol%d", i), Kind: model.SymbolFunction, Exported: true}
	}
	return syms
}

func fmtPkg(i int) string {
	return fmt.Sprintf("pkg/dep%d", i)
}
