package lint_test

import (
	"testing"

	"github.com/dpopsuev/locus/internal/arch"
	"github.com/dpopsuev/locus/internal/lint"
	"github.com/dpopsuev/locus/internal/port"
)

// testdataRoot is the path to the Go testkit fixture relative to this file.
const testdataRoot = "../../testdata/testkit/go"

func scanFixture(t *testing.T, root string) *arch.ContextReport { //nolint:unparam // keep param for future multi-fixture tests
	t.Helper()
	report, err := arch.ScanAndBuild(root, arch.ScanOpts{
		Intent:          arch.IntentHealth,
		ExcludeTests:    true,
		IncludeExternal: false,
	})
	if err != nil {
		t.Fatalf("ScanAndBuild(%s): %v", root, err)
	}
	return report
}

func TestRun_Fixture_ReturnsNonNilReport(t *testing.T) {
	report := scanFixture(t, testdataRoot)
	result := lint.Run(report, lint.RunOpts{Root: testdataRoot})

	if result == nil {
		t.Fatal("Run returned nil report")
	}
}

func TestRun_Fixture_ScoreInRange(t *testing.T) {
	report := scanFixture(t, testdataRoot)
	result := lint.Run(report, lint.RunOpts{Root: testdataRoot})

	if result.Score < 0 || result.Score > 100 {
		t.Errorf("Score out of range: got %f, want [0, 100]", result.Score)
	}
}

func TestRun_Fixture_CleanMatchesViolations(t *testing.T) {
	report := scanFixture(t, testdataRoot)
	result := lint.Run(report, lint.RunOpts{Root: testdataRoot})

	wantClean := len(result.Violations) == 0
	if result.Clean != wantClean {
		t.Errorf("Clean=%v but len(Violations)=%d", result.Clean, len(result.Violations))
	}
}

func TestRun_Fixture_ByCategoryMatchesViolations(t *testing.T) {
	report := scanFixture(t, testdataRoot)
	result := lint.Run(report, lint.RunOpts{Root: testdataRoot})

	totalFromCategories := 0
	for _, n := range result.ByCategory {
		totalFromCategories += n
	}
	if totalFromCategories != len(result.Violations) {
		t.Errorf("ByCategory sum=%d != len(Violations)=%d", totalFromCategories, len(result.Violations))
	}
}

func TestRun_Fixture_SummaryNonEmpty(t *testing.T) {
	report := scanFixture(t, testdataRoot)
	result := lint.Run(report, lint.RunOpts{Root: testdataRoot})

	if result.Summary == "" {
		t.Error("Summary is empty")
	}
}

func TestRun_DefaultLinters(t *testing.T) {
	report := scanFixture(t, testdataRoot)
	result := lint.Run(report, lint.RunOpts{Root: testdataRoot})

	// Default linters should not include layer/budget (they require config).
	for _, v := range result.Violations {
		if v.Category == lint.CategoryLayer || v.Category == lint.CategoryBudget {
			t.Errorf("Default run should not include %s violations without DesiredState config", v.Category)
		}
	}
}

func TestRun_EnabledLinters_SingleCategory(t *testing.T) {
	report := scanFixture(t, testdataRoot)
	result := lint.Run(report, lint.RunOpts{
		Root:           testdataRoot,
		EnabledLinters: []lint.Category{lint.CategoryHexa},
	})

	for _, v := range result.Violations {
		if v.Category != lint.CategoryHexa {
			t.Errorf("Expected only hexa violations, got %s", v.Category)
		}
	}
}

func TestRun_ChangedComponents_FiltersOutput(t *testing.T) {
	report := scanFixture(t, testdataRoot)

	// Run with no filter to find all violations.
	full := lint.Run(report, lint.RunOpts{Root: testdataRoot})

	if len(full.Violations) == 0 {
		t.Skip("No violations in fixture; cannot test filtering")
	}

	// Pick a component that has violations.
	target := full.Violations[0].Component

	// Run with ChangedComponents filter.
	filtered := lint.Run(report, lint.RunOpts{
		Root:              testdataRoot,
		ChangedComponents: []string{target},
	})

	for _, v := range filtered.Violations {
		if v.Component != target {
			t.Errorf("Filtered result contains violation for %q, expected only %q", v.Component, target)
		}
	}

	// Filtered should have <= full violations.
	if len(filtered.Violations) > len(full.Violations) {
		t.Errorf("Filtered violations (%d) > full violations (%d)", len(filtered.Violations), len(full.Violations))
	}
}

func TestRun_ChangedComponents_NonexistentComponent(t *testing.T) {
	report := scanFixture(t, testdataRoot)
	result := lint.Run(report, lint.RunOpts{
		Root:              testdataRoot,
		ChangedComponents: []string{"nonexistent/package/that/does/not/exist"},
	})

	if len(result.Violations) != 0 {
		t.Errorf("Expected 0 violations for nonexistent component, got %d", len(result.Violations))
	}
	if !result.Clean {
		t.Error("Expected Clean=true for nonexistent component")
	}
}

func TestRun_WithDesiredState_BudgetViolation(t *testing.T) {
	report := scanFixture(t, testdataRoot)

	if len(report.Architecture.Services) == 0 {
		t.Skip("No services in fixture")
	}

	// Set an impossibly low budget so it triggers.
	target := report.Architecture.Services[0].Name
	ds := &port.DesiredState{
		Constraints: []port.HealthConstraint{
			{Component: target, MaxChurn: 0},
		},
	}

	result := lint.Run(report, lint.RunOpts{
		Root:           testdataRoot,
		EnabledLinters: []lint.Category{lint.CategoryBudget},
		DesiredState:   ds,
	})

	// If the component has any churn, we should see a budget violation.
	for _, v := range result.Violations {
		if v.Category != lint.CategoryBudget {
			t.Errorf("Expected only budget violations, got %s", v.Category)
		}
	}
}

func TestRun_ViolationSeverityValues(t *testing.T) {
	report := scanFixture(t, testdataRoot)
	result := lint.Run(report, lint.RunOpts{Root: testdataRoot})

	validSeverities := map[string]bool{
		"info": true, "warning": true, "error": true, "critical": true,
	}

	for _, v := range result.Violations {
		if !validSeverities[v.Severity] {
			t.Errorf("Invalid severity %q for violation in %s/%s", v.Severity, v.Category, v.Component)
		}
	}
}

func TestRun_ViolationsSortedBySeverity(t *testing.T) {
	report := scanFixture(t, testdataRoot)
	result := lint.Run(report, lint.RunOpts{Root: testdataRoot})

	if len(result.Violations) < 2 {
		t.Skip("Need at least 2 violations to test sort order")
	}

	sevRank := map[string]int{"critical": 0, "error": 1, "warning": 2, "info": 3}
	for i := 1; i < len(result.Violations); i++ {
		prev := sevRank[result.Violations[i-1].Severity]
		curr := sevRank[result.Violations[i].Severity]
		if prev > curr {
			t.Errorf("Violations not sorted by severity: [%d]=%s > [%d]=%s",
				i-1, result.Violations[i-1].Severity, i, result.Violations[i].Severity)
			break
		}
	}
}

func TestRun_EmptyReport(t *testing.T) {
	// Minimal ContextReport with no services.
	report := &arch.ContextReport{
		Architecture: arch.ArchModel{},
	}

	result := lint.Run(report, lint.RunOpts{})
	if result == nil {
		t.Fatal("Run returned nil for empty report")
	}
	if !result.Clean {
		t.Error("Expected Clean=true for empty architecture")
	}
	if result.Score != 100 {
		t.Errorf("Expected score 100 for empty architecture, got %f", result.Score)
	}
}

func TestRun_NilDesiredState(t *testing.T) {
	report := scanFixture(t, testdataRoot)
	// Should not panic with nil DesiredState.
	result := lint.Run(report, lint.RunOpts{
		Root:         testdataRoot,
		DesiredState: nil,
	})
	if result == nil {
		t.Fatal("Run returned nil with nil DesiredState")
	}
}
