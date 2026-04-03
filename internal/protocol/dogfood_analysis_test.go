package protocol

import (
	"context"
	"strings"
	"testing"
)

func TestDogfood_GetDependencies(t *testing.T) {
	if testing.Short() {
		t.Skip("dogfood: skipping expensive self-scan in -short mode")
	}
	p, path := setupBurnTest(t)
	ctx := context.Background()

	result, err := p.GetDependencies(ctx, path, "internal/graph")
	if err != nil {
		t.Fatalf("GetDependencies: %v", err)
	}
	if result.Component != "internal/graph" {
		t.Errorf("expected component internal/graph, got %s", result.Component)
	}
	if len(result.FanIn) < 3 {
		t.Errorf("expected internal/graph fan-in >= 3, got %d", len(result.FanIn))
	}
	if len(result.FanOut) == 0 {
		t.Log("internal/graph has 0 fan-out (pure leaf) — expected for algorithm package")
	}
	t.Logf("internal/graph: fan_in=%d, fan_out=%d", len(result.FanIn), len(result.FanOut))
}

func TestDogfood_GetImpact(t *testing.T) {
	if testing.Short() {
		t.Skip("dogfood: skipping expensive self-scan in -short mode")
	}
	p, path := setupBurnTest(t)
	ctx := context.Background()

	result, err := p.GetImpact(ctx, path, "internal/graph")
	if err != nil {
		t.Fatalf("GetImpact: %v", err)
	}
	if result.Component != "internal/graph" {
		t.Errorf("expected component internal/graph, got %s", result.Component)
	}
	if len(result.DirectDeps) < 3 {
		t.Errorf("expected >= 3 direct dependents for graph, got %d", len(result.DirectDeps))
	}
	if result.BlastRadius == 0 {
		t.Error("expected non-zero blast radius for internal/graph")
	}
	if result.RiskLevel == "" {
		t.Error("expected non-empty risk level")
	}
	t.Logf("internal/graph impact: direct=%d, transitive=%d, blast=%d%%, risk=%s",
		len(result.DirectDeps), len(result.TransDeps), result.BlastRadius, result.RiskLevel)
}

func TestDogfood_GetCouplingTable(t *testing.T) {
	if testing.Short() {
		t.Skip("dogfood: skipping expensive self-scan in -short mode")
	}
	p, path := setupBurnTest(t)
	ctx := context.Background()

	result, err := p.GetCouplingTable(ctx, path, "fan_in", 5)
	if err != nil {
		t.Fatalf("GetCouplingTable: %v", err)
	}
	if result == "" {
		t.Error("expected non-empty coupling table")
	}
	if !strings.Contains(result, "internal/graph") && !strings.Contains(result, "internal/arch") {
		t.Error("expected coupling table to contain high fan-in component (graph or arch)")
	}
	t.Logf("coupling table (top 5): %d chars", len(result))
}

func TestDogfood_GetCycles(t *testing.T) {
	if testing.Short() {
		t.Skip("dogfood: skipping expensive self-scan in -short mode")
	}
	p, path := setupBurnTest(t)
	ctx := context.Background()

	result, err := p.GetCycles(ctx, path, nil)
	if err != nil {
		t.Fatalf("GetCycles: %v", err)
	}
	if len(result.Cycles) != 0 {
		t.Errorf("expected 0 cycles in Locus, got %d", len(result.Cycles))
		for i, c := range result.Cycles {
			t.Logf("  cycle %d: %v", i+1, c)
		}
	}
	if len(result.ImportDepth) == 0 {
		t.Error("expected non-empty import depth map")
	}
	t.Logf("cycles: %d, import depth entries: %d", len(result.Cycles), len(result.ImportDepth))
}

func TestDogfood_GetViolations(t *testing.T) {
	if testing.Short() {
		t.Skip("dogfood: skipping expensive self-scan in -short mode")
	}
	p, path := setupBurnTest(t)
	ctx := context.Background()

	result, err := p.GetViolations(ctx, path, nil)
	if err != nil {
		t.Fatalf("GetViolations: %v", err)
	}
	if len(result.Layers) == 0 {
		t.Error("expected auto-detected layers")
	}
	if result.Summary == "" {
		t.Error("expected non-empty summary")
	}
	t.Logf("violations: %d layers, %d violations, summary: %s",
		len(result.Layers), len(result.Violations), result.Summary)
}
