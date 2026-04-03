package impact

import (
	"testing"

	"github.com/dpopsuev/locus/internal/arch"
)

func TestLeverage_MixedConsumers(t *testing.T) {
	services := []arch.ArchService{
		{Name: "target"},
		{Name: "binary_consumer"},
		{Name: "enrichment_consumer1"},
		{Name: "enrichment_consumer2"},
		{Name: "unrelated"},
	}
	edges := []arch.ArchEdge{
		{From: "binary_consumer", To: "target", Weight: 1, CallSites: 0},
		{From: "enrichment_consumer1", To: "target", Weight: 3, CallSites: 8, LOCSurface: 15},
		{From: "enrichment_consumer2", To: "target", Weight: 2, CallSites: 5, LOCSurface: 10},
	}

	report, err := ComputeLeverage(edges, services, "target")
	if err != nil {
		t.Fatal(err)
	}
	if report.TotalConsumers != 3 {
		t.Errorf("expected 3 consumers, got %d", report.TotalConsumers)
	}
	if report.Binary != 1 {
		t.Errorf("expected 1 binary, got %d", report.Binary)
	}
	if report.Enrichment != 2 {
		t.Errorf("expected 2 enrichment, got %d", report.Enrichment)
	}
	if report.LeverageScore == 0 {
		t.Error("expected non-zero leverage score")
	}
}

func TestLeverage_NoConsumers(t *testing.T) {
	services := []arch.ArchService{
		{Name: "leaf"},
		{Name: "other"},
	}
	edges := []arch.ArchEdge{
		{From: "leaf", To: "other", Weight: 1},
	}

	report, err := ComputeLeverage(edges, services, "leaf")
	if err != nil {
		t.Fatal(err)
	}
	if report.TotalConsumers != 0 {
		t.Errorf("expected 0 consumers, got %d", report.TotalConsumers)
	}
	if report.LeverageScore != 0 {
		t.Errorf("expected score 0, got %d", report.LeverageScore)
	}
}

func TestLeverage_AllEnrichment(t *testing.T) {
	services := []arch.ArchService{
		{Name: "core"}, {Name: "a"}, {Name: "b"}, {Name: "c"},
	}
	edges := []arch.ArchEdge{
		{From: "a", To: "core", Weight: 3, CallSites: 10},
		{From: "b", To: "core", Weight: 5, CallSites: 15},
		{From: "c", To: "core", Weight: 4, CallSites: 12},
	}

	report, err := ComputeLeverage(edges, services, "core")
	if err != nil {
		t.Fatal(err)
	}
	if report.Binary != 0 {
		t.Errorf("expected 0 binary, got %d", report.Binary)
	}
	if report.Enrichment != 3 {
		t.Errorf("expected 3 enrichment, got %d", report.Enrichment)
	}
	// 3 enrichment out of 4 total: score = (3*3)/(4*3)*100 = 75
	if report.LeverageScore != 75 {
		t.Errorf("expected score 75, got %d", report.LeverageScore)
	}
}

func TestLeverage_ScoreCalculation(t *testing.T) {
	tests := []struct {
		name       string
		enrichment int
		binary     int
		total      int
		want       int
	}{
		{"all enrichment", 3, 0, 4, 75}, // (3*3)/(4*3)=75%
		{"all binary", 0, 3, 4, 25},     // (0*3+3)/(4*3)=25%
		{"mixed", 2, 1, 10, 23},         // (2*3+1)/(10*3)=23%
		{"zero total", 0, 0, 0, 0},
		{"single enrichment of 1", 1, 0, 1, 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeLeverageScore(tt.enrichment, tt.binary, tt.total)
			if got != tt.want {
				t.Errorf("computeLeverageScore(%d, %d, %d) = %d, want %d",
					tt.enrichment, tt.binary, tt.total, got, tt.want)
			}
		})
	}
}

func TestLeverage_ComponentNotFound(t *testing.T) {
	services := []arch.ArchService{{Name: "a"}}
	_, err := ComputeLeverage(nil, services, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent component")
	}
}
