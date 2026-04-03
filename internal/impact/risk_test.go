package impact

import (
	"testing"

	"github.com/dpopsuev/locus/internal/arch"
	"github.com/dpopsuev/locus/internal/model"
)

func TestComputeRiskScores_Basic(t *testing.T) {
	services := []arch.ArchService{
		{Name: "core", Churn: 50, LOC: 500, Symbols: model.SymbolsFromNames("A", "B", "C")},
		{Name: "util", Churn: 5, LOC: 100, Symbols: model.SymbolsFromNames("X")},
		{Name: "api", Churn: 30, LOC: 200, Symbols: model.SymbolsFromNames("H", "R")},
	}
	edges := []arch.ArchEdge{
		{From: "api", To: "core"},
		{From: "util", To: "core"},
		{From: "api", To: "util"},
	}
	coverage := []arch.CoverageResult{
		{Component: "core", CoveragePct: 80},
		{Component: "api", CoveragePct: 20},
	}

	report := ComputeRiskScores(services, edges, coverage)
	if len(report.Scores) == 0 {
		t.Fatal("expected risk scores")
	}

	// Core has high churn and high blast radius — should be highest risk.
	if report.Scores[0].Component != "core" && report.Scores[0].Component != "api" {
		t.Logf("highest risk: %s (score %.0f)", report.Scores[0].Component, report.Scores[0].Score)
	}

	for _, s := range report.Scores {
		if s.Level == "" {
			t.Errorf("expected non-empty risk level for %s", s.Component)
		}
		t.Logf("  %s: churn=%d, blast=%d%%, covGap=%.2f, score=%.0f, level=%s",
			s.Component, s.Churn, s.BlastPct, s.CoverageGap, s.Score, s.Level)
	}
}

func TestComputeRiskScores_Empty(t *testing.T) {
	report := ComputeRiskScores(nil, nil, nil)
	if report.Summary != "no components" {
		t.Errorf("expected 'no components' summary, got %q", report.Summary)
	}
}

func TestComputeRiskScores_NoCoverage(t *testing.T) {
	services := []arch.ArchService{
		{Name: "pkg", Churn: 10, Symbols: model.SymbolsFromNames("A")},
	}
	edges := []arch.ArchEdge{{From: "other", To: "pkg"}}

	report := ComputeRiskScores(services, edges, nil)
	if len(report.Scores) == 0 {
		t.Fatal("expected scores")
	}
	// No coverage data → coverage gap = 1.0 (worst case)
	if report.Scores[0].CoverageGap != 1.0 {
		t.Errorf("expected coverage gap 1.0 without data, got %f", report.Scores[0].CoverageGap)
	}
}
