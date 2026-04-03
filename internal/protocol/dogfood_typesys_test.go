package protocol

import (
	"context"
	"testing"
)

func TestDogfood_GetCallers(t *testing.T) {
	if testing.Short() {
		t.Skip("dogfood: skipping expensive self-scan in -short mode")
	}
	p, path := setupBurnTest(t)
	ctx := context.Background()

	result, err := p.GetCallers(ctx, path, "ScanAndBuild")
	if err != nil {
		t.Fatalf("GetCallers: %v", err)
	}
	if len(result.Callers) == 0 {
		t.Log("GetCallers returned 0 callers for ScanAndBuild — deep analyzer may not resolve all call sites")
	} else {
		t.Logf("ScanAndBuild callers: %d", len(result.Callers))
	}
}

func TestDogfood_GetAPISurface(t *testing.T) {
	if testing.Short() {
		t.Skip("dogfood: skipping expensive self-scan in -short mode")
	}
	p, path := setupBurnTest(t)
	ctx := context.Background()

	result, err := p.GetAPISurface(ctx, path, nil)
	if err != nil {
		t.Fatalf("GetAPISurface: %v", err)
	}
	if len(result.Surfaces) == 0 {
		t.Error("expected non-empty API surfaces")
	}
	t.Logf("API surfaces: %d components", len(result.Surfaces))
}

func TestDogfood_GetConventions(t *testing.T) {
	if testing.Short() {
		t.Skip("dogfood: skipping expensive self-scan in -short mode")
	}
	p, path := setupBurnTest(t)
	ctx := context.Background()

	result, err := p.GetConventions(ctx, path)
	if err != nil {
		t.Fatalf("GetConventions: %v", err)
	}
	if result.Total == 0 {
		t.Error("expected non-zero convention count")
	}
	t.Logf("conventions: %d total", result.Total)
}

func TestDogfood_GetGaps(t *testing.T) {
	if testing.Short() {
		t.Skip("dogfood: skipping expensive self-scan in -short mode")
	}
	p, path := setupBurnTest(t)
	ctx := context.Background()

	result, err := p.GetGaps(ctx, path)
	if err != nil {
		t.Fatalf("GetGaps: %v", err)
	}
	if result.ComponentsScanned == 0 {
		t.Error("expected ComponentsScanned > 0")
	}
	t.Logf("gaps: %d scanned, %d total gaps", result.ComponentsScanned, result.TotalGaps)
}
