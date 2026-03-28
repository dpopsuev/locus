package protocol

import (
	"testing"

	"github.com/dpopsuev/locus/internal/arch"
)

func TestComputeImportDirection_ExcludesEntrypoints(t *testing.T) {
	// BUG-14: cmd/ at depth 0 should NOT be flagged as a violation.
	// Composition roots import everything by design.
	edges := []arch.ArchEdge{
		{From: "cmd/app", To: "internal/protocol"},
		{From: "cmd/app", To: "internal/store"},
		{From: "internal/protocol", To: "internal/store"},
	}
	depths := arch.DepthMap{
		"cmd/app":           0,
		"internal/protocol": 2,
		"internal/store":    3,
	}

	report := ComputeImportDirection(edges, depths)

	// cmd/app edges should be excluded (depth 0 = entrypoint)
	for _, v := range report.Violations {
		if v.From == "cmd/app" {
			t.Errorf("entrypoint cmd/app should be excluded from violations, got: %s → %s", v.From, v.To)
		}
	}

	// internal/protocol → internal/store (depth 2 → 3) is still a valid violation
	found := false
	for _, v := range report.Violations {
		if v.From == "internal/protocol" && v.To == "internal/store" {
			found = true
		}
	}
	if !found {
		t.Error("expected violation internal/protocol → internal/store (depth 2 → 3)")
	}
}

func TestComputeImportDirection_SkipsCycleNodes(t *testing.T) {
	edges := []arch.ArchEdge{
		{From: "a", To: "b"},
	}
	depths := arch.DepthMap{"a": -1, "b": 2}

	report := ComputeImportDirection(edges, depths)
	if len(report.Violations) != 0 {
		t.Errorf("cycle nodes (depth -1) should be skipped, got %d violations", len(report.Violations))
	}
}

func TestComputeImportDirection_CleanArchitecture(t *testing.T) {
	// High depth imports low depth = correct direction
	edges := []arch.ArchEdge{
		{From: "adapter/http", To: "domain/user"},
		{From: "adapter/db", To: "domain/user"},
	}
	depths := arch.DepthMap{
		"adapter/http": 5,
		"adapter/db":   5,
		"domain/user":  2,
	}

	report := ComputeImportDirection(edges, depths)
	if len(report.Violations) != 0 {
		t.Errorf("correct direction (high→low) should produce 0 violations, got %d", len(report.Violations))
	}
}

func TestComputeImportDirection_SeverityLevels(t *testing.T) {
	edges := []arch.ArchEdge{
		{From: "domain", To: "adapter"},      // depth diff = 1 → warning
		{From: "domain", To: "infra/detail"}, // depth diff = 3 → error
	}
	depths := arch.DepthMap{
		"domain":       2,
		"adapter":      3,
		"infra/detail": 5,
	}

	report := ComputeImportDirection(edges, depths)

	warnings, errors := 0, 0
	for _, v := range report.Violations {
		switch v.Severity {
		case "warning":
			warnings++
		case "error":
			errors++
		}
	}

	if warnings != 1 {
		t.Errorf("expected 1 warning, got %d", warnings)
	}
	if errors != 1 {
		t.Errorf("expected 1 error, got %d", errors)
	}
}

func TestComputeImportDirection_NoEdges(t *testing.T) {
	report := ComputeImportDirection(nil, nil)
	if len(report.Violations) != 0 {
		t.Errorf("expected 0 violations for nil edges, got %d", len(report.Violations))
	}
	if report.Summary != "Clean: no import direction violations" {
		t.Errorf("unexpected summary: %s", report.Summary)
	}
}
