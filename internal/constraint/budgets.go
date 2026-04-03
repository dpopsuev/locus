package constraint

import (
	"fmt"

	"github.com/dpopsuev/locus/internal/arch"
	"github.com/dpopsuev/locus/internal/port"
)

// BudgetViolation records a single metric exceeding its budget.
type BudgetViolation struct {
	Component string  `json:"component"`
	Metric    string  `json:"metric"`
	Actual    float64 `json:"actual"`
	Budget    float64 `json:"budget"`
	Severity  string  `json:"severity"`
}

// BudgetReport summarizes budget compliance across all constrained components.
type BudgetReport struct {
	Violations []BudgetViolation `json:"violations"`
	Passing    int               `json:"passing"`
	Failing    int               `json:"failing"`
	Summary    string            `json:"summary"`
}

// ComputeBudgetViolations checks each HealthConstraint against the actual
// architecture metrics and returns a BudgetReport.
func ComputeBudgetViolations(
	services []arch.ArchService,
	edges []arch.ArchEdge,
	constraints []port.HealthConstraint,
) *BudgetReport {
	// Build service lookup by name.
	svcMap := make(map[string]*arch.ArchService, len(services))
	for i := range services {
		svcMap[services[i].Name] = &services[i]
	}

	fanIn := arch.ComputeFanIn(edges)

	var violations []BudgetViolation
	passing := 0

	for _, c := range constraints {
		svc, ok := svcMap[c.Component]
		if !ok {
			continue
		}

		type check struct {
			metric string
			actual float64
			budget float64
		}
		var checks []check

		if c.MaxFanIn > 0 {
			checks = append(checks, check{
				metric: "fan_in",
				actual: float64(fanIn[c.Component]),
				budget: float64(c.MaxFanIn),
			})
		}
		if c.MaxChurn > 0 {
			checks = append(checks, check{
				metric: "churn",
				actual: float64(svc.Churn),
				budget: float64(c.MaxChurn),
			})
		}
		if c.MaxNesting > 0 {
			checks = append(checks, check{
				metric: "max_nesting",
				actual: float64(svc.MaxNesting),
				budget: float64(c.MaxNesting),
			})
		}

		for _, ch := range checks {
			if ch.actual > ch.budget {
				severity := port.SeverityWarning
				if ch.actual > ch.budget*2 {
					severity = port.SeverityError
				}
				violations = append(violations, BudgetViolation{
					Component: c.Component,
					Metric:    ch.metric,
					Actual:    ch.actual,
					Budget:    ch.budget,
					Severity:  severity,
				})
			} else {
				passing++
			}
		}
	}

	failing := len(violations)
	summary := fmt.Sprintf("%d budget check(s) passing, %d failing", passing, failing)
	if failing == 0 {
		summary = fmt.Sprintf("All %d budget check(s) passing", passing)
	}

	return &BudgetReport{
		Violations: violations,
		Passing:    passing,
		Failing:    failing,
		Summary:    summary,
	}
}
