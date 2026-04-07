package impact

import (
	"fmt"
	"sort"

	"github.com/dpopsuev/locus/internal/arch"
	archgit "github.com/dpopsuev/locus/internal/arch/git"
	"github.com/dpopsuev/locus/internal/graph"
	"github.com/dpopsuev/locus/internal/port"
)

// RiskScore holds a composite risk assessment for a single component.
type RiskScore struct {
	Component   string         `json:"component"`
	Churn       int            `json:"churn"`
	BlastPct    int            `json:"blast_pct"`
	CoverageGap float64        `json:"coverage_gap"`
	Score       port.Score     `json:"score"`
	Level       port.RiskLevel `json:"level"`
}

// RiskReport holds risk scores for all components in a codebase.
type RiskReport struct {
	Scores  []RiskScore `json:"scores"`
	Summary string      `json:"summary"`
}

// Risk formula weights.
const (
	riskWeightChurn    = 0.4
	riskWeightBlast    = 0.4
	riskWeightCoverage = 0.2
)

// ComputeRiskScores produces a composite risk score per component:
// risk = norm(churn)×0.4 + norm(blast)×0.4 + coverageGap×0.2
// Language-agnostic — works on any scanned codebase.
func ComputeRiskScores(
	services []arch.ArchService,
	edges []arch.ArchEdge,
	coverage []archgit.CoverageResult,
) *RiskReport {
	if len(services) == 0 {
		return &RiskReport{Summary: "no components"}
	}

	// Pre-compute per-component blast radius.
	blastMap := make(map[string]int, len(services))
	for i := range services {
		result, err := ComputeImpact(edges, services, services[i].Name)
		if err == nil {
			blastMap[services[i].Name] = result.BlastRadius
		}
	}

	// Build coverage map.
	coverageMap := make(map[string]float64, len(coverage))
	for _, c := range coverage {
		coverageMap[c.Component] = c.CoveragePct
	}

	// Find max churn and blast for normalization.
	maxChurn := 1
	maxBlast := 1
	for i := range services {
		if services[i].Churn > maxChurn {
			maxChurn = services[i].Churn
		}
		if blastMap[services[i].Name] > maxBlast {
			maxBlast = blastMap[services[i].Name]
		}
	}

	fanIn := graph.FanIn(edges)

	scores := make([]RiskScore, 0, len(services))
	for i := range services {
		svc := &services[i]

		// Skip components with zero activity.
		if svc.Churn == 0 && fanIn[svc.Name] == 0 {
			continue
		}

		normChurn := float64(svc.Churn) / float64(maxChurn)
		normBlast := float64(blastMap[svc.Name]) / float64(maxBlast)

		covGap := 1.0 // assume worst if no coverage data
		if pct, ok := coverageMap[svc.Name]; ok {
			covGap = (100.0 - pct) / 100.0
		}

		raw := normChurn*riskWeightChurn + normBlast*riskWeightBlast + covGap*riskWeightCoverage
		score := port.Score(raw * 100)
		if score > 100 {
			score = 100
		}

		scores = append(scores, RiskScore{
			Component:   svc.Name,
			Churn:       svc.Churn,
			BlastPct:    blastMap[svc.Name],
			CoverageGap: covGap,
			Score:       score,
			Level:       riskLevel(score),
		})
	}

	sort.Slice(scores, func(i, j int) bool {
		return scores[i].Score > scores[j].Score
	})

	summary := fmt.Sprintf("%d components scored", len(scores))
	if len(scores) > 0 {
		summary = fmt.Sprintf("%d components scored, highest risk: %s (%.0f)",
			len(scores), scores[0].Component, scores[0].Score)
	}

	return &RiskReport{Scores: scores, Summary: summary}
}

func riskLevel(score port.Score) port.RiskLevel {
	switch {
	case score >= 75:
		return port.RiskCritical
	case score >= 50:
		return port.RiskHigh
	case score >= 25:
		return port.RiskMedium
	default:
		return port.RiskLow
	}
}
