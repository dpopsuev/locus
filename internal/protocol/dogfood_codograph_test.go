package protocol

import (
	"context"
	"testing"

	"github.com/dpopsuev/locus/internal/port"
)

func TestDogfood_ScanProject(t *testing.T) {
	if testing.Short() {
		t.Skip("dogfood: skipping expensive self-scan in -short mode")
	}
	p, path := setupBurnTest(t)
	ctx := context.Background()

	result, err := p.ScanProject(ctx, path, ScanOpts{})
	if err != nil {
		t.Fatalf("ScanProject: %v", err)
	}
	if result.Report == nil {
		t.Fatal("expected non-nil report")
	}
	if len(result.Report.Architecture.Services) == 0 {
		t.Error("expected services > 0")
	}
	if result.CacheKey == "" {
		t.Error("expected non-empty cache key")
	}
	if result.SHA == "" {
		t.Error("expected non-empty SHA")
	}
	t.Logf("scan: %d components, cache_key=%s, sha=%s",
		len(result.Report.Architecture.Services), result.CacheKey, result.SHA[:8])
}

func TestDogfood_History(t *testing.T) {
	if testing.Short() {
		t.Skip("dogfood: skipping expensive self-scan in -short mode")
	}
	p, path := setupBurnTest(t)
	ctx := context.Background()

	entries, err := p.GetHistory(ctx, path, 5)
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	if len(entries) == 0 {
		t.Error("expected at least 1 history entry after scan")
	}
	t.Logf("history: %d entries", len(entries))
}

func TestDogfood_Status(t *testing.T) {
	if testing.Short() {
		t.Skip("dogfood: skipping expensive self-scan in -short mode")
	}
	p, _ := setupBurnTest(t)
	ctx := context.Background()

	result, err := p.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if result.Workspaces == nil {
		t.Error("expected non-nil workspaces")
	}
	if len(result.Workspaces) == 0 {
		t.Error("expected at least 1 workspace")
	}
	t.Logf("status: %d workspaces", len(result.Workspaces))
}

func TestDogfood_DesiredState_RoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("dogfood: skipping expensive self-scan in -short mode")
	}
	p, path := setupBurnTest(t)
	ctx := context.Background()

	// Set desired state.
	ds := &port.DesiredState{
		Layers: []string{"model", "graph", "arch", "protocol", "mcp"},
	}
	if err := p.SetDesiredState(ctx, path, ds); err != nil {
		t.Fatalf("SetDesiredState: %v", err)
	}

	// Get and verify round-trip.
	got, err := p.GetDesiredState(ctx, path)
	if err != nil {
		t.Fatalf("GetDesiredState: %v", err)
	}
	if len(got.Layers) != 5 {
		t.Errorf("expected 5 layers, got %d", len(got.Layers))
	}

	// Accept a violation.
	if err := p.AcceptViolation(ctx, path, port.AcceptedViolation{
		Component: "internal/cli",
		Principle: "DIP",
		Reason:    "dogfood test",
	}); err != nil {
		t.Fatalf("AcceptViolation: %v", err)
	}

	// Verify accepted violation persisted.
	got2, err := p.GetDesiredState(ctx, path)
	if err != nil {
		t.Fatalf("GetDesiredState after accept: %v", err)
	}
	if len(got2.Accepted) == 0 {
		t.Error("expected accepted violation to be persisted")
	}
	t.Logf("desired state: %d layers, %d accepted", len(got2.Layers), len(got2.Accepted))
}

func TestDogfood_SuggestArchitecture(t *testing.T) {
	if testing.Short() {
		t.Skip("dogfood: skipping expensive self-scan in -short mode")
	}
	p, path := setupBurnTest(t)
	ctx := context.Background()

	result, err := p.SuggestArchitecture(ctx, path)
	if err != nil {
		t.Fatalf("SuggestArchitecture: %v", err)
	}
	if len(result.Layers) == 0 {
		t.Error("expected suggested layers > 0")
	}
	t.Logf("suggested architecture: %d layers", len(result.Layers))
}
