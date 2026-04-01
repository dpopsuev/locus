package constraint

import (
	"path"
	"strings"

	"github.com/dpopsuev/locus/internal/arch"
	"github.com/dpopsuev/locus/internal/port"
)

// BoundaryViolation records a single edge that violates a boundary rule.
type BoundaryViolation struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Rule     string `json:"rule"`
	Severity string `json:"severity"`
}

// CheckBoundaryRules checks edges against boundary rules and returns violations.
// A violation occurs when an edge matches a rule's from/to patterns and the rule
// disallows the dependency (Allow == false).
func CheckBoundaryRules(edges []arch.ArchEdge, rules []port.BoundaryRule) []BoundaryViolation {
	if len(rules) == 0 || len(edges) == 0 {
		return nil
	}

	var violations []BoundaryViolation
	for _, e := range edges {
		for _, r := range rules {
			if !matchPattern(e.From, r.FromPattern) {
				continue
			}
			if !matchPattern(e.To, r.ToPattern) {
				continue
			}
			// Edge matches the rule's patterns.
			if !r.Allow {
				violations = append(violations, BoundaryViolation{
					From:     e.From,
					To:       e.To,
					Rule:     r.FromPattern + " -> " + r.ToPattern,
					Severity: port.SeverityError,
				})
			}
		}
	}
	return violations
}

// matchPattern checks if a component name matches a pattern.
// Supports glob patterns via path.Match and substring matching via strings.Contains.
func matchPattern(component, pattern string) bool {
	if pattern == "" || pattern == "*" {
		return true
	}
	// Try glob match first (supports *, ?, [...]).
	if matched, err := path.Match(pattern, component); err == nil && matched {
		return true
	}
	// Fall back to substring match for simple patterns.
	return strings.Contains(component, pattern)
}
