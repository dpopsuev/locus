package protocol

import (
	"fmt"
	"sort"

	"github.com/dpopsuev/locus/internal/arch"
)

// ImportDirectionViolation represents a single import direction rule violation
// where a shallow-depth component imports a deeper one.
type ImportDirectionViolation struct {
	From      string `json:"from"`
	To        string `json:"to"`
	FromDepth int    `json:"from_depth"`
	ToDepth   int    `json:"to_depth"`
	Severity  string `json:"severity"`
}

// ImportDirectionReport holds the results of import direction validation.
type ImportDirectionReport struct {
	Violations []ImportDirectionViolation `json:"violations"`
	Summary    string                     `json:"summary"`
}

// ComputeImportDirection checks that imports flow from high depth to low depth.
// A violation occurs when a component at a shallow depth imports one at a greater depth,
// meaning a high-level component depends on a low-level detail.
func ComputeImportDirection(edges []arch.ArchEdge, depths arch.DepthMap) *ImportDirectionReport {
	if depths == nil {
		depths = arch.ComputeImportDepth(edges)
	}

	var violations []ImportDirectionViolation
	for _, e := range edges {
		fromDepth := depths[e.From]
		toDepth := depths[e.To]

		// Skip cycle nodes (depth == -1).
		if fromDepth == -1 || toDepth == -1 {
			continue
		}

		// Skip entrypoint/composition roots (depth 0) — they wire everything by design.
		if fromDepth == 0 {
			continue
		}

		// Violation: shallow (low depth) imports deep (high depth).
		// In a clean architecture, From should have depth >= To's depth,
		// meaning higher-level imports lower-level.
		if fromDepth < toDepth {
			diff := toDepth - fromDepth
			severity := "warning"
			if diff >= 2 {
				severity = "error"
			}
			violations = append(violations, ImportDirectionViolation{
				From:      e.From,
				To:        e.To,
				FromDepth: fromDepth,
				ToDepth:   toDepth,
				Severity:  severity,
			})
		}
	}

	sort.Slice(violations, func(i, j int) bool {
		if violations[i].Severity != violations[j].Severity {
			return violations[i].Severity == "error"
		}
		return violations[i].From < violations[j].From
	})

	errors := 0
	warnings := 0
	for _, v := range violations {
		if v.Severity == "error" {
			errors++
		} else {
			warnings++
		}
	}

	summary := fmt.Sprintf("%d import direction violation(s): %d error(s), %d warning(s)",
		len(violations), errors, warnings)
	if len(violations) == 0 {
		summary = "Clean: no import direction violations"
	}

	return &ImportDirectionReport{
		Violations: violations,
		Summary:    summary,
	}
}
