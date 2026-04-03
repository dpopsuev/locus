package protocol

import (
	"context"
	"testing"

	"github.com/dpopsuev/locus/internal/impact"
)

func TestDogfood_GetBlastRadius(t *testing.T) {
	if testing.Short() {
		t.Skip("dogfood: skipping expensive self-scan in -short mode")
	}
	p, path := setupBurnTest(t)
	ctx := context.Background()

	result, err := p.GetBlastRadius(ctx, path, []string{"internal/arch/scan.go"}, "")
	if err != nil {
		t.Fatalf("GetBlastRadius: %v", err)
	}
	if len(result.ChangedComponents) == 0 {
		t.Error("expected at least 1 changed component")
	}
	if len(result.AffectedComponents) == 0 {
		t.Error("expected at least 1 affected component")
	}
	if result.Summary == "" {
		t.Error("expected non-empty summary")
	}
	t.Logf("blast radius: %d changed, %d affected, %d%%, risk=%s",
		len(result.ChangedComponents), len(result.AffectedComponents), result.BlastRadius, result.RiskLevel)
}

func TestDogfood_GetDiffIntelligence(t *testing.T) {
	if testing.Short() {
		t.Skip("dogfood: skipping expensive self-scan in -short mode")
	}
	p, path := setupBurnTest(t)
	ctx := context.Background()

	result, err := p.GetDiffIntelligence(ctx, path, "HEAD~1")
	if err != nil {
		t.Fatalf("GetDiffIntelligence: %v", err)
	}
	if result.Summary == "" {
		t.Error("expected non-empty summary")
	}
	t.Logf("diff intelligence: %d semantic changes, summary: %s",
		len(result.SemanticChanges), result.Summary)
}

func TestDogfood_GetCrossRepo_Self(t *testing.T) {
	if testing.Short() {
		t.Skip("dogfood: skipping expensive self-scan in -short mode")
	}
	p, path := setupBurnTest(t)
	ctx := context.Background()

	result, err := p.GetCrossRepo(ctx, path, path, "", "")
	if err != nil {
		t.Fatalf("GetCrossRepo: %v", err)
	}
	if len(result.Overlap) == 0 {
		t.Error("expected overlap > 0 when comparing repo with itself")
	}
	if len(result.OnlyInA) != 0 {
		t.Errorf("expected OnlyInA == 0 for self-comparison, got %d", len(result.OnlyInA))
	}
	if len(result.OnlyInB) != 0 {
		t.Errorf("expected OnlyInB == 0 for self-comparison, got %d", len(result.OnlyInB))
	}
	t.Logf("cross-repo self: %d overlap, %d new cycles", len(result.Overlap), result.NewCycles)
}

func TestDogfood_GetWhatIf_DeleteCmd(t *testing.T) {
	if testing.Short() {
		t.Skip("dogfood: skipping expensive self-scan in -short mode")
	}
	p, path := setupBurnTest(t)
	ctx := context.Background()

	result, err := p.GetWhatIf(ctx, path, []impact.FileMove{{From: "cmd/locus"}})
	if err != nil {
		t.Fatalf("GetWhatIf: %v", err)
	}
	if result.ComponentsAfter >= result.ComponentsBefore {
		t.Errorf("expected fewer components after deletion: before=%d, after=%d",
			result.ComponentsBefore, result.ComponentsAfter)
	}
	if len(result.RemovedEdges) == 0 {
		t.Error("expected at least 1 removed edge after deleting cmd/locus")
	}
	t.Logf("what-if delete cmd/locus: %d→%d components, %d edges removed, summary: %s",
		result.ComponentsBefore, result.ComponentsAfter, len(result.RemovedEdges), result.Summary)
}

func TestDogfood_GetLeverage_Protocol(t *testing.T) {
	if testing.Short() {
		t.Skip("dogfood: skipping expensive self-scan in -short mode")
	}
	p, path := setupBurnTest(t)
	ctx := context.Background()

	result, err := p.GetLeverage(ctx, path, "internal/graph")
	if err != nil {
		t.Fatalf("GetLeverage: %v", err)
	}
	if result.TotalConsumers < 3 {
		t.Errorf("expected >= 3 consumers for internal/graph, got %d", result.TotalConsumers)
	}
	if result.LeverageScore == 0 {
		t.Error("expected non-zero leverage score")
	}
	t.Logf("leverage internal/graph: %d consumers (%d enrichment, %d binary), score=%d",
		result.TotalConsumers, result.Enrichment, result.Binary, result.LeverageScore)
}
