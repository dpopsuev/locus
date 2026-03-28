package protocol

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/dpopsuev/locus/internal/cache"
	"github.com/dpopsuev/locus/internal/store"
)

// tokenCount approximates the token count of a string (chars / 4).
func tokenCount(s string) int {
	return len(s) / 4
}

func setupBurnTest(t *testing.T) (proto *Protocol, repoPath string) {
	t.Helper()
	repoPath, err := filepath.Abs("../..")
	if err != nil {
		t.Skip("cannot resolve repo path")
	}
	if _, err := os.Stat(filepath.Join(repoPath, ".git")); err != nil {
		t.Skip("not in a git repo")
	}

	s := store.NewFilesystem(cache.New(filepath.Join(t.TempDir(), "cache")), t.TempDir())
	p := New(store.NewLRU(s, store.DefaultLRUCapacity), []string{repoPath})

	// Warm the cache with a scan.
	_, err = p.ScanProject(context.Background(), repoPath, ScanOpts{ChurnDays: 7})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	return p, repoPath
}

func TestTokenBurn_ScanSummary(t *testing.T) {
	p, path := setupBurnTest(t)
	result, err := p.ScanProject(context.Background(), path, ScanOpts{})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	summary := RenderScanSummary(result, "")
	tokens := tokenCount(summary)
	t.Logf("Scan summary: %d tokens (%d chars)", tokens, len(summary))
	if tokens > 100 {
		t.Errorf("scan summary too large: %d tokens (target <100)", tokens)
	}
}

func TestTokenBurn_Coupling(t *testing.T) {
	p, path := setupBurnTest(t)
	result, err := p.GetCouplingTable(context.Background(), path, "fan_in", 5)
	if err != nil {
		t.Fatalf("coupling: %v", err)
	}
	tokens := tokenCount(result)
	t.Logf("Coupling top-5: %d tokens (%d chars)", tokens, len(result))
	if tokens > 500 {
		t.Errorf("coupling too large: %d tokens (target <500)", tokens)
	}
}

func TestTokenBurn_Violations(t *testing.T) {
	p, path := setupBurnTest(t)
	layers := []string{"model", "survey", "arch", "analysis", "protocol", "mcp"}
	result, err := p.GetViolations(context.Background(), path, layers)
	if err != nil {
		t.Fatalf("violations: %v", err)
	}
	tokens := tokenCount(result.Summary)
	t.Logf("Violations summary: %d tokens (%d chars)", tokens, len(result.Summary))
	if tokens > 150 {
		t.Errorf("violations summary too large: %d tokens (target <150)", tokens)
	}
}

func TestTokenBurn_Cycles(t *testing.T) {
	p, path := setupBurnTest(t)
	result, err := p.GetCycles(context.Background(), path, nil)
	if err != nil {
		t.Fatalf("cycles: %v", err)
	}
	summary := fmt.Sprintf("%d cycles, %d violations", len(result.Cycles), len(result.LayerViolations))
	tokens := tokenCount(summary)
	t.Logf("Cycles summary: %d tokens (%d chars)", tokens, len(summary))
	if tokens > 300 {
		t.Errorf("cycles summary too large: %d tokens (target <300)", tokens)
	}
}

func TestTokenBurn_Preset(t *testing.T) {
	p, path := setupBurnTest(t)
	result, err := p.RunPreset(context.Background(), path, PresetArchReview)
	if err != nil {
		t.Fatalf("preset: %v", err)
	}
	tokens := tokenCount(result)
	t.Logf("architecture_review preset: %d tokens (%d chars)", tokens, len(result))
	if tokens > 500 {
		t.Errorf("preset too large: %d tokens (target <500)", tokens)
	}
}

func TestTokenBurn_DensityTable(t *testing.T) {
	p, path := setupBurnTest(t)

	type measurement struct {
		name   string
		tokens int
	}
	var results []measurement

	// Scan summary
	scan, _ := p.ScanProject(context.Background(), path, ScanOpts{})
	results = append(results, measurement{"scan_summary", tokenCount(RenderScanSummary(scan, ""))})

	// Coupling
	coupling, _ := p.GetCouplingTable(context.Background(), path, "fan_in", 5)
	results = append(results, measurement{"coupling_top5", tokenCount(coupling)})

	// Violations
	violations, _ := p.GetViolations(context.Background(), path, []string{"model", "survey", "arch", "analysis", "protocol", "mcp"})
	results = append(results, measurement{"violations", tokenCount(violations.Summary)})

	// Preset
	preset, _ := p.RunPreset(context.Background(), path, PresetArchReview)
	results = append(results, measurement{"arch_review_preset", tokenCount(preset)})

	// Hot spots
	spots, _ := p.GetHotSpots(context.Background(), path, 7, 5)
	spotStr := ""
	for _, s := range spots {
		spotStr += s.Component + "\n"
	}
	results = append(results, measurement{"hot_spots_top5", tokenCount(spotStr)})

	t.Logf("\n%-25s %8s", "Action", "Tokens")
	t.Logf("%-25s %8s", "------", "------")
	for _, r := range results {
		t.Logf("%-25s %8d", r.name, r.tokens)
	}
}
